package engine

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"
)

func TestMaterialVolumeIntegratesOccupiedArea(t *testing.T) {
	o := DefaultOptions()
	o.HueForge.ExportWidthMM = 4
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	for x, a := range []uint8{255, 255, 1, 0} {
		img.SetNRGBA(x, 0, color.NRGBA{A: a})
	}
	r := &Result{Image: img, LayerMap: []uint16{1, 2, 3, 0}, Stack: &StackPlan{Runs: []StackRun{
		{Position: 1, StartLayer: 1, EndLayer: 1}, {Position: 2, StartLayer: 2, EndLayer: 3},
	}}}
	v, err := BuildSurfaceView(context.Background(), r, o)
	if err != nil {
		t.Fatal(err)
	}
	// Three 1 mm² columns: 0.16 + 0.24 + 0.32 = 0.72 mm³.
	if math.Abs(v.VolumeMM3-.72) > 1e-12 || math.Abs(v.MeanThicknessMM-.24) > 1e-12 {
		t.Fatalf("wrong integrated volume: %+v", v)
	}
	if len(v.MaterialRuns) != 2 || math.Abs(v.MaterialRuns[0].BuriedVolumeMM3-.32) > 1e-12 || math.Abs(v.MaterialRuns[1].VolumeMM3-.24) > 1e-12 {
		t.Fatalf("wrong per-run volumes: %+v", v.MaterialRuns)
	}
	if v.MaterialRuns[0].VisibleAreaFraction != 1./3 || v.MaterialRuns[1].StartCoverageFraction != 2./3 {
		t.Fatalf("visible surface confused with printing footprint: %+v", v.MaterialRuns)
	}
	o.HueForge.ExportWidthMM = 8
	v2, _ := BuildSurfaceView(context.Background(), r, o)
	if math.Abs(v2.VolumeMM3/v.VolumeMM3-4) > 1e-12 {
		t.Fatal("volume must scale with area")
	}
}

func TestMaterialPlanningRaisesSparseAccentWithoutLosingColors(t *testing.T) {
	o, lib, _, _ := surfaceFixture()
	o.HueForge.OptimizeMaterial = true
	o.HueForge.SurfaceColorTolerance = 0
	o.ColorPriority = "vivid"
	o.HueForge.BaseFilament = FilamentKey(lib.Filaments[0])
	// Common green should terminate below sparse magenta. Importance must not
	// substitute for its real 5% area in the volume objective.
	p := []PaletteEntry{entry(RGB{}, .1, 8), entry(RGB{0, 255, 0}, .85, 8), entry(RGB{255, 0, 255}, .05, 8)}
	ctx := context.Background()
	for _, automatic := range []bool{false, true} {
		o.HueForge.AutoDepth = automatic
		out, plan, err := planStack(ctx, p, lib, o, nil)
		if err != nil {
			t.Fatal(err)
		}
		if plan.RMS > 1e-8 || len(out) != 3 {
			t.Fatalf("lost colors: %+v", plan)
		}
		green, magenta := 0, 0
		for _, c := range out {
			if c.RGB == (RGB{0, 255, 0}) {
				green = c.StackLayer
			}
			if c.RGB == (RGB{255, 0, 255}) {
				magenta = c.StackLayer
			}
		}
		if green == 0 || magenta <= green {
			t.Fatalf("sparse accent not raised: %+v", out)
		}
		// Independently check the minimum occupied thickness for this opaque
		// three-color case: .16 foundation, .08 over 90%, .08 over 5%.
		mean := 0.
		for _, c := range out {
			mean += c.Fraction * c.StackHeight
		}
		if math.Abs(mean-.236) > 1e-8 {
			t.Fatalf("missed analytical material optimum: %.9f", mean)
		}
		_, again, err := planStack(ctx, p, lib, o, nil)
		if err != nil || !reflect.DeepEqual(plan, again) {
			t.Fatal("nondeterministic planning", err)
		}
	}
}

func TestMaterialUsesAreaInsteadOfPriority(t *testing.T) {
	o, _, _, p := surfaceFixture()
	o.HueForge.OptimizeMaterial = true
	ctx := context.WithValue(context.Background(), stackAreaKey{}, paletteAreas(p))
	colors := []RGB{p[0].RGB, p[1].RGB, p[2].RGB}
	heights := []int{1, 2, 3}
	var prior float64
	for i, priority := range []string{"balanced", "distinctive", "vivid"} {
		o.ColorPriority = priority
		target, _ := targets(p, o)
		penalty := stackGeometryPenalty(ctx, stackState{}, colors, heights, target, o, nil)
		if i > 0 && math.Abs(penalty-prior) > 1e-12 {
			t.Fatal("salience changed material volume")
		}
		prior = penalty
	}
}

func TestMaterialCompatibilityCancellationAndHFP(t *testing.T) {
	o, lib, img, _ := surfaceFixture()
	o.HueForge.OptimizeMaterial = true
	o.HueForge.ReduceShowThrough = true
	o.HueForge.SearchEffort = "refine"
	r, err := Process(context.Background(), img, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.SurfaceView.VolumeMM3 <= 0 {
		t.Fatal("missing material report")
	}
	if _, err := hueForgeProject(context.Background(), r, "material.png", ImageMetadata{}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(o)
	var restored Options
	if err := json.Unmarshal(raw, &restored); err != nil || !restored.HueForge.OptimizeMaterial {
		t.Fatal("option lost", err)
	}
	if err := json.Unmarshal([]byte(`{"reduceShowThrough":false}`), &restored.HueForge); err != nil || restored.HueForge.OptimizeMaterial {
		t.Fatal("legacy settings enabled optimization")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Process(ctx, img, o, &lib, nil); err != context.Canceled {
		t.Fatal("lost cancellation", err)
	}
}

func TestMaterialCannotBypassColorFidelityLimit(t *testing.T) {
	o, lib, _, p := surfaceFixture()
	o.HueForge.OptimizeMaterial = true
	ctx := context.WithValue(context.Background(), stackAreaKey{}, paletteAreas(p))
	ctx = context.WithValue(ctx, stackColorLimitKey{}, 0.)
	target, weights := targets(p, o)
	// An all-black slab saves material but loses two required image colors.
	s := rebuildStack([]int{0}, []int{o.HueForge.BaseLayers()}, lib, o.HueForge)
	score, err := stateScore(ctx, s, target, weights, o)
	if err != nil || !math.IsInf(score, 1) {
		t.Fatalf("color guard bypassed: %g %v", score, err)
	}
}

func TestHighlightOnlyAtTopAllowsOtherFilamentReturns(t *testing.T) {
	o, lib, _, p := surfaceFixture()
	o.HueForge.HighlightFilament = FilamentKey(lib.Filaments[2])
	o.HueForge.HighlightOnlyAtTop = true
	o.HueForge.MaxRuns = 5
	o.HueForge.OptimizeMaterial = true
	o.HueForge.SearchEffort = "refine"
	if completeConstraints([]int{0, 2, 1, 2}, lib, o) {
		t.Fatal("early highlight accepted")
	}
	if !completeConstraints([]int{0, 1, 0, 2}, lib, o) {
		t.Fatal("unrelated returns disallowed")
	}
	_, plan, err := planStack(context.Background(), p, lib, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, run := range plan.Runs {
		if (FilamentKey(run.Filament) == o.HueForge.HighlightFilament) != (i == len(plan.Runs)-1) {
			t.Fatal("highlight is not exclusively final")
		}
	}
}
