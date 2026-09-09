package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
)

type StackRun struct {
	Position    int      `json:"position"`
	Filament    Filament `json:"filament"`
	Layers      int      `json:"layers"`
	StartLayer  int      `json:"startLayer"`
	EndLayer    int      `json:"endLayer"`
	StartHeight float64  `json:"startHeight"`
	EndHeight   float64  `json:"endHeight"`
}
type LayerColor struct {
	Layer       int     `json:"layer"`
	Height      float64 `json:"height"`
	RGB         RGB     `json:"rgb"`
	TopPosition int     `json:"topPosition"`
	Fraction    float64 `json:"analysisFraction"`
}
type StackPlan struct {
	Model           string          `json:"model"`
	SearchMethod    string          `json:"searchMethod"`
	Options         HueForgeOptions `json:"options"`
	Requested       int             `json:"requestedFilaments"`
	UniqueFilaments int             `json:"uniqueFilaments"`
	Eligible        int             `json:"eligibleFilaments"`
	EligibleBases   int             `json:"eligibleBases"`
	PlannedDepth    float64         `json:"plannedDepth"`
	Runs            []StackRun      `json:"runs"`
	LayerColors     []LayerColor    `json:"layerColors"`
	RMS             float64         `json:"weightedRmsDeltaE76"`
	LibrarySHA256   string          `json:"librarySHA256"`
	DepthSelection  *DepthSelection `json:"depthSelection,omitempty"`
}
type stackState struct {
	indices, runs     []int
	used              int
	current           opticalState
	rgbs              []RGB
	layers, positions []int
	distances         []float64
	score             float64
}

