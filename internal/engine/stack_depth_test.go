package engine

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestAutoDepthHardCeilingAndOldSettings(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.HueForge.AutoDepth = true
	for _, limit := range []float64{4, 4.03, 3.99, .561} {
		o.HueForge.MaxDepth = limit
		if err := o.Validate(); err != nil {
			t.Fatal(err)
		}
		actual := o.HueForge.Height(o.HueForge.MaxLayers())
		if actual > limit+1e-12 || actual+o.HueForge.LayerHeight <= limit-1e-12 {
			t.Fatalf("ceiling rounded incorrectly: limit=%g actual=%g", limit, actual)
		}
	}
	o.HueForge.MaxDepth = .55
	if o.Validate() == nil {
		t.Fatal("accepted a ceiling without room for a top layer")
	}
	o.HueForge.MaxDepth = 4
	raw, _ := json.Marshal(o)
	var restored Options
	if err := json.Unmarshal(raw, &restored); err != nil || !restored.HueForge.AutoDepth || restored.HueForge.MaxDepth != 4 {
		t.Fatal("automatic depth lost", err)
	}
	if err := json.Unmarshal([]byte(`{"layerHeight":0.08,"baseDepth":0.48,"maxDepth":2.24}`), &restored.HueForge); err != nil || restored.HueForge.AutoDepth {
		t.Fatal("old settings enabled automatic depth", err)
	}
}

func TestAutoDepthTrimsInteriorOpaquePaddingAndPreservesLaterBlends(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.Colors = "stack", 3
	o.HueForge.BaseDepth = .16
	o.HueForge.MaxDepth = o.HueForge.Height(15)
	o.HueForge.AutoDepth = true
	lib := Library{Filaments: []Filament{{Name: "Base", RGB: RGB{}, TD: .01}, {Name: "Opaque blue", RGB: RGB{20, 37, 56}, TD: .3}, {Name: "Translucent white", RGB: RGB{239, 240, 241}, TD: 6.3}}}
	initial := rebuildStack([]int{0, 1, 2}, []int{1, 6, 8}, lib, o.HueForge)
	palette := []PaletteEntry{}
	for _, rgb := range initial.rgbs {
		palette = append(palette, entry(rgb, 1./float64(len(initial.rgbs)), 8))
	}
	target, weights := targets(palette, o)
	ctx := context.WithValue(context.Background(), stackColorLimitKey{}, 1e-8)
	ctx = context.WithValue(ctx, stackConstraintsKey{}, stackConstraints{required: []int{0, 1, 2}, top: 2})
	initial.score, _ = stateScore(ctx, initial, target, weights, o)
	depths := depthCandidates{}
	if err := depths.thin(ctx, initial, lib, target, weights, o); err != nil {
		t.Fatal(err)
	}
	best, _ := depths.choose()
	if !reflect.DeepEqual(best.runs, []int{1, 1, 8}) || len(depths) < 6 {
		t.Fatalf("interior padding retained or useful white blends removed: %v depths=%d", best.runs, len(depths))
	}
	before, _, _ := uniqueStack(initial)
	after, _, _ := uniqueStack(best)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("removing opaque padding changed reachable colors")
	}
	if !reflect.DeepEqual(initial.runs, []int{1, 6, 8}) {
		t.Fatal("thinning mutated the original candidate")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := depths.thin(canceled, initial, lib, target, weights, o); err != context.Canceled {
		t.Fatal("lost cancellation", err)
	}
}

func TestAutoDepthStopsAtBaseForSolidBlack(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.Colors, o.PreblurSigma = "stack", 1, 0
	o.HueForge.AutoDepth, o.HueForge.MaxDepth = true, 4
	lib := Library{Filaments: []Filament{{RGB: RGB{}, TD: .3, Material: "PLA"}}}
	r := process(t, solid(RGB{}), o, &lib)
	if r.Stack.PlannedDepth != o.HueForge.BaseDepth || r.Stack.DepthSelection.ComparedDepths != o.HueForge.TransitionLayers()+1 {
		t.Fatalf("unnecessary thickness: %+v", r.Stack)
	}
	if r.Stack.DepthSelection.SelectedScore > r.Stack.DepthSelection.BestScore*1.01+1e-9 {
		t.Fatal("quality allowance exceeded")
	}
}

func TestAutoDepthAgainstExhaustiveTwoFilamentSearch(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.Colors, o.PreserveDetails = "stack", 2, false
	o.HueForge.AutoDepth, o.HueForge.MaxDepth = true, 1.13
	o.HueForge.BeamWidth = 64
	lib := Library{Filaments: []Filament{
		{RGB: RGB{}, TD: .3},
		{RGB: RGB{240, 245, 255}, TD: 5},
		{RGB: RGB{20, 60, 220}, TD: 2},
	}}
	palette := []PaletteEntry{entry(RGB{40, 50, 80}, .2, 8), entry(RGB{120, 155, 220}, .5, 8), entry(RGB{180, 190, 230}, .3, 8)}
	target, weights := targets(palette, o)
	// Independently enumerate every legal one/two-run stack in this small
	// problem, measuring nearest-color error directly (no palette cap/cull).
	byDepth := map[int]float64{}
	for depth := o.HueForge.BaseLayers(); depth <= o.HueForge.MaxLayers(); depth++ {
		best := math.Inf(1)
		for a := range lib.Filaments {
			for b := -1; b < len(lib.Filaments); b++ {
				if b == a || (b >= 0 && depth == o.HueForge.BaseLayers()) {
					continue
				}
				ids, runs := []int{a}, []int{depth}
				if b >= 0 {
					ids, runs = []int{a, b}, []int{o.HueForge.BaseLayers(), depth - o.HueForge.BaseLayers()}
				}
				s := rebuildStack(ids, runs, lib, o.HueForge)
				sum := 0.
				for i, color := range target {
					d := math.Inf(1)
					for _, rgb := range s.rgbs {
						d = math.Min(d, distance(color, o.colorVector(rgb)))
					}
					sum += weights[i] * d
				}
				best = math.Min(best, math.Sqrt(sum))
			}
		}
		byDepth[depth] = best
	}
	best := math.Inf(1)
	for _, score := range byDepth {
		best = math.Min(best, score)
	}
	wantDepth := o.HueForge.MaxLayers()
	for depth, score := range byDepth {
		if score <= best*1.01+1e-9 {
			wantDepth = min(wantDepth, depth)
		}
	}
	var lastDepth float64
	for i := 0; i < 3; i++ {
		_, plan, err := planStack(context.Background(), palette, lib, o, nil)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(plan.DepthSelection.BestScore-best) > 1e-8 || plan.PlannedDepth > o.HueForge.Height(wantDepth)+1e-9 {
			t.Fatalf("exhaustive mismatch: %+v want score=%g depth<=%g", plan.DepthSelection, best, o.HueForge.Height(wantDepth))
		}
		if i > 0 && plan.PlannedDepth != lastDepth {
			t.Fatal("nondeterministic depth")
		}
		lastDepth = plan.PlannedDepth
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := planStack(ctx, palette, lib, o, nil); err == nil {
		t.Fatal("ignored cancellation")
	}
}
