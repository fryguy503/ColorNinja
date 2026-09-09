package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"testing"
)

func accentFixture() (*image.NRGBA, []RGB) {
	im := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			v := uint8((x / 10) * 255 / 19)
			im.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	accents := []RGB{{240, 30, 40}, {245, 215, 30}, {25, 105, 225}}
	for j, c := range accents {
		for y := 70; y < 78; y++ {
			for x := 70 + j*40; x < 90+j*40; x++ {
				im.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], 255})
			}
		}
	}
	return im, accents
}

func TestColorPriorityPreservesAccentsAndNeutralStructure(t *testing.T) {
	src, accents := accentFixture()
	o := testOptions()
	o.TotalColors = true
	o.Colors = 8
	o.AnalysisMaxPixels = 0
	o.HistogramBits = 7
	o.MinClusterFraction = .005
	o.PreserveDetails = false
	base := process(t, src, o, nil)
	accentError := func(r *Result) float64 {
		sum := 0.
		for j, c := range accents {
			got := r.Image.NRGBAAt(75+j*40, 74)
			sum += distance(toOKLab(c), toOKLab(RGB{got.R, got.G, got.B}))
		}
		return sum
	}
	for _, priority := range []string{"distinctive", "vivid"} {
		t.Run(priority, func(t *testing.T) {
			o.ColorPriority = priority
			r := process(t, src, o, nil)
			before, after := accentError(base), accentError(r)
			t.Logf("accent squared Oklab error: %.2f -> %.2f", before, after)
			if after >= before*.5 {
				t.Fatal("distinct accents did not improve")
			}
			if r.UniqueColors > 8 {
				t.Fatal("color budget exceeded")
			}
			for _, c := range accents {
				found := false
				for _, p := range r.Palette {
					if p.RGB == c {
						found = true
						if math.Abs(p.Fraction-.008) > 1e-9 {
							t.Fatal("importance reported as pixel area", p)
						}
					}
				}
				if !found {
					t.Fatal("fixture accent lost", c)
				}
			}
			for _, x := range []int{0, 40, 100, 199} {
				p := r.Image.NRGBAAt(x, 20)
				if c := toOKLab(RGB{p.R, p.G, p.B}); math.Hypot(c[1], c[2]) > 2 {
					t.Fatal("neutral region gained a color cast", p)
				}
			}
			if r.Image.NRGBAAt(0, 20).R > 35 || r.Image.NRGBAAt(199, 20).R < 220 {
				t.Fatal("light/dark structure lost")
			}
		})
	}
}

func TestPrioritySmallBudgetsAndColorRoundTrip(t *testing.T) {
	src, _ := accentFixture()
	for _, priority := range []string{"distinctive", "vivid"} {
		o := testOptions()
		o.ColorPriority = priority
		for r := 0; r < 256; r += 51 {
			for g := 0; g < 256; g += 51 {
				for b := 0; b < 256; b += 51 {
					c := RGB{uint8(r), uint8(g), uint8(b)}
					if o.colorRGB(o.colorVector(c)) != c {
						t.Fatal("priority added saturation or changed RGB round trip")
					}
				}
			}
		}
		for _, total := range []bool{false, true} {
			for _, budget := range []int{1, 2, 4} {
				o.TotalColors = total
				o.Colors = budget
				r := process(t, src, o, nil)
				cap := budget
				if !total {
					cap *= 2
				}
				if r.UniqueColors > cap || r.UniqueColors < 1 {
					t.Fatal("invalid small palette", priority, cap, r.UniqueColors)
				}
			}
		}
	}
}

func TestPriorityAdaptsToDifferentDominantHueFamilies(t *testing.T) {
	for _, dominant := range []int{0, 1, 2} {
		src, accents := accentFixture()
		for y := 0; y < 60; y++ {
			for x := 0; x < 200; x++ {
				v := uint8(35 + (x/10)*180/19)
				c := RGB{v / 3, v / 3, v / 3}
				c[dominant] = v
				src.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], 255})
			}
		}
		for _, priority := range []string{"distinctive", "vivid"} {
			o := testOptions()
			o.ColorPriority = priority
			o.TotalColors = true
			o.Colors = 8
			o.AnalysisMaxPixels = 0
			o.HistogramBits = 7
			r := process(t, src, o, nil)
			for j, c := range accents {
				p := r.Image.NRGBAAt(75+j*40, 74)
				a, b := toOKLab(c), toOKLab(RGB{p.R, p.G, p.B})
				hueError := math.Abs(math.Atan2(math.Sin(math.Atan2(a[2], a[1])-math.Atan2(b[2], b[1])), math.Cos(math.Atan2(a[2], a[1])-math.Atan2(b[2], b[1])))) * 180 / math.Pi
				// Shared shades may merge under a tight budget; each accent must
				// retain its hue identity and color rather than turn gray or green.
				if hueError > 18 || math.Hypot(b[1], b[2]) < .6*math.Hypot(a[1], a[2]) || math.Abs(a[0]-b[0]) > 20 {
					t.Fatalf("dominant channel %d swallowed accent %v in %s: hue %.2f, output %v", dominant, c, priority, hueError, p)
				}
			}
		}
	}
}

