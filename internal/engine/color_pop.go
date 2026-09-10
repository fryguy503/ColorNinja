package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Color Pop is an opt-in treatment. Keeping it separate from Mode preserves
// existing reducer/filament settings, presets, and old project behavior.
type ColorPopOptions struct {
	Enabled       bool    `json:"enabled"`
	Selection     string  `json:"selection"` // existing or selected
	Colors        string  `json:"colors"`    // at most eight #RRGGBB samples
	HueTolerance  float64 `json:"hueTolerance"`
	GrayTolerance int     `json:"grayTolerance"`
	ColorPercent  float64 `json:"colorPercent"`
	GrayOnTop     bool    `json:"grayOnTop"`
	GapLayers     int     `json:"gapLayers"`
}

func DefaultColorPopOptions() ColorPopOptions {
	return ColorPopOptions{Selection: "existing", HueTolerance: 25, GrayTolerance: 8, ColorPercent: 50, GapLayers: 1}
}

func (c ColorPopOptions) validate(o Options) error {
	if !c.Enabled {
		return nil
	}
	if c.Selection != "existing" && c.Selection != "selected" {
		return fmt.Errorf("Color Pop selection must be existing or selected")
	}
	if !finite(c.HueTolerance) || c.HueTolerance < 0 || c.HueTolerance > 180 || c.GrayTolerance < 0 || c.GrayTolerance > 255 {
		return fmt.Errorf("Color Pop hue range must be 0–180 degrees and grayscale tolerance 0–255")
	}
	if !finite(c.ColorPercent) || c.ColorPercent < 10 || c.ColorPercent > 90 || c.GapLayers < 0 || c.GapLayers > 8 {
		return fmt.Errorf("Color Pop color height must be 10–90 percent and the gap 0–8 layers")
	}
	if len(c.Colors) > 128 || len(keys(c.Colors)) > 8 {
		return fmt.Errorf("Color Pop supports at most eight color samples")
	}
	for _, s := range keys(c.Colors) {
		if _, err := popParseRGB(s); err != nil {
			return err
		}
	}
	if o.Mode == "guided" {
		return fmt.Errorf("choose Prepare image or Plan filament stack for Color Pop")
	}
	if o.Mode == "stack" && !o.HueForge.frontlit() {
		return fmt.Errorf("Color Pop print planning requires the HueForge Front Lit model")
	}
	return nil
}

func popParseRGB(s string) (RGB, error) {
	var c RGB
	if len(s) != 7 || s[0] != '#' {
		return c, fmt.Errorf("Color Pop samples must be #RRGGBB colors")
	}
	for i := 0; i < 3; i++ {
		v, e := strconv.ParseUint(s[1+i*2:3+i*2], 16, 8)
		if e != nil || strings.ContainsAny(s[1+i*2:3+i*2], "+- ") {
			return c, fmt.Errorf("invalid Color Pop sample %q", s)
		}
		c[i] = uint8(v)
	}
	return c, nil
}

func popHue(c RGB) float64 {
	r, g, b := float64(c[0]), float64(c[1]), float64(c[2])
	hi, lo := max(r, g, b), min(r, g, b)
	if hi == lo {
		return 0
	}
	h := (r-g)/(hi-lo) + 4
	if hi == r {
		h = (g - b) / (hi - lo)
	} else if hi == g {
		h = (b-r)/(hi-lo) + 2
	}
	return math.Mod(h*60+360, 360)
}

func popGray(c RGB) uint8 {
	l := LinearRGB(c)
	y := .2126*l[0] + .7152*l[1] + .0722*l[2]
	return FromLinear(Vec{y, y, y})[0]
}

type ColorPopInfo struct {
	ColorFraction    float64 `json:"colorFraction"`
	GrayFraction     float64 `json:"grayFraction"`
	SelectionPNG     []byte  `json:"selectionPng"` // bounded thumbnail, also portable
	Warning          string  `json:"warning,omitempty"`
	ColorLayers      [2]int  `json:"colorLayers"`
	GrayLayers       [2]int  `json:"grayLayers"`
	GapLayers        int     `json:"gapLayers"`
	QualityReference string  `json:"qualityReference"`
}

