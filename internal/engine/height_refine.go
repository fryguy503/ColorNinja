package engine

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
)

func heightNodeRuns(node *popNode, base int) ([]int, []int) {
	sequence := []int{}
	for n := node; n != nil; n = n.parent {
		sequence = append(sequence, n.id)
	}
	order, runs := []int{}, []int{}
	for i := len(sequence) - 1; i >= 0; i-- {
		n := 1
		if i == len(sequence)-1 {
			n = base
		}
		if len(order) > 0 && order[len(order)-1] == sequence[i] {
			runs[len(runs)-1] += n
		} else {
			order = append(order, sequence[i])
			runs = append(runs, n)
		}
	}
	return order, runs
}

// These edges use the actual fixed layer map, so equal-colored regions at
// different heights remain distinct. Counts are bounded by the layer budget.
func heightEdges(ctx context.Context, src *image.NRGBA, layers []uint16, o Options) ([]stackBoundary, error) {
	if !o.HueForge.ReduceShowThrough {
		return nil, nil
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	counts := map[uint32]float64{}
	total := 0.
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*w + x
			a := layers[i]
			if a == 0 {
				continue
			}
			for direction, j := range []int{i - 1, i - w} {
				if j < 0 || (direction == 0 && x == 0) {
					continue
				}
				b := layers[j]
				if b == 0 || b == a {
					continue
				}
				key := uint32(min(a, b))<<16 | uint32(max(a, b))
				m := float64(min(src.Pix[y*src.Stride+x*4+3], src.Pix[(j/w)*src.Stride+(j%w)*4+3])) / 255
				counts[key] += m
				total += m
			}
		}
	}
	keys := make([]uint32, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	width, detail := o.HueForge.ExportWidthMM, o.HueForge.MeshDetailMM
	if width == 0 {
		width = 200
	}
	if detail == 0 {
		detail = .2
	}
	scale := max(.0625, min(16, math.Pow(.2/max(detail, width/float64(w)), 2)))
	out := make([]stackBoundary, 0, len(keys))
	for _, k := range keys {
		out = append(out, stackBoundary{int(k >> 16), int(k & 65535), counts[k] / total, scale})
	}
	return out, nil
}

func fixedHeightScore(s stackState, targets []popTarget, o Options, edges []stackBoundary) (float64, StackSurface) {
	colors := make([]RGB, len(targets))
	present := make([]bool, len(targets))
	score := 0.
	for i, l := range s.layers {
		if l < len(targets) {
			colors[l] = s.rgbs[i]
			present[l] = true
			score += targets[l].mass * distance(o.colorVector(s.rgbs[i]), targets[l].mean)
		}
	}
	stats := StackSurface{BoundaryPairs: len(edges)}
	for _, edge := range edges {
		if !present[edge.a] || !present[edge.b] {
			continue
		}
		jump := o.HueForge.Height(edge.b) - o.HueForge.Height(edge.a)
		a, b := toOKLab(colors[edge.a]), toOKLab(colors[edge.b])
		detour := 0.
		for l := edge.a + 1; l < edge.b; l++ {
			detour += colorSegmentDistance(toOKLab(colors[l]), a, b)
		}
		stats.MeanHeightJumpMM += edge.weight * jump
		if edge.b-edge.a > 1 {
			stats.RMSColorDetour += edge.weight * detour / float64(edge.b-edge.a-1)
		}
		stats.Penalty += edge.weight * (16*jump*jump*edge.geometryScale + .1*detour*o.HueForge.LayerHeight)
	}
	stats.RMSColorDetour = math.Sqrt(stats.RMSColorDetour)
	return score, stats
}

// Refinement keeps the image's assigned heights fixed. It substitutes complete
// runs and moves run boundaries, rebuilding all subsequent translucent blends.
// It cannot move a pixel into another brightness/color band to improve a score.
func refineHeightPlan(ctx context.Context, finalists []*popNode, ids []int, targets []popTarget, lib Library, o Options, runLimit int, grayLayer func(int) bool, neutral map[int]bool, edges []stackBoundary) (stackState, StackSurface, error) {
	best := stackState{}
	bestScore, bestColor := math.Inf(1), math.Inf(1)
	surface := StackSurface{}
	states := []stackState{}
	for _, n := range finalists {
		order, runs := heightNodeRuns(n, o.HueForge.BaseLayers())
		if !completeConstraints(order, lib, o) {
			continue
		}
		s := rebuildStack(order, runs, lib, o.HueForge)
		color, _ := fixedHeightScore(s, targets, o, nil)
		bestColor = min(bestColor, color)
		states = append(states, s)
	}
	if len(states) == 0 {
		return best, surface, fmt.Errorf("no height plan satisfies every required filament")
	}
	limit := bestColor*math.Pow(1+o.HueForge.SurfaceColorTolerance/100, 2) + 1e-9
	consider := func(s stackState) bool {
		color, stats := fixedHeightScore(s, targets, o, edges)
		if len(edges) > 0 && color > limit {
			return false
		}
		score := color + stats.Penalty
		if score < bestScore-1e-12 {
			s.score = math.Sqrt(score)
			best, bestScore, surface = s, score, stats
			return true
		}
		return false
	}
	for _, s := range states {
		consider(s)
	}
	passes := 0
	if len(edges) > 0 {
		passes = 2
	}
	if o.HueForge.SearchEffort == "refine" {
		passes = 6
	}
	trials := 0
	valid := func(order, runs []int) bool {
		if len(order) > runLimit || !completeConstraints(order, lib, o) || runs[0] < o.HueForge.BaseLayers() {
			return false
		}
		unique := map[int]bool{}
		layer := 1
		for i, id := range order {
			if runs[i] < 1 {
				return false
			}
			unique[id] = true
			for l := layer; l < layer+runs[i]; l++ {
				if grayLayer(l) && !neutral[id] {
					return false
				}
			}
			layer += runs[i]
		}
		return len(unique) <= o.Colors
	}
	for pass := 0; pass < passes; pass++ {
		initial := best
		improved := false
		try := func(order, runs []int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			trials++
			if trials > 4096 || !valid(order, runs) {
				return nil
			}
			s := rebuildStack(order, runs, lib, o.HueForge)
			improved = consider(s) || improved
			return nil
		}
		for pos := range initial.indices {
			for _, id := range ids {
				if id == initial.indices[pos] {
					continue
				}
				order := append([]int(nil), initial.indices...)
				order[pos] = id
				// Repeated adjacent identities would change optical new-run
				// semantics. Coalesce them before rebuilding and validating.
				co, cr := []int{}, []int{}
				for i, v := range order {
					if len(co) > 0 && co[len(co)-1] == v {
						cr[len(cr)-1] += initial.runs[i]
					} else {
						co = append(co, v)
						cr = append(cr, initial.runs[i])
					}
				}
				if err := try(co, cr); err != nil {
					return best, surface, err
				}
			}
			if pos+1 < len(initial.runs) {
				for _, delta := range []int{-1, 1} {
					runs := append([]int(nil), initial.runs...)
					runs[pos] += delta
					runs[pos+1] -= delta
					if err := try(initial.indices, runs); err != nil {
						return best, surface, err
					}
				}
			}
		}
		if !improved || trials >= 4096 {
			break
		}
	}
	return best, surface, ctx.Err()
}
