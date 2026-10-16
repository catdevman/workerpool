package workerpool

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type atomicFloat struct {
	bits atomic.Uint64
}

func (a *atomicFloat) Store(v float64) { a.bits.Store(math.Float64bits(v)) }
func (a *atomicFloat) Load() float64   { return math.Float64frombits(a.bits.Load()) }

func TestAutoScalerThresholdStrategy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		pool := New(5, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := pool.Run(ctx)
		defer close(inputs)
		go func() {
			for range results {
			}
		}()

		var metric atomicFloat
		scaler := NewAutoScaler(pool, 10*time.Millisecond, 1, 10, metric.Load)
		scaler.WithThresholdStrategy(20, 80, 2)
		scaler.Start()
		defer scaler.Stop()

		metric.Store(90)
		synctest.Sleep(50 * time.Millisecond)
		if got := pool.CurrentWorkers(); got <= 5 {
			t.Fatalf("workers = %d, want a scale-up from 5", got)
		}

		metric.Store(10)
		synctest.Sleep(50 * time.Millisecond)
		if got := pool.CurrentWorkers(); got >= 7 {
			t.Fatalf("workers = %d, want a scale-down below 7", got)
		}
	})
}

func TestAutoScalerInverseStrategy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		pool := New(5, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := pool.Run(ctx)
		defer close(inputs)
		go func() {
			for range results {
			}
		}()

		var metric atomicFloat
		metric.Store(95)
		scaler := NewAutoScaler(pool, 10*time.Millisecond, 1, 10, metric.Load)
		scaler.WithInverseThresholdStrategy(20, 80, 2)
		scaler.Start()
		defer scaler.Stop()

		synctest.Sleep(50 * time.Millisecond)
		if got := pool.CurrentWorkers(); got >= 5 {
			t.Fatalf("workers = %d, want a scale-down from 5", got)
		}
	})
}

func TestAutoScalerHardBounds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		pool := New(2, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := pool.Run(ctx)
		defer close(inputs)
		go func() {
			for range results {
			}
		}()

		scaler := NewAutoScaler(pool, 10*time.Millisecond, 1, 5, func() float64 { return 100 })
		scaler.SetStrategy(func(curr int, _ float64) int { return curr + 10 })
		scaler.Start()
		defer scaler.Stop()

		synctest.Sleep(50 * time.Millisecond)
		if got := pool.CurrentWorkers(); got != 5 {
			t.Fatalf("workers = %d, want the max of 5", got)
		}
	})
}

func TestAutoScalerStopIsIdempotentAndRestartable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		pool := New(1, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := pool.Run(ctx)
		defer close(inputs)
		go func() {
			for range results {
			}
		}()

		var metric atomicFloat
		scaler := NewAutoScaler(pool, time.Second, 1, 4, metric.Load)
		scaler.WithThresholdStrategy(10, 50, 1)
		scaler.Start()
		scaler.Start()

		metric.Store(100)
		synctest.Sleep(3 * time.Second)
		scaled := pool.CurrentWorkers()
		if scaled <= 1 {
			t.Fatalf("workers = %d, want a scale-up from 1", scaled)
		}

		scaler.Stop()
		scaler.Stop()
		synctest.Sleep(3 * time.Second)
		if got := pool.CurrentWorkers(); got != scaled {
			t.Fatalf("workers changed after Stop: %d to %d", scaled, got)
		}

		metric.Store(0)
		scaler.Start()
		synctest.Sleep(3 * time.Second)
		if got := pool.CurrentWorkers(); got >= scaled {
			t.Fatalf("workers after restart = %d, want a scale-down from %d", got, scaled)
		}
		scaler.Stop()
	})
}

func TestAutoscaledWorkersStopWithRunContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		// Block until the Run context is cancelled. A worker started with
		// context.Background would stay here and fail the bubble.
		task := func(ctx context.Context, n int) (int, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		}

		pool := New(1, inputs, task, nil)
		ctx, cancel := context.WithCancel(t.Context())
		results := pool.Run(ctx)
		defer close(inputs)
		go func() {
			for range results {
			}
		}()

		scaler := NewAutoScaler(pool, time.Millisecond, 1, 4, func() float64 { return 100 })
		scaler.WithThresholdStrategy(0, 1, 1)
		scaler.Start()
		synctest.Sleep(10 * time.Millisecond)
		scaler.Stop()

		if got := pool.CurrentWorkers(); got < 2 {
			t.Fatalf("workers = %d, want the autoscaler to add workers", got)
		}

		// Give the added workers a task so they are inside callTask, not
		// merely parked on the input select.
		go func() {
			for i := 0; i < pool.CurrentWorkers(); i++ {
				select {
				case inputs <- i:
				case <-ctx.Done():
					return
				}
			}
		}()
		synctest.Wait()
		cancel()
	})
}
