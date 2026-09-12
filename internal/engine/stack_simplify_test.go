package engine

import (
	"context"
	"math"
	"reflect"
	"testing"
)

// Reduced from the Babymetal saved project: a white/red/white detour had no
// surface pixels, but directly merging it slightly worsened the objective.
// Reallocating the simpler order crosses that local minimum without relaxing
// color, depth, spool, or run constraints.
func TestSimplifyBuriedRunsReallocatesAcrossPlateau(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.Colors, o.ColorPriority = "stack", 4, "distinctive"
	o.HueForge.BaseDepth, o.HueForge.MaxDepth = .88, 2.2
	o.HueForge.MaxRuns, o.HueForge.SearchEffort = 10, "preview"
	o.HueForge.ReduceShowThrough = true
	o.HueForge.LayerPreference = "auto"
	o.HueForge.RequiredFilaments = "white"
	lib := Library{Filaments: []Filament{
		{UUID: "white", RGB: RGB{255, 255, 255}, TD: 2.9},
		{UUID: "red", RGB: RGB{199, 0, 0}, TD: 2},
		{UUID: "burgundy", RGB: RGB{128, 0, 32}, TD: 4.3},
		{UUID: "black", RGB: RGB{}, TD: .3},
	}}
	palette := []PaletteEntry{
		entry(RGB{8, 7, 7}, .631336440684431, 8),
		entry(RGB{229, 25, 23}, .3154107577410422, 8),
		entry(RGB{240, 233, 220}, .017654305918969843, 8),
		entry(RGB{62, 11, 9}, .015039630169276893, 8),
		entry(RGB{142, 16, 14}, .013498668822995415, 8),
		entry(RGB{116, 109, 101}, .007060196663284689, 8),
	}
	boundaries := []stackBoundary{
		{0, 1, .017699058601423578, 1}, {0, 2, .02659004515651708, 1},
		{0, 3, .2166364772814246, 1}, {0, 4, .11407122994106692, 1},
		{0, 5, .10791642217516646, 1}, {1, 2, .031533025486644384, 1},
		{1, 3, .10459346378549379, 1}, {1, 4, .17760925581039366, 1},
		{1, 5, .003055080745975457, 1}, {2, 3, .01607903666096895, 1},
		{2, 4, .0019835701711865706, 1}, {2, 5, .0843368114906753, 1},
		{3, 4, .07872413705130495, 1}, {3, 5, .01773094879710182, 1},
		{4, 5, .0014414368446564788, 1},
	}
	target, weights := targets(palette, o)
	ctx := context.WithValue(context.Background(), stackConstraintsKey{}, stackConstraints{required: []int{0}, top: -1})
	ctx = context.WithValue(ctx, stackAreaKey{}, paletteAreas(palette))
	families := make([]string, len(palette))
	for i, p := range palette {
		families[i] = layerColorFamily(p.RGB)
	}
	ctx = context.WithValue(ctx, layerFamiliesKey{}, families)
	initial := rebuildStack([]int{0, 1, 0, 1, 2, 3, 2, 0}, []int{10, 1, 1, 4, 1, 1, 1, 3}, lib, o.HueForge)
	initial.score, _ = stateScore(ctx, initial, target, weights, o, boundaries...)
	if math.Abs(initial.score-10.257800503893833) > 1e-9 {
		t.Fatal("fixture no longer reproduces the saved objective", initial.score)
	}
	merged := rebuildStack([]int{0, 1, 2, 3, 2, 0}, []int{12, 4, 1, 1, 1, 3}, lib, o.HueForge)
	merged.score, _ = stateScore(ctx, merged, target, weights, o, boundaries...)
	if merged.score <= initial.score {
		t.Fatal("fixture no longer exercises merge lookahead")
	}
	for _, effort := range []string{"preview", "refine"} {
		o.HueForge.SearchEffort = effort
		got, err := simplifyStack(ctx, initial, lib, []int{0, 1, 2, 3}, target, weights, o, nil, boundaries)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.indices) > 6 || got.score > initial.score+1e-9 || stackLayers(got) != stackLayers(initial) {
			t.Fatalf("%s retained detour or degraded objective/depth: %v %v %g", effort, got.indices, got.runs, got.score)
		}
		if !validStackOrder(got.indices, o) || !completeConstraints(got.indices, lib, o) || got.runs[0] < o.HueForge.BaseLayers() {
			t.Fatal("simplification violated a physical constraint")
		}
		selection, err := selectStackPalette(ctx, got, target, weights, o, boundaries)
		if err != nil || math.Abs(selection.score-got.score) > 1e-9 {
			t.Fatal("search and output palette disagree", err)
		}
		used := map[int]bool{}
		for i, id := range selection.ids {
			if selection.masses[i] > 0 {
				used[selection.positions[id]] = true
			}
		}
		for pos := 2; pos <= len(got.indices); pos++ {
			if !used[pos] {
				t.Fatal("unnecessary buried switch retained", pos)
			}
		}
		again, err := simplifyStack(ctx, initial, lib, []int{0, 1, 2, 3}, target, weights, o, nil, boundaries)
		if err != nil || !reflect.DeepEqual(got, again) {
			t.Fatal("nondeterministic simplification", err)
		}
	}
	if !sameInts(initial.runs, []int{10, 1, 1, 4, 1, 1, 1, 3}) {
		t.Fatal("mutated the input schedule")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := simplifyStack(canceled, initial, lib, []int{0}, target, weights, o, nil, boundaries); err != context.Canceled {
		t.Fatal("lost cancellation", err)
	}
}

