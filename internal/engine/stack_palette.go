package engine

import (
	"context"
	"math"
)

// Search and export use the same palette, heights, area fractions and score.
// RGBs remain actual stack colors; a selected RGB still has one global height.
type stackPalette struct {
	colors                 []RGB
	layers, positions, ids []int
	masses                 []float64
	colorRMS, score        float64
}

func selectStackPalette(ctx context.Context, s stackState, target []Vec, weights []float64, o Options, boundaries []stackBoundary) (stackPalette, error) {
	colors, baseLayers, basePositions := uniqueStack(s)
	vectors := o.colorVectors(colors)
	ids, masses, colorRMS, err := selectReachable(ctx, vectors, target, weights, o.HueForge.MaxPerceivedColors, o.selectionFraction())
	if err != nil {
		return stackPalette{}, err
	}
	geometry := len(boundaries) > 0 || o.materialOptimization() || o.layerOptimization()
	searching := ctx.Value(layerSearchBarrierKey{}) == true
	limit, limited := ctx.Value(stackColorLimitKey{}).(float64)
	if !limited {
		limit = colorRMS*(1+o.HueForge.SurfaceColorTolerance/100) + 1e-8
	}
	evaluate := func(ids []int, masses []float64, rms float64) (stackPalette, error) {
		p := stackPalette{colors: colors, ids: ids, masses: masses, colorRMS: rms, score: math.Inf(1)}
		barrier := 0.
		if (limited || geometry) && rms > limit {
			if !searching {
				return p, ctx.Err()
			}
			barrier = 1024 * (rms - limit) * (rms - limit)
		}
		selected := make([]RGB, len(ids))
		for i, id := range ids {
			selected[i] = colors[id]
		}
		if excess := layerColorExcess(ctx, selected, target, o); excess > 0 {
			if !searching {
				return p, ctx.Err()
			}
			barrier += 1024 * excess
		}
		if len(boundaries) > 0 && s.surfaceCache == nil {
			// Palette and height trials reuse this stack. Allocate only after
			// the color guards pass, and keep the cache private to this call.
			s.surfaceCache = newStackSurfaceCache(s)
		}
		p.layers = append([]int(nil), baseLayers...)
		p.positions = append([]int(nil), basePositions...)
		if err := chooseStackHeights(ctx, s, colors, p.layers, p.positions, ids, target, o, boundaries); err != nil {
			return p, err
		}
		if !enforceHeightConstraints(ctx, s, colors, p.layers, p.positions, ids, target, o, boundaries) {
			return p, ctx.Err()
		}
		p.score = rms
		if geometry {
			heights := make([]int, len(ids))
			for i, id := range ids {
				heights[i] = p.layers[id]
			}
			p.score = math.Sqrt(rms*rms + barrier + stackGeometryPenalty(ctx, s, selected, heights, target, o, boundaries))
		}
		return p, ctx.Err()
	}
	best, err := evaluate(ids, masses, colorRMS)
	if err != nil || !geometry {
		return best, err
	}
	// Precompute color distances once per stack. Reject color/culling failures
	// before the more expensive height, boundary and material evaluation.
	distances := make([][]float64, len(target))
	for i, v := range target {
		distances[i] = make([]float64, len(colors))
		for j, c := range vectors {
			distances[i][j] = distance(v, c)
		}
	}
	budget := 128
	if o.HueForge.SearchEffort == "refine" {
		budget = 256
	}
	trials := 0
	try := func(trial []int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if trials >= budget {
			return nil
		}
		trials++
		trial = sorted(trial) // stable first-match ties, identical to export
		m, square := make([]float64, len(trial)), 0.
		for i := range target {
			d, at := math.Inf(1), 0
			for j, id := range trial {
				if distances[i][id] < d {
					d, at = distances[i][id], j
				}
			}
			square += weights[i] * d
			m[at] += weights[i]
		}
		for _, mass := range m {
			if mass <= 1e-15 || mass < o.selectionFraction() {
				return nil
			}
		}
		if square >= best.score*best.score-1e-12 {
			return nil
		}
		p, err := evaluate(trial, m, math.Sqrt(square))
		if err == nil && p.score < best.score-1e-9 {
			best = p
		}
		return err
	}
	for pass := 0; pass < 3 && trials < budget; pass++ {
		initial := best
		// Deletion can lower a relief boundary; additions can retain a useful
		// intermediate height. Neither may introduce unused or culled colors.
		for pos := range initial.ids {
			if len(initial.ids) > 1 {
				trial := append([]int(nil), initial.ids[:pos]...)
				trial = append(trial, initial.ids[pos+1:]...)
				if err := try(trial); err != nil {
					return best, err
				}
			}
		}
		// Visit nearby reachable colors first, interleaving slots so a large
		// candidate table cannot consume the budget on the first palette entry.
		neighbors := make([][]int, len(initial.ids))
		for pos, id := range initial.ids {
			neighbors[pos] = nearestStackColors(vectors, id, initial.ids, min(32, len(colors)))
		}
		for rank := 0; rank < min(32, len(colors)) && trials < budget; rank++ {
			for pos := range initial.ids {
				if rank >= len(neighbors[pos]) {
					continue
				}
				id := neighbors[pos][rank]
				trial := append([]int(nil), initial.ids...)
				trial[pos] = id
				if err := try(trial); err != nil {
					return best, err
				}
				if len(initial.ids) < o.HueForge.MaxPerceivedColors {
					if err := try(append(append([]int(nil), initial.ids...), id)); err != nil {
						return best, err
					}
				}
			}
		}
		if best.score >= initial.score-1e-9 {
			break
		}
	}
	return best, ctx.Err()
}

// Maintain a small sorted neighborhood without sorting the whole layer table.
func nearestStackColors(vectors []Vec, id int, selected []int, limit int) []int {
	ids, scores := []int{}, []float64{}
	for i, v := range vectors {
		if contains(selected, i) {
			continue
		}
		d, pos := distance(v, vectors[id]), len(ids)
		for pos > 0 && d < scores[pos-1] {
			pos--
		}
		if pos >= limit {
			continue
		}
		ids, scores = append(ids, 0), append(scores, 0)
		copy(ids[pos+1:], ids[pos:len(ids)-1])
		copy(scores[pos+1:], scores[pos:len(scores)-1])
		ids[pos], scores[pos] = i, d
		if len(ids) > limit {
			ids, scores = ids[:limit], scores[:limit]
		}
	}
	return ids
}
