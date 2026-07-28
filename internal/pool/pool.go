// Package pool provides a minimal static work-partitioning helper used by the
// parallel graph metrics. It deliberately avoids channels in the hot path: the
// index range [0, n) is split into contiguous chunks, one per worker, and each
// worker processes its whole chunk in one call so it can accumulate into local
// variables and publish a single result per worker slot.
package pool

import (
	"runtime"
	"sync"
)

// Workers returns the degree of parallelism used by ParallelChunks for a problem
// of size n: GOMAXPROCS, but never more workers than items.
func Workers(n int) int {
	return max(min(runtime.GOMAXPROCS(0), n), 1)
}

// ParallelChunks splits [0, n) into w contiguous chunks and invokes body once per
// chunk with its half-open range [start, end) and the worker's slot in [0, w).
// Chunks run concurrently; ParallelChunks blocks until all have completed.
//
// Because each worker owns an entire chunk, body can accumulate into locals and
// write per-worker results once at the end, rather than updating shared slices
// element-by-element (which would false-share cache lines between workers).
//
// The partition is static and contiguous, so for a fixed n and w the assignment
// of indices to workers is deterministic — callers relying on order-independent
// reductions therefore get reproducible results regardless of scheduling.
func ParallelChunks(n, w int, body func(worker, start, end int)) {
	if n <= 0 {
		return
	}
	if w <= 1 || n == 1 {
		body(0, 0, n)
		return
	}
	var wg sync.WaitGroup
	chunk := (n + w - 1) / w
	for worker := range w {
		start := worker * chunk
		if start >= n {
			break
		}
		end := min(start+chunk, n)
		wg.Add(1)
		go func() {
			defer wg.Done()
			body(worker, start, end)
		}()
	}
	wg.Wait()
}
