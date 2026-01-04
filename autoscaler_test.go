package workerpool

import (
	"context"
	"testing"
	"time"
)

// noOpTask is a helper for tests that don't need real work
func noOpTask(_ context.Context, _ int) (int, error) {
	return 0, nil
}

func TestAutoScaler_ThresholdStrategy(t *testing.T) {
	// 1. Setup Pool (Start with 5 workers)
	inputs := make(chan int)
	pool := New(5, inputs, noOpTask, nil)
	pool.Run(context.Background())

	// 2. Setup Metric Mock
	// We control this value to simulate CPU load
	var currentMetric float64 = 0.0
	mockFetcher := func() float64 {
		return currentMetric
	}

	// 3. Setup Scaler
	// Check every 10ms. Min: 1, Max: 10.
	interval := 10 * time.Millisecond
	scaler := NewAutoScaler(pool, interval, 1, 10, mockFetcher)

	// Strategy:
	// > 80.0 -> Add 2 workers
	// < 20.0 -> Remove 2 workers
	scaler.WithThresholdStrategy(20.0, 80.0, 2)

	scaler.Start()
	defer scaler.Stop()

	// --- SCENARIO 1: High Load (Scale Up) ---
	currentMetric = 90.0              // Above 80.0 threshold
	time.Sleep(50 * time.Millisecond) // Wait for a few ticks

	if pool.CurrentWorkers() <= 5 {
		t.Errorf("Expected scaling UP from 5, got %d", pool.CurrentWorkers())
	}

	// --- SCENARIO 2: Low Load (Scale Down) ---
	currentMetric = 10.0 // Below 20.0 threshold
	// It might take a few cycles to scale down if the step is small,
	// but our step is 2, so it should happen quickly.
	time.Sleep(50 * time.Millisecond)

	if pool.CurrentWorkers() >= 7 { // It likely went 5->7->9, then should drop
		t.Errorf("Expected scaling DOWN, got %d", pool.CurrentWorkers())
	}
}

func TestAutoScaler_InverseStrategy(t *testing.T) {
	// Inverse is for Memory: High Metric = Scale DOWN (Panic mode)

	inputs := make(chan int)
	pool := New(5, inputs, noOpTask, nil)
	pool.Run(context.Background())

	var currentMetric float64 = 0.0
	mockFetcher := func() float64 { return currentMetric }

	scaler := NewAutoScaler(pool, 10*time.Millisecond, 1, 10, mockFetcher)

	// Strategy:
	// > 80.0 -> Remove 2 workers (Save RAM)
	// < 20.0 -> Add 2 workers (Use idle RAM)
	scaler.WithInverseThresholdStrategy(20.0, 80.0, 2)

	scaler.Start()
	defer scaler.Stop()

	// --- SCENARIO: OOM Risk (Scale Down) ---
	currentMetric = 95.0
	time.Sleep(50 * time.Millisecond)

	if pool.CurrentWorkers() >= 5 {
		t.Errorf("Expected scaling DOWN from 5 due to high memory, got %d", pool.CurrentWorkers())
	}
}

func TestAutoScaler_HardBounds(t *testing.T) {
	// Test Min/Max enforcement

	inputs := make(chan int)
	pool := New(2, inputs, noOpTask, nil) // Start at 2
	pool.Run(context.Background())

	// Strategy that ALWAYS adds 10 workers
	mockFetcher := func() float64 { return 100.0 }

	// Min: 1, Max: 5
	scaler := NewAutoScaler(pool, 10*time.Millisecond, 1, 5, mockFetcher)

	// Custom aggressive strategy
	scaler.SetStrategy(func(curr int, m float64) int {
		return curr + 10 // Try to go way above Max
	})

	scaler.Start()
	defer scaler.Stop()

	time.Sleep(50 * time.Millisecond)

	if pool.CurrentWorkers() > 5 {
		t.Errorf("Expected to hit MAX ceiling of 5, but got %d", pool.CurrentWorkers())
	}
}
