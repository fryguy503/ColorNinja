package engine

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
)

type popTarget struct {
	mean Vec
	mass float64
}
type popNode struct {
	parent   *popNode
	id, runs int
	used     []int
	current  opticalState
	score    float64
	order    int
}

// Color Pop assigns brightness to two disjoint layer bands first, then fits
// one physical stack to those fixed surface targets. The ordinary free-color
// stack search cannot preserve this separation, so its algorithm is untouched.
func planColorPop(ctx context.Context, src *image.NRGBA, mask []byte, info *ColorPopInfo, o Options, lib *Library, progress Reporter) (*Result, error) {
	if lib == nil || len(lib.Filaments) == 0 {
		return nil, fmt.Errorf("load a filament library with eligible filaments first")
	}
	if info.ColorFraction == 0 || info.GrayFraction == 0 {
		return nil, fmt.Errorf("Color Pop print planning needs both color and grayscale regions; adjust the selection or use Prepare image")
	}
	if o.Colors < 2 {
		return nil, fmt.Errorf("Color Pop print planning needs at least two filaments for color and grayscale")
	}
	if o.TrueBlack {
		v := trueBlackLibrary(*lib)
		lib = &v
	}
	h := o.HueForge
	required, base, top, err := constraintIDs(*lib, o)
	if err != nil {
		return nil, err
	}
	count := h.MaxLayers() - h.BaseLayers() + 1 - o.ColorPop.GapLayers
	if count < 4 {
		return nil, fmt.Errorf("increase Color Pop thickness: each region needs at least two layers plus the boundary gap")
	}
	if h.MaxLayers() > 998 {
		return nil, fmt.Errorf("Color Pop supports at most 998 printable layers")
	}
	nc := max(2, min(count-2, int(math.Round(float64(count)*o.ColorPop.ColorPercent/100))))
	ng := count - nc
	start := h.BaseLayers()
	if o.ColorPop.GrayOnTop {
		info.ColorLayers = [2]int{start, start + nc - 1}
		info.GrayLayers = [2]int{start + nc + o.ColorPop.GapLayers, h.MaxLayers()}
	} else {
		info.GrayLayers = [2]int{start, start + ng - 1}
		info.ColorLayers = [2]int{start + ng + o.ColorPop.GapLayers, h.MaxLayers()}
	}
	info.GapLayers = o.ColorPop.GapLayers
	if h.MaxPerceivedColors < 2 {
		return nil, fmt.Errorf("Color Pop print planning needs at least two surface colors")
	}
	// Smooth/reduce separately before brightness assignment, keeping the original
	// mask immutable. This uses the same smoothing controls as Prepare image.
	analysis := o
	analysis.Mode = "standard"
	analysis.Colors = min(256, h.AnalysisColors)
	analysis.TotalColors = true
	flat, err := reduceColorPop(ctx, src, mask, info, analysis, progress)
	if err != nil {
		return nil, err
	}
	w, height := src.Bounds().Dx(), src.Bounds().Dy()
	mins, maxs := [2]int{255, 255}, [2]int{}
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			if src.Pix[i+3] == 0 {
				continue
			}
			region := int(mask[y*w+x])
			v := int(popGray(RGB{flat.Image.Pix[i], flat.Image.Pix[i+1], flat.Image.Pix[i+2]}))
			mins[region] = min(mins[region], v)
			maxs[region] = max(maxs[region], v)
		}
	}
	targets := make([]popTarget, h.MaxLayers()+1)
	layerMap := make([]uint16, w*height)
	budget := min(h.MaxPerceivedColors, count)
	colorLevels := max(1, min(nc, budget-1, int(math.Round(float64(budget)*float64(nc)/float64(count)))))
	grayLevels := min(ng, budget-colorLevels)
	colorLevels = min(nc, budget-grayLevels)
	bands := [2][2]int{info.GrayLayers, info.ColorLayers}
	levels := [2]int{grayLevels, colorLevels}
	total := 0.
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			if src.Pix[i+3] == 0 {
				continue
			}
			region := int(mask[y*w+x])
			rgb := RGB{flat.Image.Pix[i], flat.Image.Pix[i+1], flat.Image.Pix[i+2]}
			v := int(popGray(rgb))
			f := .5
			if maxs[region] > mins[region] {
				f = float64(v-mins[region]) / float64(maxs[region]-mins[region])
			}
			if levels[region] > 1 {
				f = math.Round(f*float64(levels[region]-1)) / float64(levels[region]-1)
			} else {
				f = .5
			}
			layer := bands[region][0] + int(math.Round(f*float64(bands[region][1]-bands[region][0])))
			layerMap[y*w+x] = uint16(layer)
			m := float64(src.Pix[i+3]) / 255
			total += m
			lab := o.colorVector(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]})
			targets[layer].mass += m
			for c := range lab {
				targets[layer].mean[c] += lab[c] * m
			}
		}
	}
	planned := h.BaseLayers()
	for layer := range targets {
		if targets[layer].mass > 0 {
			for c := range targets[layer].mean {
				targets[layer].mean[c] /= targets[layer].mass
			}
			targets[layer].mass /= total
			planned = layer
		}
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
		return nil, fmt.Errorf("required filaments exceed the Color Pop run limit")
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
	if !hasNeutral {
		return nil, fmt.Errorf("Color Pop needs an eligible black, white, or gray filament for its grayscale band")
	}
	grayLayer := func(layer int) bool { return layer >= info.GrayLayers[0] && layer <= info.GrayLayers[1] }
	upperStart := max(info.GrayLayers[0], info.ColorLayers[0])
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
		return nil, fmt.Errorf("no eligible Color Pop base filament")
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
		return nodes[:min(len(nodes), h.BeamWidth)]
	}
	// Give every possible base a chance to blend before pruning.
	for layer := start + 1; layer <= planned; layer++ {
		if err := report(ctx, progress, fmt.Sprintf("Planning Color Pop · layer %d of %d", layer, planned), .35+.45*float64(layer-start)/float64(planned-start)); err != nil {
			return nil, err
		}
		expanded := []*popNode{}
		for _, parent := range beam {
			for _, id := range ids {
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
				if layer < upperStart-o.ColorPop.GapLayers && runs > runLimit-min(2, runLimit-1) {
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
				if layer < upperStart && len(used) == o.Colors && (o.ColorPop.GrayOnTop || hasColor) {
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
			return nil, fmt.Errorf("no Color Pop stack meets the filament/run constraints; increase the budget or clear a constraint")
		}
		beam = prune(expanded)
	}
	best := prune(beam)[0]
	sequence := []int{}
	for node := best; node != nil; node = node.parent {
		sequence = append(sequence, node.id)
	}
	for i, j := 0, len(sequence)-1; i < j; i, j = i+1, j-1 {
		sequence[i], sequence[j] = sequence[j], sequence[i]
	}
	order, runs := []int{sequence[0]}, []int{h.BaseLayers()}
	for _, id := range sequence[1:] {
		if id == order[len(order)-1] {
			runs[len(runs)-1]++
		} else {
			order = append(order, id)
			runs = append(runs, 1)
		}
	}
	if !completeConstraints(order, *lib, o) {
		return nil, fmt.Errorf("Color Pop stack could not include every required filament")
	}
	physical := rebuildStack(order, runs, *lib, h)
	colors, positions := map[int]RGB{}, map[int]int{}
	for i, l := range physical.layers {
		colors[l] = physical.rgbs[i]
		positions[l] = physical.positions[i]
	}
	plan := &StackPlan{Model: FrontlitModel, SearchMethod: "color-pop-fixed-bands-bounded-beam", Options: h, Requested: o.Colors, UniqueFilaments: len(best.used), Eligible: len(lib.Filaments), EligibleBases: len(ids), PlannedDepth: h.Height(planned), LibrarySHA256: lib.SHA256, OptimizationScore: math.Sqrt(best.score), OptimizationMetric: "area-weighted Oklab at fixed Color Pop heights"}
	startRun := 1
	for i, id := range order {
		end := startRun + runs[i] - 1
		plan.Runs = append(plan.Runs, StackRun{i + 1, lib.Filaments[id], runs[i], startRun, end, h.Height(startRun - 1), h.Height(end)})
		startRun = end + 1
	}
	baseState := newFrontlit(h.light())
	for l := 1; l <= planned; l++ {
		rgb := colors[l]
		position := positions[l]
		if l <= h.BaseLayers() {
			step := h.LayerHeight
			if l == 1 {
				step = h.FirstHeight()
			}
			baseState.step(lib.Filaments[order[0]], step, l == 1)
			rgb = baseState.RGB()
			position = 1
		}
		plan.LayerColors = append(plan.LayerColors, LayerColor{l, h.Height(l), rgb, position, targets[l].mass})
	}
	r := &Result{Image: image.NewNRGBA(src.Bounds()), SourceSize: [2]int{w, height}, AnalysisSize: flat.AnalysisSize, LayerMap: layerMap, Stack: plan}
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
