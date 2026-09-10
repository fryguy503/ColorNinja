package engine

import (
	"context"
	"image"
	"sort"
)

// Two bands can have the same visible RGB at different heights. HueForge's
// Color Match needs unique image keys, so only collisions get a nearby RGB
// key in the virtual Mesh Core. The physical stack and result PNG stay exact.
func colorPopMeshColors(r *Result) map[int]RGB {
	byLayer := map[int]RGB{}
	for _, p := range r.Palette {
		if p.PixelFraction > 0 {
			byLayer[p.StackLayer] = p.RGB
		}
	}
	layers := []int{}
	for l := range byLayer {
		layers = append(layers, l)
	}
	sort.Ints(layers)
	used := map[RGB]bool{}
	for _, l := range layers {
		c := byLayer[l]
		if used[c] {
		search:
			for radius := 1; radius <= 255; radius++ {
				for red := -radius; red <= radius; red++ {
					for green := -radius; green <= radius; green++ {
						for blue := -radius; blue <= radius; blue++ {
							rr, gg, bb := int(c[0])+red, int(c[1])+green, int(c[2])+blue
							if rr < 0 || rr > 255 || gg < 0 || gg > 255 || bb < 0 || bb > 255 {
								continue
							}
							v := RGB{uint8(rr), uint8(gg), uint8(bb)}
							if !used[v] {
								c = v
								break search
							}
						}
					}
				}
			}
		}
		byLayer[l] = c
		used[c] = true
	}
	return byLayer
}

func colorPopMeshResult(ctx context.Context, original *Result) (*Result, error) {
	r := *original
	s := *r.Stack
	r.Stack = &s
	s.Options.MeshMode = "color-match"
	s.Options.MeshCore = "legacy-flat"
	colors := colorPopMeshColors(original)
	r.Image = image.NewNRGBA(original.Image.Bounds())
	for y := 0; y < r.Image.Bounds().Dy(); y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < r.Image.Bounds().Dx(); x++ {
			i := y*r.Image.Stride + x*4
			c := colors[int(r.LayerMap[y*r.Image.Bounds().Dx()+x])]
			r.Image.Pix[i], r.Image.Pix[i+1], r.Image.Pix[i+2], r.Image.Pix[i+3] = c[0], c[1], c[2], original.Image.Pix[i+3]
		}
	}
	return &r, nil
}
