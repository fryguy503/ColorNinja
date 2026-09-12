package engine

import (
	"context"
	"sort"
)

// A buried run can still tint every later surface. Removing it must rebuild
// the optics and reallocate the remaining runs: a direct merge can be slightly
// worse even when the simpler order has a better reachable allocation.
func simplifyStack(ctx context.Context, initial stackState, lib Library, bases []int, target []Vec, weights []float64, o Options, progress Reporter, boundaries []stackBoundary) (stackState, error) {
	if !o.HueForge.compatibleOptics() {
		return initial, ctx.Err()
	}
	best := initial
	for pass := 0; pass < 4; pass++ {
		palette, err := selectStackPalette(ctx, best, target, weights, o, boundaries)
		if err != nil {
			return best, err
		}
		visible := make([]bool, len(best.indices))
		for i, id := range palette.ids {
			if palette.masses[i] > 0 {
				visible[palette.positions[id]-1] = true
			}
		}
		buried := false
		for _, used := range visible[1:] {
			buried = buried || !used
		}
		if !buried {
			break
		}
		candidates := []stackState{}
		seen := map[string]bool{}
		err = mergeStackRuns(best, func(ids, runs []int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			key := stackScheduleKey(ids, runs)
			if seen[key] || !validStackOrder(ids, o) || !completeConstraints(ids, lib, o) {
				return nil
			}
			seen[key] = true
			s := rebuildStack(ids, runs, lib, o.HueForge)
			var err error
			s.score, err = stateScore(ctx, s, target, weights, o, boundaries...)
			if err == nil && finite(s.score) {
				candidates = append(candidates, s)
			}
			return err
		})
		if err != nil {
			return best, err
		}
		sort.SliceStable(candidates, func(i, j int) bool { return depthStateLess(candidates[i], candidates[j]) })
		// Bound lookahead independently of the number of spools or run ceiling.
		// Keep separate allocations: merging left or right can lead to different
		// blends even when the remaining spool order is identical.
		next := best
		for _, candidate := range candidates[:min(4, len(candidates))] {
			search := o
			if search.HueForge.MaxRuns > 0 {
				search.HueForge.MaxRuns = len(candidate.indices)
			}
			refined, err := refineStack(ctx, candidate, lib, bases, target, weights, search, progress, boundaries...)
			if err != nil {
				return best, err
			}
			if len(refined.indices) < len(best.indices) && refinedStackLess(refined, next) {
				next = refined
			}
		}
		if len(next.indices) >= len(best.indices) {
			break
		}
		best = next
	}
	return best, ctx.Err()
}
