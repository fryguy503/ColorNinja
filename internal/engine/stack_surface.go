package engine

import (
	"context"
	"image"
	"math"
)

// Boundaries connect source palette groups, not an arbitrary ordering of RGBs.
// Weights describe how often groups touch on a bounded sample of the image.
type stackBoundary struct {
	a, b   int
	weight float64
}

type StackSurface struct {
	BoundaryPairs    int     `json:"boundaryPairs"`
	MeanHeightJumpMM float64 `json:"meanHeightJumpMm"`
	RMSColorDetour   float64 `json:"rmsColorDetour"`
	Penalty          float64 `json:"penalty"`
}

func stackBoundaries(ctx context.Context, src *image.NRGBA, palette []PaletteEntry, o Options) ([]stackBoundary, error) {
	if !o.HueForge.ReduceShowThrough || o.Mode != "stack" {
		return nil, nil
	}
	// Nearest samples retain actual colors instead of creating blended edges.
	// The cap bounds both memory and work independently of export resolution.
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w == 0 || h == 0 || len(palette) < 2 {
		return nil, ctx.Err()
	}
	scale := math.Min(1, math.Sqrt(262144/float64(w*h)))
	sw, sh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	sw = min(sw, 262144)
	sh = min(sh, 262144/sw)
	colors, _ := targets(palette, o)
	previous, current := make([]int, sw), make([]int, sw)
	previousAlpha, currentAlpha := make([]uint8, sw), make([]uint8, sw)
	counts := make([]float64, len(palette)*len(palette))
	total := 0.
	add := func(a, b int, alpha, other uint8) {
		if a == b || alpha == 0 || other == 0 {
			return
		}
		if a > b {
			a, b = b, a
		}
		weight := float64(min(alpha, other)) / 255
		counts[a*len(palette)+b] += weight
		total += weight
	}
	for y := 0; y < sh; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sy := min(h-1, int((float64(y)+.5)*float64(h)/float64(sh)))
		for x := 0; x < sw; x++ {
			if x%1024 == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			sx := min(w-1, int((float64(x)+.5)*float64(w)/float64(sw)))
			pixel := src.NRGBAAt(src.Rect.Min.X+sx, src.Rect.Min.Y+sy)
			current[x], currentAlpha[x] = 0, pixel.A
			if pixel.A == 0 {
				continue
			}
			v, best := o.colorVector(RGB{pixel.R, pixel.G, pixel.B}), math.Inf(1)
			for i, c := range colors {
				if d := distance(v, c); d < best {
					best, current[x] = d, i
				}
			}
			if x > 0 {
				add(current[x-1], current[x], currentAlpha[x-1], pixel.A)
			}
			if y > 0 {
				add(previous[x], current[x], previousAlpha[x], pixel.A)
			}
		}
		previous, current = current, previous
		previousAlpha, currentAlpha = currentAlpha, previousAlpha
	}
	boundaries := []stackBoundary{}
	for a := range palette {
		for b := a + 1; b < len(palette); b++ {
			if count := counts[a*len(palette)+b]; count > 0 {
				boundaries = append(boundaries, stackBoundary{a, b, count / total})
			}
		}
	}
	return boundaries, ctx.Err()
}

// Distance from an intermediate layer color to the segment joining the two
// endpoint colors. Ordinary shading between endpoints is harmless here; a pink
// band between black and green is a detour. This is a planning heuristic, not a
// simulation of HueForge's triangulation, lighting, or a physical print.
func colorSegmentDistance(v, a, b Vec) float64 {
	length, projection := 0., 0.
	for c := range v {
		length += (b[c] - a[c]) * (b[c] - a[c])
		projection += (v[c] - a[c]) * (b[c] - a[c])
	}
	t := 0.
	if length > 0 {
		t = math.Max(0, math.Min(1, projection/length))
	}
	nearest := a
	for c := range nearest {
		nearest[c] += t * (b[c] - a[c])
	}
	return distance(v, nearest)
}

func stackSurface(s stackState, selected []RGB, layers []int, target []Vec, o Options, boundaries []stackBoundary) StackSurface {
	stats := StackSurface{BoundaryPairs: len(boundaries)}
	if len(boundaries) == 0 {
		return stats
	}
	vectors := o.colorVectors(selected)
	assigned := make([]int, len(target))
	for i, t := range target {
		best := math.Inf(1)
		for j, c := range vectors {
			if d := distance(t, c); d < best {
				best, assigned[i] = d, j
			}
		}
	}
	// Use unweighted Oklab for the geometric color detour even if the source
	// matching prioritizes chroma or uses the legacy Lab pipeline.
	path := make([]Vec, len(s.rgbs))
	for i, c := range s.rgbs {
		path[i] = toOKLab(c)
	}
	for _, edge := range boundaries {
		a, b := assigned[edge.a], assigned[edge.b]
		lo, hi := min(layers[a], layers[b]), max(layers[a], layers[b])
		jump := float64(hi-lo) * o.HueForge.LayerHeight
		stats.MeanHeightJumpMM += edge.weight * jump
		start, end := toOKLab(selected[a]), toOKLab(selected[b])
		detour, count := 0., 0
		for i, layer := range s.layers {
			if layer > lo && layer < hi {
				detour += colorSegmentDistance(path[i], start, end)
				count++
			}
		}
		if count > 0 {
			detour /= float64(count)
		}
		stats.RMSColorDetour += edge.weight * detour
		// One mm of boundary relief costs four working-space color units.
		// Detours have a smaller influence so exact color fidelity still matters.
		stats.Penalty += edge.weight * (16*jump*jump + .1*detour)
	}
	stats.RMSColorDetour = math.Sqrt(stats.RMSColorDetour)
	return stats
}
