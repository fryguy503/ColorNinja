package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
)

const RegionMaxPixels = 4096 * 4096
const RegionMaxSpans = 1 << 20

// PixelSpan is a half-open run in row-major image coordinates, independent of
// image.Rect.Min. Runs are sorted, nonempty and do not overlap.
type PixelSpan struct {
	Start  int `json:"start"`
	Length int `json:"length"`
}
type RegionPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type RegionSelection struct {
	Tool    string        `json:"tool"`
	Scope   string        `json:"scope"`
	Combine string        `json:"combine"`
	Points  []RegionPoint `json:"points"`
	Radius  int           `json:"radius"`
	Above   int           `json:"above"`
	Below   int           `json:"below"`
}
type RegionIndex struct {
	Width, Height int
	Labels        []int32
	Areas         []int
	Layers        []uint16
}

func ValidateSpans(spans []PixelSpan, n int) error {
	if len(spans) > RegionMaxSpans {
		return fmt.Errorf("selection has too many runs")
	}
	end := 0
	for _, s := range spans {
		if s.Start < end || s.Start < 0 || s.Length < 1 || s.Start > n || s.Length > n-s.Start {
			return fmt.Errorf("invalid selection footprint")
		}
		end = s.Start + s.Length
	}
	return nil
}
func MaskSpans(mask []bool) []PixelSpan {
	s := []PixelSpan{}
	for i := 0; i < len(mask); {
		if !mask[i] {
			i++
			continue
		}
		start := i
		for i < len(mask) && mask[i] {
			i++
		}
		s = append(s, PixelSpan{start, i - start})
	}
	return s
}
func SpanMask(spans []PixelSpan, n int) ([]bool, error) {
	if err := ValidateSpans(spans, n); err != nil {
		return nil, err
	}
	m := make([]bool, n)
	for _, s := range spans {
		for i := s.Start; i < s.Start+s.Length; i++ {
			m[i] = true
		}
	}
	return m, nil
}
func regionNeighbors(i, w, n int, visit func(int)) {
	if i%w > 0 {
		visit(i - 1)
	}
	if i%w < w-1 {
		visit(i + 1)
	}
	if i >= w {
		visit(i - w)
	}
	if i+w < n {
		visit(i + w)
	}
}
func BuildRegionIndex(ctx context.Context, r, coverage *Result) (*RegionIndex, error) {
	if r == nil || r.Image == nil || r.Stack == nil || coverage == nil || coverage.Image == nil {
		return nil, fmt.Errorf("generate a Color Match stack before editing regions")
	}
	w, h := r.Image.Bounds().Dx(), r.Image.Bounds().Dy()
	n := w * h
	if w < 1 || h < 1 || w > 4096 || h > 4096 || n > RegionMaxPixels || len(r.LayerMap) != n || coverage.Image.Bounds() != r.Image.Bounds() {
		return nil, fmt.Errorf("region editing supports images up to 4096 × 4096")
	}
	x := &RegionIndex{w, h, make([]int32, n), []int{0}, r.LayerMap}
	q := make([]int32, 0, min(n, 65536))
	for i := 0; i < n; i++ {
		if i%65536 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		px := coverage.Image.PixOffset(coverage.Image.Rect.Min.X+i%w, coverage.Image.Rect.Min.Y+i/w)
		if x.Labels[i] != 0 || coverage.Image.Pix[px+3] == 0 {
			continue
		}
		if len(x.Areas) > 1000000 {
			return nil, fmt.Errorf("image has more than one million regions; reduce its resolution before editing")
		}
		id := int32(len(x.Areas))
		q = q[:0]
		q = append(q, int32(i))
		x.Labels[i] = id
		for head := 0; head < len(q); head++ {
			if head%65536 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			regionNeighbors(int(q[head]), w, n, func(j int) {
				if x.Labels[j] != 0 || r.LayerMap[j] != r.LayerMap[i] {
					return
				}
				p := coverage.Image.PixOffset(coverage.Image.Rect.Min.X+j%w, coverage.Image.Rect.Min.Y+j/w)
				if coverage.Image.Pix[p+3] == 0 {
					return
				}
				x.Labels[j] = id
				q = append(q, int32(j))
			})
		}
		x.Areas = append(x.Areas, len(q))
	}
	return x, nil
}

