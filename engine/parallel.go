package engine

import "sync"

// NumWorkers is the number of goroutines for parallel operations.
var NumWorkers = 4

// rngPool is set by NewWorld; workers access via WorkerRNG.
var rngPool *RNGPool

// SetRNGPool stores the pool for WorkerRNG access.
func SetRNGPool(p *RNGPool) { rngPool = p }

// WorkerRNG returns the dedicated RNG for worker idx (0-based, no lock).
func WorkerRNG(idx int) *RNG {
	if rngPool == nil || idx >= len(rngPool.RNGs) {
		return nil
	}
	return rngPool.RNGs[idx]
}

// ParaFor splits [0, n) across goroutines.
// fn receives (start, end) range only — for code that doesn't need RNG.
func ParaFor(n int, fn func(start, end int)) {
	if n <= 1 || NumWorkers <= 1 {
		fn(0, n)
		return
	}
	chunkSize := (n + NumWorkers - 1) / NumWorkers
	var wg sync.WaitGroup
	for i := 0; i < n; i += chunkSize {
		end := i + chunkSize
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			fn(s, e)
		}(i, end)
	}
	wg.Wait()
}

// ParaForRNG splits [0, n) across goroutines, passing a per-worker RNG.
// fn receives (start, end, workerIdx). Use WorkerRNG(idx) to get the local RNG.
func ParaForRNG(n int, fn func(start, end, workerIdx int)) {
	if n <= 1 || NumWorkers <= 1 {
		fn(0, n, 0)
		return
	}
	chunkSize := (n + NumWorkers - 1) / NumWorkers
	var wg sync.WaitGroup
	workerIdx := 0
	for i := 0; i < n; i += chunkSize {
		end := i + chunkSize
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(s, e, wid int) {
			defer wg.Done()
			fn(s, e, wid)
		}(i, end, workerIdx)
		workerIdx++
	}
	wg.Wait()
}