// Classification reads the original normalized source, before any smoothing or
// palette reduction. Thus a nearby gray pixel cannot become selected by blur.
func prepareColorPop(ctx context.Context, src *image.NRGBA, o Options, progress Reporter) (*image.NRGBA, []byte, *ColorPopInfo, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w == 0 || h == 0 {
		return nil, nil, nil, fmt.Errorf("image dimensions must be positive")
	}
	selected := []float64{}
	for _, s := range keys(o.ColorPop.Colors) {
		c, err := popParseRGB(s)
		if err != nil {
			return nil, nil, nil, err
		}
		selected = append(selected, popHue(c))
	}
	prepared := image.NewNRGBA(image.Rect(0, 0, w, h))
	mask := make([]byte, w*h)
	info := &ColorPopInfo{QualityReference: "prepared Color Pop image"}
	total, colorMass := 0., 0.
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		for x := 0; x < w; x++ {
			c := src.NRGBAAt(x+src.Rect.Min.X, y+src.Rect.Min.Y)
			rgb := RGB{c.R, c.G, c.B}
			keep := int(max(c.R, c.G, c.B))-int(min(c.R, c.G, c.B)) > o.ColorPop.GrayTolerance
			if keep && o.ColorPop.Selection == "selected" {
				keep = false
				hue := popHue(rgb)
				for _, target := range selected {
					d := math.Abs(hue - target)
					if min(d, 360-d) <= o.ColorPop.HueTolerance {
						keep = true
						break
					}
				}
			}
			if c.A > 0 {
				mass := float64(c.A) / 255
				total += mass
				if keep {
					mask[y*w+x] = 1
					colorMass += mass
				}
			}
			if !keep {
				gray := popGray(rgb)
				c.R, c.G, c.B = gray, gray, gray
			}
			prepared.SetNRGBA(x, y, c)
		}
	}
	if total > 0 {
		info.ColorFraction = colorMass / total
		info.GrayFraction = 1 - info.ColorFraction
	}
	if o.ColorPop.Selection == "selected" && len(selected) == 0 {
		info.Warning = "Choose a color from the original image to keep. The current result is grayscale."
	} else if colorMass == 0 {
		info.Warning = "No visible pixels match the color selection. Pick another color or widen the hue range."
	} else if colorMass == total {
		info.Warning = "Every visible pixel is in the color region. Narrow the selection or increase grayscale tolerance to create a grayscale region."
	}
	// A nearest-sampled mask shows selected pixels in white and grayscale pixels
	// in charcoal. Source alpha survives and hidden RGB is never counted.
	tw, th := AnalysisSize(w, h, 256*256)
	thumb := image.NewNRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			sx, sy := min(w-1, x*w/tw), min(h-1, y*h/th)
			i := y*thumb.Stride + x*4
			v := uint8(38)
			if mask[sy*w+sx] != 0 {
				v = 245
			}
			thumb.Pix[i], thumb.Pix[i+1], thumb.Pix[i+2], thumb.Pix[i+3] = v, v, v, prepared.Pix[sy*prepared.Stride+sx*4+3]
		}
	}
	var png bytes.Buffer
	if err := WritePNG(ctx, &png, thumb, [2]float64{}); err != nil {
		return nil, nil, nil, err
	}
	info.SelectionPNG = png.Bytes()
	return prepared, mask, info, nil
}

