package workerpool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TaskFunc is the logic that processes a single item.
type TaskFunc[T any, R any] func(ctx context.Context, input T) (R, error)

// Observer handles metrics/observability hooks.
type Observer interface {
	TaskStarted()
	TaskFinished(duration time.Duration, err error)
	WorkerCountChanged(count int)
}

// Pool manages the execution of concurrent tasks.
type Pool[T any, R any] struct {
	task     TaskFunc[T, R]
	observer Observer

	mu            sync.Mutex
	workerCount   int
	initialConfig int
	wg            sync.WaitGroup

	inputs   <-chan T
	results  chan Result[R]
	removeCh chan struct{}
}

// Result captures the output or error of a task.
type Result[R any] struct {
	Value R
	Err   error
}

// New creates the pool. Pass nil for observer if metrics aren't needed.
func New[T any, R any](initialWorkers int, inputs <-chan T, task TaskFunc[T, R], obs Observer) *Pool[T, R] {
	if initialWorkers < 0 {
		initialWorkers = 0
	}
	return &Pool[T, R]{
		workerCount:   0,
		initialConfig: initialWorkers,
		inputs:        inputs,
		task:          task,
		observer:      obs,
		removeCh:      make(chan struct{}),
	}
}

// Run starts the pool and returns the results channel.
func (p *Pool[T, R]) Run(ctx context.Context) <-chan Result[R] {
	p.results = make(chan Result[R])
	p.Resize(ctx, p.initialConfig)

	go func() {
		p.wg.Wait()
		close(p.results)
	}()

	return p.results
}

// Resize changes the number of active workers.
func (p *Pool[T, R]) Resize(ctx context.Context, targetWorkers int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if targetWorkers < 0 {
		targetWorkers = 0
	}

	current := p.workerCount
	diff := targetWorkers - current

	if diff == 0 {
		return
	}

	if diff > 0 {
		// SCALE UP
		for i := 0; i < diff; i++ {
			p.wg.Add(1)
			go p.worker(ctx)
		}
	} else {
		// SCALE DOWN
		workersToRemove := -diff
		go func() {
			for i := 0; i < workersToRemove; i++ {
				select {
				case p.removeCh <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	p.workerCount = targetWorkers

	// Metric Hook
	if p.observer != nil {
		p.observer.WorkerCountChanged(targetWorkers)
	}
}

// CurrentWorkers returns the current number of active workers.
// This is thread-safe.
func (p *Pool[T, R]) CurrentWorkers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.workerCount
}

func (p *Pool[T, R]) worker(ctx context.Context) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.removeCh:
			return
		case input, ok := <-p.inputs:
			if !ok {
				return
			}
			p.safeExecute(ctx, input)
		}
	}
}

func (p *Pool[T, R]) safeExecute(ctx context.Context, input T) {
	var start time.Time
	if p.observer != nil {
		start = time.Now()
		p.observer.TaskStarted()
	}

	var err error
	var val R

	// Panic barrier
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		val, err = p.task(ctx, input)
	}()

	// Metric Hook
	if p.observer != nil {
		p.observer.TaskFinished(time.Since(start), err)
	}

	select {
	case p.results <- Result[R]{Value: val, Err: err}:
	case <-ctx.Done():
	}
}