func TestSimplifyPreservesOpticalUnderlayersAndRequiredSpools(t *testing.T) {
	for _, required := range []bool{false, true} {
		o := DefaultOptions()
		o.Mode, o.Colors = "stack", 3
		o.HueForge.BaseDepth, o.HueForge.MaxDepth = .16, .8
		o.HueForge.MaxRuns = 5
		lib := Library{Filaments: []Filament{
			{UUID: "base", RGB: RGB{}, TD: .3},
			{UUID: "red", RGB: RGB{220, 10, 10}, TD: 2},
			{UUID: "white", RGB: RGB{255, 255, 255}, TD: 7},
		}}
		o.HueForge.BaseFilament, o.HueForge.HighlightFilament = "base", "white"
		o.HueForge.HighlightOnlyAtTop = true
		if required {
			lib.Filaments[1].RGB, lib.Filaments[2].RGB = RGB{}, RGB{}
			o.HueForge.RequiredFilaments = "red"
		}
		req, _, top, err := constraintIDs(lib, o)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.WithValue(context.Background(), stackConstraintsKey{}, stackConstraints{req, top})
		ctx = context.WithValue(ctx, stackColorLimitKey{}, 1e-8)
		initial := rebuildStack([]int{0, 1, 2}, []int{1, 1, 1}, lib, o.HueForge)
		target, weights := targets([]PaletteEntry{entry(initial.rgbs[len(initial.rgbs)-1], 1, 8)}, o)
		initial.score, err = stateScore(ctx, initial, target, weights, o)
		if err != nil || initial.score > 1e-8 {
			t.Fatal("invalid optical fixture", err)
		}
		got, err := simplifyStack(ctx, initial, lib, []int{0}, target, weights, o, nil, nil)
		if err != nil || !reflect.DeepEqual(got, initial) {
			t.Fatal("removed a necessary buried spool", required, got, err)
		}
	}
}

func TestRefineRemovesEqualScoreSwapsInPreview(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.Colors = "stack", 2
	o.HueForge.MaxRuns = 5
	lib := Library{Filaments: []Filament{{UUID: "a", RGB: RGB{}, TD: .3}, {UUID: "b", RGB: RGB{}, TD: .3}}}
	initial := rebuildStack([]int{0, 1, 0}, []int{o.HueForge.BaseLayers(), 1, 1}, lib, o.HueForge)
	target, weights := targets([]PaletteEntry{entry(RGB{}, 1, 8)}, o)
	got, err := refineStack(context.Background(), initial, lib, []int{0, 1}, target, weights, o, nil)
	if err != nil || len(got.indices) != 1 || got.score != 0 || stackLayers(got) != stackLayers(initial) {
		t.Fatal("equal-score swaps survived", got, err)
	}
}
