package engine

import (
	"context"
	"image"
	"math"
)

type layerRegionsKey struct{}
type layerRegion struct {
	compactness float64
	coherence   float64
	contacts    map[string]float64
}

// Bounded spatial analysis supplies evidence for automatic ordering. A color
// scattered across foliage should not be promoted like a compact subject.
// The full-resolution palette and boundary scan still preserve fine details;
// this grid is only used to decide whether an accent preference is justified.
func analyzeLayerRegions(ctx context.Context, src *image.NRGBA) (map[string]layerRegion, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w == 0 || h == 0 {
		return nil, ctx.Err()
	}
	scale := math.Min(1, 384./float64(max(w, h)))
	sw, sh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	names := []string{"neutral", "red", "yellow", "green", "cyan", "blue", "purple"}
	ids := map[string]int{}
	for i, name := range names {
		ids[name] = i + 1
	}
	grid := make([]int, sw*sh)
	counts, largest := make([]int, 8), make([]int, 8)
	xcounts, ycounts := make([][]int, 8), make([][]int, 8)
	contacts := make([][]float64, 8)
	for i := range xcounts {
		xcounts[i], ycounts[i], contacts[i] = make([]int, sw), make([]int, sh), make([]float64, 8)
	}
	for y := 0; y < sh; y++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for x := 0; x < sw; x++ {
			p := src.NRGBAAt(src.Rect.Min.X+min(w-1, int((float64(x)+.5)*float64(w)/float64(sw))), src.Rect.Min.Y+min(h-1, int((float64(y)+.5)*float64(h)/float64(sh))))
			if p.A == 0 {
				continue
			}
			id := ids[layerColorFamily(RGB{p.R, p.G, p.B})]
			grid[y*sw+x] = id
			counts[id]++
			xcounts[id][x]++
			ycounts[id][y]++
			for _, n := range []int{y*sw + x - 1, (y-1)*sw + x} {
				if n < 0 || (n == y*sw+x-1 && x == 0) {
					continue
				}
				other := grid[n]
				if other > 0 && other != id {
					contacts[id][other]++
					contacts[other][id]++
				}
			}
		}
	}
	seen, queue := make([]bool, len(grid)), make([]int, 0, len(grid))
	for start, id := range grid {
		if seen[start] || id == 0 {
			continue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		queue = append(queue[:0], start)
		seen[start] = true
		for at := 0; at < len(queue); at++ {
			p := queue[at]
			x, y := p%sw, p/sw
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || nx >= sw || ny < 0 || ny >= sh {
					continue
				}
				n := ny*sw + nx
				if !seen[n] && grid[n] == id {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		largest[id] = max(largest[id], len(queue))
	}
	span := func(hist []int, total int) float64 {
		low, high, sum := 0, len(hist)-1, 0
		for i, n := range hist {
			sum += n
			if float64(sum) >= .05*float64(total) {
				low = i
				break
			}
		}
		sum = 0
		for i, n := range hist {
			sum += n
			if float64(sum) >= .95*float64(total) {
				high = i
				break
			}
		}
		return float64(high-low+1) / float64(len(hist))
	}
	out := map[string]layerRegion{}
	for i, name := range names {
		id := i + 1
		if counts[id] == 0 {
			continue
		}
		r := layerRegion{compactness: span(xcounts[id], counts[id]) * span(ycounts[id], counts[id]), coherence: float64(largest[id]) / float64(counts[id]), contacts: map[string]float64{}}
		for j, other := range names {
			r.contacts[other] = contacts[id][j+1]
		}
		out[name] = r
	}
	return out, ctx.Err()
}
