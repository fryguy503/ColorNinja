package engine

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestAutomaticLayerPreferenceUsesRegionsNotJustRarity(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.HueForge.LayerPreference = "auto"
	// More cyan than red, but cyan is distributed over the background. The
	// coherent red subject is the appropriate automatic accent candidate.
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	p := []PaletteEntry{entry(RGB{20, 140, 40}, .86, 8), entry(RGB{0, 180, 210}, .09, 8), entry(RGB{220, 30, 30}, .05, 8)}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			c := p[0].RGB
			if x%10 == 0 {
				c = p[1].RGB
			}
			if x >= 40 && x < 60 && y >= 60 && y < 85 {
				c = p[2].RGB
			}
			img.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], 255})
		}
	}
	regions, err := analyzeLayerRegions(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), layerRegionsKey{}, regions)
	pref := imageLayerPreference(ctx, p, o)
	if pref == nil || pref.family != "red" {
		t.Fatalf("wrong spatial accent: %+v %+v", pref, regions)
	}
	// Hue-agnostic: swap the red subject to blue and detect it the same way.
	for y := 60; y < 85; y++ {
		for x := 40; x < 60; x++ {
			img.SetNRGBA(x, y, color.NRGBA{30, 30, 220, 255})
		}
	}
	p[2] = entry(RGB{30, 30, 220}, .05, 8)
	regions, _ = analyzeLayerRegions(context.Background(), img)
	pref = imageLayerPreference(context.WithValue(context.Background(), layerRegionsKey{}, regions), p, o)
	if pref == nil || pref.family != "blue" {
		t.Fatalf("hardcoded foreground hue: %+v", pref)
	}
}

