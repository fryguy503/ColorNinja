package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"os"
	"reflect"
	"slices"
	"testing"
)

func TestBorderHFPKeepsImageAndMeshCore(t *testing.T) {
	for _, core := range []string{"compact-blends", "legacy-flat", "filament-blends"} {
		t.Run(core, func(t *testing.T) {
			r := hfpFixture(t)
			h := r.Stack.Options
			h.MeshCore = core
			r = ReframeResult(r, Options{HueForge: h})
			original, err := hueForgeProject(context.Background(), r, "fixture.png", ImageMetadata{})
			if err != nil {
				t.Fatal(err)
			}
			for _, placement := range []string{"external", "internal"} {
				h.Border = BorderOptions{true, placement, .1, 4}
				next := ReframeResult(r, Options{HueForge: h})
				doc, err := hueForgeProject(context.Background(), next, "fixture.png", ImageMetadata{})
				if err != nil {
					t.Fatal(err)
				}
				if doc["borderless"] != false || doc["external_border"] != (placement == "external") || doc["border_width"] != .1 || doc["border_height"] != 4. {
					t.Fatal("wrong border contract", doc)
				}
				for _, key := range []string{"image_binary", "width_in_mm", "height_in_mm", "match_filament_set", "match_slider_values", "filament_set", "color_match_method", "min_depth"} {
					if !reflect.DeepEqual(original[key], doc[key]) {
						t.Fatalf("border changed %s", key)
					}
				}
				last := r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer
				top := int(math.Round(h.layerCount(4)))
				ends := doc["slider_values"].([]int)
				if ends[len(ends)-1] != top || doc["max_depth"].(float64) <= 4 {
					t.Fatal("missing border print headroom")
				}
				for layer := last + 1; layer <= top+1; layer++ {
					if !slices.Contains(doc["disabled_match_layers"].([]int), layer) {
						t.Fatal("border-only layer can match image", layer)
					}
				}
				if r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer != last {
					t.Fatal("mutated image plan")
				}
			}
			h.Border.Enabled = false
			disabled, err := hueForgeProject(context.Background(), ReframeResult(r, Options{HueForge: h}), "fixture.png", ImageMetadata{})
			if err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(original)
			b, _ := json.Marshal(disabled)
			if !bytes.Equal(a, b) {
				t.Fatal("border opt-out did not restore byte-identical HFP")
			}
		})
	}
}

func TestBorderDimensionsDepthAndTransparency(t *testing.T) {
	r := hfpFixture(t)
	r.Image = image.NewNRGBA(image.Rect(0, 0, 200, 100)) // Fully transparent content does not shape the frame.
	h := r.Stack.Options
	h.ExportWidthMM = 200
	h.Border = BorderOptions{true, "external", 4, 3}
	b, err := resolveBorder(r, h)
	if err != nil {
		t.Fatal(err)
	}
	if b.OuterWidthMM != 208 || b.OuterHeightMM != 108 || b.VolumeMM3 != 7392 {
		t.Fatal("external frame", b)
	}
	h.Border.Placement = "internal"
	b, err = resolveBorder(r, h)
	if err != nil {
		t.Fatal(err)
	}
	if b.ImageWidthMM != 184 || b.ImageHeightMM != 92 || b.OuterWidthMM != 192 || b.OuterHeightMM != 100 {
		t.Fatal("internal short-side shrink", b)
	}
	h.Border.HeightMM = 0
	b, err = resolveBorder(r, h)
	if err != nil {
		t.Fatal(err)
	}
	if b.HeightMM != h.Height(r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer) || b.ExtraLayers != 0 {
		t.Fatal("follow image depth", b)
	}
	for _, l := range r.Stack.LayerColors {
		h.Border.HeightMM = math.Round(l.Height*100) / 100
		b, err = resolveBorder(r, h)
		if err != nil {
			t.Fatal(err)
		}
		if b.TopRGB != l.RGB || b.TopLayer != l.Layer {
			t.Fatal("border color differs from same-height image", l, b)
		}
	}
	h.Border.WidthMM = 50
	if _, err = resolveBorder(r, h); err == nil {
		t.Fatal("exhausted internal image grid accepted")
	}
}