func stateLess(a, b stackState) bool {
	if a.score != b.score {
		return a.score < b.score
	}
	if lexLess(a.indices, b.indices) {
		return true
	}
	if lexLess(b.indices, a.indices) {
		return false
	}
	return lexLess(a.runs, b.runs)
}
func uniqueStack(s stackState) ([]RGB, []int, []int) {
	colors := []RGB{}
	layers, positions := []int{}, []int{}
	seen := map[RGB]bool{}
	for i, c := range s.rgbs {
		if !seen[c] {
			seen[c] = true
			colors = append(colors, c)
			layers = append(layers, s.layers[i])
			positions = append(positions, s.positions[i])
		}
	}
	return colors, layers, positions
}
func stateScore(ctx context.Context, s stackState, target []Vec, weights []float64, o Options) (float64, error) {
	colors, _, _ := uniqueStack(s)
	_, _, v, e := selectReachable(ctx, o.colorVectors(colors), target, weights, o.HueForge.MaxPerceivedColors, o.selectionFraction())
	return v, e
}
func planStack(ctx context.Context, palette []PaletteEntry, lib Library, o Options, progress Reporter) ([]PaletteEntry, *StackPlan, error) {
	target, weights := targets(palette, o)
	h := o.HueForge
	bases := []int{}
	for i, f := range lib.Filaments {
		t := math.Pow(h.TDTransmission, h.BaseDepth/(f.TD*h.TDScale))
		if h.frontlit() || t <= h.BaseTransmissionLimit+1e-12 {
			bases = append(bases, i)
		}
	}
	if len(bases) == 0 {
		return nil, nil, fmt.Errorf("no filament is opaque enough for this base; increase base depth or base transmission limit")
	}
	makeInitial := func() []stackState {
		out := []stackState{}
		for _, i := range bases {
			optics := baseOptics(lib.Filaments[i], h)
			rgb := optics.RGB(h)
			lab := o.colorVector(rgb)
			d := make([]float64, len(target))
			for j, t := range target {
				d[j] = distance(t, lab)
			}
			out = append(out, stackState{indices: []int{i}, runs: []int{h.BaseLayers()}, current: optics, rgbs: []RGB{rgb}, layers: []int{h.BaseLayers()}, positions: []int{1}, distances: d, score: dot(weights, d)})
		}
		sort.SliceStable(out, func(i, j int) bool { return stateLess(out[i], out[j]) })
		// Every base gets to demonstrate a useful two-filament blend before
		// pruning. A poor solid match can be the best blending substrate.
		if h.frontlit() {
			return out
		}
		return out[:min(h.BeamWidth, len(out))]
	}
	terminals := []stackState{}
	depths := depthCandidates{}
	maximum := min(o.Colors, len(lib.Filaments), h.TransitionLayers()+1)
	if h.MaxRuns > 0 && min(o.Colors, len(lib.Filaments)) > 1 {
		maximum = min(h.MaxRuns, h.TransitionLayers()+1)
	}
	for size := 1; size <= maximum; size++ {
		if e := report(ctx, progress, fmt.Sprintf("Planning stacks · %d of %d runs", size, maximum), .35+.28*float64(size-1)/float64(maximum)); e != nil {
			return nil, nil, e
		}
		beam := makeInitial()
		if size == 1 {
			for i := range beam {
				if h.AutoDepth {
					if err := depths.consider(ctx, beam[i], target, weights, o); err != nil {
						return nil, nil, err
					}
				}
				if h.frontlit() {
					for run := 1; run <= h.TransitionLayers(); run++ {
						rgb := beam[i].current.step(lib.Filaments[beam[i].indices[0]], h, false)
						beam[i].rgbs = append(beam[i].rgbs, rgb)
						beam[i].layers = append(beam[i].layers, h.BaseLayers()+run)
						beam[i].positions = append(beam[i].positions, 1)
						if h.AutoDepth {
							beam[i].runs[0] = h.BaseLayers() + run
							beam[i].used = run
							lab := o.colorVector(rgb)
							for j, t := range target {
								beam[i].distances[j] = math.Min(beam[i].distances[j], distance(t, lab))
							}
							if err := depths.consider(ctx, beam[i], target, weights, o); err != nil {
								return nil, nil, err
							}
						}
					}
					beam[i].runs[0] = h.MaxLayers()
					beam[i].used = h.TransitionLayers()
				}
				v, e := stateScore(ctx, beam[i], target, weights, o)
				if e != nil {
					return nil, nil, e
				}
				beam[i].score = v
			}
			sort.SliceStable(beam, func(i, j int) bool { return stateLess(beam[i], beam[j]) })
			terminals = append(terminals, beam[0])
			continue
		}
		for position := 2; position <= size; position++ {
			final := position == size
			remaining := size - position
			expanded := []stackState{}
			for _, s := range beam {
				maxRun := h.TransitionLayers() - s.used - remaining
				if maxRun < 1 {
					continue
				}
				for i, f := range lib.Filaments {
					if e := ctx.Err(); e != nil {
						return nil, nil, e
					}
					if !validStackOrder(append(append([]int{}, s.indices...), i), o) {
						continue
					}
					current := s.current
					rgbs := []RGB{}
					layers, positions := []int{}, []int{}
					distances := append([]float64{}, s.distances...)
					for run := 1; run <= maxRun; run++ {
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
							if err := depths.consider(ctx, state, target, weights, o); err != nil {
								return nil, nil, err
							}
							continue
						}
						if final {
							v, e := stateScore(ctx, state, target, weights, o)
							if e != nil {
								return nil, nil, e
							}
							state.score = v
							if h.AutoDepth {
								depths.record(state)
							}
						}
						expanded = append(expanded, state)
						// Keep a bounded pool without changing the deterministic top-k ordering.
						if len(expanded) > max(1024, h.BeamWidth*4) {
							sort.SliceStable(expanded, func(a, b int) bool { return stateLess(expanded[a], expanded[b]) })
							expanded = expanded[:h.BeamWidth]
						}
					}
				}
			}
			if len(expanded) == 0 {
				return nil, nil, fmt.Errorf("could not allocate layers to the selected filaments")
			}
			sort.SliceStable(expanded, func(i, j int) bool { return stateLess(expanded[i], expanded[j]) })
			beam = expanded[:min(len(expanded), h.BeamWidth)]
		}
		terminals = append(terminals, beam[0])
	}
	sort.SliceStable(terminals, func(i, j int) bool {
		a, b := terminals[i], terminals[j]
		if a.score != b.score {
			return a.score < b.score
		}
		if len(a.indices) != len(b.indices) {
			return len(a.indices) < len(b.indices)
		}
		return stateLess(a, b)
	})
	best := terminals[0]
	if o.PreserveDetails {
		var err error
		best, err = refineStack(ctx, best, lib, bases, target, weights, o, progress)
		if err != nil {
			return nil, nil, err
		}
	}
	var depthSelection *DepthSelection
	if h.AutoDepth {
		depths.record(best)
		best, _ = depths.choose()
		if o.PreserveDetails {
			var err error
			best, err = refineStack(ctx, best, lib, bases, target, weights, o, progress)
			if err != nil {
				return nil, nil, err
			}
			depths.record(best)
		}
		var bestScore float64
		best, bestScore = depths.choose()
		metric := "priority-weighted Oklab RMS"
		if o.LegacyColorPipeline {
			metric = "priority-weighted Lab RMS"
		}
		depthSelection = &DepthSelection{h.MaxDepth, h.Height(h.MaxLayers()), len(depths), bestScore, best.score, autoDepthTolerance * 100, metric}
	}
	colors, layers, positions := uniqueStack(best)
	ids, masses, rms, e := selectReachable(ctx, o.colorVectors(colors), target, weights, h.MaxPerceivedColors, o.selectionFraction())
	if e != nil {
		return nil, nil, e
	}
	if o.prioritizeColors() {
		selected := make([]Vec, len(ids))
		for i, id := range ids {
			selected[i] = o.colorVector(colors[id])
		}
		masses = sourceFractions(selected, palette, o)
	}
	order := make([]int, len(ids))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return layers[ids[order[i]]] < layers[ids[order[j]]] })
	planned := h.BaseLayers()
	fractions := map[int]float64{}
	out := []PaletteEntry{}
	for _, p := range order {
		i := ids[p]
		layer := layers[i]
		planned = max(planned, layer)
		v := entry(colors[i], masses[p], o.NeutralChroma)
		v.StackLayer = layer
		v.StackHeight = h.Height(layer)
		v.TopPosition = positions[i]
		out = append(out, v)
		fractions[layer] = masses[p]
	}
	if o.PreserveDetails || o.prioritizeColors() {
		actual := make([]RGB, len(out))
		for i, p := range out {
			actual[i] = p.RGB
		}
		rms = paletteRMS76(actual, palette, o)
	}
	plan := &StackPlan{Model: "independent-frontlit-scaled-td-linear-srgb-v1", SearchMethod: "deterministic-multisize-beam-with-capped-terminal-rerank", Options: h, Requested: o.Colors, Eligible: len(lib.Filaments), EligibleBases: len(bases), PlannedDepth: h.Height(planned), RMS: rms, LibrarySHA256: lib.SHA256}
	plan.DepthSelection = depthSelection
	if h.AutoDepth {
		plan.SearchMethod += "-automatic-depth-frontier"
	}
	if h.frontlit() {
		plan.Model = FrontlitModel
		plan.SearchMethod += "-all-base-pair-expansion"
	}
	if o.PreserveDetails {
		plan.SearchMethod += "-oklab-complete-stack-refinement"
	}
	start := 1
	uniqueIDs := map[int]bool{}
	for pos, id := range best.indices {
		if start > planned {
			break
		}
		count := min(best.runs[pos], planned-start+1)
		end := start + count - 1
		plan.Runs = append(plan.Runs, StackRun{pos + 1, lib.Filaments[id], count, start, end, h.Height(start - 1), h.Height(end)})
		uniqueIDs[id] = true
		start = end + 1
	}
	plan.UniqueFilaments = len(uniqueIDs)
	if h.MaxRuns > 0 {
		plan.SearchMethod += "-reusable-filaments"
	}
	base := newFrontlit(h.light())
	for layer := 1; layer <= h.BaseLayers(); layer++ {
		rgb := lib.Filaments[best.indices[0]].RGB
		if h.frontlit() {
			height := h.LayerHeight
			if layer == 1 {
				height = h.FirstHeight()
			}
			base.step(lib.Filaments[best.indices[0]], height, layer == 1)
			rgb = base.RGB()
		}
		plan.LayerColors = append(plan.LayerColors, LayerColor{layer, h.Height(layer), rgb, 1, fractions[layer]})
	}
	for i := 1; i < len(best.rgbs); i++ {
		layer := best.layers[i]
		if layer > planned {
			break
		}
		plan.LayerColors = append(plan.LayerColors, LayerColor{layer, h.Height(layer), best.rgbs[i], best.positions[i], fractions[layer]})
	}
	return out, plan, nil
}
