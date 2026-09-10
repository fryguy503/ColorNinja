package engine

import (
	"context"
	"math"
)

// SurfaceView is a bounded diagnostic grid, not a native HueForge mesh. Metrics
// come from every source pixel; the displayed surface is explicitly sampled.
type SurfaceView struct {
	Border               *BorderView   `json:"border,omitempty"`
	VolumeMM3            float64       `json:"volumeMm3"`
	MeanThicknessMM      float64       `json:"meanThicknessMm"`
	MaterialRuns         []RunMaterial `json:"materialRuns"`
	Width                int           `json:"width"`
	Height               int           `json:"height"`
	Layers               []uint16      `json:"layers"`
	WidthMM              float64       `json:"widthMm"`
	HeightMM             float64       `json:"heightMm"`
	SpacingMM            float64       `json:"spacingMm"`
	RequestedSpacingMM   float64       `json:"requestedSpacingMm"`
	PixelMM              float64       `json:"pixelMm"`
	Sampled              bool          `json:"sampled"`
	MeanJumpMM           float64       `json:"meanJumpMm"`
	P95JumpMM            float64       `json:"p95JumpMm"`
	MaxJumpMM            float64       `json:"maxJumpMm"`
	BoundaryAreaFraction float64       `json:"boundaryAreaFraction"`
	SolidifiedFraction   float64       `json:"solidifiedFraction"`
}

func BuildSurfaceView(ctx context.Context, r *Result, o Options) (*SurfaceView, error) {
	if r.Stack == nil || r.Image == nil {
		return nil, nil
	}
	w, h := r.Image.Bounds().Dx(), r.Image.Bounds().Dy()
	if w == 0 || h == 0 || len(r.LayerMap) != w*h {
		return nil, nil
	}
	width, spacing := o.HueForge.ExportWidthMM, o.HueForge.MeshDetailMM
	if width == 0 {
		width = 200
	}
	if spacing == 0 {
		spacing = .2
	}
	height := width * float64(h) / float64(w)
	border, err := resolveBorder(r, o.HueForge)
	if err != nil {
		return nil, err
	}
	if border != nil {
		width, height = border.ImageWidthMM, border.ImageHeightMM
	}
	step := math.Max(spacing, math.Max(width, height)/255)
	nx, ny := max(2, int(math.Ceil(width/step))+1), max(2, int(math.Ceil(height/step))+1)
	v := &SurfaceView{Width: nx, Height: ny, WidthMM: width, HeightMM: height, RequestedSpacingMM: spacing, SpacingMM: step, PixelMM: width / float64(w), Sampled: step > spacing+1e-9, Layers: make([]uint16, nx*ny)}
	for y := 0; y < ny; y++ {
		for x := 0; x < nx; x++ {
			sx, sy := int(math.Round(float64(x)*float64(w-1)/float64(nx-1))), int(math.Round(float64(y)*float64(h-1)/float64(ny-1)))
			v.Layers[y*nx+x] = r.LayerMap[sy*w+sx]
		}
	}
	hist := make([]float64, 4097)
	tops := make([]float64, 4097)
	visible, partial, edges := 0., 0., 0.
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*w + x
			layer := r.LayerMap[i]
			if layer == 0 {
				continue
			}
			visible++
			if int(layer) < len(tops) {
				tops[layer]++
			}
			if a := r.Image.Pix[y*r.Image.Stride+x*4+3]; a > 0 && a < 255 {
				partial++
			}
			for direction, other := range []int{i - 1, i - w} {
				if other < 0 || (direction == 0 && x == 0) {
					continue
				}
				l := r.LayerMap[other]
				if l == 0 || l == layer {
					continue
				}
				jump := int(layer) - int(l)
				if jump < 0 {
					jump = -jump
				}
				if jump >= len(hist) {
					continue
				}
				hist[jump]++
				edges++
				v.MeanJumpMM += float64(jump) * o.HueForge.LayerHeight
			}
		}
	}
	if visible > 0 {
		v.SolidifiedFraction = partial / visible
		v.BoundaryAreaFraction = math.Min(1, edges*math.Max(spacing, v.PixelMM)/(visible*v.PixelMM))
	}
	if edges > 0 {
		v.MeanJumpMM /= edges
		sum := 0.
		for i, n := range hist {
			if n > 0 {
				v.MaxJumpMM = float64(i) * o.HueForge.LayerHeight
			}
			sum += n
			if v.P95JumpMM == 0 && sum >= edges*.95 {
				v.P95JumpMM = float64(i) * o.HueForge.LayerHeight
			}
		}
	}
	surfaceMaterial(v, tops, visible, r.Stack, o.HueForge)
	v.Border = border
	return v, nil
}