func TestBorderValidationAndOldOptions(t *testing.T) {
	o := DefaultOptions()
	if o.HueForge.Border.Enabled {
		t.Fatal("default border enabled")
	}
	for _, b := range []BorderOptions{{true, "bad", 4, 3}, {true, "external", 0, 3}, {true, "external", 4, -1}, {true, "external", math.NaN(), 3}, {true, "external", 4, math.Inf(1)}, {true, "external", 4, .01}, {true, "external", .001, 3}} {
		o.HueForge.Border = b
		if o.HueForge.Validate() == nil {
			t.Fatal("invalid border accepted", b)
		}
	}
	o = DefaultOptions()
	o.HueForge.Border = BorderOptions{true, "internal", 6, 3}
	raw, _ := json.Marshal(o)
	var restored Options
	if err := json.Unmarshal(raw, &restored); err != nil || restored.HueForge.Border != o.HueForge.Border {
		t.Fatal("border settings lost", err)
	}
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	delete(fields["hueforge"].(map[string]any), "border")
	raw, _ = json.Marshal(fields)
	if err := json.Unmarshal(raw, &restored); err != nil || restored.HueForge.Border.Enabled || restored.HueForge.Border.WidthMM != 4 {
		t.Fatal("old project inherited active border", err)
	}
}

func TestBorderReusesImageAndRetainsRegionPlan(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.Colors = 2
	o.PreblurSigma = 0
	o.HueForge.MaxDepth = .8
	lib := Library{Filaments: []Filament{{RGB: RGB{0, 0, 0}, TD: .2}, {RGB: RGB{255, 255, 255}, TD: 2}}}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			v := uint8(x * 17)
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	p := &Processor{}
	first, err := p.Process(context.Background(), img, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	nextOptions := o
	nextOptions.HueForge.Border = BorderOptions{true, "external", 4, 4}
	next, err := p.Process(context.Background(), img, nextOptions, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Reused, "rendered result") || next.Image != first.Image || !slices.Equal(next.LayerMap, first.LayerMap) || next.SurfaceView.Border == nil {
		t.Fatal("border reprocessed image or lost frame")
	}
	if !SameRegionPlanOptions(o, nextOptions) {
		t.Fatal("border invalidates region edits")
	}
	if first.SurfaceView.Border != nil || first.Stack.Options.Border.Enabled {
		t.Fatal("cached result mutated")
	}
}

func TestBorderNativeMeshFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/hueforge-border-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Fixtures []struct {
			ImageWidthMM  float64    `json:"imageWidthMm"`
			ImageHeightMM float64    `json:"imageHeightMm"`
			WidthMM       float64    `json:"widthMm"`
			HeightMM      float64    `json:"heightMm"`
			Minimum       [3]float64 `json:"minimum"`
			Maximum       [3]float64 `json:"maximum"`
			VolumeMM3     float64    `json:"volumeMm3"`
		} `json:"fixtures"`
	}
	if err = json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Fixtures) != 4 {
		t.Fatal("native fixtures missing")
	}
	r := hfpFixture(t)
	for _, f := range reference.Fixtures {
		r.Image = image.NewNRGBA(image.Rect(0, 0, int(f.ImageWidthMM), int(f.ImageHeightMM)))
		h := r.Stack.Options
		h.ExportWidthMM = f.ImageWidthMM
		h.Border = BorderOptions{true, "external", f.WidthMM, f.HeightMM}
		b, err := resolveBorder(r, h)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(b.OuterWidthMM-(f.Maximum[0]-f.Minimum[0])) > 1e-5 || math.Abs(b.OuterHeightMM-(f.Maximum[1]-f.Minimum[1])) > 1e-5 || math.Abs(b.VolumeMM3-f.VolumeMM3) > 1e-3 {
			t.Fatal("native frame disagreement", f, b)
		}
	}
}

func TestBorderColorPopAndBacklitExport(t *testing.T) {
	for _, model := range []string{FrontlitModel, BacklitModel} {
		o := popOptions()
		o.Mode = "stack"
		o.HueForge.OpticalModel = model
		if model == BacklitModel {
			o.HueForge.TDScale = 1.2
		}
		lib := popLibrary()
		r, err := Process(context.Background(), popFixture(), o, &lib, nil)
		if err != nil {
			t.Fatal(err)
		}
		before, err := hueForgeProject(context.Background(), r, "pop.png", ImageMetadata{})
		if err != nil {
			t.Fatal(err)
		}
		o.HueForge.Border = BorderOptions{true, "external", 3, 4}
		after, err := hueForgeProject(context.Background(), ReframeResult(r, o), "pop.png", ImageMetadata{})
		if err != nil {
			t.Fatal(err)
		}
		if after["borderless"] != false || after["border_height"] != 4. {
			t.Fatal("missing border", model)
		}
		for _, key := range []string{"image_binary", "match_filament_set", "match_slider_values", "lighting_visualizer", "light_intensity"} {
			if !reflect.DeepEqual(before[key], after[key]) {
				t.Fatal("border changed Color Pop or Backlit", model, key)
			}
		}
	}
}
