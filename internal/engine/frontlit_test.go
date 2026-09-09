package engine

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"
)

func TestFrontlitReference(t *testing.T) {
	var corpus struct {
		Cases []struct {
			Name                          string
			Filaments                     []Filament
			Runs                          []int
			LayerHeight, FirstLayerHeight float64
			Colors                        [][4]float32
			LightRGB                      [3]float32
		}
	}
	raw, err := os.ReadFile("testdata/frontlit-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 246 {
		t.Fatalf("reference corpus incomplete: %d cases", len(corpus.Cases))
	}
	maxError := 0.0
	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			s := newFrontlit(c.LightRGB)
			layer := 1
			for run, f := range c.Filaments {
				for n := 0; n < c.Runs[run]; n++ {
					height := c.LayerHeight
					if layer == 1 {
						height = c.FirstLayerHeight
					}
					s.step(f, height, n == 0)
					for k, v := range s.rgb() {
						d := math.Abs(float64(v - c.Colors[layer][k]))
						maxError = max(maxError, d)
						if d > 2e-6 {
							t.Fatalf("layer %d channel %d got %.9g reference %.9g error %.9g", layer, k, v, c.Colors[layer][k], d)
						}
					}
					layer++
				}
			}
			// Independently validate the planner's cumulative state and its base
			// handling against the same reference table, not a second Go simulation.
			h := DefaultOptions().HueForge
			h.LayerHeight, h.FirstLayerHeight = c.LayerHeight, c.FirstLayerHeight
			for _, p := range []string{"neutral-white", "hueforge-default", "warm-white"} {
				if strings.HasSuffix(c.Name, p) {
					h.LightPreset = p
				}
			}
			h.BaseDepth = h.Height(c.Runs[0])
			h.MaxDepth = h.Height(layer - 1)
			ids := make([]int, len(c.Filaments))
			for i := range ids {
				ids[i] = i
			}
			state := rebuildStack(ids, c.Runs, Library{Filaments: c.Filaments}, h)
			for i, rgb := range state.rgbs {
				for k, v := range rgb {
					want := byteRound(float64(c.Colors[state.layers[i]][k]) * 255)
					if v != want {
						t.Fatalf("rebuild layer %d channel %d: %d != reference %d", state.layers[i], k, v, want)
					}
				}
			}
		})
	}
	t.Logf("%d reference cases; maximum normalized RGB error %.9g", len(corpus.Cases), maxError)
}

func TestFrontlitFirstLayerAndLegacyJSON(t *testing.T) {
	h := DefaultOptions().HueForge
	if h.BaseLayers() != 5 || h.MaxLayers() != 27 || h.Height(0) != 0 || math.Abs(h.Height(5)-.48) > 1e-12 {
		t.Fatal(h)
	}
	for _, bad := range []float64{.15, .17} {
		trial := h
		trial.BaseDepth = bad
		if trial.Validate() == nil {
			t.Fatal("accepted impossible depth", bad)
		}
	}
	for _, initialized := range []bool{false, true} {
		var o Options
		if initialized {
			o = DefaultOptions()
		}
		if err := json.Unmarshal([]byte(`{"hueforge":{"layerHeight":0.08,"baseDepth":0.48,"maxDepth":2.24}}`), &o); err != nil {
			t.Fatal(err)
		}
		if o.HueForge.frontlit() || o.HueForge.BaseLayers() != 6 || o.HueForge.FirstHeight() != .08 {
			t.Fatal("legacy geometry changed", o.HueForge)
		}
	}
}

func TestFrontlitSingleFilamentCanGrowBeyondBase(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.Colors = 1
	o.PreblurSigma = 0
	lib := Library{Filaments: []Filament{{RGB: RGB{255, 255, 255}, TD: 7.5}}}
	r := process(t, solid(RGB{255, 255, 255}), o, &lib)
	if len(r.Stack.Runs) != 1 || r.Palette[0].RGB != (RGB{255, 255, 255}) || r.Stack.PlannedDepth <= o.HueForge.BaseDepth {
		t.Fatal(r.Stack)
	}
	if math.Abs(r.Stack.Runs[0].EndHeight-o.HueForge.Height(r.Stack.Runs[0].EndLayer)) > 1e-12 {
		t.Fatal("bad first layer geometry")
	}
}

func TestCoherentRareDetailSurvivesButSpecklesCull(t *testing.T) {
	for _, coherent := range []bool{true, false} {
		img := image.NewNRGBA(image.Rect(0, 0, 1000, 1000))
		for i := 0; i < len(img.Pix); i += 4 {
			copy(img.Pix[i:i+4], []byte{240, 240, 240, 255})
		}
		for i := 0; i < 500; i++ {
			x, y := 500, i+250
			if !coherent {
				x = (i % 25) * 40
				y = (i / 25) * 40
			}
			img.SetNRGBA(x, y, color.NRGBA{10, 10, 10, 255})
		}
		o := DefaultOptions()
		o.Colors = 8
		o.TotalColors = true
		o.PreblurSigma = 0
		r := process(t, img, o, nil)
		if coherent && (r.UniqueColors != 2 || r.Image.NRGBAAt(500, 500).R != 10 || r.Quality.Max > 1e-9) {
			t.Fatal("erased coherent line", r.UniqueColors, r.Quality)
		}
		if !coherent && r.UniqueColors != 1 {
			t.Fatal("preserved isolated noise", r.UniqueColors)
		}
	}
}

func TestFrontlitGuideScoresFinalStrengthAndCap(t *testing.T) {
	o := testOptions()
	o.Mode = "guided"
	o.Colors = 2
	o.HueForge.MaxPerceivedColors = 2
	lib := testLibrary()
	palette := []PaletteEntry{entry(RGB{120, 70, 180}, .6, 8), entry(RGB{30, 30, 30}, .3, 8), entry(RGB{190, 190, 230}, .1, 8)}
	target, weights := targets(palette, o)
	for _, strength := range []float64{0, .35, 1} {
		o.GuidanceStrength = strength
		out, _, err := guide(context.Background(), palette, lib, o, nil)
		if err != nil {
			t.Fatal(err)
		}
		actual := 0.0
		for i, p := range target {
			best := math.Inf(1)
			for _, c := range out {
				best = math.Min(best, distance(p, o.colorVector(c.RGB)))
			}
			actual += weights[i] * best
		}
		oracle := math.Inf(1)
		for i := range lib.Filaments {
			for j := i; j < len(lib.Filaments); j++ {
				ids := []int{i}
				if j != i {
					ids = append(ids, j)
				}
				c, err := guidanceCandidates(context.Background(), ids, lib, o.HueForge)
				if err != nil {
					t.Fatal(err)
				}
				score, err := candidateScore(context.Background(), c, target, weights, o)
				if err != nil {
					t.Fatal(err)
				}
				oracle = math.Min(oracle, score)
			}
		}
		if math.Abs(actual-oracle) > 1e-8 {
			t.Fatalf("strength %.2f: selected error %g vs exhaustive pair optimum %g", strength, actual, oracle)
		}
	}
}
