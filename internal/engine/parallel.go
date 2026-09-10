package engine

import (
	"context"
	"errors"
	"runtime"
	"sync"
)

const maxProcessingWorkers = 16
const parallelScratchBudget int64 = 32 << 20

// Leave a logical CPU available on larger machines, respect GOMAXPROCS
// (including an explicit user limit), and avoid workers without useful work. Scratch
// is an estimate of each worker's live temporary data, not total allocations.
// One worker is always allowed: parallelism must not reject a valid serial job.
func processingWorkers(items int, work, minimumWork, scratchPerWorker int64) int {
	return workerCount(runtime.GOMAXPROCS(0), items, work, minimumWork, scratchPerWorker)
}

func workerCount(cpus, items int, work, minimumWork, scratchPerWorker int64) int {
	capacity := max(1, cpus)
	// Reserving a whole core on a small CPU severely slows existing smoothing
	// and mapping. Preserve their available concurrency on one-to-four CPUs.
	if capacity > 4 {
		capacity--
	}
	n := min(maxProcessingWorkers, capacity, max(1, items))
	if minimumWork > 0 {
		n = min(n, int(max(1, min(int64(maxProcessingWorkers), work/minimumWork))))
	}
	if scratchPerWorker > 0 {
		n = min(n, int(max(1, parallelScratchBudget/scratchPerWorker)))
	}
	return n
}

// Each slot owns one contiguous range and its scratch/output. The caller merges
// slots in source order after this returns; completion order never breaks ties
// or changes floating-point summation. No queue grows with the input. All workers
// are joined, including on cancellation/error, before the caller reuses buffers.
// Worker callbacks must not invoke a shared Reporter or start nested pools.
func parallelRanges(ctx context.Context, items, workers int, run func(context.Context, int, int, int) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if items <= 0 {
		return nil
	}
	workers = max(1, min(workers, items))
	if workers == 1 {
		if err := run(ctx, 0, 0, items); err != nil {
			return err
		}
		return ctx.Err()
	}
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make([]error, workers)
	var wg sync.WaitGroup
	work := func(slot int) {
		errs[slot] = run(ctx, slot, items*slot/workers, items*(slot+1)/workers)
		if errs[slot] != nil {
			cancel()
		}
	}
	for slot := 1; slot < workers; slot++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(slot)
		}()
	}
	work(0)
	wg.Wait()
	if err := parent.Err(); err != nil {
		return err
	}
	// A sibling's cancellation must not hide the original failure.
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Beam pools, retained depth winners, optical histories, and palette scoring
// dominate stack scratch. Include slice growth/headroom in this conservative
// estimate; a deep stack or wide beam automatically reduces parallelism.
func stackWorkerScratch(o Options, targetCount int) int64 {
	layers := int64(o.HueForge.MaxLayers())
	state := 512 + layers*64 + int64(targetCount)*16
	pool := int64(max(1024, o.HueForge.BeamWidth*4))
	if o.HueForge.AutoDepth {
		pool += layers
	}
	return state*pool*2 + layers*int64(targetCount)*32 + (1 << 20)
}
