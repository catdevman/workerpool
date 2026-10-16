package workerpool

import (
	"bytes"
	"context"
	"errors"
	"runtime/pprof"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestBasicProcessing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const inputCount = 100
		inputs := make(chan int, inputCount)
		for i := 0; i < inputCount; i++ {
			inputs <- i
		}
		close(inputs)

		var sawLabel atomic.Bool
		task := func(ctx context.Context, n int) (int, error) {
			role, roleOK := pprof.Label(ctx, "workerpool")
			id, idOK := pprof.Label(ctx, "worker_id")
			if roleOK && role == "worker" && idOK && id != "" {
				sawLabel.Store(true)
			}
			return n * 2, nil
		}

		pool := New(5, inputs, task, nil)
		ctx := t.Context()
		results := pool.Run(ctx)
		if again := pool.Run(ctx); again != results {
			t.Fatal("second Run returned a different results channel")
		}

		count := 0
		for res := range results {
			if res.Err != nil {
				t.Errorf("unexpected error: %v", res.Err)
			}
			count++
		}
		if count != inputCount {
			t.Errorf("got %d results, want %d", count, inputCount)
		}
		if !sawLabel.Load() {
			t.Error("worker did not publish goroutine labels on the task context")
		}
	})
}

func TestPanicIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int, 4)
		boom := errors.New("boom")
		task := func(ctx context.Context, n int) (int, error) {
			switch n {
			case 666:
				panic("something went wrong!")
			case 7:
				panic(boom)
			default:
				return n, nil
			}
		}
		inputs <- 1
		inputs <- 666
		inputs <- 7
		inputs <- 2
		close(inputs)

		pool := New(2, inputs, task, nil)
		results := pool.Run(t.Context())

		var panicFound, unwrapped bool
		success := 0
		for res := range results {
			if res.Err == nil {
				success++
				continue
			}
			pe, ok := errors.AsType[*PanicError](res.Err)
			if !ok {
				t.Fatalf("error type %T, want *PanicError", res.Err)
			}
			if len(pe.Stack) == 0 {
				t.Fatal("panic stack is empty")
			}
			if pe.Value == "something went wrong!" {
				if pe.Error() != "panic: something went wrong!" {
					t.Fatalf("Error() = %q", pe.Error())
				}
				panicFound = true
			}
			if errors.Is(res.Err, boom) {
				unwrapped = true
			}
		}
		if !panicFound || !unwrapped {
			t.Fatalf("string panic found=%v, unwrapped error panic=%v", panicFound, unwrapped)
		}
		if success != 2 {
			t.Fatalf("successful tasks = %d, want 2", success)
		}
	})
}

func TestContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		task := func(ctx context.Context, n int) (int, error) {
			select {
			case <-time.After(time.Hour):
				return n, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}

		pool := New(5, inputs, task, nil)
		ctx, cancel := context.WithCancel(t.Context())
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

		// Workers are inside the hour-long task. Cancellation must unblock
		// them and let the results channel close. synctest.Test fails the
		// test if any of those goroutines stay behind.
		synctest.Wait()
		cancel()
		for range results {
		}
	})
}

func TestDynamicResizing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		var inFlight atomic.Int32
		task := func(ctx context.Context, n int) (int, error) {
			inFlight.Add(1)
			defer inFlight.Add(-1)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(time.Hour):
				return n, nil
			}
		}

		pool := New(1, inputs, task, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := pool.Run(ctx)
		go func() {
			for range results {
			}
		}()
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

		synctest.Wait()
		if got := pool.CurrentWorkers(); got != 1 {
			t.Fatalf("workers = %d, want 1", got)
		}
		if got := inFlight.Load(); got != 1 {
			t.Fatalf("tasks in flight = %d, want 1", got)
		}

		pool.Resize(ctx, 10)
		synctest.Wait()
		if got := pool.CurrentWorkers(); got != 10 {
			t.Fatalf("workers = %d, want 10", got)
		}
		if got := inFlight.Load(); got != 10 {
			t.Fatalf("tasks in flight = %d, want 10", got)
		}

		pool.Resize(ctx, 2)
		if got := pool.CurrentWorkers(); got != 2 {
			t.Fatalf("workers = %d, want 2", got)
		}
		// The retired workers are still inside their current task. They are
		// no longer part of the configured count, and they leave once the
		// task returns.
		synctest.Wait()
		if got := inFlight.Load(); got != 10 {
			t.Fatalf("in-flight tasks after scale-down = %d, want 10", got)
		}
	})
}

func TestResizeDoesNotRetireReplacementWorkers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		pool := New(2, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := pool.Run(ctx)

		// Park the original workers, retire them, then start replacements.
		// A shared stop queue would let the new workers consume the old
		// "please exit" signals and never read these inputs.
		synctest.Wait()
		pool.Resize(ctx, 0)
		synctest.Wait()
		pool.Resize(ctx, 2)
		synctest.Wait()
		if got := pool.CurrentWorkers(); got != 2 {
			t.Fatalf("workers = %d, want 2", got)
		}

		go func() {
			inputs <- 1
			inputs <- 2
			close(inputs)
		}()
		got := 0
		for res := range results {
			if res.Err != nil {
				t.Fatalf("result error: %v", res.Err)
			}
			got++
		}
		if got != 2 {
			t.Fatalf("results = %d, want 2", got)
		}
	})
}

func TestScaleToZeroDeliversInFlightResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		release := make(chan struct{})
		task := func(ctx context.Context, n int) (int, error) {
			select {
			case <-release:
				return n, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}

		pool := New(2, inputs, task, nil)
		ctx, cancel := context.WithCancel(t.Context())
		results := pool.Run(ctx)
		inputs <- 1
		synctest.Wait()

		pool.Resize(ctx, 0)
		if got := pool.CurrentWorkers(); got != 0 {
			t.Fatalf("workers = %d, want 0", got)
		}
		close(release)
		// The retired worker is blocked delivering its result. Receive it
		// before cancelling, or the pool context would drop the send.
		synctest.Wait()
		res := <-results
		if res.Err != nil || res.Value != 1 {
			t.Fatalf("result = %+v, want value 1", res)
		}

		// No worker is left to observe close(inputs). Cancellation is what
		// finishes a pool that was scaled to zero.
		close(inputs)
		cancel()
		for range results {
			t.Fatal("unexpected extra result")
		}
	})
}

func TestResizeBeforeRunAndCancelledResize(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		close(inputs)

		pool := New(4, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, nil)
		if got := pool.CurrentWorkers(); got != 4 {
			t.Fatalf("workers = %d, want 4", got)
		}

		ctx, cancel := context.WithCancel(t.Context())
		pool.Resize(ctx, 2)
		if got := pool.CurrentWorkers(); got != 2 {
			t.Fatalf("workers after Resize = %d, want 2", got)
		}

		cancel()
		pool.Resize(ctx, 7)
		if got := pool.CurrentWorkers(); got != 2 {
			t.Fatalf("cancelled Resize changed workers to %d", got)
		}

		for range pool.Run(t.Context()) {
		}
		if got := pool.CurrentWorkers(); got != 2 {
			t.Fatalf("workers after shutdown = %d, want 2", got)
		}
	})
}

func TestObserverMayReenterPool(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		obs := &reenterObserver{}
		pool := New(1, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, obs)
		obs.pool = pool

		ctx, cancel := context.WithCancel(t.Context())
		results := pool.Run(ctx)
		pool.Resize(ctx, 3)
		synctest.Wait()
		close(inputs)
		cancel()
		for range results {
		}

		if len(obs.counts) != 2 || obs.counts[0] != 1 || obs.counts[1] != 3 {
			t.Fatalf("WorkerCountChanged calls = %v, want [1 3]", obs.counts)
		}
	})
}

type reenterObserver struct {
	pool   *Pool[int, int]
	counts []int
}

func (o *reenterObserver) TaskStarted()                      {}
func (o *reenterObserver) TaskFinished(time.Duration, error) {}
func (o *reenterObserver) WorkerCountChanged(n int) {
	// CurrentWorkers takes the pool lock. This deadlocks if Resize calls
	// the observer while holding that lock.
	_ = o.pool.CurrentWorkers()
	o.counts = append(o.counts, n)
}

func TestStressResizing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inputs := make(chan int)
		pool := New(5, inputs, func(ctx context.Context, n int) (int, error) {
			return n, nil
		}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := pool.Run(ctx)

		go func() {
			defer close(inputs)
			for i := 0; i < 10; i++ {
				select {
				case inputs <- i:
				case <-ctx.Done():
					return
				}
			}
			<-ctx.Done()
		}()
		go func() {
			for range results {
			}
		}()

		for i := 0; i < 50; i++ {
			want := 1
			if i%2 == 0 {
				want = 20
			}
			pool.Resize(ctx, want)
			synctest.Wait()
			if got := pool.CurrentWorkers(); got != want {
				t.Fatalf("iteration %d: workers = %d, want %d", i, got, want)
			}
		}
	})
}

func TestCleanShutdownHasNoGoroutineLeak(t *testing.T) {
	before, _ := leakSnapshot(t)

	inputs := make(chan int, 8)
	pool := New(2, inputs, func(ctx context.Context, n int) (int, error) {
		return n, nil
	}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	results := pool.Run(ctx)

	pool.Resize(ctx, 6)
	pool.Resize(ctx, 1)
	for i := 0; i < 8; i++ {
		inputs <- i
	}
	close(inputs)
	for range results {
	}

	after, text := leakSnapshot(t)
	if after > before {
		t.Fatalf("goroutineleak profile grew from %d to %d\n%s", before, after, text)
	}
}

func leakSnapshot(t *testing.T) (int, string) {
	t.Helper()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("goroutineleak profile is unavailable")
	}
	// WriteTo runs a leak-detection GC. Count alone reports the previous one.
	var buf bytes.Buffer
	if err := p.WriteTo(&buf, 1); err != nil {
		t.Fatalf("write goroutineleak profile: %v", err)
	}
	return p.Count(), buf.String()
}
