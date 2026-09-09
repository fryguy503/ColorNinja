package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestOKLabReferenceAndRoundTrip(t *testing.T) {
	// Independent reference coordinates for sRGB primaries, in Oklab x 100.
	for _, tt := range []struct {
		c RGB
		v Vec
	}{
		{RGB{}, Vec{}}, {RGB{255, 255, 255}, Vec{100, 0, 0}},
		{RGB{255, 0, 0}, Vec{62.795536, 22.486306, 12.584630}},
		{RGB{0, 255, 0}, Vec{86.643961, -23.388757, 17.949848}},
		{RGB{0, 0, 255}, Vec{45.201372, -3.245698, -31.152815}},
	} {
		if d := math.Sqrt(distance(toOKLab(tt.c), tt.v)); d > .0001 {
			t.Fatalf("%v: error %g", tt.c, d)
		}
	}
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 17 {
				c := RGB{uint8(r), uint8(g), uint8(b)}
				if got := fromOKLab(toOKLab(c)); got != c {
					t.Fatalf("%v -> %v", c, got)
				}
			}
		}
	}
}

func TestDetailSmoothingFlattensNoiseWithoutCrossingEdges(t *testing.T) {
	for _, pair := range [][2]RGB{
		{{25, 45, 90}, {35, 100, 150}}, {{90, 25, 30}, {160, 50, 60}},
		{{20, 70, 35}, {45, 140, 65}}, {{35, 35, 35}, {150, 150, 150}},
	} {
		src := image.NewNRGBA(image.Rect(0, 0, 80, 48))
		for y := 0; y < 48; y++ {
			for x := 0; x < 80; x++ {
				c := pair[0]
				if x >= 40 {
					c = pair[1]
				}
				noise := 2
				if (x+y)%2 == 0 {
					noise = -2
				}
				src.SetNRGBA(x, y, color.NRGBA{uint8(int(c[0]) + noise), uint8(int(c[1]) + noise), uint8(int(c[2]) + noise), 255})
			}
		}
		out, err := detailSmooth(context.Background(), src, 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		before, after := 0., 0.
		for y := 4; y < 44; y++ {
			for x := 4; x < 76; x++ {
				want := pair[0]
				if x >= 40 {
					want = pair[1]
				}
				a, b := src.NRGBAAt(x, y), out.NRGBAAt(x, y)
				for c, v := range []uint8{a.R, a.G, a.B} {
					before += math.Pow(float64(v)-float64(want[c]), 2)
				}
				for c, v := range []uint8{b.R, b.G, b.B} {
					after += math.Pow(float64(v)-float64(want[c]), 2)
				}
			}
		}
		if after > before*.2 {
			t.Fatalf("%v: residual noise %.1f / %.1f", pair, after, before)
		}
		for _, x := range []int{39, 40} {
			got := out.NRGBAAt(x, 24)
			want := pair[x/40]
			if math.Sqrt(distance(ToLab(RGB{got.R, got.G, got.B}), ToLab(want))) > 1 {
				t.Fatal("edge bled", pair, x, got)
			}
		}
	}
}

func TestOnePixelLinesSurviveAllModes(t *testing.T) {
	for _, ink := range []RGB{{15, 45, 85}, {85, 20, 25}, {20, 70, 30}, {35, 35, 35}} {
		for _, mode := range []string{"standard", "guided", "stack"} {
			t.Run(fmt.Sprint(ink)+"/"+mode, func(t *testing.T) {
				paper := RGB{245, 245, 245}
				src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
				for y := 0; y < 64; y++ {
					for x := 0; x < 64; x++ {
						c := paper
						if x == 21 || y == 42 || x == y {
							c = ink
						}
						src.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], 255})
					}
				}
				lib := Library{Filaments: []Filament{{Name: "Ink", RGB: ink, TD: .3, Owned: true}, {Name: "Paper", RGB: paper, TD: .3, Owned: true}}}
				o := DefaultOptions()
				o.Mode = mode
				o.Colors = 2
				o.HueForge.BeamWidth = 4
				o.HueForge.MaxDepth = .8
				r := process(t, src, o, &lib)
				line, background := r.Image.NRGBAAt(21, 10), r.Image.NRGBAAt(30, 10)
				if line == background {
					t.Fatal("thin line lost")
				}
				contrast := math.Sqrt(distance(ToLab(RGB{line.R, line.G, line.B}), ToLab(RGB{background.R, background.G, background.B})))
				if contrast < .9*math.Sqrt(distance(ToLab(ink), ToLab(paper))) {
					t.Fatal("line survived but its contrast was washed out", contrast)
				}
				for y := 0; y < 64; y++ {
					for x := 0; x < 64; x++ {
						want := background
						if x == 21 || y == 42 || x == y {
							want = line
						}
						if got := r.Image.NRGBAAt(x, y); got != want {
							t.Fatalf("outline displaced/fragmented at %d,%d: %v != %v", x, y, got, want)
						}
					}
				}
				if r.UniqueColors != 2 {
					t.Fatal("extra colors introduced", r.UniqueColors)
				}
			})
		}
	}
}

