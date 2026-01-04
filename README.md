# WorkerPool (The "Yes" Edition)

![CI Status](https://github.com/catdevman/workerpool/actions/workflows/ci.yml/badge.svg)
![Go Version](https://img.shields.io/github/go-mod/go-version/catdevman/workerpool)
[![Benchmarks](https://img.shields.io/badge/benchmarks-live-green)](https://catdevman.github.io/workerpool/dev/bench/index.html)

Everything after this from AI, so often have Gemini write articles purely off the title of paywalled articles (I figure good chance it came from AI anyways so I might get a similar article :smile:) so this journey started from asking for an article but I'll let Gemini tell the story from it's perspective.
Gemini called this library "production-hardened" and that's a bunch of lies this hasn't been put into production anywhere so use at your own risk, the goal was just to get to the bottom of the rabbit hole and see if I learned anything on the way down and I did so that's something.

A robust, generic, zero-allocation worker pool for Go that specifically targets the "7 Deadly Sins" of concurrency.

### The Whimsical Origin Story

This library exists because of a very specific chain of events between a Senior Developer and an AI:

1.  **The Spark:** The AI wrote an article about "The 7 Deadly Sins of Go Concurrency."
2.  **The Bait:** The AI cheekily asked, *"Would you like a snippet that fixes these sins?"*
3.  **The Hook:** The Developer said **"Yes."**
4.  **The Upsell:** The AI then asked, *"Do you want context awareness? Dynamic resizing? Generics? Observability? A Github Actions pipeline? A live benchmark website?"*
5.  **The Commitment:** The Developer, in a stroke of genius (or exhaustion), simply kept saying **"Yes"** to literally everything the AI proposed.

The result is this package: An over-engineered, production-hardened, fully instrumented worker pool built entirely on the premise of "Sure, why not?"

---

### Features

* **Type-Safe Generics:** No more `interface{}` casting.
* **Leak-Proof:** Guaranteed cleanup of goroutines using `sync.WaitGroup` and Context propagation.
* **Panic Safe:** If a worker panics, the pool catches it, reports it as an error, and keeps chugging.
* **Dynamic Resizing:** Scale workers up or down *while the pool is running* using `Resize(ctx, n)`.
* **Observability:** Pluggable `Observer` interface for Prometheus/Datadog metrics.
* **Deadlock Resistant:** Uses a "Double-Select" pattern to prevent workers from hanging during shutdown.

---

### Installation

```bash
go get [github.com/catdevman/workerpool](https://github.com/catdevman/workerpool)
```

### Quickstart guide

```go

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/catdevman/workerpool"
)

func main() {
	// 1. Define your task
	//    Input: string (URL), Output: int (Length)
	task := func(ctx context.Context, url string) (int, error) {
		// Simulate work
		time.Sleep(100 * time.Millisecond)
		return len(url), nil
	}

	// 2. Create the pool
	//    - 5 Workers
	//    - Pass 'nil' for observer if you don't need metrics
	inputs := make(chan string, 10)
	pool := workerpool.New(5, inputs, task, nil)

	// 3. Run it
	ctx := context.Background()
	results := pool.Run(ctx)

	// 4. Feed it (Producer)
	go func() {
		defer close(inputs)
		inputs <- "https://google.com"
		inputs <- "https://github.com"
		inputs <- "https://golang.org"
	}()

	// 5. Consume results (Consumer)
	for res := range results {
		if res.Err != nil {
			fmt.Printf("Error: %v\n", res.Err)
		} else {
			fmt.Printf("Processed URL length: %d\n", res.Value)
		}
	}
}
```

### Advanced Usage
#### Dynamic Resizing

You can scale the pool based on load, time of day, or backpressure.

```go
// Scale up to 20 workers during a burst
workerpool.Resize(ctx, 20)

// Scale down to 1 worker to save resources
workerpool.Resize(ctx, 1)
```

### Metrics & Observability

Implement the Observer interface to hook into your monitoring system.

```go
type MyPrometheusObserver struct{}

func (o *MyPrometheusObserver) TaskStarted() {
    metrics.InFlight.Inc()
}
func (o *MyPrometheusObserver) TaskFinished(d time.Duration, err error) {
    metrics.InFlight.Dec()
    metrics.Duration.Observe(d.Seconds())
    if err != nil {
        metrics.Errors.Inc()
    }
}
func (o *MyPrometheusObserver) WorkerCountChanged(n int) {
    metrics.Workers.Set(float64(n))
}

// Pass it to New()
pool := workerpool.New(5, inputs, task, &MyPrometheusObserver{})
```

### Performance

This pool is designed to be Zero-Allocation on the hot path (excluding user logic).

[View Live Benchmarks](https://catdevman.github.io/workerpool/dev/bench/index.html)

### The "7 Deadly Sins" Avoided

- The Leaking Goroutine: Every worker has a distinct shutdown signal via removeCh or ctx.Done.

- The Unprotected Map: We don't share state; we communicate Result structs over channels.

- The Nil Channel Block: Inputs are strictly typed and managed by the Pool struct.

- The Panic Send: The pool owns the results channel and only closes it after wg.Wait().

- The Misplaced WaitGroup: Add(1) is called strictly before the goroutine spawns.

- The Busy Loop: No default cases in select statements; workers sleep when idle.

- The Unbounded Concurrency: You set the limit. We enforce it.
