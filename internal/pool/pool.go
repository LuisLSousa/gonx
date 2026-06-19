// Package pool provides a minimal static work-partitioning helper used by the
// parallel graph metrics. It deliberately avoids channels in the hot path: the
// index range [0, n) is split into contiguous chunks, one per worker, and each
// worker is handed a private slot index so it can accumulate into per-worker
// storage without false sharing or locking.
package pool

import (
	"runtime"
	"sync"
)

// Workers returns the degree of parallelism used by ParallelFor for a problem of
// size n: GOMAXPROCS, but never more workers than items.
func Workers(n int) int {
	w := runtime.GOMAXPROCS(0)
	if w > n {
		w = n
	}
	if w < 1 {
		w = 1
	}
	return w
}

// ParallelFor splits [0, n) across w workers and invokes body for each index.
// worker is the caller's slot in [0, w); body for a given index always runs on
// exactly one worker, so body may write to per-worker[worker] without
// synchronization. ParallelFor blocks until all indices have been processed.
//
// The partition is static and contiguous, so for a fixed n and w the assignment
// of indices to workers is deterministic — callers relying on order-independent
// reductions therefore get reproducible results regardless of scheduling.
func ParallelFor(n, w int, body func(i, worker int)) {
	if n <= 0 {
		return
	}
	if w <= 1 || n == 1 {
		for i := 0; i < n; i++ {
			body(i, 0)
		}
		return
	}
	var wg sync.WaitGroup
	chunk := (n + w - 1) / w
	for worker := 0; worker < w; worker++ {
		start := worker * chunk
		if start >= n {
			break
		}
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(start, end, worker int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				body(i, worker)
			}
		}(start, end, worker)
	}
	wg.Wait()
}
