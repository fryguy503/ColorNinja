package engine

import (
	"context"
	"math"
)

// Runs are contiguous uses, while the color budget counts distinct spools.
// Adjacent equal IDs must remain one cumulative optical run.
func validStackOrder(ids []int, o Options) bool {
	seen := make(map[int]bool, len(ids))
	for pos, id := range ids {
		if pos > 0 && ids[pos-1] == id {
			return false
		}
		if seen[id] && o.HueForge.MaxRuns == 0 {
			return false
		}
		seen[id] = true
	}
	return len(seen) <= o.Colors && (o.HueForge.MaxRuns == 0 || len(ids) <= o.HueForge.MaxRuns)
}

// rebuildStack evaluates a complete, physically reachable stack after changing
// its order, filaments, or layer allocation. The base thickness stays fixed.
func rebuildStack(ids, runs []int, lib Library, h HueForgeOptions) stackState {
	s := stackState{indices: append([]int(nil), ids...), runs: append([]int(nil), runs...)}
	s.current = baseOptics(lib.Filaments[ids[0]], h)
	s.rgbs = append(s.rgbs, s.current.RGB(h))
	s.layers = append(s.layers, h.BaseLayers())
	s.positions = append(s.positions, 1)
	for layer := h.BaseLayers(); h.frontlit() && layer < runs[0]; layer++ {
		rgb := s.current.step(lib.Filaments[ids[0]], h, false)
		s.used++
		s.rgbs = append(s.rgbs, rgb)
		s.layers = append(s.layers, layer+1)
		s.positions = append(s.positions, 1)
	}
	for pos := 1; pos < len(ids); pos++ {
		for layer := 0; layer < runs[pos]; layer++ {
			rgb := s.current.step(lib.Filaments[ids[pos]], h, layer == 0)
			s.used++
			s.rgbs = append(s.rgbs, rgb)
			s.layers = append(s.layers, h.BaseLayers()+s.used)
			s.positions = append(s.positions, pos+1)
		}
	}
	return s
}

// A beam judges incomplete prefixes before their useful later blends exist.
// Refine completed stacks so an early greedy choice cannot permanently consume
// the layers needed for a different shade. Every accepted move improves the
// same capped, culled palette objective used by the final export.
func refineStack(ctx context.Context, initial stackState, lib Library, bases []int, target []Vec, weights []float64, o Options, progress Reporter, boundaries ...stackBoundary) (stackState, error) {
	best := initial
	for pass := 0; pass < 16; pass++ {
		if err := report(ctx, progress, "Refining filament order and layers", .63+.015*float64(pass)/16); err != nil {
			return best, err
		}
		next := best
		try := func(ids, runs []int) error {
			if !validStackOrder(ids, o) {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			s := rebuildStack(ids, runs, lib, o.HueForge)
			// The uncapped nearest-color score is a lower bound; skip a full
			// palette selection when even that cannot improve the incumbent.
			d := make([]float64, len(target))
			for j := range d {
				d[j] = math.Inf(1)
			}
			for _, rgb := range s.rgbs {
				lab := o.colorVector(rgb)
				for j, t := range target {
					d[j] = math.Min(d[j], distance(t, lab))
				}
			}
			if dot(weights, d) >= next.score*next.score-1e-12 {
				return nil
			}
			var err error
			s.score, err = stateScore(ctx, s, target, weights, o, boundaries...)
			if err == nil && s.score < next.score-1e-9 {
				next = s
			}
			return err
		}
		for pos := range best.indices {
			for id := range lib.Filaments {
				if best.indices[pos] == id || (pos == 0 && !contains(bases, id)) {
					continue
				}
				ids := append([]int(nil), best.indices...)
				ids[pos] = id
				if err := try(ids, best.runs); err != nil {
					return best, err
				}
			}
			for other := pos + 1; other < len(best.indices); other++ {
				if pos == 0 && !contains(bases, best.indices[other]) {
					continue
				}
				ids := append([]int(nil), best.indices...)
				ids[pos], ids[other] = ids[other], ids[pos]
				if err := try(ids, best.runs); err != nil {
					return best, err
				}
			}
			if pos == 0 || best.runs[pos] <= 1 {
				continue
			}
			for other := 1; other < len(best.indices); other++ {
				if pos == other {
					continue
				}
				runs := append([]int(nil), best.runs...)
				runs[pos]--
				runs[other]++
				if err := try(best.indices, runs); err != nil {
					return best, err
				}
			}
		}
		if next.score >= best.score-1e-9 {
			break
		}
		best = next
	}
	return best, ctx.Err()
}
