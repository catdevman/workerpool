package workerpool

import (
	"context"
	"sync"
	"time"
)

// MetricFunc returns the value the autoscaler should react to.
// Examples are CPU percent, memory bytes, or queue depth.
type MetricFunc func() float64

// AutoScaler polls a metric and resizes a pool inside fixed bounds.
type AutoScaler[T any, R any] struct {
	pool     *Pool[T, R]
	interval time.Duration
	min      int
	max      int

	metricFetcher MetricFunc

	mu       sync.Mutex
	strategy func(currentCount int, metric float64) int
	running  bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewAutoScaler builds a scaler. It does not start it. Call Start.
//
// interval is the time between evaluations and must be positive.
// min and max are inclusive bounds. A max below min is raised to min.
// A negative min is treated as zero. fetcher must not be nil.
//
// The initial strategy leaves the worker count unchanged until
// WithThresholdStrategy, WithInverseThresholdStrategy, or SetStrategy
// replaces it.
func NewAutoScaler[T any, R any](
	pool *Pool[T, R],
	interval time.Duration,
	min, max int,
	fetcher MetricFunc,
) *AutoScaler[T, R] {
	if fetcher == nil {
		panic("workerpool: NewAutoScaler metric fetcher is nil")
	}
	if min < 0 {
		min = 0
	}
	if max < min {
		max = min
	}
	return &AutoScaler[T, R]{
		pool:          pool,
		interval:      interval,
		min:           min,
		max:           max,
		metricFetcher: fetcher,
		strategy:      func(curr int, _ float64) int { return curr },
	}
}

// SetStrategy replaces the decision function.
// It receives the current worker count and the latest metric value,
// and it returns the desired worker count before min and max are applied.
// A nil strategy panics.
func (a *AutoScaler[T, R]) SetStrategy(fn func(currentCount int, metric float64) int) {
	if fn == nil {
		panic("workerpool: nil autoscaler strategy")
	}
	a.mu.Lock()
	a.strategy = fn
	a.mu.Unlock()
}

// Start starts the polling loop.
// Start does nothing when the loop is already running.
// After Stop has returned, Start begins a new loop.
func (a *AutoScaler[T, R]) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.wg.Go(func() {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
		}()
		a.loop(ctx)
	})
}

// Stop cancels the polling loop and waits for it to exit.
// Stop is safe to call more than once, including when Start was never called.
// Start may be called again after Stop returns.
func (a *AutoScaler[T, R]) Stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	cancel := a.cancel
	a.mu.Unlock()
	cancel()
	a.wg.Wait()
}

func (a *AutoScaler[T, R]) loop(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.evaluate()
		}
	}
}

func (a *AutoScaler[T, R]) evaluate() {
	a.mu.Lock()
	strategy := a.strategy
	a.mu.Unlock()

	current := a.pool.CurrentWorkers()
	desired := strategy(current, a.metricFetcher())
	if desired < a.min {
		desired = a.min
	}
	if desired > a.max {
		desired = a.max
	}
	if desired == current {
		return
	}
	// Background is intentional. Worker lifetime comes from Pool.Run.
	// A cancelled job context must not veto a scaling decision, and it
	// must not become the lifetime of the workers this call adds.
	a.pool.Resize(context.Background(), desired)
}

// WithThresholdStrategy scales up when the metric is above high and down
// when it is below low. step is how many workers to add or remove at once.
// A metric between low and high leaves the count unchanged.
// Use this for a queue depth or a CPU target where a high value means more
// workers would help.
func (a *AutoScaler[T, R]) WithThresholdStrategy(low, high float64, step int) {
	a.SetStrategy(func(current int, metric float64) int {
		if metric > high {
			return current + step
		}
		if metric < low {
			return current - step
		}
		return current
	})
}

// WithInverseThresholdStrategy scales down when the metric is above high
// and up when it is below low. Use this for a memory target, where a high
// value means the pool should do less work.
func (a *AutoScaler[T, R]) WithInverseThresholdStrategy(low, high float64, step int) {
	a.SetStrategy(func(current int, metric float64) int {
		if metric > high {
			return current - step
		}
		if metric < low {
			return current + step
		}
		return current
	})
}
