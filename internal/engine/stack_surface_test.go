package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"reflect"
	"strings"
	"testing"
)

func surfaceFixture() (Options, Library, *image.NRGBA, []PaletteEntry) {
	o := DefaultOptions()
	o.Mode, o.Colors, o.PreblurSigma = "stack", 3, 0
	o.HueForge.BaseDepth, o.HueForge.MaxDepth = .16, .88
	o.HueForge.BeamWidth, o.HueForge.AnalysisColors = 64, 3
	lib := Library{Filaments: []Filament{
		{Name: "Black", RGB: RGB{}, TD: .01, SourceIndex: 0},
		{Name: "Green", RGB: RGB{0, 255, 0}, TD: .01, SourceIndex: 1},
		{Name: "Magenta", RGB: RGB{255, 0, 255}, TD: .01, SourceIndex: 2},
	}}
	img := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			id := (x / 4 % 2) * 2
			if x >= 56 {
				id = 1
			}
			c := lib.Filaments[id].RGB
			img.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], 255})
		}
	}
	palette := []PaletteEntry{entry(RGB{}, .4375, 8), entry(RGB{0, 255, 0}, .125, 8), entry(RGB{255, 0, 255}, .4375, 8)}
	return o, lib, img, palette
}

func TestSurfacePlanningMovesNeighboringColorsTogether(t *testing.T) {
	o, lib, img, palette := surfaceFixture()
	ctx := context.Background()
	o.HueForge.ReduceShowThrough = true
	edges, err := stackBoundaries(ctx, img, palette, o)
	if err != nil || len(edges) != 2 {
		t.Fatalf("missing image boundaries: %+v %v", edges, err)
	}
	target, weights := targets(palette, o)
	for _, automatic := range []bool{false, true} {
		for _, preserve := range []bool{false, true} {
			o.HueForge.AutoDepth, o.PreserveDetails = automatic, preserve
			basePalette, basePlan, err := planStack(ctx, palette, lib, o, nil)
			if err != nil {
				t.Fatal(err)
			}
			newPalette, plan, err := planStack(ctx, palette, lib, o, nil, edges...)
			if err != nil {
				t.Fatal(err)
			}
			state := stackState{}
			for _, layer := range basePlan.LayerColors {
				state.rgbs, state.layers = append(state.rgbs, layer.RGB), append(state.layers, layer.Layer)
			}
			colors, layers := []RGB{}, []int{}
			for _, p := range basePalette {
				colors, layers = append(colors, p.RGB), append(layers, p.StackLayer)
			}
			before := stackSurface(state, colors, layers, target, o, edges)
			after := plan.Surface
			if after == nil || after.MeanHeightJumpMM >= before.MeanHeightJumpMM || after.Penalty >= before.Penalty || after.RMSColorDetour >= before.RMSColorDetour {
				t.Fatalf("boundary exposure did not improve: before=%+v after=%+v", before, after)
			}
			if len(newPalette) != 3 || plan.RMS > 1e-6 || basePlan.RMS > 1e-6 || plan.PlannedDepth > o.HueForge.MaxDepth || plan.UniqueFilaments != 3 {
				t.Fatalf("changed colors or exceeded print budgets: %+v", plan)
			}
			if automatic && !strings.Contains(plan.DepthSelection.ScoreMetric, "boundary penalty") {
				t.Fatal("automatic depth mislabeled its combined score")
			}
			// Independently enumerate every 3-filament order and allocation.
			// The fixture is small enough to verify the combined objective globally.
			best := math.Inf(1)
			for a := range lib.Filaments {
				for b := range lib.Filaments {
					for c := range lib.Filaments {
						if a == b || a == c || b == c {
							continue
						}
						for middle := 1; middle < o.HueForge.TransitionLayers(); middle++ {
							s := rebuildStack([]int{a, b, c}, []int{o.HueForge.BaseLayers(), middle, o.HueForge.TransitionLayers() - middle}, lib, o.HueForge)
							score, e := stateScore(ctx, s, target, weights, o, edges...)
							if e != nil {
								t.Fatal(e)
							}
							best = math.Min(best, score)
						}
					}
				}
			}
			if math.Abs(math.Sqrt(after.Penalty)-best) > 1e-8 {
				t.Fatalf("did not find the enumerated optimum: %g versus %g", math.Sqrt(after.Penalty), best)
			}
			t.Logf("auto=%v preserve=%v: mean boundary step %.4f -> %.4f mm, color RMS %.4f -> %.4f", automatic, preserve, before.MeanHeightJumpMM, after.MeanHeightJumpMM, basePlan.RMS, plan.RMS)
		}
	}
}

