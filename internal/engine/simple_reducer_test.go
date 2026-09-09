package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestTotalColorBudgetIncludesNeutrals(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 80, 16))
	colors := []RGB{{10, 10, 10}, {90, 90, 90}, {160, 160, 160}, {250, 250, 250}, {255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 230, 0}}
	for y := 0; y < 16; y++ {
		for x := 0; x < 80; x++ {
			c := colors[x/10]
			src.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], uint8(y * 17)})
		}
	}
	for _, preserve := range []bool{true, false} {
		for _, budget := range []int{1, 2, 4, 8} {
			o := testOptions()
			o.TotalColors = true
			o.Colors = budget
			o.PreserveDetails = preserve
			a := process(t, src, o, nil)
			b := process(t, src, o, nil)
			if a.UniqueColors > budget || len(a.Palette) > budget {
				t.Fatal("exceeded total budget", budget, a.UniqueColors)
			}
			if !bytes.Equal(a.Image.Pix, b.Image.Pix) {
				t.Fatal("nondeterministic reduction")
			}
			for i := 3; i < len(src.Pix); i += 4 {
				if a.Image.Pix[i] != src.Pix[i] {
					t.Fatal("alpha changed")
				}
			}
		}
	}
	o := testOptions()
	o.Colors = 2
	if r := process(t, src, o, nil); r.UniqueColors <= 2 {
		t.Fatal("legacy split budget was changed")
	}
}

