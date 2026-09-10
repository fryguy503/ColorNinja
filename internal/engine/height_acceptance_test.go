package engine

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"reflect"
	"testing"
)

func TestHeightPlanningRetainsFullResolutionThinFeatures(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1024, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 1024; x++ {
			v := uint8(20)
			if x == 513 {
				v = 235
			}
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	for _, mode := range []string{"standard", "combo", "max-channel", "scaled-max-channel", "color-aware"} {
		o := heightOptions(mode)
		o.AnalysisMaxPixels = 64
		o.HueForge.MaxDepth = .8
		lib := popLibrary()
		r, err := Process(context.Background(), src, o, &lib, nil)
		if err != nil {
			t.Fatal(mode, err)
		}
		for y := 0; y < 256; y++ {
			if r.LayerMap[y*1024+513] <= r.LayerMap[y*1024+512] {
				t.Fatalf("%s erased one-pixel stripe at row %d", mode, y)
			}
		}
	}
}

func BenchmarkHeightWorkflow(b *testing.B) {
	src := image.NewNRGBA(image.Rect(0, 0, 512, 384))
	for y := 0; y < 384; y++ {
		for x := 0; x < 512; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x / 2), uint8(y * 255 / 383), uint8((x + y) % 256), 255})
		}
	}
	lib := popLibrary()
	for _, mode := range []string{"standard", "combo", "max-channel", "scaled-max-channel", "color-aware", "color-pop"} {
		b.Run(mode, func(b *testing.B) {
			o := heightOptions(mode)
			o.HueForge.MaxDepth = 1.44
			if mode == "color-pop" {
				o.HeightMap.Mode = ""
				o.ColorPop.Enabled = true
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := Process(context.Background(), src, o, &lib, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestColorAwareNativeChannelBrightness(t *testing.T) {
	var corpus struct {
		Cases []struct {
			RGB        RGB
			Shifts     [3]int
			Model      string
			Brightness float64
			Region     int
		}
	}
	raw, err := os.ReadFile("testdata/color-aware-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 512 {
		t.Fatal("incomplete reference")
	}
	for i, c := range corpus.Cases {
		h := DefaultHeightMapOptions()
		h.Mode = "color-aware"
		h.StandardModel = c.Model
		h.ChannelShift = c.Shifts
		_, v := heightSample(c.RGB, h)
		if math.Abs(v-c.Brightness) > 1e-12 {
			t.Fatalf("case %d: %.17g != native %.17g", i, v, c.Brightness)
		}
	}
}

func TestHeightRefinementConstraintsAndCancellation(t *testing.T) {
	lib := popLibrary()
	for _, mode := range []string{"standard", "color-aware", "color-pop"} {
		t.Run(mode, func(t *testing.T) {
			o := heightOptions(mode)
			if mode == "color-pop" {
				o.HeightMap.Mode = ""
				o.ColorPop.Enabled = true
			}
			o.HueForge.MaxDepth = 1.6
			o.HueForge.OpticalModel = BacklitModel
			o.HueForge.TDScale = 1.2
			o.HueForge.MaxRuns = 8
			base, err := Process(context.Background(), popFixture(), o, &lib, nil)
			if err != nil {
				t.Fatal(err)
			}
			o.HueForge.RequiredFilaments = FilamentKey(base.Stack.Runs[0].Filament)
			o.HueForge.BaseFilament = o.HueForge.RequiredFilaments
			o.HueForge.HighlightFilament = FilamentKey(base.Stack.Runs[len(base.Stack.Runs)-1].Filament)
			o.HueForge.SearchEffort = "refine"
			o.HueForge.ReduceShowThrough = true
			refined, err := Process(context.Background(), popFixture(), o, &lib, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(base.LayerMap, refined.LayerMap) {
				t.Fatal("refinement moved assigned pixels")
			}
			if FilamentKey(refined.Stack.Runs[0].Filament) != o.HueForge.BaseFilament || FilamentKey(refined.Stack.Runs[len(refined.Stack.Runs)-1].Filament) != o.HueForge.HighlightFilament || len(refined.Stack.Runs) > 8 || refined.Stack.UniqueFilaments > o.Colors {
				t.Fatal("refinement violated constraints")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			called := false
			_, err = Process(ctx, popFixture(), o, &lib, func(p Progress) {
				if p.Fraction > .35 {
					called = true
					cancel()
				}
			})
			if !called || !errors.Is(err, context.Canceled) {
				t.Fatal("did not cancel during planning", err)
			}
		})
	}
}

func TestBacklitHFPKeepsPhysicalOpticsAndHeights(t *testing.T) {
	o := heightOptions("standard")
	o.HueForge.OpticalModel = BacklitModel
	o.HueForge.TDScale = 1.2
	lib := popLibrary()
	r, err := Process(context.Background(), popFixture(), o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := hueForgeProject(context.Background(), r, "test.png", ImageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if doc["lighting_visualizer"] != 1 || doc["light_intensity"] != 20 || doc["luminance_method"] != 6 {
		t.Fatal("wrong Backlit HFP settings")
	}
	if r.Stack.Model != BacklitModel || r.StackView.MeshCore != "legacy-flat" {
		t.Fatal("model or height encoding drift")
	}
}
