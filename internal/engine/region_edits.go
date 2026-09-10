package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"math"
)

type RegionGroup struct {
	ID        uint64      `json:"id"`
	Name      string      `json:"name"`
	Enabled   bool        `json:"enabled"`
	Locked    bool        `json:"locked"`
	Operation string      `json:"operation"`
	Value     int         `json:"value"`
	Mask      []PixelSpan `json:"mask"`
}
type RegionDocument struct {
	Version    int           `json:"version"`
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	BaseSHA256 string        `json:"baseSHA256"`
	NextID     uint64        `json:"nextId"`
	Groups     []RegionGroup `json:"groups"`
}
type RegionSummary struct {
	Groups           int    `json:"groups"`
	ChangedPixels    int    `json:"changedPixels"`
	QualityReference string `json:"qualityReference"`
	Warning          string `json:"warning"`
}

func (d RegionDocument) Validate(base *Result) error {
	if base == nil || base.Image == nil || base.Stack == nil || len(base.Stack.Runs) == 0 {
		return fmt.Errorf("region edits require a generated stack")
	}
	if d.Version != 1 || d.Width < 1 || d.Height < 1 || d.Width > 4096 || d.Height > 4096 || d.Width != base.Image.Bounds().Dx() || d.Height != base.Image.Bounds().Dy() || d.BaseSHA256 != base.SHA256 || len(base.LayerMap) != d.Width*d.Height {
		return fmt.Errorf("region edits do not match the generated image")
	}
	if len(d.Groups) > 128 {
		return fmt.Errorf("at most 128 edit groups are supported")
	}
	seen := map[uint64]bool{}
	spans := 0
	lo, hi := base.Stack.Options.BaseLayers(), base.Stack.Runs[len(base.Stack.Runs)-1].EndLayer
	if base.Stack.Options.Validate() != nil || lo < 1 || hi < lo || hi > 4096 {
		return fmt.Errorf("invalid generated stack bounds")
	}
	colorLayers := map[int]bool{}
	for _, c := range base.Stack.LayerColors {
		if c.Layer < 1 || c.Layer > hi || colorLayers[c.Layer] {
			return fmt.Errorf("invalid generated layer colors")
		}
		colorLayers[c.Layer] = true
	}
	for l := lo; l <= hi; l++ {
		if !colorLayers[l] {
			return fmt.Errorf("generated stack is missing layer %d", l)
		}
	}
	for _, g := range d.Groups {
		if g.ID == 0 || g.ID >= d.NextID || seen[g.ID] || len(g.Name) > 120 {
			return fmt.Errorf("invalid region group identity")
		}
		seen[g.ID] = true
		if err := ValidateSpans(g.Mask, d.Width*d.Height); err != nil {
			return err
		}
		spans += len(g.Mask)
		if spans > RegionMaxSpans {
			return fmt.Errorf("edit footprints exceed the supported size")
		}
		switch g.Operation {
		case "assign":
			if g.Value < lo || g.Value > hi {
				return fmt.Errorf("target layer is outside the generated stack")
			}
		case "shift":
			if g.Value < -4096 || g.Value > 4096 {
				return fmt.Errorf("invalid layer adjustment")
			}
		case "smooth":
			if g.Value < 1 || g.Value > 4 {
				return fmt.Errorf("smoothing radius must be 1–4 pixels")
			}
		case "restore", "cut":
		default:
			return fmt.Errorf("unknown region edit operation")
		}
	}
	return nil
}