func TestAutomaticLayerPreferenceDoesNotPromoteShadeOrScatteredNoise(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.HueForge.LayerPreference = "auto"
	p := []PaletteEntry{entry(RGB{0, 80, 20}, .6, 8), entry(RGB{140, 220, 160}, .03, 8), entry(RGB{160, 160, 160}, .37, 8)}
	if imageLayerPreference(context.Background(), p, o) != nil {
		t.Fatal("small green shade split from its dominant family")
	}
	p = append(p, entry(RGB{220, 20, 20}, .02, 8))
	ctx := context.WithValue(context.Background(), layerRegionsKey{}, map[string]layerRegion{"red": {compactness: .8, coherence: .02}})
	if imageLayerPreference(ctx, p, o) != nil {
		t.Fatal("scattered background speckles promoted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := analyzeLayerRegions(ctx, image.NewNRGBA(image.Rect(0, 0, 10, 10))); err != context.Canceled {
		t.Fatal("lost cancellation", err)
	}
}

func TestAutomaticAccentSeedsRepresentDistinctiveColorInsteadOfSkin(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.HueForge.LayerPreference = "auto"
	red := RGB{220, 30, 30}
	p := []PaletteEntry{entry(RGB{100, 100, 100}, .90, 8), entry(RGB{246, 185, 142}, .07, 8), entry(red, .03, 8)}
	pref := imageLayerPreference(context.Background(), p, o)
	if pref == nil || pref.representative != red {
		t.Fatalf("pale skin dominated the accent seed: %+v", pref)
	}
}

func TestLayerPreferenceUsesMappedHeightsAndDoesNotRewardExtraHeight(t *testing.T) {
	o, _, _, _ := surfaceFixture()
	o.HueForge.LayerPreference = "auto"
	p := []PaletteEntry{entry(RGB{0, 255, 0}, .95, 8), entry(RGB{255, 0, 255}, .05, 8)}
	pref := imageLayerPreference(context.Background(), p, o)
	if pref == nil {
		t.Fatal("missing accent")
	}
	low, _ := preferencePenalty(pref, []float64{.5, .2}, o)
	high, r := preferencePenalty(pref, []float64{.2, .3}, o)
	tower, _ := preferencePenalty(pref, []float64{.2, .8}, o)
	if low <= 0 || high != 0 || tower != high || !r.Satisfied {
		t.Fatal("direction or needless height reward", low, high, tower, r)
	}
	// Equal output RGBs at early and late returns must choose the actual later
	// height. A last-run label by itself cannot satisfy this assertion.
	ctx := context.WithValue(context.Background(), layerPreferenceKey{}, pref)
	s := stackState{rgbs: []RGB{{255, 0, 255}, {0, 255, 0}, {255, 0, 255}}, layers: []int{1, 2, 3}, positions: []int{1, 2, 3}}
	colors, layers, positions := uniqueStack(s)
	target, _ := targets(p, o)
	if err := chooseStackHeights(ctx, s, colors, layers, positions, []int{0, 1}, target, o, nil); err != nil {
		t.Fatal(err)
	}
	if layers[0] != 3 || positions[0] != 3 {
		t.Fatal("accent still assigned to early return", layers, positions)
	}
}

func TestAutomaticLayerPlanningColorGuardAndRoundTrip(t *testing.T) {
	o, lib, _, _ := surfaceFixture()
	o.HueForge.LayerPreference = "auto"
	o.HueForge.MaxRuns = 5
	o.HueForge.SurfaceColorTolerance = 0
	o.HueForge.BaseFilament = FilamentKey(lib.Filaments[0])
	p := []PaletteEntry{entry(RGB{}, .1, 8), entry(RGB{0, 255, 0}, .85, 8), entry(RGB{255, 0, 255}, .05, 8)}
	for _, effort := range []string{"preview", "refine"} {
		o.HueForge.SearchEffort = effort
		_, plan, err := planStack(context.Background(), p, lib, o, nil)
		if err != nil || plan.RMS > 1e-8 || plan.LayerPreference == nil || !plan.LayerPreference.Satisfied {
			t.Fatalf("order/color failed: %+v %v", plan, err)
		}
	}
	ctx := context.WithValue(context.Background(), stackColorLimitKey{}, 0.)
	ctx = context.WithValue(ctx, layerPreferenceKey{}, imageLayerPreference(ctx, p, o))
	s := rebuildStack([]int{0}, []int{o.HueForge.BaseLayers()}, lib, o.HueForge)
	target, weights := targets(p, o)
	score, err := stateScore(ctx, s, target, weights, o)
	if err != nil || !math.IsInf(score, 1) {
		t.Fatal("preference bypassed color guard")
	}
	searchCtx := context.WithValue(ctx, layerSearchBarrierKey{}, true)
	if relaxed, e := stateScore(searchCtx, s, target, weights, o); e != nil || !finite(relaxed) {
		t.Fatal("cannot refine a near-miss order", e)
	}
	if strict, e := stateScore(ctx, s, target, weights, o); e != nil || !math.IsInf(strict, 1) {
		t.Fatal("relaxed search leaked into final validation", e)
	}
	raw, _ := json.Marshal(o)
	var restored Options
	if err := json.Unmarshal(raw, &restored); err != nil || restored != o {
		t.Fatal("preference persistence", err)
	}
	var h HueForgeOptions
	if err := json.Unmarshal([]byte(`{"optimizeMaterial":true,"surfaceColorTolerance":0}`), &h); err != nil || h.LayerPreference != "auto" || h.OptimizeMaterial {
		t.Fatal("dev1 migration", h, err)
	}
	if h.SurfaceColorTolerance != 5 {
		t.Fatal("dev1 did not migrate to automatic preset")
	}
	if err := json.Unmarshal([]byte(`{"layerPreference":"auto","surfaceColorTolerance":0}`), &h); err != nil || h.SurfaceColorTolerance != 0 {
		t.Fatal("explicit new-format zero allowance ignored")
	}
	if err := json.Unmarshal([]byte(`{"optimizeMaterial":false}`), &h); err != nil || h.LayerPreference != "" {
		t.Fatal("old settings enabled preference")
	}
}

func TestAutomaticOrderProtectsSmallColorFamiliesAndGroupsRelatedShades(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.HueForge.LayerPreference = "auto"
	p := []PaletteEntry{entry(RGB{0, 80, 30}, .49, 8), entry(RGB{100, 200, 120}, .49, 8), entry(RGB{220, 30, 30}, .02, 8)}
	areas := paletteAreas(p)
	families := []string{"green", "green", "red"}
	target, _ := targets(p, o)
	colors := []RGB{p[0].RGB, p[1].RGB, p[2].RGB}
	ctx := context.WithValue(context.Background(), stackAreaKey{}, areas)
	ctx = context.WithValue(ctx, layerFamiliesKey{}, families)
	ctx = context.WithValue(ctx, layerColorLimitsKey{}, map[string]float64{"red": 1, "green": 1})
	if !layerColorsAllowed(ctx, colors, target, o) {
		t.Fatal("exact palette failed family guard")
	}
	if layerColorsAllowed(ctx, colors[:2], target, o) {
		t.Fatal("small red family lost behind low total error")
	}
	together := layerPreferencePenalty(ctx, colors, []int{1, 2, 10}, target, o)
	island := layerPreferencePenalty(ctx, colors, []int{1, 10, 10}, target, o)
	if island <= together {
		t.Fatal("isolated pale shade not penalized", together, island)
	}
	key := processingKey(o)
	o.HueForge.ExportWidthMM = 160
	if processingKey(o) == key {
		t.Fatal("automatic geometry cache ignored print width")
	}
}