func TestSurfaceProcessAndHFPKeepAssignmentsAndAlpha(t *testing.T) {
	o, lib, img, _ := surfaceFixture()
	img.Pix[3], img.Pix[7] = 0, 100
	o.HueForge.ReduceShowThrough = true
	ctx := context.Background()
	for _, preserve := range []bool{false, true} {
		o.PreserveDetails = preserve
		r, err := Process(ctx, img, o, &lib, nil)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Process(ctx, img, o, &lib, nil)
		if err != nil || r.SHA256 != again.SHA256 || !reflect.DeepEqual(r.Stack, again.Stack) {
			t.Fatal("nondeterministic planning", err)
		}
		if r.Stack.Surface == nil || r.Stack.Surface.BoundaryPairs == 0 {
			t.Fatal("processing ignored boundary option")
		}
		colors := map[int]RGB{}
		for _, p := range r.Palette {
			colors[p.StackLayer] = p.RGB
		}
		for i, layer := range r.LayerMap {
			if r.Image.Pix[i*4+3] != img.Pix[i*4+3] {
				t.Fatal("alpha changed")
			}
			if img.Pix[i*4+3] == 0 {
				if layer != 0 {
					t.Fatal("transparent pixel acquired height")
				}
			} else if colors[int(layer)] != (RGB{r.Image.Pix[i*4], r.Image.Pix[i*4+1], r.Image.Pix[i*4+2]}) {
				t.Fatal("image and layer assignment disagree")
			}
		}
		doc, err := hueForgeProject(ctx, r, "detail.png", ImageMetadata{})
		if err != nil {
			t.Fatal(err)
		}
		pngImage, err := png.Decode(bytes.NewReader(doc["image_binary"].([]byte)))
		if err != nil {
			t.Fatal(err)
		}
		mesh, ends := doc["match_filament_set"].([]hfpFilament), doc["match_slider_values"].([]int)
		for i, layer := range r.LayerMap {
			pixel := color.NRGBAModel.Convert(pngImage.At(i%64, i/64)).(color.NRGBA)
			if layer == 0 {
				if pixel.A != 0 {
					t.Fatal("HFP filled an empty pixel")
				}
				continue
			}
			band := 0
			for band < len(ends)-1 && int(layer) > ends[band] {
				band++
			}
			if pixel.A != 255 || (RGB{pixel.R, pixel.G, pixel.B}).Hex() != mesh[len(mesh)-1-band].Color {
				t.Fatal("HFP changed an optimized assignment")
			}
		}
	}
}

func TestSurfaceBoundariesRespectEmptySpaceAndCancellation(t *testing.T) {
	o, _, _, palette := surfaceFixture()
	o.HueForge.ReduceShowThrough = true
	img := image.NewNRGBA(image.Rect(4, 7, 8, 8))
	img.SetNRGBA(4, 7, color.NRGBA{A: 255})
	img.SetNRGBA(6, 7, color.NRGBA{G: 255, A: 100})
	img.SetNRGBA(7, 7, color.NRGBA{R: 255, B: 255, A: 255})
	edges, err := stackBoundaries(context.Background(), img, palette, o)
	if err != nil || len(edges) != 1 || edges[0].a != 1 || edges[0].b != 2 || edges[0].weight != 1 {
		t.Fatalf("connected colors across transparent space: %+v %v", edges, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := stackBoundaries(ctx, img, palette, o); err != context.Canceled {
		t.Fatal("boundary analysis ignored cancellation")
	}
	if colorSegmentDistance(Vec{50, 0, 0}, Vec{}, Vec{100, 0, 0}) != 0 || colorSegmentDistance(Vec{50, 20, 0}, Vec{}, Vec{100, 0, 0}) != 400 {
		t.Fatal("ordinary shade and unrelated hue were not distinguished")
	}
}

func TestSurfaceOptionCompatibility(t *testing.T) {
	o, lib, img, _ := surfaceFixture()
	if o.HueForge.ReduceShowThrough {
		t.Fatal("option must start off")
	}
	o.HueForge.ReduceShowThrough = true
	raw, _ := json.Marshal(o)
	var decoded Options
	if err := json.Unmarshal(raw, &decoded); err != nil || !decoded.HueForge.ReduceShowThrough {
		t.Fatal("setting did not round trip", err)
	}
	if err := json.Unmarshal([]byte(`{"layerHeight":0.08}`), &decoded.HueForge); err != nil || decoded.HueForge.ReduceShowThrough {
		t.Fatal("older settings inherited enabled optimization", err)
	}
	for _, mode := range []string{"standard", "guided"} {
		o.Mode, o.HueForge.ReduceShowThrough = mode, false
		off, err := Process(context.Background(), img, o, &lib, nil)
		if err != nil {
			t.Fatal(err)
		}
		o.HueForge.ReduceShowThrough = true
		on, err := Process(context.Background(), img, o, &lib, nil)
		if err != nil || off.SHA256 != on.SHA256 || !reflect.DeepEqual(off.Palette, on.Palette) {
			t.Fatal("stack preference affected another mode", err)
		}
	}
}
