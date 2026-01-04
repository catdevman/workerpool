package workerpool

import (
	"context"
	"sync"
	"time"
)

// MetricFunc returns a current value (e.g., CPU %, Memory bytes, Queue Depth).
type MetricFunc func() float64

// AutoScaler monitors a metric and resizes the pool within bounds.
type AutoScaler[T any, R any] struct {
	pool     *Pool[T, R]
	interval time.Duration
	min      int
	max      int

	// strategy calculates the new worker count based on the metric
	strategy func(currentCount int, metric float64) int

	// metricFetcher retrieves the value to judge against
	metricFetcher MetricFunc

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewAutoScaler creates the sidecar.
// It does NOT start it automatically; call Start().
func NewAutoScaler[T any, R any](
	pool *Pool[T, R],
	interval time.Duration,
	min, max int,
	fetcher MetricFunc,
) *AutoScaler[T, R] {
	return &AutoScaler[T, R]{
		pool:          pool,
		interval:      interval,
		min:           min,
		max:           max,
		metricFetcher: fetcher,
		stopCh:        make(chan struct{}),
		// Default Strategy: Linear Scaling (can be overridden)
		// This is just a placeholder; usually you configure this via a helper.
		strategy: func(curr int, m float64) int { return curr },
	}
}

// SetStrategy allows you to define custom logic.
// The function receives (currentWorkers, currentMetric) and returns desiredWorkers.
func (a *AutoScaler[T, R]) SetStrategy(fn func(currentCount int, metric float64) int) {
	a.strategy = fn
}

// Start begins the monitoring loop in a background goroutine.
func (a *AutoScaler[T, R]) Start() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()

		for {
			select {
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.evaluate()
			}
		}
	}()
}

// Stop halts the autoscaler.
func (a *AutoScaler[T, R]) Stop() {
	close(a.stopCh)
	a.wg.Wait()
}

func (a *AutoScaler[T, R]) evaluate() {
	// 1. Get the metric (e.g., CPU Usage)
	val := a.metricFetcher()

	// 2. Get current state
	current := a.pool.CurrentWorkers()

	// 3. Ask strategy for desired count
	desired := a.strategy(current, val)

	// 4. Enforce Bounds (Min/Max)
	if desired < a.min {
		desired = a.min
	}
	if desired > a.max {
		desired = a.max
	}

	// 5. Act (only if changed)
	if desired != current {
		// We use a TODO context or Background here because this is a
		// system-level event, separate from the job context.
		a.pool.Resize(context.Background(), desired)
	}
}

// WithThresholdStrategy configures the scaler to target a specific metric range.
// Example: Keep CPU between 40% (low) and 80% (high).
// step: How many workers to add/remove at once (e.g., 1 or 5).
func (a *AutoScaler[T, R]) WithThresholdStrategy(low, high float64, step int) {
	a.SetStrategy(func(current int, metric float64) int {
		if metric > high {
			// Resource is hot! Scale DOWN to relieve pressure?
			// OR Scale UP to process faster?
			// usually for CPU/Queue, High Metric = Scale UP.
			// usually for Memory, High Metric = Scale DOWN (to prevent OOM).

			// Let's assume High Metric = Need More Workers (Queue/CPU logic)
			return current + step
		}
		if metric < low {
			// Resource is idle. Scale DOWN.
			return current - step
		}
		// In the "Goldilocks" zone, do nothing.
		return current
	})
}

// WithInverseThresholdStrategy is for Memory.
// If Memory is High -> Scale DOWN (stop eating RAM).
// If Memory is Low -> Scale UP.
func (a *AutoScaler[T, R]) WithInverseThresholdStrategy(low, high float64, step int) {
	a.SetStrategy(func(current int, metric float64) int {
		if metric > high {
			return current - step // Danger zone, reduce load
		}
		if metric < low {
			return current + step // Free RAM, work harder
		}
		return current
	})
}