func (x *RegionIndex) Select(ctx context.Context, s RegionSelection) ([]bool, error) {
	n := len(x.Labels)
	mask := make([]bool, n)
	if len(s.Points) == 0 || len(s.Points) > 8192 || s.Radius < 0 || s.Radius > 128 || s.Above < 0 || s.Below < 0 || s.Above > 256 || s.Below > 256 {
		return nil, fmt.Errorf("invalid selection gesture")
	}
	for _, p := range s.Points {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) || math.Abs(p.X) > 1e7 || math.Abs(p.Y) > 1e7 {
			return nil, fmt.Errorf("invalid selection coordinates")
		}
	}
	pointIndex := func(p RegionPoint) int {
		if p.X < 0 || p.Y < 0 || p.X >= float64(x.Width) || p.Y >= float64(x.Height) {
			return -1
		}
		return int(p.Y)*x.Width + int(p.X)
	}
	if s.Tool == "click" {
		i := pointIndex(s.Points[0])
		if i < 0 || x.Labels[i] == 0 {
			return mask, nil
		}
		lo, hi := int(x.Layers[i])-s.Below, int(x.Layers[i])+s.Above
		q := []int{i}
		mask[i] = true
		for head := 0; head < len(q); head++ {
			if head%65536 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			regionNeighbors(q[head], x.Width, n, func(j int) {
				if !mask[j] && x.Labels[j] != 0 && int(x.Layers[j]) >= lo && int(x.Layers[j]) <= hi {
					mask[j] = true
					q = append(q, j)
				}
			})
		}
		return mask, nil
	}
	if s.Tool == "brush" {
		radius := max(1, s.Radius)
		work := int64(0)
		stamp := func(p RegionPoint) {
			cx, cy := int(p.X), int(p.Y)
			for y := max(0, cy-radius); y <= min(x.Height-1, cy+radius); y++ {
				for xx := max(0, cx-radius); xx <= min(x.Width-1, cx+radius); xx++ {
					if (xx-cx)*(xx-cx)+(y-cy)*(y-cy) <= radius*radius {
						mask[y*x.Width+xx] = true
					}
				}
			}
		}
		stamp(s.Points[0])
		for i := 1; i < len(s.Points); i++ {
			a, b := s.Points[i-1], s.Points[i]
			steps := int(math.Ceil(math.Hypot(a.X-b.X, a.Y-b.Y) / float64(max(1, radius/2))))
			if steps > 16384 {
				return nil, fmt.Errorf("brush stroke is too long")
			}
			work += int64(steps) * int64((2*radius+1)*(2*radius+1))
			if work > 256*1024*1024 {
				return nil, fmt.Errorf("brush stroke is too complex; use shorter strokes")
			}
			for j := 1; j <= steps; j++ {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				f := float64(j) / float64(steps)
				stamp(RegionPoint{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f})
			}
		}
	} else {
		poly := s.Points
		if s.Tool == "rectangle" {
			if len(poly) != 2 {
				return nil, fmt.Errorf("rectangle needs two corners")
			}
			a, b := poly[0], poly[1]
			poly = []RegionPoint{a, {b.X, a.Y}, b, {a.X, b.Y}}
		} else if s.Tool != "lasso" && s.Tool != "polygon" {
			return nil, fmt.Errorf("unknown selection tool")
		}
		if len(poly) < 3 {
			return nil, fmt.Errorf("draw an enclosed selection")
		}
		cross := make([]float64, 0, len(poly))
		for y := 0; y < x.Height; y++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			cross = cross[:0]
			py := float64(y) + .5
			for i, a := range poly {
				b := poly[(i+1)%len(poly)]
				if (a.Y > py) != (b.Y > py) {
					cross = append(cross, a.X+(py-a.Y)*(b.X-a.X)/(b.Y-a.Y))
				}
			}
			sort.Float64s(cross)
			for i := 0; i+1 < len(cross); i += 2 {
				for xx := max(0, int(math.Ceil(cross[i]-.5))); xx < min(x.Width, int(math.Ceil(cross[i+1]-.5))); xx++ {
					mask[y*x.Width+xx] = true
				}
			}
		}
	}
	if s.Scope != "pixels" {
		if s.Scope != "inside" && s.Scope != "touch" {
			return nil, fmt.Errorf("choose enclosed regions, touching regions, or pixels")
		}
		counts := make([]int, len(x.Areas))
		for i, v := range mask {
			if v {
				counts[x.Labels[i]]++
			}
		}
		for i, id := range x.Labels {
			mask[i] = id > 0 && counts[id] > 0 && (s.Scope == "touch" || counts[id] == x.Areas[id])
		}
	} else {
		for i, id := range x.Labels {
			mask[i] = mask[i] && id != 0
		}
	}
	return mask, nil
}
func CombineRegionMasks(a, b []bool, operation string) ([]bool, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("selection dimensions changed")
	}
	m := make([]bool, len(a))
	for i := range a {
		switch operation {
		case "replace":
			m[i] = b[i]
		case "add":
			m[i] = a[i] || b[i]
		case "subtract":
			m[i] = a[i] && !b[i]
		case "intersect":
			m[i] = a[i] && b[i]
		default:
			return nil, fmt.Errorf("unknown selection operation")
		}
	}
	return m, nil
}