func TestConstrainedMappingKeepsSourceGroupsTogether(t *testing.T) {
	// A weak gray gradient crosses a remote palette's decision boundary. It
	// belongs to one analyzed color group, so mapping must not invent a contour.
	src := image.NewNRGBA(image.Rect(0, 0, 32, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 32; x++ {
			v := uint8(110 + x/4)
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	palette := []PaletteEntry{entry(RGB{60, 60, 60}, .5, 8), entry(RGB{170, 170, 170}, .5, 8)}
	sourceGroups := []PaletteEntry{entry(RGB{110, 110, 110}, 1, 8)}
	o := DefaultOptions()
	o.Mode = "guided"
	out, q, _, err := mapImage(context.Background(), src, src, palette, sourceGroups, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := CountUniqueColors(context.Background(), out); n != 1 {
		t.Fatal("invented contour inside source group", n)
	}
	legacy := o
	legacy.PreserveDetails = false
	old, _, _, err := mapImage(context.Background(), src, src, palette, sourceGroups, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := CountUniqueColors(context.Background(), old); n != 2 {
		t.Fatal("fixture no longer exercises the old artificial contour", n)
	}
	// Report metrics remain measured against original pixels in CIELAB.
	sum := 0.
	for y := 0; y < 12; y++ {
		for x := 0; x < 32; x++ {
			a, b := src.NRGBAAt(x, y), out.NRGBAAt(x, y)
			sum += distance(ToLab(RGB{a.R, a.G, a.B}), ToLab(RGB{b.R, b.G, b.B}))
		}
	}
	if math.Abs(q.RMS-math.Sqrt(sum/(32*12))) > 1e-9 {
		t.Fatal("wrong metric units", q)
	}
}

func TestCompleteStackRefinementRestoresMissingShades(t *testing.T) {
	for _, ink := range []RGB{{180, 30, 45}, {25, 145, 60}, {25, 60, 180}} {
		o := DefaultOptions()
		o.HueForge.OpticalModel, o.HueForge.FirstLayerHeight = LegacyModel, 0
		o.Colors = 3
		o.Mode = "stack"
		o.HueForge.MaxDepth = 1.12
		lib := Library{Filaments: []Filament{{RGB: RGB{}, TD: 1}, {RGB: ink, TD: 2}, {RGB: RGB{245, 245, 245}, TD: 5}}}
		ideal := rebuildStack([]int{0, 1, 2}, []int{6, 5, 3}, lib, o.HueForge)
		colors, _, _ := uniqueStack(ideal)
		palette := make([]PaletteEntry, len(colors))
		for i, c := range colors {
			palette[i] = entry(c, 1/float64(len(colors)), 8)
		}
		target, weights := targets(palette, o)
		initial := rebuildStack([]int{0, 1, 2}, []int{6, 1, 7}, lib, o.HueForge)
		initial.score, _ = stateScore(context.Background(), initial, target, weights, o)
		best, err := refineStack(context.Background(), initial, lib, []int{0, 1, 2}, target, weights, o, nil)
		if err != nil {
			t.Fatal(err)
		}
		if best.score >= initial.score*.1 {
			t.Fatalf("missing shades were not recovered for %v: %.3f -> %.3f", ink, initial.score, best.score)
		}
		if best.runs[0] != 6 || best.used != 8 || len(best.indices) > o.Colors {
			t.Fatal("changed base, depth, or budget")
		}
		// Every output RGB must still describe the exact layer in one global stack.
		for i, c := range best.rgbs {
			if c != ideal.rgbs[i] || best.layers[i] != ideal.layers[i] {
				t.Fatal("failed to recover reachable shade", i, c)
			}
		}
	}
}

func TestSmoothedStackPaletteAndLayerMapAgree(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		t.Run(fmt.Sprintf("preserve=%v", preserve), func(t *testing.T) {
			o := testOptions()
			o.Mode = "stack"
			o.PreblurSigma, o.SmoothingColorSigma = 2.5, 10
			o.PreserveDetails = preserve
			o.HueForge.MaxPerceivedColors = 3
			lib := testLibrary()
			src := gradient(81, 63)
			r := process(t, src, o, &lib)
			if len(r.Palette) > 3 || len(r.Stack.Runs) > o.Colors {
				t.Fatal("exceeded requested budget")
			}
			layers := map[int]RGB{}
			for _, p := range r.Stack.LayerColors {
				layers[p.Layer] = p.RGB
			}
			for y := 0; y < 63; y++ {
				for x := 0; x < 81; x++ {
					i := y*81 + x
					pixel := r.Image.NRGBAAt(x, y)
					if pixel.A != src.NRGBAAt(x, y).A {
						t.Fatal("changed alpha")
					}
					if pixel.A == 0 {
						if r.LayerMap[i] != 0 {
							t.Fatal("transparent pixel has a layer")
						}
						continue
					}
					if c, ok := layers[int(r.LayerMap[i])]; !ok || c != (RGB{pixel.R, pixel.G, pixel.B}) {
						t.Fatal("pixel is not reachable at reported layer", i)
					}
				}
			}
		})
	}
}

func TestDetailSmoothingAlphaStrideTilesAndCancellation(t *testing.T) {
	// Cross both scanline tile boundaries using a nonzero-origin subimage.
	for _, vertical := range []bool{false, true} {
		w, h := 8204, 5
		if vertical {
			w, h = h, w
		}
		parent := image.NewNRGBA(image.Rect(0, 0, w+4, h+4))
		src := parent.SubImage(image.Rect(2, 2, w+2, h+2)).(*image.NRGBA)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*src.Stride + x*4
				copy(src.Pix[i:i+4], []byte{80, 100, 120, 128})
				if (x+y)%7 == 0 {
					copy(src.Pix[i:i+4], []byte{255, 0, 255, 0})
				}
			}
		}
		before := append([]byte(nil), src.Pix...)
		out, err := detailSmooth(context.Background(), src, 1.5, nil)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				a := src.Pix[y*src.Stride+x*4+3]
				b := out.NRGBAAt(x, y)
				if b.A != a || (a != 0 && (RGB{b.R, b.G, b.B}) != (RGB{80, 100, 120})) {
					t.Fatal("hidden RGB, stride, tile, or alpha changed", x, y, b)
				}
			}
		}
		if !bytes.Equal(before, src.Pix) {
			t.Fatal("mutated original")
		}
	}
	src := gradient(100, 100)
	if out, err := detailSmooth(context.Background(), src, 0, nil); err != nil || out != src {
		t.Fatal("zero smoothing changed input")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := detailSmooth(ctx, src, 100, func(p Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatal("smoothing not cancellable", err)
	}
}

func TestPreserveDetailsOldSettingsAndExplicitOptOut(t *testing.T) {
	if !DefaultOptions().PreserveDetails {
		t.Fatal("must default on")
	}
	raw, _ := json.Marshal(DefaultOptions())
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "preserveDetails")
	raw, _ = json.Marshal(fields)
	var restored Options
	if err := json.Unmarshal(raw, &restored); err != nil || !restored.PreserveDetails {
		t.Fatal("legacy settings missing default", err)
	}
	restored.PreserveDetails = false
	raw, _ = json.Marshal(restored)
	var explicit Options
	if err := json.Unmarshal(raw, &explicit); err != nil || explicit.PreserveDetails {
		t.Fatal("opt out lost", err)
	}
	if err := json.Unmarshal([]byte(`{"colors":12}`), &explicit); err != nil || explicit.PreserveDetails {
		t.Fatal("partial CLI override lost opt out", err)
	}
}
