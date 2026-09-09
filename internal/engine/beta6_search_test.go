package engine

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestRefinedSmallStacksAgainstExhaustiveSearch(t *testing.T) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.Colors = 2
	o.PreserveDetails = false
	o.HueForge.MaxDepth = .8
	o.HueForge.SearchEffort = "refine"
	o.HueForge.BeamWidth = 24
	lib := Library{Filaments: []Filament{{UUID: "white", RGB: RGB{250, 245, 235}, TD: 4}, {UUID: "blue", RGB: RGB{10, 50, 230}, TD: 2}, {UUID: "red", RGB: RGB{235, 30, 30}, TD: 3}}}
	for _, rgb := range [][]RGB{{{220, 110, 100}, {120, 110, 230}}, {{50, 65, 225}, {244, 223, 231}}, {{245, 230, 201}, {233, 47, 67}}} {
		p := []PaletteEntry{entry(rgb[0], .5, 8), entry(rgb[1], .5, 8)}
		target, weights := targets(p, o)
		best := math.Inf(1)
		for a := range lib.Filaments {
			for n := o.HueForge.BaseLayers(); n <= o.HueForge.MaxLayers(); n++ {
				s := rebuildStack([]int{a}, []int{n}, lib, o.HueForge)
				score, e := stateScore(context.Background(), s, target, weights, o)
				if e != nil {
					t.Fatal(e)
				}
				best = math.Min(best, score)
				for b := range lib.Filaments {
					if b == a {
						continue
					}
					for m := 1; m+n <= o.HueForge.MaxLayers(); m++ {
						s = rebuildStack([]int{a, b}, []int{n, m}, lib, o.HueForge)
						score, e = stateScore(context.Background(), s, target, weights, o)
						if e != nil {
							t.Fatal(e)
						}
						best = math.Min(best, score)
					}
				}
			}
		}
		_, plan, e := planStack(context.Background(), p, lib, o, nil)
		if e != nil {
			t.Fatal(e)
		}
		if plan.OptimizationScore > best+1e-7 {
			t.Fatalf("small-case search gap %g vs exhaustive %g", plan.OptimizationScore, best)
		}
	}
}

func BenchmarkBeta6Iteration(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 1200, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 1200; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / 1200), uint8(y * 255 / 900), uint8((x + y) % 256), 255})
		}
	}
	o := DefaultOptions()
	o.Colors = 8
	o.TotalColors = true
	b.Run("fresh", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := Process(context.Background(), img, o, nil, nil); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("export-only", func(b *testing.B) {
		p := Processor{}
		if _, e := p.Process(context.Background(), img, o, nil, nil); e != nil {
			b.Fatal(e)
		}
		b.ReportAllocs()
		for b.Loop() {
			o.HueForge.ExportWidthMM = 100
			if _, e := p.Process(context.Background(), img, o, nil, nil); e != nil {
				b.Fatal(e)
			}
		}
	})
}