func (x *RegionIndex) Refine(ctx context.Context, mask []bool, action string, value int, r *Result, reference int) ([]bool, error) {
	if len(mask) != len(x.Labels) {
		return nil, fmt.Errorf("selection dimensions changed")
	}
	m := append([]bool(nil), mask...)
	n := len(m)
	switch action {
	case "clear":
		clear(m)
	case "all", "invert":
		for i := range m {
			m[i] = x.Labels[i] != 0 && (action == "all" || !m[i])
		}
	case "grow", "shrink":
		if value < 1 || value > 64 {
			return nil, fmt.Errorf("grow/shrink must be 1–64 pixels")
		}
		for k := 0; k < value; k++ {
			next := append([]bool(nil), m...)
			for i := range m {
				if i%65536 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				if action == "grow" && m[i] {
					regionNeighbors(i, x.Width, n, func(j int) { next[j] = x.Labels[j] != 0 })
				}
				if action == "shrink" && m[i] {
					if i%x.Width == 0 || i%x.Width == x.Width-1 || i < x.Width || i >= n-x.Width {
						next[i] = false
					}
					regionNeighbors(i, x.Width, n, func(j int) {
						if !m[j] {
							next[i] = false
						}
					})
				}
			}
			m = next
		}
	case "fill":
		outside := make([]bool, n)
		q := make([]int, 0)
		for i := range m {
			if !m[i] && (i%x.Width == 0 || i%x.Width == x.Width-1 || i < x.Width || i >= n-x.Width || x.Labels[i] == 0) {
				outside[i] = true
				q = append(q, i)
			}
		}
		for head := 0; head < len(q); head++ {
			if head%65536 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			regionNeighbors(q[head], x.Width, n, func(j int) {
				if !m[j] && !outside[j] {
					outside[j] = true
					q = append(q, j)
				}
			})
		}
		for i := range m {
			m[i] = x.Labels[i] != 0 && !outside[i]
		}
	case "same-layer", "similar":
		if value < 0 || value > 100 {
			return nil, fmt.Errorf("color tolerance must be 0–100")
		}
		layers := map[uint16]bool{}
		for i, v := range m {
			if v {
				layers[x.Layers[i]] = true
			}
		}
		wanted := map[uint16]bool{}
		if reference > 0 {
			layers = map[uint16]bool{uint16(reference): true}
		}
		for _, a := range r.Stack.LayerColors {
			for _, b := range r.Stack.LayerColors {
				if layers[uint16(b.Layer)] && (action == "same-layer" && a.Layer == b.Layer || action == "similar" && math.Sqrt(distance(ToLab(a.RGB), ToLab(b.RGB))) <= float64(value)) {
					wanted[uint16(a.Layer)] = true
				}
			}
		}
		for i := range m {
			m[i] = x.Labels[i] != 0 && wanted[x.Layers[i]]
		}
	case "speckles":
		if value < 1 || value > 100000 {
			return nil, fmt.Errorf("speckle area must be 1–100000 pixels")
		}
		contained := make([]int, len(x.Areas))
		any := false
		for i, v := range m {
			if v {
				contained[x.Labels[i]]++
				any = true
			}
		}
		for i, id := range x.Labels {
			m[i] = id > 0 && x.Areas[id] <= value && (!any || contained[id] == x.Areas[id])
		}
	default:
		return nil, fmt.Errorf("unknown selection refinement")
	}
	return m, nil
}
