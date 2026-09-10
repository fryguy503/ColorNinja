package engine

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func allocationFixture() (Options, Library) {
	o := DefaultOptions()
	o.Mode, o.Colors, o.TrueBlack = "stack", 2, false
	o.HueForge.LightPreset = "neutral-white"
	o.HueForge.MaxDepth = o.HueForge.Height(16)
	lib := Library{Filaments: []Filament{
		{UUID: "blue", Name: "Dark blue", RGB: RGB{15, 35, 90}, TD: 5},
		{UUID: "white", Name: "Warm white", RGB: RGB{245, 240, 225}, TD: 7.5},
		{UUID: "red", Name: "Red", RGB: RGB{220, 35, 25}, TD: 4},
	}}
	return o, lib
}

func TestLayerAllocationEscapesPlateaus(t *testing.T) {
	o, lib := allocationFixture()
	for _, colors := range [][]RGB{
		{{107, 35, 61}, {15, 35, 90}, {20, 39, 92}},
		{{90, 102, 134}, {229, 225, 212}, {153, 158, 171}},
		{{236, 231, 218}, {245, 240, 225}, {237, 177, 164}},
		{{236, 175, 162}, {221, 217, 205}, {243, 238, 223}},
		{{236, 231, 218}, {243, 238, 223}, {231, 138, 125}},
		{{227, 94, 83}, {233, 139, 127}, {233, 139, 127}},
	} {
		p := []PaletteEntry{}
		for _, rgb := range colors {
			p = append(p, entry(rgb, 1./3, 8))
		}
		for _, effort := range []string{"preview", "refine"} {
			o.HueForge.SearchEffort = effort
			out, plan, err := planStack(context.Background(), p, lib, o, nil)
			if err != nil {
				t.Fatal(err)
			}
			if plan.OptimizationScore > 1e-8 || plan.RMS > 1e-8 {
				t.Fatalf("%s missed reachable colors %v: score=%g RMS=%g", effort, colors, plan.OptimizationScore, plan.RMS)
			}
			if plan.UniqueFilaments > 2 || plan.PlannedDepth > o.HueForge.MaxDepth+1e-9 || plan.Runs[0].Layers < o.HueForge.BaseLayers() {
				t.Fatal("allocation escaped physical constraints", plan)
			}
			for _, target := range colors {
				found := false
				for _, c := range out {
					found = found || c.RGB == target
				}
				if !found {
					t.Fatal("score and emitted palette disagree", out)
				}
			}
		}
	}
}

func TestLayerTransferBudgetAndAllocationDiversity(t *testing.T) {
	for _, effort := range []string{"preview", "refine"} {
		for _, n := range []int{1, 11, 32, 1000} {
			moves := layerTransfers(n, effort)
			if len(moves) > 32 || moves[0] != 1 || moves[len(moves)-1] != n {
				t.Fatal("unbounded or incomplete block endpoints", moves)
			}
			for i, move := range moves {
				if move < 1 || move > n || (i > 0 && move <= moves[i-1]) {
					t.Fatal("invalid block transfer", moves)
				}
			}
		}
	}
	states := []stackState{
		{indices: []int{0, 1}, runs: []int{5, 11}, score: 1},
		{indices: []int{0, 1}, runs: []int{6, 10}, score: 1.1},
		{indices: []int{1, 0}, runs: []int{5, 11}, score: 1.2},
		{indices: []int{0, 1}, runs: []int{9, 7}, score: 1.3},
	}
	got := diverseStacks(states, 2)
	if len(got) != 3 || !reflect.DeepEqual(got[0], states[0]) || !reflect.DeepEqual(got[1], states[2]) || !reflect.DeepEqual(got[2], states[3]) {
		t.Fatal("lost order winner or separated allocation", got)
	}
}

