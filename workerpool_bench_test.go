package workerpool

import (
	"context"
	"testing"
	"time"
)

// noOpObserver fits the interface but does nothing, to measure overhead of calling it
type noOpObserver struct{}

func (n noOpObserver) TaskStarted()                      {}
func (n noOpObserver) TaskFinished(time.Duration, error) {}
func (n noOpObserver) WorkerCountChanged(int)            {}

func BenchmarkThroughput(b *testing.B) {
	// Task: CPU bound, very fast.
	// This measures the raw overhead of the pool (channels, context switching).
	task := func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	}

	// Setup
	inputs := make(chan int, b.N)
	pool := New(10, inputs, task, noOpObserver{})
	ctx := context.Background()
	results := pool.Run(ctx)

	// Pre-fill inputs to avoid benchmarking channel send speed
	for i := 0; i < b.N; i++ {
		inputs <- i
	}
	close(inputs)

	b.ResetTimer() // Start clock

	// Consume
	for range results {
		// drain
	}
}

func BenchmarkIOBound(b *testing.B) {
	// Task: Simulates IO (Network call)
	// We want to see how well it handles context switching under load
	task := func(_ context.Context, n int) (int, error) {
		time.Sleep(1 * time.Millisecond)
		return n, nil
	}

	// High concurrency count for IO bound
	inputs := make(chan int, b.N)
	pool := New(100, inputs, task, nil) // nil observer to test raw speed
	ctx := context.Background()
	results := pool.Run(ctx)

	go func() {
		defer close(inputs)
		for i := 0; i < b.N; i++ {
			inputs <- i
		}
	}()

	b.ResetTimer()
	for range results {
	}
}

// BenchmarkAllocations checks memory pressure.
// Run with: go test -bench=Alloc -benchmem
func BenchmarkAllocations(b *testing.B) {
	task := func(_ context.Context, n int) (int, error) {
		return n, nil
	}

	inputs := make(chan int, b.N)
	pool := New(5, inputs, task, nil)
	ctx := context.Background()
	results := pool.Run(ctx)

	for i := 0; i < b.N; i++ {
		inputs <- i
	}
	close(inputs)

	b.ResetTimer()
	for range results {
	}
}
