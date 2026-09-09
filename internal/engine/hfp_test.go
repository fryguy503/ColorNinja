package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func returnStackFixture() (Options, Library, []PaletteEntry) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.Colors = 2
	o.PreserveDetails = false
	o.PreblurSigma = 0
	o.HueForge.MaxRuns = 3
	o.HueForge.BeamWidth = 64
	o.HueForge.MaxDepth = o.HueForge.Height(11)
	lib := Library{Filaments: []Filament{
		{Brand: "Fixture", Name: "Ivory", Material: "PLA", RGB: RGB{250, 245, 235}, TD: 4, SourceIndex: 0},
		{Brand: "Fixture", Name: "Blue", Material: "PLA", RGB: RGB{10, 50, 230}, TD: 2, SourceIndex: 1},
	}}
	s := rebuildStack([]int{0, 1, 0}, []int{5, 2, 4}, lib, o.HueForge)
	p := []PaletteEntry{}
	for _, rgb := range s.rgbs {
		p = append(p, entry(rgb, 1/float64(len(s.rgbs)), o.NeutralChroma))
	}
	return o, lib, p
}

func TestStackFilamentReturns(t *testing.T) {
	o, lib, p := returnStackFixture()
	for _, preserve := range []bool{false, true} {
		o.PreserveDetails = preserve
		_, plan, e := planStack(context.Background(), p, lib, o, nil)
		if e != nil {
			t.Fatal(e)
		}
		legacy := o
		legacy.HueForge.MaxRuns = 0
		_, old, e := planStack(context.Background(), p, lib, legacy, nil)
		if e != nil {
			t.Fatal(e)
		}
		if plan.UniqueFilaments != 2 || len(plan.Runs) != 3 || plan.Runs[0].Filament.SourceIndex != plan.Runs[2].Filament.SourceIndex || plan.RMS >= old.RMS {
			t.Fatalf("returns did not improve: new=%+v old=%g", plan, old.RMS)
		}
		for i, r := range plan.Runs {
			if i > 0 && r.StartLayer != plan.Runs[i-1].EndLayer+1 {
				t.Fatal("noncontiguous runs")
			}
		}
		t.Logf("preserve=%v: %d spools, %d runs; RMS %.6f -> %.6f", preserve, plan.UniqueFilaments, len(plan.Runs), old.RMS, plan.RMS)
	}
	if validStackOrder([]int{0, 0}, o) || validStackOrder([]int{0, 1, 2}, o) || validStackOrder([]int{0, 1, 0, 1}, o) {
		t.Fatal("invalid run budget accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := planStack(ctx, p, lib, o, nil); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func hfpFixture(t *testing.T) *Result {
	t.Helper()
	o, lib, p := returnStackFixture()
	palette, plan, err := planStack(context.Background(), p, lib, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, len(palette)+1, 1))
	r := &Result{Image: img, SourceSize: [2]int{img.Rect.Dx(), 1}, Stack: plan, Palette: palette, LayerMap: make([]uint16, img.Rect.Dx())}
	for i, p := range palette {
		img.SetNRGBA(i, 0, color.NRGBA{p.RGB[0], p.RGB[1], p.RGB[2], 255})
		r.LayerMap[i] = uint16(p.StackLayer)
	}
	img.Pix[3] = 100 // Fractional alpha must not change the selected print height.
	return r
}

func TestHFPExport(t *testing.T) {
	r := hfpFixture(t)
	r.Stack.Options.MeshCore = "legacy-flat"
	ctx := context.Background()
	doc, err := hueForgeProject(ctx, r, "source.png", ImageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if doc["luminance_method"] != 6 || doc["color_match_method"] != 0 || doc["depth_mode"] != 2 || doc["lighting_visualizer"] != 0 {
		t.Fatal("wrong Color Match / Front Lit selectors")
	}
	decoded, err := png.Decode(bytes.NewReader(doc["image_binary"].([]byte)))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, solid := decoded.At(0, 0).RGBA()
	_, _, _, empty := decoded.At(decoded.Bounds().Max.X-1, 0).RGBA()
	if solid != 65535 || empty != 0 {
		t.Fatal("HFP alpha policy changed")
	}
	fs := doc["filament_set"].([]hfpFilament)
	sliders := doc["slider_values"].([]int)
	if len(fs) != 3 || fs[0].UUID != fs[2].UUID || fs[0].UUID == fs[1].UUID {
		t.Fatal("repeat identity lost")
	}
	for i, run := range r.Stack.Runs {
		if sliders[i] != run.EndLayer {
			t.Fatal("slider is not inclusive end layer")
		}
		if fs[len(fs)-1-i].Color != run.Filament.RGB.Hex() {
			t.Fatal("Filament Painting order reversed")
		}
	}
	mesh := doc["match_filament_set"].([]hfpFilament)
	ends := doc["match_slider_values"].([]int)
	for _, m := range mesh {
		if m.Material != "IMAGE" || m.Owned {
			t.Fatal("virtual color exported as a real spool")
		}
	}
	for _, p := range r.Palette {
		id := 0
		for id < len(ends)-1 && p.StackLayer > ends[id] {
			id++
		}
		if mesh[len(mesh)-1-id].Color != p.Hex {
			t.Fatalf("wrong mesh color at layer %d", p.StackLayer)
		}
		for _, v := range doc["disabled_match_layers"].([]int) {
			if v == p.StackLayer {
				t.Fatal("used layer disabled")
			}
		}
	}
	path := filepath.Join(t.TempDir(), "export.hfp")
	if err = SaveHFP(ctx, path, r, "source.png", ImageMetadata{}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var parsed map[string]any
	if err = json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed["image_binary"].(string)) == 0 {
		t.Fatal("missing embedded image")
	}
	if err = SaveHFP(ctx, path, r, "source.png", ImageMetadata{}, false); err == nil {
		t.Fatal("overwrote output")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err = SaveHFP(canceled, path, r, "source.png", ImageMetadata{}, true); err == nil {
		t.Fatal("ignored cancellation")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(raw) {
		t.Fatal("failed write damaged prior export")
	}
	for mode, n := range map[string]int{"combo": 2, "color-aware": 4, "color-pop": 5} {
		r.Stack.Options.MeshMode = mode
		d, e := hueForgeProject(ctx, r, "source.png", ImageMetadata{})
		if e != nil || d["luminance_method"] != n {
			t.Fatalf("mode %s: %v", mode, e)
		}
	}
	r.Stack.Options.MeshMode = "color-match"
	r.Stack.Options.MeshCore = "filament-blends"
	d, err := hueForgeProject(ctx, r, "source.png", ImageMetadata{})
	if err != nil || len(d["match_filament_set"].([]hfpFilament)) != 3 {
		t.Fatal(err)
	}
	r.Stack.Options.OpticalModel = LegacyModel
	if _, err = hueForgeProject(ctx, r, "source.png", ImageMetadata{}); err == nil {
		t.Fatal("legacy optics accepted")
	}
}

func TestHFPOverrideIdentityAndOptions(t *testing.T) {
	f := Filament{UUID: "{existing}", Name: "Black", RGB: RGB{}, TD: .3, LibraryRGB: &RGB{8, 10, 13}}
	if got := hfpMaterial(f); got.UUID == f.UUID || got.Color != "#000000" {
		t.Fatal("overridden library identity aliased")
	}
	o := DefaultOptions()
	o.HueForge.MaxRuns = 6
	if err := json.Unmarshal([]byte(`{"hueforge":{"layerHeight":0.08}}`), &o); err != nil || o.HueForge.MaxRuns != 0 {
		t.Fatal("old saved search changed")
	}
	for _, edit := range []func(*HueForgeOptions){func(h *HueForgeOptions) { h.MaxRuns = 65 }, func(h *HueForgeOptions) { h.MeshMode = "other" }, func(h *HueForgeOptions) { h.MeshCore = "other" }, func(h *HueForgeOptions) { h.ExportWidthMM = -1 }} {
		h := DefaultOptions().HueForge
		edit(&h)
		if h.Validate() == nil {
			t.Fatal("invalid HFP option accepted")
		}
	}
}

func TestHFPDepthRetainsFinalLayer(t *testing.T) {
	for _, step := range []float64{.02, .04, .08, .1, .12, .16, .2} {
		for _, first := range []float64{.1, .16, .2, .24} {
			for _, layers := range []int{2, 5, 11, 25, 27, 100, 998} {
				h := HueForgeOptions{LayerHeight: step, FirstLayerHeight: first}
				limit := float32(hfpMaxDepth(h, layers))
				count := 1
				for z := float32(first) + float32(step); z < limit; z += float32(step) {
					count++
				}
				if count < layers || count > layers+1 {
					t.Fatalf("first=%g step=%g layers=%d imported=%d", first, step, layers, count)
				}
			}
		}
	}
	h := DefaultOptions().HueForge
	base := h.BaseLayers()
	r := &Result{Image: image.NewNRGBA(image.Rect(0, 0, 1, 1)), LayerMap: []uint16{uint16(base)}, Stack: &StackPlan{Options: h, Runs: []StackRun{{EndLayer: base}}}}
	r.Image.SetNRGBA(0, 0, color.NRGBA{A: 255})
	doc, err := hueForgeProject(context.Background(), r, "solid.png", ImageMetadata{})
	if err != nil || doc["max_depth"].(float64) <= h.Height(base+1) {
		t.Fatal("base-only mesh needs a nonzero Color Match depth interval", err)
	}
}