func TestJointPaletteRespectsColorAndHeightConstraints(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.Colors = "stack", 2
	o.HueForge.OptimizeMaterial = true
	o.HueForge.LightPreset = "neutral-white"
	o.HueForge.MaxPerceivedColors = 1
	lib := Library{Filaments: []Filament{{RGB: RGB{}, TD: .3}, {RGB: RGB{255, 255, 255}, TD: 7.5}}}
	s := rebuildStack([]int{0, 1}, []int{5, 22}, lib, o.HueForge)
	for _, tc := range []struct {
		rgb   RGB
		layer int
	}{
		{RGB{158, 229, 212}, 11}, {RGB{144, 151, 205}, 8}, {RGB{221, 160, 141}, 9},
	} {
		target, weights := targets([]PaletteEntry{entry(tc.rgb, 1, 8)}, o)
		ctx := context.WithValue(context.Background(), stackAreaKey{}, []float64{1})
		plain := o
		plain.HueForge.OptimizeMaterial = false
		base, err := selectStackPalette(ctx, s, target, weights, plain, nil)
		if err != nil {
			t.Fatal(err)
		}
		ctx = context.WithValue(ctx, stackColorLimitKey{}, base.colorRMS*1.05+1e-8)
		p, err := selectStackPalette(ctx, s, target, weights, o, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.ids) != 1 || p.layers[p.ids[0]] != tc.layer || p.colorRMS > base.colorRMS*1.05+1e-8 || p.masses[0] != 1 {
			t.Fatal("missed permitted material saving", p)
		}
		strict := context.WithValue(ctx, stackColorLimitKey{}, base.colorRMS+1e-8)
		guarded, err := selectStackPalette(strict, s, target, weights, o, nil)
		if err != nil || guarded.colorRMS > base.colorRMS+1e-8 {
			t.Fatal("color limit bypassed", guarded, err)
		}
		// A family guard remains authoritative even with a looser global limit.
		family := context.WithValue(ctx, layerFamiliesKey{}, []string{"red"})
		family = context.WithValue(family, layerColorLimitsKey{}, map[string]float64{"red": base.colorRMS})
		familyOptions := o
		familyOptions.HueForge.LayerPreference = "red"
		guarded, err = selectStackPalette(family, s, target, weights, familyOptions, nil)
		if err != nil || guarded.colorRMS > base.colorRMS+1e-8 {
			t.Fatal("family limit bypassed", guarded, err)
		}
		again, _ := selectStackPalette(ctx, s, target, weights, o, nil)
		if !reflect.DeepEqual(p, again) {
			t.Fatal("nondeterministic selection")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := selectStackPalette(canceled, s, []Vec{{}}, []float64{1}, o, nil); err != context.Canceled {
		t.Fatal("lost cancellation", err)
	}
}

func TestJointPaletteExportMatchesSearch(t *testing.T) {
	o, lib := allocationFixture()
	o.HueForge.OptimizeMaterial = true
	o.HueForge.ReduceShowThrough = true
	o.HueForge.BaseFilament = FilamentKey(lib.Filaments[0])
	o.HueForge.HighlightFilament = FilamentKey(lib.Filaments[1])
	o.HueForge.HighlightOnlyAtTop = true
	o.HueForge.MaxPerceivedColors = 2
	o.HueForge.AutoDepth = true
	img := solid(RGB{158, 229, 212})
	r := process(t, img, o, &lib)
	if _, err := hueForgeProject(context.Background(), r, "joint.png", ImageMetadata{}); err != nil {
		t.Fatal(err)
	}
	if r.Stack.UniqueFilaments != 2 || len(r.Palette) > 2 || r.Stack.PlannedDepth > o.HueForge.MaxDepth+1e-9 {
		t.Fatal("export constraints changed", r.Stack)
	}
	for _, p := range r.Palette {
		found := false
		for _, l := range r.Stack.LayerColors {
			if l.Layer == p.StackLayer && l.RGB == p.RGB && l.TopPosition == p.TopPosition {
				found = true
			}
		}
		if !found {
			t.Fatal("export palette is not a physical layer", p)
		}
	}
}

func BenchmarkLayerPlanning(b *testing.B) {
	for _, material := range []bool{false, true} {
		b.Run(fmt.Sprintf("material-%t", material), func(b *testing.B) {
			o := DefaultOptions()
			o.Mode = "stack"
			o.Colors = 4
			o.HueForge.MaxDepth = 1.28
			o.HueForge.OptimizeMaterial = material
			lib := Library{}
			for i := 0; i < 45; i++ {
				lib.Filaments = append(lib.Filaments, Filament{UUID: fmt.Sprint(i), RGB: RGB{uint8(i * 73), uint8(i * 37), uint8(i * 113)}, TD: .3 + float64(i%15)/2})
			}
			p := []PaletteEntry{entry(RGB{25, 40, 60}, .4, 8), entry(RGB{140, 180, 210}, .4, 8), entry(RGB{220, 65, 35}, .2, 8)}
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := planStack(context.Background(), p, lib, o, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
