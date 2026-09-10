package engine

import (
	"context"
	"image"
	"math"
)

// Boundaries connect source palette groups, not an arbitrary ordering of RGBs.
// Weights describe how often groups touch. Full-resolution scanning uses only
// two rows and a bounded color lookup, so thin lines cannot fall between samples.
type stackBoundary struct {
	a, b          int
	weight        float64
	geometryScale float64
}

type StackSurface struct {
	BoundaryPairs    int     `json:"boundaryPairs"`
	MeanHeightJumpMM float64 `json:"meanHeightJumpMm"`
	RMSColorDetour   float64 `json:"rmsColorDetour"`
	Penalty          float64 `json:"penalty"`
}

func stackBoundaries(ctx context.Context, src *image.NRGBA, palette []PaletteEntry, o Options) ([]stackBoundary, error) {
	if (!o.HueForge.ReduceShowThrough && !o.layerOptimization()) || o.Mode != "stack" {
		return nil, nil
	}
	// Scan every pixel, retaining fixed memory independently of image area.
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w == 0 || h == 0 || len(palette) < 2 {
		return nil, ctx.Err()
	}
	sw, sh := w, h
	colors, _ := targets(palette, o)
	lookup := make([]uint16, 1<<18)
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
			code := int(pixel.R>>2)<<12 | int(pixel.G>>2)<<6 | int(pixel.B>>2)
			if lookup[code] == 0 {
				// Bin midpoints make the classification independent of scan order.
				v, best, id := o.colorVector(RGB{pixel.R&252 | 2, pixel.G&252 | 2, pixel.B&252 | 2}), math.Inf(1), 0
				for i, c := range colors {
					if d := distance(v, c); d < best {
						best, id = d, i
					}
				}
				lookup[code] = uint16(id + 1)
			}
			current[x] = int(lookup[code]) - 1
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
	width, detail := o.HueForge.ExportWidthMM, o.HueForge.MeshDetailMM
	if width == 0 {
		width = 200
	}
	if detail == 0 {
		detail = .2
	}
	spacing := math.Max(detail, width/float64(w))
	geometryScale := math.Max(.0625, math.Min(16, math.Pow(.2/spacing, 2)))
	for a := range palette {
		for b := a + 1; b < len(palette); b++ {
			if count := counts[a*len(palette)+b]; count > 0 {
				boundaries = append(boundaries, stackBoundary{a: a, b: b, weight: count / total, geometryScale: geometryScale})
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

type surfaceIntervalKey struct {
	start, end RGB
	lo, hi     int
}

type surfaceInterval struct {
	detour float64
	count  int
	ready  bool
}

// A palette trial changes source assignments, but an interval between the same
// endpoint colors and heights has the same physical layers. Keep endpoint order:
// reversing a segment can change floating-point rounding near a search tie.
type stackSurfaceCache struct {
	path      []Vec
	colors    map[RGB]Vec
	intervals map[surfaceIntervalKey]surfaceInterval
	dense     []surfaceInterval
}

func newStackSurfaceCache(s stackState) *stackSurfaceCache {
	c := &stackSurfaceCache{
		path: make([]Vec, len(s.rgbs)), colors: make(map[RGB]Vec, len(s.rgbs)),
		intervals: make(map[surfaceIntervalKey]surfaceInterval),
	}
	for i, rgb := range s.rgbs {
		v, ok := c.colors[rgb]
		if !ok {
			v = toOKLab(rgb)
			c.colors[rgb] = v
		}
		c.path[i] = v
	}
	if len(s.rgbs) <= 64 {
		c.dense = make([]surfaceInterval, len(s.rgbs)*len(s.rgbs))
	}
	return c
}

func (c *stackSurfaceCache) interval(s stackState, start, end RGB, startLayer, endLayer int) surfaceInterval {
	lo, hi := min(startLayer, endLayer), max(startLayer, endLayer)
	if hi-lo <= 1 {
		return surfaceInterval{}
	}
	// Ordinary stack layers are contiguous. A bounded direct table avoids
	// hashing in the innermost boundary loop. Verify the physical endpoints
	// before using it; arbitrary/noncontiguous inputs retain the general path.
	denseIndex := -1
	if len(c.dense) > 0 && len(s.layers) > 0 {
		a, b := startLayer-s.layers[0], endLayer-s.layers[0]
		if a >= 0 && a < len(s.rgbs) && b >= 0 && b < len(s.rgbs) &&
			s.layers[a] == startLayer && s.layers[b] == endLayer && s.rgbs[a] == start && s.rgbs[b] == end {
			denseIndex = a*len(s.rgbs) + b
			if v := c.dense[denseIndex]; v.ready {
				return v
			}
		}
	}
	key := surfaceIntervalKey{start, end, lo, hi}
	if denseIndex < 0 {
		if v, ok := c.intervals[key]; ok {
			return v
		}
	}
	a, ok := c.colors[start]
	if !ok {
		a = toOKLab(start)
	}
	b, ok := c.colors[end]
	if !ok {
		b = toOKLab(end)
	}
	v := surfaceInterval{ready: true}
	for i, layer := range s.layers {
		if layer > lo && layer < hi {
			v.detour += colorSegmentDistance(c.path[i], a, b)
			v.count++
		}
	}
	// Deep stacks must not retain a quadratic-sized table. Once full, new
	// intervals still use the exact calculation, just without caching them.
	if denseIndex >= 0 {
		c.dense[denseIndex] = v
	} else if len(c.intervals) < 4096 {
		c.intervals[key] = v
	}
	return v
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
	cache := s.surfaceCache
	if cache == nil {
		cache = newStackSurfaceCache(s)
	}
	for _, edge := range boundaries {
		a, b := assigned[edge.a], assigned[edge.b]
		lo, hi := min(layers[a], layers[b]), max(layers[a], layers[b])
		jump := float64(hi-lo) * o.HueForge.LayerHeight
		stats.MeanHeightJumpMM += edge.weight * jump
		interval := cache.interval(s, selected[a], selected[b], layers[a], layers[b])
		detour, count := interval.detour, interval.count
		// Score integrated color deviation through the vertical interval.
		// An average lets extra opaque endpoint-colored layers dilute the
		// offending bands, rewarding padding without improving appearance.
		integratedDetour := detour * o.HueForge.LayerHeight
		if count > 0 {
			stats.RMSColorDetour += edge.weight * detour / float64(count)
		}
		// One mm of boundary relief costs four working-space color units.
		// Detours have a smaller influence so exact color fidelity still matters.
		scale := edge.geometryScale
		if scale == 0 {
			scale = 1
		}
		stats.Penalty += edge.weight * (16*jump*jump*scale + .1*integratedDetour)
	}
	stats.RMSColorDetour = math.Sqrt(stats.RMSColorDetour)
	return stats
}