func TestStrongerSmoothingFlattensTextureAndPreservesInk(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(118)
			if (x+y)%2 == 0 {
				v = 138
			}
			if x == 32 {
				v = 10
			}
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	gentle, err := detailSmoothWithTolerance(context.Background(), src, 2.5, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	strong, err := detailSmoothWithTolerance(context.Background(), src, 2.5, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, after := 0., 0.
	for y := 5; y < 35; y++ {
		for x := 5; x < 59; x++ {
			if x == 32 {
				if strong.NRGBAAt(x, y).R != 10 {
					t.Fatal("one-pixel ink line softened")
				}
				continue
			}
			before += math.Pow(float64(gentle.NRGBAAt(x, y).R)-128, 2)
			after += math.Pow(float64(strong.NRGBAAt(x, y).R)-128, 2)
		}
	}
	if before == 0 || after >= before*.6 {
		t.Fatalf("texture not improved enough: %g -> %g", before, after)
	}
	t.Logf("Texture squared error: %.0f -> %.0f; one-pixel ink line retained", before, after)
	old, _ := detailSmooth(context.Background(), src, 1.5, nil)
	compatible, _ := detailSmoothWithTolerance(context.Background(), src, 1.5, 0, nil)
	if !bytes.Equal(old.Pix, compatible.Pix) {
		t.Fatal("legacy smoothing changed")
	}
	// Exercise the public processing path, not just the filter helper.
	o := testOptions()
	o.TotalColors = true
	o.PreblurSigma = 2.5
	o.SmoothingColorSigma = 10
	wantPalette, _, err := discoverPrepared(context.Background(), strong, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, _, _, err := mapImage(context.Background(), src, strong, wantPalette, wantPalette, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := process(t, src, o, nil); !bytes.Equal(got.Image.Pix, want.Pix) {
		t.Fatal("processing ignored smoothing tolerance")
	}
}

func TestNewOptionsRoundTripAndLegacyDefaults(t *testing.T) {
	o := testOptions()
	o.TotalColors = true
	o.SmoothingColorSigma = 10
	o.LegacyColorPipeline = true
	b, _ := json.Marshal(o)
	var got Options
	if err := json.Unmarshal(b, &got); err != nil || got != o {
		t.Fatal("options did not round trip", err)
	}
	var legacy Options
	if err := json.Unmarshal([]byte(`{"colors":8}`), &legacy); err != nil || legacy.TotalColors || legacy.SmoothingColorSigma != 0 || legacy.LegacyColorPipeline {
		t.Fatal("legacy settings changed")
	}
	for _, v := range []float64{-1, 26, math.NaN(), math.Inf(1)} {
		o.SmoothingColorSigma = v
		if o.Validate() == nil {
			t.Fatal("accepted bad tolerance", v)
		}
	}
}

// Texture in a flat shape should not be assigned alternating palette colors
// just because detail-aware clustering is disabled. The surrounding swatches
// provide valid gray levels; an ink line must remain sharp in either setting.
func TestSmoothingIndependentOfDetailPreservation(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 96, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 96; x++ {
			v := uint8(118 + 10*(x/32))
			if x >= 40 && x < 56 && y >= 16 && y < 48 {
				v = uint8(118 + 20*((x+y)%2))
			}
			if x == 70 {
				v = 10
			}
			a := uint8(255)
			if y == 63 {
				a = uint8(x * 2)
			}
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, a})
		}
	}
	transitions := func(img *image.NRGBA) int {
		n := 0
		for y := 20; y < 44; y++ {
			for x := 44; x < 51; x++ {
				if img.NRGBAAt(x, y) != img.NRGBAAt(x+1, y) {
					n++
				}
			}
		}
		return n
	}
	for _, preserve := range []bool{false, true} {
		t.Run(fmt.Sprintf("preserve=%v", preserve), func(t *testing.T) {
			o := testOptions()
			o.AnalysisMaxPixels = 0
			o.HistogramBits = 7
			o.TotalColors = true
			o.Colors = 4
			o.PreserveDetails = preserve
			off := process(t, src, o, nil)
			o.PreblurSigma, o.SmoothingColorSigma = 2.5, 10
			strong := process(t, src, o, nil)
			before, after := transitions(off.Image), transitions(strong.Image)
			if before == 0 || after >= before/2 {
				t.Fatalf("texture transitions did not fall: %d -> %d", before, after)
			}
			t.Logf("texture transitions: %d -> %d", before, after)
			for y := 0; y < 63; y++ {
				if strong.Image.NRGBAAt(70, y).R != 10 {
					t.Fatal("ink line lost")
				}
			}
			palette := make(map[RGB]bool)
			for _, p := range strong.Palette {
				palette[p.RGB] = true
			}
			if strong.UniqueColors > o.Colors {
				t.Fatal("exceeded color budget")
			}
			squaredError, weight := 0., 0.
			for i := 0; i < len(src.Pix); i += 4 {
				if strong.Image.Pix[i+3] != src.Pix[i+3] {
					t.Fatal("alpha changed")
				}
				if src.Pix[i+3] > 0 && !palette[RGB{strong.Image.Pix[i], strong.Image.Pix[i+1], strong.Image.Pix[i+2]}] {
					t.Fatal("off-palette color")
				}
				mass := float64(src.Pix[i+3]) / 255
				a := ToLab(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]})
				b := ToLab(RGB{strong.Image.Pix[i], strong.Image.Pix[i+1], strong.Image.Pix[i+2]})
				squaredError += mass * distance(a, b)
				weight += mass
			}
			if math.Abs(strong.Quality.RMS-math.Sqrt(squaredError/weight)) > 1e-9 {
				t.Fatal("quality must measure original pixels in CIELAB, independently of preservation")
			}
			// The discovery-only API must use the same smoothing as Process.
			discovered, _, err := Discover(context.Background(), src, o, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(discovered) != len(strong.Palette) {
				t.Fatal("discovery and processing disagree")
			}
			for _, p := range discovered {
				if !palette[p.RGB] {
					t.Fatal("discovery and processing disagree")
				}
			}
		})
	}
}

func TestDetailToggleDoesNotChangeColorMatchingOrSmoothing(t *testing.T) {
	o := testOptions()
	o.PreblurSigma, o.SmoothingColorSigma = 2.5, 10
	// With no area culling or constrained palette, this switch has no detail
	// decisions to change. It must not silently select a different color space.
	o.MinClusterFraction = 0
	src := gradient(96, 64)
	for _, legacy := range []bool{false, true} {
		o.LegacyColorPipeline = legacy
		o.PreserveDetails = true
		a := process(t, src, o, nil)
		o.PreserveDetails = false
		b := process(t, src, o, nil)
		if !bytes.Equal(a.Image.Pix, b.Image.Pix) {
			t.Fatalf("detail toggle changed color matching or smoothing (legacy=%v)", legacy)
		}
	}
}
