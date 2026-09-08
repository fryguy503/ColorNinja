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
	Model         string          `json:"model"`
	SearchMethod  string          `json:"searchMethod"`
	Options       HueForgeOptions `json:"options"`
	Requested     int             `json:"requestedFilaments"`
	Eligible      int             `json:"eligibleFilaments"`
	EligibleBases int             `json:"eligibleBases"`
	PlannedDepth  float64         `json:"plannedDepth"`
	Runs          []StackRun      `json:"runs"`
	LayerColors   []LayerColor    `json:"layerColors"`
	RMS           float64         `json:"weightedRmsDeltaE76"`
	LibrarySHA256 string          `json:"librarySHA256"`
}
type stackState struct {
	indices, runs     []int
	used              int
	current           Vec
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
	_, _, v, e := selectReachable(ctx, o.colorVectors(colors), target, weights, o.HueForge.MaxPerceivedColors, o.MinClusterFraction)
	return v, e
}
func planStack(ctx context.Context, palette []PaletteEntry, lib Library, o Options, progress Reporter) ([]PaletteEntry, *StackPlan, error) {
	target, weights := targets(palette, o)
	h := o.HueForge
	bases := []int{}
	for i, f := range lib.Filaments {
		t := math.Pow(h.TDTransmission, h.BaseDepth/(f.TD*h.TDScale))
		if t <= h.BaseTransmissionLimit+1e-12 {
			bases = append(bases, i)
		}
	}
	if len(bases) == 0 {
		return nil, nil, fmt.Errorf("no filament is opaque enough for this base; increase base depth or base transmission limit")
	}
	makeInitial := func() []stackState {
		out := []stackState{}
		for _, i := range bases {
			rgb := lib.Filaments[i].RGB
			lab := o.colorVector(rgb)
			d := make([]float64, len(target))
			for j, t := range target {
				d[j] = distance(t, lab)
			}
			out = append(out, stackState{[]int{i}, []int{h.BaseLayers()}, 0, LinearRGB(rgb), []RGB{rgb}, []int{h.BaseLayers()}, []int{1}, d, dot(weights, d)})
		}
		sort.SliceStable(out, func(i, j int) bool { return stateLess(out[i], out[j]) })
		return out[:min(h.BeamWidth, len(out))]
	}
	terminals := []stackState{}
	maximum := min(o.Colors, len(lib.Filaments), h.TransitionLayers()+1)
	for size := 1; size <= maximum; size++ {
		if e := report(ctx, progress, fmt.Sprintf("Planning stacks · %d of %d filaments", size, maximum), .35+.28*float64(size-1)/float64(maximum)); e != nil {
			return nil, nil, e
		}
		beam := makeInitial()
		if size == 1 {
			for i := range beam {
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
					if contains(s.indices, i) {
						continue
					}
					current := s.current
					rgbs := []RGB{}
					layers, positions := []int{}, []int{}
					distances := append([]float64{}, s.distances...)
					for run := 1; run <= maxRun; run++ {
						current = blend(current, f, h)
						rgb := FromLinear(current)
						rgbs = append(rgbs, rgb)
						layers = append(layers, h.BaseLayers()+s.used+run)
						positions = append(positions, position)
						lab := o.colorVector(rgb)
						for j, t := range target {
							distances[j] = math.Min(distances[j], distance(t, lab))
						}
						if final && run != maxRun {
							continue
						}
						state := stackState{append(append([]int{}, s.indices...), i), append(append([]int{}, s.runs...), run), s.used + run, current, append(append([]RGB{}, s.rgbs...), rgbs...), append(append([]int{}, s.layers...), layers...), append(append([]int{}, s.positions...), positions...), append([]float64{}, distances...), dot(weights, distances)}
						if final {
							v, e := stateScore(ctx, state, target, weights, o)
							if e != nil {
								return nil, nil, e
							}
							state.score = v
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
	colors, layers, positions := uniqueStack(best)
	ids, masses, rms, e := selectReachable(ctx, o.colorVectors(colors), target, weights, h.MaxPerceivedColors, o.MinClusterFraction)
	if e != nil {
		return nil, nil, e
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
		v.StackHeight = float64(layer) * h.LayerHeight
		v.TopPosition = positions[i]
		out = append(out, v)
		fractions[layer] = masses[p]
	}
	if o.PreserveDetails {
		actual := make([]RGB, len(out))
		for i, p := range out {
			actual[i] = p.RGB
		}
		rms = paletteRMS76(actual, palette, o)
	}
	plan := &StackPlan{Model: "independent-frontlit-scaled-td-linear-srgb-v1", SearchMethod: "deterministic-multisize-beam-with-capped-terminal-rerank", Options: h, Requested: o.Colors, Eligible: len(lib.Filaments), EligibleBases: len(bases), PlannedDepth: float64(planned) * h.LayerHeight, RMS: rms, LibrarySHA256: lib.SHA256}
	if o.PreserveDetails {
		plan.SearchMethod += "-oklab-complete-stack-refinement"
	}
	start := 1
	for pos, id := range best.indices {
		if start > planned {
			break
		}
		count := min(best.runs[pos], planned-start+1)
		end := start + count - 1
		plan.Runs = append(plan.Runs, StackRun{pos + 1, lib.Filaments[id], count, start, end, float64(start-1) * h.LayerHeight, float64(end) * h.LayerHeight})
		start = end + 1
	}
	for layer := 1; layer <= h.BaseLayers(); layer++ {
		plan.LayerColors = append(plan.LayerColors, LayerColor{layer, float64(layer) * h.LayerHeight, lib.Filaments[best.indices[0]].RGB, 1, fractions[layer]})
	}
	for i := 1; i < len(best.rgbs); i++ {
		layer := best.layers[i]
		if layer > planned {
			break
		}
		plan.LayerColors = append(plan.LayerColors, LayerColor{layer, float64(layer) * h.LayerHeight, best.rgbs[i], best.positions[i], fractions[layer]})
	}
	return out, plan, nil
}
