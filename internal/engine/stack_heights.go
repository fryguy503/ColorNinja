package engine

import "context"

// A single RGB still has one global matching height, preserving HueForge's
// Color Match contract. Later equal RGBs remain available as height choices.
func chooseStackHeights(ctx context.Context, s stackState, colors []RGB, layers, positions, ids []int, target []Vec, o Options, boundaries []stackBoundary) error {
	if len(boundaries) == 0 && !o.materialOptimization() && !o.layerOptimization() {
		return ctx.Err()
	}
	selected, heights := make([]RGB, len(ids)), make([]int, len(ids))
	for i, id := range ids {
		selected[i], heights[i] = colors[id], layers[id]
	}
	best := stackGeometryPenalty(ctx, s, selected, heights, target, o, boundaries)
	trials := 0
	for pass := 0; pass < 4; pass++ {
		changed := false
		for p, id := range ids {
			winner, position := heights[p], 0
			for i, rgb := range s.rgbs {
				if rgb != colors[id] || s.layers[i] == winner {
					continue
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				trials++
				if trials > 2048 {
					return nil
				}
				heights[p] = s.layers[i]
				score := stackGeometryPenalty(ctx, s, selected, heights, target, o, boundaries)
				if score < best-1e-9 {
					best, winner, position = score, s.layers[i], s.positions[i]
					changed = true
				}
			}
			heights[p], layers[id] = winner, winner
			if positions != nil && position > 0 {
				positions[id] = position
			}
		}
		if !changed {
			break
		}
	}
	return ctx.Err()
}
