package engine

import (
	"context"
	"math"
	"testing"
)

func TestStackViewAgreesWithHFPAndPixelCoverage(t *testing.T) {
	for _, core := range []string{"legacy-flat", "filament-blends", "", "planned-colors", "compact-blends"} {
		t.Run(core, func(t *testing.T) {
			r := hfpFixture(t)
			r.Stack.Options.MeshCore = core
			// This fixture sets its layer map explicitly; calculate the actual
			// visible pixel fractions that Process normally supplies.
			counts, visible := map[int]int{}, 0
			for i, layer := range r.LayerMap {
				if r.Image.Pix[i*4+3] > 0 {
					counts[int(layer)]++
					visible++
				}
			}
			for i := range r.Palette {
				r.Palette[i].PixelFraction = float64(counts[r.Palette[i].StackLayer]) / float64(visible)
			}
			view := buildStackCoreView(r)
			doc, err := hueForgeProject(context.Background(), r, "source.png", ImageMetadata{})
			if err != nil {
				t.Fatal(err)
			}
			disabled := map[int]bool{}
			for _, layer := range doc["disabled_match_layers"].([]int) {
				disabled[layer] = true
			}
			mesh := doc["match_filament_set"].([]hfpFilament)
			ends := doc["match_slider_values"].([]int)
			if view.MeshCore != doc["colorninja"].(map[string]any)["meshCore"] {
				t.Fatal("view and export resolved different mesh strategies")
			}
			forward := append([]hfpFilament(nil), mesh...)
			for i, j := 0, len(forward)-1; i < j; i, j = i+1, j-1 {
				forward[i], forward[j] = forward[j], forward[i]
			}
			colors := meshSimulate(forward, ends, r.Stack.Options)
			sum := 0.
			for _, row := range view.Layers {
				if row.MeshEnabled != (row.Layer >= r.Stack.Options.BaseLayers() && !disabled[row.Layer]) {
					t.Fatal("mesh enable state differs from HFP", row)
				}
				if row.Height != r.Stack.Options.Height(row.Layer) {
					t.Fatal("incorrect first-layer geometry")
				}
				if core == "legacy-flat" {
					index := 0
					for index+1 < len(ends) && row.Layer > ends[index] {
						index++
					}
					if row.MeshRGB == nil || row.MeshRGB.Hex() != mesh[len(mesh)-1-index].Color {
						t.Fatal("visual core differs from HFP", row)
					}
				} else if core == "filament-blends" && (row.MeshRGB == nil || *row.MeshRGB != row.PredictedRGB) {
					t.Fatal("blend core changed layer colors")
				} else if view.MeshCore == "compact-blends" {
					var rgb RGB
					for i, v := range colors[row.Layer] {
						rgb[i] = byteRound(float64(v) * 255)
					}
					if row.MeshRGB == nil || *row.MeshRGB != rgb || view.Optimization == nil {
						t.Fatal("tuned core display differs from exported TD blends", row)
					}
				}
				sum += row.PixelFraction
			}
			if math.Abs(sum-1) > 1e-9 {
				t.Fatal("coverage is not based on actual visible pixels", sum)
			}
			r.Stack.Options.MeshMode = "combo"
			if buildStackCoreView(r).HasMeshCore {
				t.Fatal("invented a mesh core for Combo mode")
			}
		})
	}
}
