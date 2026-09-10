package engine

import (
	"context"
	"math"
	"sort"
)

func pruneStackStates(states []stackState, limit int) []stackState {
	sort.SliceStable(states, func(i, j int) bool { return stateLess(states[i], states[j]) })
	keep := min(limit, len(states))
	// Discarded states must not keep their optical/palette arrays alive.
	clear(states[keep:])
	return states[:keep]
}

// Different complete schedules refine independently. Keep each refinement's
// incumbent, visited cache, and ordered moves private; only the original caller
// reports progress and compares the finished schedules in their original order.
func refineStackCandidates(ctx context.Context, candidates []stackState, lib Library, bases []int, target []Vec, weights []float64, o Options, progress Reporter, boundaries []stackBoundary) ([]stackState, error) {
	work := int64(len(candidates)) * int64(len(lib.Filaments)) * int64(o.HueForge.MaxLayers()) * int64(max(1, len(target)))
	workers := processingWorkers(len(candidates), work, 1024, stackWorkerScratch(o, len(target)))
	if err := report(ctx, progress, "Refining filament order and layers", .63); err != nil {
		return nil, err
	}
	out := make([]stackState, len(candidates))
	err := parallelRanges(ctx, len(candidates), workers, func(ctx context.Context, slot, lo, hi int) error {
		var reporter Reporter
		if workers == 1 {
			reporter = progress
		}
		for i := lo; i < hi; i++ {
			var err error
			out[i], err = refineStack(ctx, candidates[i], lib, bases, target, weights, o, reporter, boundaries...)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// Partition the beam into contiguous ranges. Each worker keeps only a bounded
// candidate pool and one winner per depth. Top-k of their union is identical to
// serial top-k, including the original schedule tie-breaks. Shared context data
// and the prior depth frontier are read-only until every worker has joined.
func expandStackBeam(ctx context.Context, beam []stackState, lib Library, o Options, position, size int, target []Vec, weights []float64, depths depthCandidates, required []int, requiredTop int, boundaries []stackBoundary) ([]stackState, error) {
	work := int64(len(beam)) * int64(len(lib.Filaments)) * int64(o.HueForge.TransitionLayers()) * int64(max(1, len(target)))
	workers := processingWorkers(len(beam), work, 2048, stackWorkerScratch(o, len(target)))
	if workers == 1 {
		return expandStackRange(ctx, beam, lib, o, position, size, target, weights, depths, required, requiredTop, boundaries)
	}
	type expansion struct {
		states []stackState
		depths depthCandidates
	}
	results := make([]expansion, workers)
	err := parallelRanges(ctx, len(beam), workers, func(ctx context.Context, slot, lo, hi int) error {
		local := depthCandidates{}
		for layer, state := range depths {
			local[layer] = state
		}
		states, err := expandStackRange(ctx, beam[lo:hi], lib, o, position, size, target, weights, local, required, requiredTop, boundaries)
		results[slot] = expansion{states, local}
		return err
	})
	if err != nil {
		return nil, err
	}
	expanded := []stackState{}
	for _, result := range results {
		expanded = append(expanded, result.states...)
		for _, state := range result.depths {
			depths.record(state)
		}
	}
	return pruneStackStates(expanded, o.HueForge.BeamWidth), ctx.Err()
}

func expandStackRange(ctx context.Context, beam []stackState, lib Library, o Options, position, size int, target []Vec, weights []float64, depths depthCandidates, required []int, requiredTop int, boundaries []stackBoundary) ([]stackState, error) {
	h := o.HueForge
	final := position == size
	remaining := size - position
	expanded := []stackState{}
	for _, s := range beam {
		if h.HighlightOnlyAtTop && requiredTop >= 0 && contains(s.indices, requiredTop) {
			continue
		}
		maxRun := h.TransitionLayers() - s.used - remaining
		if maxRun < 1 {
			continue
		}
		for i, f := range lib.Filaments {
			if h.HighlightOnlyAtTop && !final && i == requiredTop {
				continue
			}
			if final && requiredTop >= 0 && i != requiredTop {
				continue
			}
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if !validStackOrder(append(append([]int{}, s.indices...), i), o) {
				continue
			}
			missing := 0
			for _, id := range required {
				if id != i && !contains(s.indices, id) {
					missing++
				}
			}
			if missing > remaining {
				continue
			}
			current := s.current
			rgbs := []RGB{}
			layers, positions := []int{}, []int{}
			distances := append([]float64{}, s.distances...)
			for run := 1; run <= maxRun; run++ {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				rgb := current.step(f, h, run == 1)
				rgbs = append(rgbs, rgb)
				layers = append(layers, h.BaseLayers()+s.used+run)
				positions = append(positions, position)
				lab := o.colorVector(rgb)
				for j, t := range target {
					distances[j] = math.Min(distances[j], distance(t, lab))
				}
				if final && run != maxRun && !h.AutoDepth {
					continue
				}
				state := stackState{indices: append(append([]int{}, s.indices...), i), runs: append(append([]int{}, s.runs...), run), used: s.used + run, current: current, rgbs: append(append([]RGB{}, s.rgbs...), rgbs...), layers: append(append([]int{}, s.layers...), layers...), positions: append(append([]int{}, s.positions...), positions...), distances: append([]float64{}, distances...), score: dot(weights, distances)}
				if final && h.AutoDepth && run != maxRun {
					if err := depths.consider(ctx, state, target, weights, o, boundaries...); err != nil {
						return nil, err
					}
					continue
				}
				if final {
					v, e := stateScore(ctx, state, target, weights, o, boundaries...)
					if e != nil {
						return nil, e
					}
					state.score = v
					if h.AutoDepth {
						depths.record(state)
					}
				}
				expanded = append(expanded, state)
				// Keep a bounded pool without changing the deterministic top-k ordering.
				if len(expanded) > max(1024, h.BeamWidth*4) {
					expanded = pruneStackStates(expanded, h.BeamWidth)
				}
			}
		}
	}
	return pruneStackStates(expanded, h.BeamWidth), ctx.Err()
}
