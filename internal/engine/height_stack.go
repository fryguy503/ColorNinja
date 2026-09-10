package engine

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
)

// fitHeightStack fits physical filaments to an immutable image-to-layer map.
// Color Pop supplies neutral-band constraints; brightness and Color Aware use
// the same search without those constraints. Every branch owns optical history.
func fitHeightStack(ctx context.Context, src *image.NRGBA, analysisSize [2]int, layerMap []uint16, targets []popTarget, planned int, info *ColorPopInfo, o Options, lib *Library, progress Reporter) (*Result, error) {
	h := o.HueForge
	beamWidth := h.BeamWidth
	if h.SearchEffort == "refine" {
		beamWidth = min(512, beamWidth*2)
	}
	start := h.BaseLayers()
	w, height := src.Bounds().Dx(), src.Bounds().Dy()
	required, base, top, err := constraintIDs(*lib, o)
	if err != nil {
		return nil, err
	}
	// Include nearest anchors for used heights and every explicit constraint.
	pool := map[int]bool{}
	anchors := map[int]bool{}
	for _, i := range required {
		pool[i] = true
		anchors[i] = true
	}
	darkest, lightest := -1, -1
	darkL, lightL := math.Inf(1), math.Inf(-1)
	for i, f := range lib.Filaments {
		lab := ToLab(f.RGB)
		if math.Hypot(lab[1], lab[2]) > 12 {
			continue
		}
		if lab[0] < darkL {
			darkest, darkL = i, lab[0]
		}
		if lab[0] > lightL {
			lightest, lightL = i, lab[0]
		}
	}
	for _, i := range []int{darkest, lightest} {
		if i >= 0 {
			pool[i], anchors[i] = true, true
		}
	}
	if len(lib.Filaments) <= 64 {
		for i := range lib.Filaments {
			pool[i] = true
		}
	} else {
		vectors := make([]Vec, len(lib.Filaments))
		for i, f := range lib.Filaments {
			vectors[i] = o.colorVector(f.RGB)
		}
		for _, target := range targets {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if target.mass == 0 {
				continue
			}
			ids := make([]int, len(lib.Filaments))
			for i := range ids {
				ids[i] = i
			}
			sort.SliceStable(ids, func(i, j int) bool {
				return distance(vectors[ids[i]], target.mean) < distance(vectors[ids[j]], target.mean)
			})
			for _, i := range ids[:min(4, len(ids))] {
				pool[i] = true
			}
		}
	}
	ids := []int{}
	for i := range pool {
		ids = append(ids, i)
	}
	sort.Ints(ids)
	runLimit := h.MaxRuns
	if runLimit == 0 {
		runLimit = 8
	}
	if len(required) > runLimit {
		return nil, fmt.Errorf("required filaments exceed the height-planning run limit")
	}
	if len(ids) > 64 {
		sort.SliceStable(ids, func(i, j int) bool {
			a, b := ids[i], ids[j]
			if anchors[a] != anchors[b] {
				return anchors[a]
			}
			sa, sb := 0., 0.
			va, vb := o.colorVector(lib.Filaments[a].RGB), o.colorVector(lib.Filaments[b].RGB)
			for _, t := range targets {
				sa += t.mass / (1 + distance(va, t.mean))
				sb += t.mass / (1 + distance(vb, t.mean))
			}
			return sa > sb
		})
		ids = ids[:64]
		sort.Ints(ids)
	}
	neutral := map[int]bool{}
	hasNeutral, hasColor := false, false
	for _, id := range ids {
		lab := ToLab(lib.Filaments[id].RGB)
		neutral[id] = math.Hypot(lab[1], lab[2]) <= 12
		hasNeutral = hasNeutral || neutral[id]
		hasColor = hasColor || !neutral[id]
	}
	if info != nil && !hasNeutral {
		return nil, fmt.Errorf("height-planning needs an eligible black, white, or gray filament for its grayscale band")
	}
	grayLayer := func(layer int) bool { return info != nil && layer >= info.GrayLayers[0] && layer <= info.GrayLayers[1] }
	upperStart := 0
	if info != nil {
		upperStart = max(info.GrayLayers[0], info.ColorLayers[0])
	}
	beam := []*popNode{}
	for _, id := range ids {
		if grayLayer(start) && !neutral[id] {
			continue
		}
		if base >= 0 && id != base {
			continue
		}
		current := baseOptics(lib.Filaments[id], h)
		v := distance(o.colorVector(current.RGB(h)), targets[start].mean) * targets[start].mass
		beam = append(beam, &popNode{id: id, runs: 1, used: []int{id}, current: current, score: v, order: len(beam)})
	}
	if len(beam) == 0 {
		return nil, fmt.Errorf("no eligible height-planning base filament")
	}
	prune := func(nodes []*popNode) []*popNode {
		sort.SliceStable(nodes, func(i, j int) bool {
			a, b := nodes[i], nodes[j]
			if a.score != b.score {
				return a.score < b.score
			}
			if a.runs != b.runs {
				return a.runs < b.runs
			}
			return a.order < b.order
		})
		return nodes[:min(len(nodes), beamWidth)]
	}
	// Give every possible base a chance to blend before pruning.
	for layer := start + 1; layer <= planned; layer++ {
		if err := report(ctx, progress, fmt.Sprintf("Planning filament stack · layer %d of %d", layer, planned), .35+.45*float64(layer-start)/float64(planned-start)); err != nil {
			return nil, err
		}
		expanded := []*popNode{}
		for _, parent := range beam {
			for _, id := range ids {
				// Once the final-only highlight starts, it must remain the last
				// contiguous run. An earlier return cannot satisfy this constraint.
				if h.HighlightOnlyAtTop && top >= 0 && parent.id == top && id != top {
					continue
				}
				if grayLayer(layer) && !neutral[id] {
					continue
				}
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if layer == planned && top >= 0 && id != top {
					continue
				}
				runs := parent.runs
				if id != parent.id {
					runs++
				}
				if runs > runLimit {
					continue
				}
				// Leave room to rebase and color the second region instead of
				// spending every swap chasing small improvements in the first.
				if info != nil && layer < upperStart-o.ColorPop.GapLayers && runs > runLimit-min(2, runLimit-1) {
					continue
				}
				used := parent.used
				if !contains(used, id) {
					if len(used) >= o.Colors {
						continue
					}
					used = append(append([]int(nil), used...), id)
				}
				// Do not fill the unique-spool budget in the lower band before
				// reserving an appropriate spool for the upper band's color family.
				if info != nil && layer < upperStart && len(used) == o.Colors && (o.ColorPop.GrayOnTop || hasColor) {
					upperSpool := false
					for _, spool := range used {
						upperSpool = upperSpool || neutral[spool] == o.ColorPop.GrayOnTop
					}
					if !upperSpool {
						continue
					}
				}
				missing := 0
				for _, r := range required {
					if !contains(used, r) {
						missing++
					}
				}
				if missing > planned-layer || missing > runLimit-runs {
					continue
				}
				current := parent.current
				rgb := current.step(lib.Filaments[id], h, id != parent.id)
				score := parent.score + distance(o.colorVector(rgb), targets[layer].mean)*targets[layer].mass
				expanded = append(expanded, &popNode{parent: parent, id: id, runs: runs, used: used, current: current, score: score, order: len(expanded)})
			}
		}
		if len(expanded) == 0 {
			return nil, fmt.Errorf("no height-planning stack meets the filament/run constraints; increase the budget or clear a constraint")
		}
		beam = prune(expanded)
	}
	edges, err := heightEdges(ctx, src, layerMap, o)
	if err != nil {
		return nil, err
	}
	physical, surface, err := refineHeightPlan(ctx, prune(beam), ids, targets, *lib, o, runLimit, grayLayer, neutral, edges)
	if err != nil {
		return nil, err
	}
	order, runs := physical.indices, physical.runs
	unique := map[int]bool{}
	for _, id := range order {
		unique[id] = true
	}
	colors, positions := map[int]RGB{}, map[int]int{}
	for i, l := range physical.layers {
		colors[l] = physical.rgbs[i]
		positions[l] = physical.positions[i]
	}
	plan := &StackPlan{Model: h.OpticalModel, SearchMethod: "fixed-heights-bounded-beam", Options: h, Requested: o.Colors, UniqueFilaments: len(unique), Eligible: len(lib.Filaments), EligibleBases: len(ids), PlannedDepth: h.Height(planned), LibrarySHA256: lib.SHA256, OptimizationScore: physical.score, OptimizationMetric: "area-weighted Oklab at fixed image heights"}
	if len(edges) > 0 {
		plan.Surface = &surface
		plan.OptimizationMetric += " plus boundary penalty"
	}
	startRun := 1
	for i, id := range order {
		end := startRun + runs[i] - 1
		plan.Runs = append(plan.Runs, StackRun{i + 1, lib.Filaments[id], runs[i], startRun, end, h.Height(startRun - 1), h.Height(end)})
		startRun = end + 1
	}
	baseState := newOptics(lib.Filaments[order[0]], h)
	for l := 1; l <= planned; l++ {
		rgb := colors[l]
		position := positions[l]
		if l <= h.BaseLayers() {
			step := h.LayerHeight
			if l == 1 {
				step = h.FirstHeight()
			}
			rgb = baseState.layer(lib.Filaments[order[0]], step, h, l == 1)
			position = 1
		}
		plan.LayerColors = append(plan.LayerColors, LayerColor{l, h.Height(l), rgb, position, targets[l].mass})
	}
	r := &Result{Image: image.NewNRGBA(src.Bounds()), SourceSize: [2]int{w, height}, AnalysisSize: analysisSize, LayerMap: layerMap, Stack: plan}
	for l, t := range targets {
		if t.mass > 0 {
			p := entry(colors[l], t.mass, o.NeutralChroma)
			p.PixelFraction = t.mass
			p.StackLayer = l
			p.StackHeight = h.Height(l)
			p.TopPosition = positions[l]
			r.Palette = append(r.Palette, p)
		}
	}
	sum, square, weight := 0., 0., 0.
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			a := src.Pix[i+3]
			c := colors[int(layerMap[y*w+x])]
			r.Image.Pix[i], r.Image.Pix[i+1], r.Image.Pix[i+2], r.Image.Pix[i+3] = c[0], c[1], c[2], a
			if a == 0 {
				continue
			}
			d := distance(ToLab(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]}), ToLab(c))
			m := float64(a) / 255
			weight += m
			sum += math.Sqrt(d) * m
			square += d * m
			r.Quality.Max = max(r.Quality.Max, math.Sqrt(d))
		}
	}
	if weight > 0 {
		r.Quality.Mean = sum / weight
		r.Quality.RMS = math.Sqrt(square / weight)
	}
	plan.RMS = r.Quality.RMS
	return r, nil
}