// ApplyRegionDocument always replays from an immutable generated result. Masks
// are the selected pixels, not unstable connected-component identifiers.
func ApplyRegionDocument(ctx context.Context, base *Result, src *image.NRGBA, o Options, d RegionDocument) (*Result, error) {
	if err := d.Validate(base); err != nil {
		return nil, err
	}
	if src != nil && (src.Bounds().Dx() != d.Width || src.Bounds().Dy() != d.Height) {
		return nil, fmt.Errorf("source and edited image dimensions disagree")
	}
	if len(d.Groups) == 0 {
		r := ReframeResult(base, o)
		view, err := BuildSurfaceView(ctx, r, o)
		if err != nil {
			return nil, err
		}
		r.SurfaceView = view
		return r, nil
	}
	r := *base
	r.Image = image.NewNRGBA(base.Image.Bounds())
	draw.Draw(r.Image, r.Image.Bounds(), base.Image, base.Image.Rect.Min, draw.Src)
	r.LayerMap = append([]uint16(nil), base.LayerMap...)
	stack := *base.Stack
	stack.Options = o.HueForge
	stack.LayerColors = append([]LayerColor(nil), base.Stack.LayerColors...)
	r.Stack = &stack
	w, n := d.Width, len(r.LayerMap)
	lo, hi := stack.Options.BaseLayers(), stack.Runs[len(stack.Runs)-1].EndLayer
	colors := make(map[int]LayerColor, len(stack.LayerColors))
	for _, c := range stack.LayerColors {
		colors[c.Layer] = c
	}
	protected := make([]bool, n)
	for _, g := range d.Groups {
		if !g.Enabled {
			continue
		}
		var before []uint16
		if g.Operation == "smooth" {
			before = append([]uint16(nil), r.LayerMap...)
		}
		for _, span := range g.Mask {
			for i := span.Start; i < span.Start+span.Length; i++ {
				if i%16384 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				if protected[i] {
					continue
				}
				p := base.Image.PixOffset(base.Image.Rect.Min.X+i%w, base.Image.Rect.Min.Y+i/w)
				if base.Image.Pix[p+3] == 0 {
					continue
				}
				v := int(r.LayerMap[i])
				switch g.Operation {
				case "assign":
					v = g.Value
				case "shift":
					if v == 0 {
						continue
					}
					v = max(lo, min(hi, v+g.Value))
				case "cut":
					v = 0
				case "restore":
					v = int(base.LayerMap[i])
				case "smooth":
					if v == 0 {
						continue
					}
					sum, count := 0, 0
					for y := max(0, i/w-g.Value); y <= min(d.Height-1, i/w+g.Value); y++ {
						for x := max(0, i%w-g.Value); x <= min(w-1, i%w+g.Value); x++ {
							l := int(before[y*w+x])
							if l > 0 {
								sum += l
								count++
							}
						}
					}
					if count > 0 {
						v = max(lo, min(hi, int(math.Round(float64(sum)/float64(count)))))
					}
				}
				r.LayerMap[i] = uint16(v)
				if v == 0 {
					r.Image.Pix[p], r.Image.Pix[p+1], r.Image.Pix[p+2], r.Image.Pix[p+3] = 0, 0, 0, 0
				} else {
					c, ok := colors[v]
					if !ok {
						return nil, fmt.Errorf("generated stack has no color for layer %d", v)
					}
					r.Image.Pix[p], r.Image.Pix[p+1], r.Image.Pix[p+2], r.Image.Pix[p+3] = c.RGB[0], c.RGB[1], c.RGB[2], base.Image.Pix[p+3]
				}
			}
		}
		if g.Locked {
			for _, s := range g.Mask {
				for i := s.Start; i < s.Start+s.Length; i++ {
					protected[i] = true
				}
			}
		}
	}
	counts := make([]float64, hi+1)
	total, sum, square, maxError := 0., 0., 0., 0.
	changed := 0
	for i, l := range r.LayerMap {
		if i%16384 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if l != base.LayerMap[i] {
			changed++
		}
		p := r.Image.PixOffset(r.Image.Rect.Min.X+i%w, r.Image.Rect.Min.Y+i/w)
		a := float64(r.Image.Pix[p+3]) / 255
		if a == 0 {
			continue
		}
		counts[l] += a
		total += a
		if src != nil {
			sp := src.PixOffset(src.Rect.Min.X+i%w, src.Rect.Min.Y+i/w)
			c := RGB{src.Pix[sp], src.Pix[sp+1], src.Pix[sp+2]}
			out := RGB{r.Image.Pix[p], r.Image.Pix[p+1], r.Image.Pix[p+2]}
			e := math.Sqrt(distance(ToLab(c), ToLab(out)))
			sum += e * a
			square += e * e * a
			maxError = math.Max(maxError, e)
		}
	}
	if total == 0 {
		return nil, fmt.Errorf("keep at least one printable pixel in the image")
	}
	r.Palette = []PaletteEntry{}
	for i, c := range stack.LayerColors {
		f := counts[c.Layer] / total
		stack.LayerColors[i].Fraction = f
		if f == 0 {
			continue
		}
		p := entry(c.RGB, f, o.NeutralChroma)
		p.PixelFraction = f
		p.StackLayer = c.Layer
		p.StackHeight = c.Height
		p.TopPosition = c.TopPosition
		r.Palette = append(r.Palette, p)
	}
	r.Quality = Quality{sum / total, math.Sqrt(square / total), maxError}
	stack.RMS = r.Quality.RMS
	// Optimizer diagnostics describe the generated assignment, not manual edits.
	stack.Surface = nil
	stack.ColorOrder = nil
	stack.LayerPreference = nil
	stack.DepthSelection = nil
	stack.OptimizationScore = r.Quality.RMS
	stack.OptimizationMetric = "manual region edits: source Delta E76"
	r.UniqueColors = usedPaletteColorCount(r.Palette)
	r.SHA256 = fmt.Sprintf("%x", sha256.Sum256(r.Image.Pix))
	r.RegionEdits = &RegionSummary{len(d.Groups), changed, "original source, visible edited pixels", "Manual edits can exceed the color budget and change preferred ordering. The physical filament stack is unchanged."}
	if r.ColorPop != nil {
		info := *r.ColorPop
		info.QualityReference = "original source after manual region edits"
		info.Warning = "Region edits can move pixels outside their generated Color Pop bands."
		r.ColorPop = &info
	}
	r.StackView = buildStackCoreView(&r)
	var err error
	r.SurfaceView, err = BuildSurfaceView(ctx, &r, o)
	if err != nil {
		return nil, err
	}
	r.Calibration, err = calibrationInfo(ctx, &r, o)
	return &r, err
}

// Display-only changes may reframe an edited result. Changes that regenerate
// assignments require an explicit reset of region edits first.
func SameRegionPlanOptions(a, b Options) bool { return processingKey(a) == processingKey(b) }