func processColorPop(ctx context.Context, src *image.NRGBA, o Options, lib *Library, progress Reporter) (*Result, error) {
	if err := report(ctx, progress, "Separating Color Pop regions", .01); err != nil {
		return nil, err
	}
	prepared, mask, info, err := prepareColorPop(ctx, src, o, progress)
	if err != nil {
		return nil, err
	}
	var r *Result
	if o.Mode == "stack" {
		r, err = planColorPop(ctx, prepared, mask, info, o, lib, progress)
	} else {
		r, err = reduceColorPop(ctx, prepared, mask, info, o, progress)
	}
	if err != nil {
		return nil, err
	}
	r.ColorPop = info
	r.UniqueColors, err = CountUniqueColors(ctx, r.Image)
	if err != nil {
		return nil, err
	}
	r.SHA256 = fmt.Sprintf("%x", sha256.Sum256(r.Image.Pix))
	r.StackView = buildStackCoreView(r)
	r.SurfaceView, err = BuildSurfaceView(ctx, r, o)
	if err != nil {
		return nil, err
	}
	r.Calibration, err = calibrationInfo(ctx, r, o)
	if err != nil {
		return nil, err
	}
	return r, report(ctx, progress, "Color Pop ready", 1)
}

// Independent region budgets and mapping prevent palette reduction from
// swallowing a small accent or tinting grayscale. The combined ceiling holds.
func reduceColorPop(ctx context.Context, src *image.NRGBA, mask []byte, info *ColorPopInfo, o Options, progress Reporter) (*Result, error) {
	budget := o.Colors
	if !o.TotalColors {
		budget *= 2
	}
	both := info.ColorFraction > 0 && info.GrayFraction > 0
	if both && budget < 2 {
		return nil, fmt.Errorf("Color Pop needs at least two colors to keep both regions")
	}
	colorBudget := budget
	if both {
		colorBudget = max(1, min(budget-1, int(math.Round(float64(budget)*.5))))
	} else if info.ColorFraction == 0 {
		colorBudget = 0
	}
	budgets := [2]int{budget - colorBudget, colorBudget}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	r := &Result{Image: image.NewNRGBA(src.Bounds()), SourceSize: [2]int{w, h}, AnalysisSize: [2]int{w, h}}
	palette := map[RGB]*PaletteEntry{}
	weight, sum, square := 0., 0., 0.
	for region := 0; region < 2; region++ {
		if budgets[region] == 0 {
			continue
		}
		part := image.NewNRGBA(src.Bounds())
		for y := 0; y < h; y++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for x := 0; x < w; x++ {
				i := y*src.Stride + x*4
				if int(mask[y*w+x]) == region {
					copy(part.Pix[i:i+4], src.Pix[i:i+4])
				}
			}
		}
		plain := o
		plain.ColorPop.Enabled = false
		plain.Mode = "standard"
		plain.TotalColors = true
		plain.Colors = min(256, budgets[region])
		plain.ProtectedColors = ""
		p, err := Process(ctx, part, plain, nil, progress)
		if err != nil {
			return nil, err
		}
		r.AnalysisSize = p.AnalysisSize
		for y := 0; y < h; y++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for x := 0; x < w; x++ {
				if int(mask[y*w+x]) != region {
					continue
				}
				i := y*src.Stride + x*4
				c := RGB{p.Image.Pix[i], p.Image.Pix[i+1], p.Image.Pix[i+2]}
				if region == 0 {
					gray := popGray(c)
					c = RGB{gray, gray, gray}
				}
				a := src.Pix[i+3]
				r.Image.Pix[i], r.Image.Pix[i+1], r.Image.Pix[i+2], r.Image.Pix[i+3] = c[0], c[1], c[2], a
				if a == 0 {
					continue
				}
				m := float64(a) / 255
				weight += m
				d := distance(ToLab(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]}), ToLab(c))
				sum += math.Sqrt(d) * m
				square += d * m
				r.Quality.Max = max(r.Quality.Max, math.Sqrt(d))
				if palette[c] == nil {
					v := entry(c, 0, o.NeutralChroma)
					palette[c] = &v
				}
				palette[c].PixelFraction += m
			}
		}
	}
	for _, p := range palette {
		if weight > 0 {
			p.PixelFraction /= weight
		}
		p.Fraction = p.PixelFraction
		r.Palette = append(r.Palette, *p)
	}
	sort.Slice(r.Palette, func(i, j int) bool { return r.Palette[i].Hex < r.Palette[j].Hex })
	if weight > 0 {
		r.Quality.Mean = sum / weight
		r.Quality.RMS = math.Sqrt(square / weight)
	}
	return r, nil
}
