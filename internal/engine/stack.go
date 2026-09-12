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
	OptimizationScore  float64                `json:"optimizationScore"`
	OptimizationMetric string                 `json:"optimizationMetric"`
	Model              string                 `json:"model"`
	SearchMethod       string                 `json:"searchMethod"`
	Options            HueForgeOptions        `json:"options"`
	Requested          int                    `json:"requestedFilaments"`
	UniqueFilaments    int                    `json:"uniqueFilaments"`
	Eligible           int                    `json:"eligibleFilaments"`
	EligibleBases      int                    `json:"eligibleBases"`
	PlannedDepth       float64                `json:"plannedDepth"`
	Runs               []StackRun             `json:"runs"`
	LayerColors        []LayerColor           `json:"layerColors"`
	RMS                float64                `json:"weightedRmsDeltaE76"`
	LibrarySHA256      string                 `json:"librarySHA256"`
	DepthSelection     *DepthSelection        `json:"depthSelection,omitempty"`
	Surface            *StackSurface          `json:"surface,omitempty"`
	ColorOrder         *ColorOrderReport      `json:"colorOrder,omitempty"`
	LayerPreference    *LayerPreferenceReport `json:"layerPreference,omitempty"`
}
type stackState struct {
	indices, runs     []int
	used              int
	current           opticalState
	rgbs              []RGB
	layers, positions []int
	distances         []float64
	score             float64
	// Owned by one palette selection, never retained in beam/refinement states.
	surfaceCache *stackSurfaceCache
}
type stackColorLimitKey struct{}

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
func stateScore(ctx context.Context, s stackState, target []Vec, weights []float64, o Options, boundaries ...stackBoundary) (float64, error) {
	p, err := selectStackPalette(ctx, s, target, weights, o, boundaries)
	return p.score, err
}
func planStack(ctx context.Context, palette []PaletteEntry, lib Library, o Options, progress Reporter, boundaries ...stackBoundary) ([]PaletteEntry, *StackPlan, error) {
	required, requiredBase, requiredTop, constraintErr := constraintIDs(lib, o)
	if constraintErr != nil {
		return nil, nil, constraintErr
	}
	ctx = context.WithValue(ctx, stackConstraintsKey{}, stackConstraints{required, requiredTop})
	target, weights := targets(palette, o)
	ctx = context.WithValue(ctx, stackAreaKey{}, paletteAreas(palette))
	families := make([]string, len(palette))
	for i, p := range palette {
		families[i] = layerColorFamily(p.RGB)
	}
	ctx = context.WithValue(ctx, layerFamiliesKey{}, families)
	ctx = context.WithValue(ctx, colorOrderKey{}, orderSource(palette))
	pref := imageLayerPreference(ctx, palette, o)
	ctx = context.WithValue(ctx, layerPreferenceKey{}, pref)
	h := o.HueForge
	var colorBaseline *stackState
	if len(boundaries) > 0 || o.materialOptimization() || o.layerOptimization() {
		plain := o
		plain.HueForge.ReduceShowThrough = false
		plain.HueForge.OptimizeMaterial = false
		plain.HueForge.LayerPreference = ""
		plain.HueForge.ColorOrder = ""
		baselinePalette, plan, err := planStack(ctx, palette, lib, plain, progress)
		if err != nil {
			return nil, nil, err
		}
		ids, runs := []int{}, []int{}
		for _, run := range plan.Runs {
			for i, f := range lib.Filaments {
				if FilamentKey(f) == FilamentKey(run.Filament) {
					ids = append(ids, i)
					runs = append(runs, run.Layers)
					break
				}
			}
		}
		baseline := rebuildStack(ids, runs, lib, h)
		colorScore, err := stateScore(ctx, baseline, target, weights, plain)
		if err != nil {
			return nil, nil, err
		}
		ctx = context.WithValue(ctx, stackColorLimitKey{}, colorScore*(1+h.SurfaceColorTolerance/100)+1e-8)
		if o.layerOptimization() {
			colors := make([]RGB, len(baselinePalette))
			for i, c := range baselinePalette {
				colors[i] = c.RGB
			}
			limits := layerFamilyErrors(colors, target, paletteAreas(palette), families, o)
			for f, v := range limits {
				limits[f] = v*(1+h.SurfaceColorTolerance/100) + 1
			}
			ctx = context.WithValue(ctx, layerColorLimitsKey{}, limits)
		}
		baseline.score, err = stateScore(ctx, baseline, target, weights, o, boundaries...)
		if err != nil {
			return nil, nil, err
		}
		colorBaseline = &baseline
	}
	bases := []int{}
	for i, f := range lib.Filaments {
		if requiredBase >= 0 && i != requiredBase {
			continue
		}
		t := math.Pow(h.TDTransmission, h.BaseDepth/(f.TD*h.TDScale))
		if h.compatibleOptics() || t <= h.BaseTransmissionLimit+1e-12 {
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
		if h.compatibleOptics() {
			return out
		}
		return out[:min(h.BeamWidth, len(out))]
	}
	terminals := []stackState{}
	if colorBaseline != nil {
		terminals = append(terminals, *colorBaseline)
	}
	if o.layerOptimization() {
		seeds, err := preferenceSeeds(ctx, palette, lib, o, progress)
		if err != nil {
			return nil, nil, err
		}
		for _, seed := range seeds {
			ids, runs := []int{}, []int{}
			for _, run := range seed.Runs {
				for i, f := range lib.Filaments {
					if FilamentKey(f) == FilamentKey(run.Filament) {
						ids, runs = append(ids, i), append(runs, run.Layers)
						break
					}
				}
			}
			candidate := rebuildStack(ids, runs, lib, h)
			candidate.score, err = stateScore(ctx, candidate, target, weights, o, boundaries...)
			if err != nil {
				return nil, nil, err
			}
			if !finite(candidate.score) {
				// Refine a near-miss order using a steep color barrier. It may
				// enter the feasible set after adjusting its blend thicknesses.
				// Never admit the relaxed score into the final candidate set.
				searchCtx := context.WithValue(ctx, layerSearchBarrierKey{}, true)
				candidate.score, err = stateScore(searchCtx, candidate, target, weights, o, boundaries...)
				if err != nil {
					return nil, nil, err
				}
				candidate, err = refineStack(searchCtx, candidate, lib, bases, target, weights, o, progress, boundaries...)
				if err != nil {
					return nil, nil, err
				}
				candidate.score, err = stateScore(ctx, candidate, target, weights, o, boundaries...)
				if err != nil {
					return nil, nil, err
				}
			}
			if finite(candidate.score) {
				terminals = append(terminals, candidate)
			}
		}
	}
	depths := depthCandidates{}
	maximum := min(o.Colors, len(lib.Filaments), h.TransitionLayers()+1)
	if h.MaxRuns > 0 && min(o.Colors, len(lib.Filaments)) > 1 {
		maximum = min(h.MaxRuns, h.TransitionLayers()+1)
	}
	for size := 1; size <= maximum; size++ {
		if size < len(required) {
			continue
		}
		if e := report(ctx, progress, fmt.Sprintf("Planning stacks · %d of %d runs", size, maximum), .35+.28*float64(size-1)/float64(maximum)); e != nil {
			return nil, nil, e
		}
		beam := makeInitial()
		if size == 1 {
			for i := range beam {
				if !completeConstraints(beam[i].indices, lib, o) {
					beam[i].score = math.Inf(1)
					continue
				}
				if h.AutoDepth {
					if err := depths.consider(ctx, beam[i], target, weights, o, boundaries...); err != nil {
						return nil, nil, err
					}
				}
				if h.compatibleOptics() {
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
							if err := depths.consider(ctx, beam[i], target, weights, o, boundaries...); err != nil {
								return nil, nil, err
							}
						}
					}
					beam[i].runs[0] = h.MaxLayers()
					beam[i].used = h.TransitionLayers()
				}
				v, e := stateScore(ctx, beam[i], target, weights, o, boundaries...)
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
			expanded, err := expandStackBeam(ctx, beam, lib, o, position, size, target, weights, depths, required, requiredTop, boundaries)
			if err != nil {
				return nil, nil, err
			}
			if len(expanded) == 0 {
				beam = nil
				break
			}
			beam = expanded
		}
		if len(beam) > 0 {
			terminals = append(terminals, diverseStacks(beam, 4)...)
		}
	}
	if len(terminals) == 0 {
		return nil, nil, fmt.Errorf("no stack fits these filament, run and layer constraints")
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
	if !finite(best.score) {
		return nil, nil, fmt.Errorf("no stack fits the required base and highlight")
	}
	if h.compatibleOptics() || o.PreserveDetails || len(boundaries) > 0 {
		count := 4
		if h.SearchEffort == "refine" {
			count = 8
		}
		candidates, err := refineStackCandidates(ctx, diverseStacks(terminals, count), lib, bases, target, weights, o, progress, boundaries)
		if err != nil {
			return nil, nil, err
		}
		for _, refined := range candidates {
			if depthStateLess(refined, best) {
				best = refined
			}
		}
	}
	var simplifyErr error
	best, simplifyErr = simplifyStack(ctx, best, lib, bases, target, weights, o, progress, boundaries)
	if simplifyErr != nil {
		return nil, nil, simplifyErr
	}
	var depthSelection *DepthSelection
	if h.AutoDepth {
		if err := depths.thin(ctx, best, lib, target, weights, o, boundaries...); err != nil {
			return nil, nil, err
		}
		best, _ = depths.choose(h.DepthTolerance)
		if h.compatibleOptics() || o.PreserveDetails || len(boundaries) > 0 {
			var err error
			best, err = refineStack(ctx, best, lib, bases, target, weights, o, progress, boundaries...)
			if err != nil {
				return nil, nil, err
			}
			best, err = simplifyStack(ctx, best, lib, bases, target, weights, o, progress, boundaries)
			if err != nil {
				return nil, nil, err
			}
			if err = depths.thin(ctx, best, lib, target, weights, o, boundaries...); err != nil {
				return nil, nil, err
			}
		}
		var bestScore float64
		best, bestScore = depths.choose(h.DepthTolerance)
		metric := "priority-weighted Oklab RMS"
		if o.LegacyColorPipeline {
			metric = "priority-weighted Lab RMS"
		}
		if len(boundaries) > 0 {
			metric += " with boundary penalty"
		}
		if o.materialOptimization() {
			metric += " with material penalty"
		}
		if o.layerOptimization() {
			metric += " with source-color layer preference"
		}
		tolerance := h.DepthTolerance
		if tolerance == 0 {
			tolerance = autoDepthTolerance * 100
		}
		depthSelection = &DepthSelection{h.MaxDepth, h.Height(h.MaxLayers()), len(depths), bestScore, best.score, tolerance, metric}
	}
	selection, e := selectStackPalette(ctx, best, target, weights, o, boundaries)
	if e != nil {
		return nil, nil, e
	}
	if !finite(selection.score) {
		return nil, nil, fmt.Errorf("no selected output reaches the required spools; adjust the palette, constraints or depth")
	}
	colors, layers, positions := selection.colors, selection.layers, selection.positions
	ids, masses, rms := selection.ids, selection.masses, selection.colorRMS
	if o.prioritizeColors() || o.ProtectedColors != "" {
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
	{
		actual := make([]RGB, len(out))
		for i, p := range out {
			actual[i] = p.RGB
		}
		rms = paletteRMS76(actual, palette, o)
	}
	plan := &StackPlan{Model: "independent-frontlit-scaled-td-linear-srgb-v1", SearchMethod: "deterministic-multisize-beam-with-capped-terminal-rerank", Options: h, Requested: o.Colors, Eligible: len(lib.Filaments), EligibleBases: len(bases), PlannedDepth: h.Height(planned), RMS: rms, LibrarySHA256: lib.SHA256}
	plan.ColorOrder = colorOrderReport(palette, out, o)
	plan.DepthSelection = depthSelection
	plan.OptimizationScore = best.score
	plan.OptimizationMetric = optimizationMetric(o, len(boundaries) > 0)
	if o.materialOptimization() {
		plan.SearchMethod += "-area-weighted-material-penalty"
	}
	if len(boundaries) > 0 || o.materialOptimization() || o.layerOptimization() {
		plan.SearchMethod += "-joint-color-height-selection"
	}
	if o.customColorOrder() {
		plan.SearchMethod += "-weighted-color-order"
	}
	if o.layerOptimization() && !o.customColorOrder() {
		plan.SearchMethod += "-source-color-layer-preference"
		plan.LayerPreference = &LayerPreferenceReport{Message: "No clear compact accent found. Optimizing related shades and transitions without assuming a foreground color."}
		if pref != nil {
			selected, heights := make([]RGB, len(out)), make([]int, len(out))
			for i, p := range out {
				selected[i], heights[i] = p.RGB, p.StackLayer
			}
			_, plan.LayerPreference = preferencePenalty(pref, preferenceHeights(selected, heights, target, o), o)
		}
	}
	if len(boundaries) > 0 {
		selected, heights := make([]RGB, len(out)), make([]int, len(out))
		for i, p := range out {
			selected[i], heights[i] = p.RGB, p.StackLayer
		}
		surface := stackSurface(best, selected, heights, target, o, boundaries)
		plan.Surface = &surface
		plan.SearchMethod += "-image-boundary-penalty"
	}
	if h.AutoDepth {
		plan.SearchMethod += "-automatic-depth-frontier"
	}
	if h.compatibleOptics() {
		plan.Model = h.OpticalModel
		plan.SearchMethod += "-all-base-pair-expansion"
	}
	if h.compatibleOptics() || o.PreserveDetails || len(boundaries) > 0 {
		plan.SearchMethod += "-diverse-complete-stack-refinement"
		plan.SearchMethod += "-block-transfers-allocation-alternatives"
		if h.SearchEffort == "refine" {
			plan.SearchMethod += "-structural-moves"
		}
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
	base := newOptics(lib.Filaments[best.indices[0]], h)
	for layer := 1; layer <= h.BaseLayers(); layer++ {
		rgb := lib.Filaments[best.indices[0]].RGB
		if h.compatibleOptics() {
			height := h.LayerHeight
			if layer == 1 {
				height = h.FirstHeight()
			}
			rgb = base.layer(lib.Filaments[best.indices[0]], height, h, layer == 1)
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
