window.BENCHMARK_DATA = {
  "lastUpdate": 1767567425452,
  "repoUrl": "https://github.com/catdevman/workerpool",
  "entries": {
    "Benchmark": [
      {
        "commit": {
          "author": {
            "email": "catdevman@gmail.com",
            "name": "Lucas Pearson",
            "username": "catdevman"
          },
          "committer": {
            "email": "catdevman@gmail.com",
            "name": "Lucas Pearson",
            "username": "catdevman"
          },
          "distinct": true,
          "id": "e53b4318f700059faf73151ff0b77467f5983534",
          "message": "add a new workflow for benchmarking",
          "timestamp": "2026-01-04T17:15:21-05:00",
          "tree_id": "a5b06c04d7ba283a502cbee2ac2fe45e23873407",
          "url": "https://github.com/catdevman/workerpool/commit/e53b4318f700059faf73151ff0b77467f5983534"
        },
        "date": 1767565058698,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkThroughput",
            "value": 528.2,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "2294203 times\n4 procs"
          },
          {
            "name": "BenchmarkThroughput - ns/op",
            "value": 528.2,
            "unit": "ns/op",
            "extra": "2294203 times\n4 procs"
          },
          {
            "name": "BenchmarkThroughput - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "2294203 times\n4 procs"
          },
          {
            "name": "BenchmarkThroughput - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "2294203 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound",
            "value": 11556,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "103802 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound - ns/op",
            "value": 11556,
            "unit": "ns/op",
            "extra": "103802 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "103802 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "103802 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations",
            "value": 469.1,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "2528704 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations - ns/op",
            "value": 469.1,
            "unit": "ns/op",
            "extra": "2528704 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "2528704 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "2528704 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "catdevman@gmail.com",
            "name": "Lucas Pearson",
            "username": "catdevman"
          },
          "committer": {
            "email": "catdevman@gmail.com",
            "name": "Lucas Pearson",
            "username": "catdevman"
          },
          "distinct": true,
          "id": "4db9421527d9c352c6e21906766a498b96932e3b",
          "message": "well here we go again... I thought it'd be neat to track a metric and scale this... well Gemini had some ideas on how to do that",
          "timestamp": "2026-01-04T17:56:26-05:00",
          "tree_id": "ef7c20d514f27fb59aca6c860dbf8bc8843844e6",
          "url": "https://github.com/catdevman/workerpool/commit/4db9421527d9c352c6e21906766a498b96932e3b"
        },
        "date": 1767567424980,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkThroughput",
            "value": 495.2,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "2450253 times\n4 procs"
          },
          {
            "name": "BenchmarkThroughput - ns/op",
            "value": 495.2,
            "unit": "ns/op",
            "extra": "2450253 times\n4 procs"
          },
          {
            "name": "BenchmarkThroughput - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "2450253 times\n4 procs"
          },
          {
            "name": "BenchmarkThroughput - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "2450253 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound",
            "value": 11311,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "105673 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound - ns/op",
            "value": 11311,
            "unit": "ns/op",
            "extra": "105673 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "105673 times\n4 procs"
          },
          {
            "name": "BenchmarkIOBound - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "105673 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations",
            "value": 366.1,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "3240577 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations - ns/op",
            "value": 366.1,
            "unit": "ns/op",
            "extra": "3240577 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "3240577 times\n4 procs"
          },
          {
            "name": "BenchmarkAllocations - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "3240577 times\n4 procs"
          }
        ]
      }
    ]
  }
}