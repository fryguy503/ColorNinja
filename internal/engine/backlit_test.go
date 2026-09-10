package engine

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestBacklitReference(t *testing.T) {
	var corpus struct {
		Cases []struct {
			Name, LightPreset                      string
			Filaments                              []Filament
			Runs                                   []int
			LayerHeight, FirstLayerHeight, TDScale float64
			Colors                                 [][4]float32
		}
	}
	raw, err := os.ReadFile("testdata/backlit-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 300 {
		t.Fatal("incomplete reference corpus")
	}
	maxError := 0.
	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			h := DefaultOptions().HueForge
			h.OpticalModel = BacklitModel
			h.LightPreset = c.LightPreset
			h.TDScale = c.TDScale
			s := newBacklit(h)
			layer := 1
			for run, f := range c.Filaments {
				for n := 0; n < c.Runs[run]; n++ {
					height := c.LayerHeight
					if layer == 1 {
						height = c.FirstLayerHeight
					}
					s.step(f, height, h)
					for k, v := range s.color {
						d := math.Abs(float64(v - c.Colors[layer][k]))
						maxError = max(maxError, d)
						if d > 3e-6 {
							t.Fatalf("layer %d channel %d got %.9g want %.9g error %.9g", layer, k, v, c.Colors[layer][k], d)
						}
					}
					layer++
				}
			}
			h.LayerHeight, h.FirstLayerHeight = c.LayerHeight, c.FirstLayerHeight
			h.BaseDepth = h.Height(c.Runs[0])
			h.MaxDepth = h.Height(layer - 1)
			ids := make([]int, len(c.Filaments))
			for i := range ids {
				ids[i] = i
			}
			stack := rebuildStack(ids, c.Runs, Library{Filaments: c.Filaments}, h)
			for i, rgb := range stack.rgbs {
				for k, v := range rgb {
					if v != byteRound(float64(c.Colors[stack.layers[i]][k])*255) {
						t.Fatalf("rebuilt layer %d RGB differs from native", stack.layers[i])
					}
				}
			}
		})
	}
	t.Logf("maximum native Backlit channel error: %.9g", maxError)
}
