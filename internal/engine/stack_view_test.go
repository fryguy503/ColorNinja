package engine

import (
	"context"
	"math"
	"testing"
)

func TestStackViewAgreesWithHFPAndPixelCoverage(t *testing.T) {
	for _, core := range []string{"planned-colors", "filament-blends"} {
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
			sum := 0.
			for _, row := range view.Layers {
				if row.MeshEnabled == disabled[row.Layer] {
					t.Fatal("mesh enable state differs from HFP", row)
				}
				if row.Height != r.Stack.Options.Height(row.Layer) {
					t.Fatal("incorrect first-layer geometry")
				}
				if core == "planned-colors" {
					index := 0
					for index+1 < len(ends) && row.Layer > ends[index] {
						index++
					}
					if row.MeshRGB == nil || row.MeshRGB.Hex() != mesh[len(mesh)-1-index].Color {
						t.Fatal("visual core differs from HFP", row)
					}
				} else if row.MeshRGB == nil || *row.MeshRGB != row.PredictedRGB {
					t.Fatal("blend core changed layer colors")
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
