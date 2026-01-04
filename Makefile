# Makefile for WorkerPool

# Default target
all: test

# Run standard unit tests
test:
	@echo "--- Running Unit Tests ---"
	go test -v ./...

# Run tests with Race Detector (CRITICAL for concurrency code)
race:
	@echo "--- Running Race Detector ---"
	go test -v -race ./...

# Run benchmarks
bench:
	@echo "--- Running Benchmarks ---"
	go test -bench=. ./...

bench-history:
	@echo "--- Running Benchmarks & Saving History ---"
	@date >> bench_history.txt
	@echo "Commit: $$(git rev-parse --short HEAD)" >> bench_history.txt
	go test -bench=. -benchmem ./... >> bench_history.txt
	@echo "------------------------------------------------" >> bench_history.txt
	@echo "Results saved to bench_history.txt"

# Run a specific test (usage: make run name=TestPanicIsolation)
run:
	@echo "--- Running Specific Test: $(name) ---"
	go test -v -run $(name) ./...

# Check for vet errors
vet:
	go vet ./...

.PHONY: all test race bench run vet
