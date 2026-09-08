package engine

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestTrueBlackInGuidanceAndStack(t *testing.T) {
	const originalHex = "#212721"
	original := RGB{33, 39, 33}
	for _, mode := range []string{"guided", "stack"} {
		for _, enabled := range []bool{true, false} {
			name := mode + "/off"
			if enabled {
				name = mode + "/on"
			}
			t.Run(name, func(t *testing.T) {
				lib := testLibrary()
				lib.Filaments[0].RGB, lib.Filaments[0].Hex, lib.Filaments[0].TD = original, originalHex, .3
				before, _ := json.Marshal(lib)
				src := solid(original)
				src.Pix[3], src.Pix[7] = 0, 128
				pixelsBefore := append([]byte(nil), src.Pix...)
				o := testOptions()
				o.Mode, o.Colors, o.TrueBlack = mode, 1, enabled
				r := process(t, src, o, &lib)
				want := original
				if enabled {
					want = RGB{}
				}
				if len(r.Palette) != 1 || r.Palette[0].RGB != want || r.Palette[0].Hex != want.Hex() {
					t.Fatalf("palette = %+v; want %s", r.Palette, want.Hex())
				}
				for i := 0; i < len(src.Pix); i += 4 {
					if r.Image.Pix[i+3] != src.Pix[i+3] || (RGB{r.Image.Pix[i], r.Image.Pix[i+1], r.Image.Pix[i+2]}) != want {
						t.Fatalf("wrong RGB/alpha at %d", i/4)
					}
				}
				var selected Filament
				if mode == "guided" {
					selected = r.Guidance.Selected[0]
					if r.Guidance.Colors[0].RGB != want || r.Guidance.Colors[0].ReferenceRGB != want {
						t.Fatal("guidance report disagrees with output")
					}
				} else {
					selected = r.Stack.Runs[0].Filament
					for _, layer := range r.Stack.LayerColors {
						if layer.RGB != want {
							t.Fatal("stack model disagrees with output")
						}
					}
					if r.LayerMap[0] != 0 || r.LayerMap[1] != uint16(o.HueForge.BaseLayers()) {
						t.Fatal("layer-map semantics changed")
					}
				}
				if selected.RGB != want || selected.TD != .3 {
					t.Fatal("wrong modeled filament or changed TD")
				}
				if enabled && (selected.LibraryRGB == nil || *selected.LibraryRGB != original) {
					t.Fatal("original library color missing from report")
				}
				after, _ := json.Marshal(lib)
				if !bytes.Equal(before, after) || !bytes.Equal(pixelsBefore, src.Pix) {
					t.Fatal("processing mutated library or source")
				}
			})
		}
	}
}

func TestTrueBlackDoesNotReplaceGrayOrChromaticFilaments(t *testing.T) {
	lib := Library{Filaments: []Filament{
		{Name: "Matte BLACK", RGB: RGB{33, 39, 33}},
		{Name: "Charcoal", RGB: RGB{33, 39, 33}},
		{Name: "Black Cherry", RGB: RGB{90, 15, 35}},
		{Name: "Black & White", RGB: RGB{220, 220, 220}},
	}}
	got := trueBlackLibrary(lib)
	if got.Filaments[0].RGB != (RGB{}) {
		t.Fatal("named black not normalized")
	}
	for i := 1; i < len(lib.Filaments); i++ {
		if !reflect.DeepEqual(got.Filaments[i], lib.Filaments[i]) {
			t.Fatalf("changed non-black filament %q", lib.Filaments[i].Name)
		}
	}

	// Blends must use the normalized anchor rather than patching output pixels.
	f := got.Filaments[0]
	f.TD = 1
	h := DefaultOptions().HueForge
	h.LayerHeight, h.TDScale, h.TDTransmission = 1, 1, .25
	if mixed := blend(Vec{1, 1, 1}, f, h); mixed != (Vec{.25, .25, .25}) {
		t.Fatalf("black blend is tinted: %v", mixed)
	}
}

func TestTrueBlackLeavesPerceptualAndZeroGuidanceUnchanged(t *testing.T) {
	lib := testLibrary()
	lib.Filaments[0].RGB = RGB{33, 39, 33}
	for _, mode := range []string{"standard", "guided"} {
		o := testOptions()
		o.Mode, o.GuidanceStrength = mode, 0
		src := solid(RGB{33, 39, 33})
		o.TrueBlack = false
		off := process(t, src, o, &lib)
		o.TrueBlack = true
		on := process(t, src, o, &lib)
		if !bytes.Equal(off.Image.Pix, on.Image.Pix) {
			t.Fatalf("unexpected pixel changes in %s", mode)
		}
	}
}

func TestTrueBlackJSONDefaultsAndExplicitOptOut(t *testing.T) {
	if !DefaultOptions().TrueBlack {
		t.Fatal("true black must default on")
	}
	legacy := DefaultOptions()
	raw, _ := json.Marshal(legacy)
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(raw, &fields); e != nil {
		t.Fatal(e)
	}
	delete(fields, "trueBlack")
	raw, _ = json.Marshal(fields)
	var restored Options
	if e := json.Unmarshal(raw, &restored); e != nil {
		t.Fatal(e)
	}
	if !restored.TrueBlack || restored.Validate() != nil {
		t.Fatal("legacy options did not default on")
	}
	restored.TrueBlack = false
	raw, _ = json.Marshal(restored)
	var disabled Options
	if e := json.Unmarshal(raw, &disabled); e != nil {
		t.Fatal(e)
	}
	if disabled.TrueBlack {
		t.Fatal("explicit opt-out lost")
	}
	if e := json.Unmarshal([]byte(`{"colors":7}`), &disabled); e != nil {
		t.Fatal(e)
	}
	if disabled.TrueBlack || disabled.Colors != 7 {
		t.Fatal("partial options replaced explicit CLI choice")
	}
}
