package engine

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestCoherentRareDiagonalAndCurves(t *testing.T) {
	o := DefaultOptions()
	o.Colors, o.TotalColors, o.PreblurSigma = 8, true, 0
	for _, shape := range []string{"diagonal", "curve", "speckles"} {
		t.Run(shape, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, 2048, 2048))
			for i := range img.Pix {
				img.Pix[i] = 255
			}
			for y := 0; y < 2048; y++ {
				x := y
				if shape == "curve" {
					x = 100 + int(60*math.Sin(float64(y)/100))
				}
				if shape == "speckles" {
					x = (y * 37) % 2048
				}
				img.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
			p, _, err := Discover(context.Background(), img, o, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if shape == "speckles" {
				want = 1
			}
			if len(p) != want {
				t.Fatalf("%s: got %d colors, want %d", shape, len(p), want)
			}
		})
	}
}

func TestFullResolutionBoundariesRetainTranslatedLines(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.HueForge.ReduceShowThrough = "stack", true
	p := []PaletteEntry{entry(RGB{}, .001, 8), entry(RGB{255, 255, 255}, .999, 8)}
	for _, x := range []int{1, 2, 3, 4, 5} {
		img := image.NewNRGBA(image.Rect(0, 0, 2048, 2048))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		for y := 0; y < 2048; y++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
		b, err := stackBoundaries(context.Background(), img, p, o)
		if err != nil || len(b) != 1 || b[0].weight != 1 {
			t.Fatalf("line %d lost: %v %v", x, b, err)
		}
	}
}

func TestRepeatedColorChoosesNearbyHeight(t *testing.T) {
	o := DefaultOptions()
	o.Mode, o.Colors, o.HueForge.MaxRuns = "stack", 3, 4
	lib := Library{Filaments: []Filament{{RGB: RGB{}, TD: .01}, {RGB: RGB{255, 0, 255}, TD: .01}, {RGB: RGB{0, 255, 0}, TD: .01}}}
	s := rebuildStack([]int{0, 1, 0, 2}, []int{5, 10, 1, 1}, lib, o.HueForge)
	colors, layers, positions := uniqueStack(s)
	target := o.colorVectors([]RGB{RGB{}, RGB{0, 255, 0}})
	if err := chooseStackHeights(context.Background(), s, colors, layers, positions, []int{0, 2}, target, o, []stackBoundary{{a: 0, b: 1, weight: 1}}); err != nil {
		t.Fatal(err)
	}
	if layers[0] != 16 || positions[0] != 3 {
		t.Fatalf("black retained distant height: %v %v", layers, positions)
	}
}

func TestPlanMetricsAlwaysMeasureCIELAB(t *testing.T) {
	lib := Library{Filaments: []Filament{{Name: "Black", RGB: RGB{}, TD: .01}}}
	p := []PaletteEntry{entry(RGB{255, 0, 0}, 1, 8)}
	for _, preserve := range []bool{false, true} {
		for _, priority := range []string{"balanced", "distinctive", "vivid"} {
			o := DefaultOptions()
			o.Colors, o.PreserveDetails, o.TrueBlack, o.ColorPriority = 1, preserve, false, priority
			out, plan, err := planStack(context.Background(), p, lib, o, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := paletteRMS76([]RGB{out[0].RGB}, p, o)
			if math.Abs(plan.RMS-want) > 1e-8 {
				t.Fatalf("stack metric %g want %g", plan.RMS, want)
			}
			out, g, err := guide(context.Background(), p, lib, o, nil)
			if err != nil {
				t.Fatal(err)
			}
			rgb := []RGB{}
			for _, v := range out {
				rgb = append(rgb, v.RGB)
			}
			want = paletteRMS76(rgb, p, o)
			if math.Abs(g.RMS-want) > 1e-8 {
				t.Fatalf("guide metric %g want %g", g.RMS, want)
			}
		}
	}
}