func TestPriorityIgnoresIsolatedNoiseAndHiddenColors(t *testing.T) {
	src, _ := accentFixture()
	for i, p := range [][2]int{{4, 4}, {64, 9}, {125, 40}, {198, 90}} {
		src.SetNRGBA(p[0], p[1], color.NRGBA{250, 0, 245, uint8(255 - i%2*255)})
	}
	o := testOptions()
	o.ColorPriority = "distinctive"
	o.TotalColors = true
	o.Colors = 8
	o.AnalysisMaxPixels = 0
	o.MinClusterFraction = .005
	a := process(t, src, o, nil)
	b := process(t, src, o, nil)
	if !bytes.Equal(a.Image.Pix, b.Image.Pix) {
		t.Fatal("nondeterministic output")
	}
	for _, p := range a.Palette {
		if distance(toOKLab(p.RGB), toOKLab(RGB{250, 0, 245})) < 10*10 {
			t.Fatal("isolated neon noise consumed a palette slot")
		}
	}
	for i := 3; i < len(src.Pix); i += 4 {
		if src.Pix[i] != a.Image.Pix[i] {
			t.Fatal("alpha changed")
		}
	}
}

func TestPriorityBudgetsMetricsAndStackReachability(t *testing.T) {
	for _, priority := range []string{"distinctive", "vivid"} {
		for _, mode := range []string{"standard", "guided", "stack"} {
			for _, preserve := range []bool{false, true} {
				o := testOptions()
				o.ColorPriority = priority
				o.Mode = mode
				o.PreserveDetails = preserve
				o.PreblurSigma = 2.5
				o.SmoothingColorSigma = 10
				o.HueForge.MaxPerceivedColors = 4
				src := gradient(48, 36)
				lib := testLibrary()
				r := process(t, src, o, &lib)
				limit := 4
				if mode == "standard" {
					limit = o.Colors * 2
				}
				if r.UniqueColors > limit {
					t.Fatal("exceeded output limit", mode)
				}
				palette := map[RGB]bool{}
				sum := 0.
				for _, p := range r.Palette {
					palette[p.RGB] = true
					sum += p.Fraction
				}
				if math.Abs(sum-1) > 1e-9 {
					t.Fatal("analysis fractions do not sum to one", sum)
				}
				layers := map[int]RGB{}
				if r.Stack != nil {
					for _, c := range r.Stack.LayerColors {
						layers[c.Layer] = c.RGB
					}
				}
				square, mass := 0., 0.
				for y := 0; y < 36; y++ {
					for x := 0; x < 48; x++ {
						a, b := src.NRGBAAt(x, y), r.Image.NRGBAAt(x, y)
						rgb := RGB{b.R, b.G, b.B}
						if a.A != b.A {
							t.Fatal("alpha changed")
						}
						if a.A == 0 {
							continue
						}
						if !palette[rgb] {
							t.Fatal("off-palette pixel")
						}
						if mode == "stack" && layers[int(r.LayerMap[y*48+x])] != rgb {
							t.Fatal("unreachable layer color")
						}
						w := float64(a.A) / 255
						square += w * distance(ToLab(RGB{a.R, a.G, a.B}), ToLab(rgb))
						mass += w
					}
				}
				if math.Abs(r.Quality.RMS-math.Sqrt(square/mass)) > 1e-9 {
					t.Fatal("importance contaminated source fidelity metric")
				}
			}
		}
	}
}

func TestPriorityOptionsAndGrayscale(t *testing.T) {
	o := testOptions()
	o.ColorPriority = "distinctive"
	b, _ := json.Marshal(o)
	var restored Options
	if err := json.Unmarshal(b, &restored); err != nil || restored != o {
		t.Fatal("priority did not round trip", err)
	}
	var old Options
	if err := json.Unmarshal([]byte(`{"colors":8}`), &old); err != nil || old.prioritizeColors() {
		t.Fatal("old settings changed priority")
	}
	o.ColorPriority = "unknown"
	if o.Validate() == nil {
		t.Fatal("unknown priority accepted")
	}
	o.ColorPriority = "vivid"
	o.LegacyColorPipeline = true
	if o.Validate() == nil {
		t.Fatal("conflicting legacy pipeline accepted")
	}
	o.LegacyColorPipeline = false
	o.TotalColors = true
	o.Colors = 8
	o.AnalysisMaxPixels = 0
	src := image.NewNRGBA(image.Rect(0, 0, 128, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 128; x++ {
			v := uint8(x * 2)
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	r := process(t, src, o, nil)
	if r.UniqueColors < 4 || r.UniqueColors > 8 {
		t.Fatal("grayscale structure over-simplified", r.UniqueColors)
	}
	for _, p := range r.Palette {
		if math.Hypot(toOKLab(p.RGB)[1], toOKLab(p.RGB)[2]) > 1 {
			t.Fatal("invented saturation")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Process(ctx, src, o, nil, nil); err == nil {
		t.Fatal("cancellation ignored")
	}
}
