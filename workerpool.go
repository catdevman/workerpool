// Package workerpool runs a resized set of goroutines over a stream of inputs.
//
// The caller owns the input channel. The pool owns the results channel returned
// by [Pool.Run]. That channel closes after the workers exit because the Run
// context was cancelled or a worker saw the input channel close. Scaling to
// zero workers leaves the channel open.
package workerpool

import (
	"context"
	"fmt"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"sync"
	"time"
)

// TaskFunc is the work applied to a single input.
type TaskFunc[T any, R any] func(ctx context.Context, input T) (R, error)

// Observer receives hooks for metrics. A nil Observer disables the hooks.
//
// WorkerCountChanged is called without the pool lock held, so the observer
// may call back into the pool.
type Observer interface {
	TaskStarted()
	TaskFinished(duration time.Duration, err error)
	WorkerCountChanged(count int)
}

// PanicError is the error on a [Result] when a task panics.
// The worker that caught it keeps running.
type PanicError struct {
	// Value is the argument passed to panic.
	Value any
	// Stack is the trace captured at the recover site.
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// Unwrap returns Value when the panicked value is an error.
// That lets callers use errors.Is on the result.
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

// Result is one finished task.
type Result[R any] struct {
	Value R
	Err   error
}

// Pool runs TaskFunc with a configurable number of workers.
type Pool[T any, R any] struct {
	task     TaskFunc[T, R]
	observer Observer
	inputs   <-chan T

	mu sync.Mutex
	// target is the number of workers that have not been asked to retire.
	// It is not decreased when a worker exits because the pool context is
	// done or the input channel is closed. Those exits mean the pool is
	// finished, and shrinking the count there would let an autoscaler start
	// replacements during shutdown.
	target int
	// runCtx is the context passed to Run. Every worker uses it, including
	// workers started later by Resize. Resize's own context only decides
	// whether that call may still change the size.
	runCtx context.Context
	// stops has one channel per worker that has not been asked to retire.
	// The channel is private to that worker. Closing it cannot be received
	// by a worker started later. A shared "please exit" queue can, and a
	// scale-up would then consume a leftover scale-down.
	stops  []chan struct{}
	nextID uint64

	wg           sync.WaitGroup
	results      chan Result[R]
	inputsClosed chan struct{}
	inputsOnce   sync.Once
}

// New creates a pool that will start with initialWorkers when Run is called.
// initialWorkers less than zero is treated as zero. Pass a nil observer when
// metrics are not needed. inputs is owned by the caller and must be closed
// by the caller when no further work will be sent.
func New[T any, R any](initialWorkers int, inputs <-chan T, task TaskFunc[T, R], obs Observer) *Pool[T, R] {
	if initialWorkers < 0 {
		initialWorkers = 0
	}
	return &Pool[T, R]{
		target:       initialWorkers,
		inputs:       inputs,
		task:         task,
		observer:     obs,
		inputsClosed: make(chan struct{}),
	}
}

// Run starts the configured workers and returns the results channel.
//
// The results channel stays open while the pool can still accept workers.
// It is closed after the workers have exited because ctx was cancelled or a
// worker observed that the input channel was closed. Scaling to zero does
// not close it, so a later Resize can start workers again. Cancel ctx to
// finish a pool that has no workers left to observe a closed input channel.
//
// A second Run returns the same channel and does not start another closer.
//
// Workers started later by Resize follow ctx, not the context passed to Resize.
func (p *Pool[T, R]) Run(ctx context.Context) <-chan Result[R] {
	p.mu.Lock()
	if p.results != nil {
		results := p.results
		p.mu.Unlock()
		return results
	}

	p.runCtx = ctx
	results := make(chan Result[R])
	p.results = results

	var (
		notify bool
		count  int
		obs    Observer
	)
	if ctx.Err() != nil {
		// The pool was cancelled before any worker could run.
		p.target = 0
	} else if p.target > 0 {
		count = p.target
		p.target = 0
		p.reconcile(count)
		notify = true
		obs = p.observer
	}
	p.mu.Unlock()

	if notify && obs != nil {
		obs.WorkerCountChanged(count)
	}

	// Not part of p.wg: this goroutine waits for the workers, so putting it
	// in the group would make Wait wait for itself. Scaling to zero leaves
	// the channel open; shutdown is cancellation or a closed input channel.
	go func() {
		select {
		case <-ctx.Done():
		case <-p.inputsClosed:
		}
		p.wg.Wait()
		close(results)
	}()

	return results
}

func (p *Pool[T, R]) markInputsClosed() {
	p.inputsOnce.Do(func() { close(p.inputsClosed) })
}

// Resize changes how many workers accept new work.
//
// The new count is visible from CurrentWorkers before Resize returns. Workers
// that were asked to stop finish the task they already started, then exit.
// Their stop signal cannot be taken by a worker started from a later Resize.
//
// Resize does nothing when ctx is already cancelled. ctx does not replace the
// context passed to Run. Before Run, Resize only records the size Run will start.
func (p *Pool[T, R]) Resize(ctx context.Context, targetWorkers int) {
	if targetWorkers < 0 {
		targetWorkers = 0
	}
	if ctx.Err() != nil {
		return
	}

	p.mu.Lock()
	if p.results == nil {
		changed := p.target != targetWorkers
		p.target = targetWorkers
		obs := p.observer
		p.mu.Unlock()
		if changed && obs != nil {
			obs.WorkerCountChanged(targetWorkers)
		}
		return
	}
	if p.target == targetWorkers {
		p.mu.Unlock()
		return
	}
	p.reconcile(targetWorkers)
	obs := p.observer
	p.mu.Unlock()

	if obs != nil {
		obs.WorkerCountChanged(targetWorkers)
	}
}

// reconcile moves the running set to target. The caller holds p.mu, and the
// pool must already be running.
func (p *Pool[T, R]) reconcile(target int) {
	for p.target < target {
		stop := make(chan struct{})
		p.stops = append(p.stops, stop)
		id := p.nextID
		p.nextID++
		p.target++
		ctx := p.runCtx
		p.wg.Go(func() { p.worker(ctx, stop, id) })
	}
	if p.target == target {
		return
	}

	retiring := p.stops[target:]
	// Cap the slice at target so a later scale-up does not keep the
	// retired channels reachable from the pool.
	p.stops = p.stops[:target:target]
	p.target = target
	for _, stop := range retiring {
		close(stop)
	}
}

// CurrentWorkers returns the number of workers that have not been asked to
// retire. A worker finishing its current task after a scale-down is not
// included. The count stays at the configured size when workers leave because
// the pool context is done or the input channel is closed.
func (p *Pool[T, R]) CurrentWorkers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.target
}

func (p *Pool[T, R]) worker(ctx context.Context, stop <-chan struct{}, id uint64) {
	pprof.Do(ctx, pprof.Labels(
		"workerpool", "worker",
		"worker_id", strconv.FormatUint(id, 10),
	), func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case input, ok := <-p.inputs:
				if !ok {
					p.markInputsClosed()
					return
				}
				p.safeExecute(ctx, input)
			}
		}
	})
}

func (p *Pool[T, R]) safeExecute(ctx context.Context, input T) {
	obs := p.observer
	var start time.Time
	if obs != nil {
		start = time.Now()
		obs.TaskStarted()
	}

	val, err := p.callTask(ctx, input)

	if obs != nil {
		obs.TaskFinished(time.Since(start), err)
	}

	// A cancelled pool must not block forever on a consumer that has gone
	// away. A worker that was only asked to scale down still delivers the
	// result it already computed.
	select {
	case p.results <- Result[R]{Value: val, Err: err}:
	case <-ctx.Done():
	}
}

func (p *Pool[T, R]) callTask(ctx context.Context, input T) (val R, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{Value: r, Stack: debug.Stack()}
		}
	}()
	return p.task(ctx, input)
}
