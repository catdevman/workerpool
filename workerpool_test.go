package workerpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Helper for generic tasks
func doubleTask(_ context.Context, n int) (int, error) {
	return n * 2, nil
}

// TestBasicProcessing ensures the pool processes all items and closes correctly.
func TestBasicProcessing(t *testing.T) {
	inputCount := 100
	inputs := make(chan int, inputCount)
	for i := 0; i < inputCount; i++ {
		inputs <- i
	}
	close(inputs)

	pool := New(5, inputs, doubleTask, nil)
	results := pool.Run(context.Background())

	count := 0
	for res := range results {
		if res.Err != nil {
			t.Errorf("Unexpected error: %v", res.Err)
		}
		count++
	}

	if count != inputCount {
		t.Errorf("Expected %d results, got %d", inputCount, count)
	}
}

// TestPanicIsolation ensures a panic in a worker is caught and returned as an error
func TestPanicIsolation(t *testing.T) {
	inputs := make(chan int, 5)

	flakyTask := func(ctx context.Context, n int) (int, error) {
		if n == 666 {
			panic("something went wrong!")
		}
		return n, nil
	}

	inputs <- 1
	inputs <- 666
	inputs <- 2
	close(inputs)

	pool := New(2, inputs, flakyTask, nil)
	results := pool.Run(context.Background())

	var panicFound bool
	var successCount int

	for res := range results {
		if res.Err != nil {
			if res.Err.Error() != "panic: something went wrong!" {
				t.Errorf("Expected panic error, got: %v", res.Err)
			}
			panicFound = true
		} else {
			successCount++
		}
	}

	if !panicFound {
		t.Error("Expected to receive a panic error, but didn't")
	}
	if successCount != 2 {
		t.Errorf("Expected 2 successful tasks, got %d", successCount)
	}
}

// TestContextCancellation ensures the pool stops processing when context dies.
func TestContextCancellation(t *testing.T) {
	inputs := make(chan int)

	slowTask := func(ctx context.Context, n int) (int, error) {
		select {
		case <-time.After(1 * time.Hour):
			return n, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	pool := New(5, inputs, slowTask, nil)

	ctx, cancel := context.WithCancel(context.Background())
	results := pool.Run(ctx)

	go func() {
		defer close(inputs)
		for i := 0; i < 100; i++ {
			select {
			case inputs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	for range results {
		// drain
	}
}

// TestDynamicResizing stresses the scaling logic.
func TestDynamicResizing(t *testing.T) {
	inputs := make(chan int)
	var activeWorkers int32

	trackingTask := func(ctx context.Context, n int) (int, error) {
		atomic.AddInt32(&activeWorkers, 1)
		defer atomic.AddInt32(&activeWorkers, -1)
		time.Sleep(50 * time.Millisecond)
		return n, nil
	}

	pool := New(1, inputs, trackingTask, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := pool.Run(ctx)

	go func() {
		defer close(inputs)
		for i := 0; i < 1000; i++ {
			select {
			case inputs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		for range results {
		}
	}()

	if pool.CurrentWorkers() != 1 {
		t.Errorf("Expected 1 worker, got %d", pool.CurrentWorkers())
	}

	pool.Resize(ctx, 10)
	time.Sleep(100 * time.Millisecond)

	if pool.CurrentWorkers() != 10 {
		t.Errorf("Expected 10 workers, got %d", pool.CurrentWorkers())
	}

	pool.Resize(ctx, 2)
	time.Sleep(200 * time.Millisecond)

	if pool.CurrentWorkers() != 2 {
		t.Errorf("Expected 2 workers, got %d", pool.CurrentWorkers())
	}
}

// TestStressResizing checks for Race Conditions.
func TestStressResizing(t *testing.T) {
	inputs := make(chan int)

	task := func(ctx context.Context, n int) (int, error) {
		return n, nil
	}

	pool := New(5, inputs, task, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	results := pool.Run(ctx)

	go func() {
		defer close(inputs)
		for i := 0; ; i++ {
			select {
			case inputs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		for range results {
		}
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			target := 1
			if t.Nanosecond()%2 == 0 {
				target = 20
			}
			pool.Resize(ctx, target)
		}
	}
}
