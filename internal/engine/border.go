package engine

import (
	"fmt"
	"math"
)

// BorderOptions controls a HueForge mesh frame independently of image pixels.
// Zero height follows the image top. Old projects remain disabled.
type BorderOptions struct {
	Enabled   bool    `json:"enabled"`
	Placement string  `json:"placement"`
	WidthMM   float64 `json:"widthMm"`
	HeightMM  float64 `json:"heightMm"`
}

func DefaultBorderOptions() BorderOptions { return BorderOptions{Placement: "external", WidthMM: 4} }

func (b BorderOptions) validate(h HueForgeOptions) error {
	if b.Placement != "" && b.Placement != "external" && b.Placement != "internal" {
		return fmt.Errorf("border placement must be external or internal")
	}
	if !finite(b.WidthMM) || b.WidthMM < 0 || b.WidthMM > 100 || !finite(b.HeightMM) || b.HeightMM < 0 || b.HeightMM > 40 {
		return fmt.Errorf("border width must be 0–100 mm and depth 0–40 mm; depth 0 follows the image top")
	}
	if !b.Enabled {
		return nil
	}
	if b.WidthMM < .01 || math.Abs(b.WidthMM*100-math.Round(b.WidthMM*100)) > 1e-7 {
		return fmt.Errorf("enabled border width must be at least 0.01 mm, in 0.01 mm increments")
	}
	if b.HeightMM > 0 && (b.HeightMM < h.FirstHeight() || h.layerCount(b.HeightMM) > 998+1e-7 || math.Abs(b.HeightMM*100-math.Round(b.HeightMM*100)) > 1e-7) {
		return fmt.Errorf("border depth must cover the first layer, use 0.01 mm increments, and fit within 998 layers")
	}
	return nil
}

// Dimensions are nominal: HueForge rounds the image grid to its mesh spacing.
// The continuous frame samples the nearest print layer for its top color.
type BorderView struct {
	Placement     string  `json:"placement"`
	WidthMM       float64 `json:"widthMm"`
	HeightMM      float64 `json:"heightMm"`
	ImageWidthMM  float64 `json:"imageWidthMm"`
	ImageHeightMM float64 `json:"imageHeightMm"`
	OuterWidthMM  float64 `json:"outerWidthMm"`
	OuterHeightMM float64 `json:"outerHeightMm"`
	TopLayer      int     `json:"topLayer"`
	TopRGB        RGB     `json:"topRGB"`
	ExtraLayers   int     `json:"extraLayers"`
	PrintHeightMM float64 `json:"printHeightMm"`
	VolumeMM3     float64 `json:"volumeMm3"`
	FinalFilament string  `json:"finalFilament"`
}

func resolveBorder(r *Result, h HueForgeOptions) (*BorderView, error) {
	b := h.Border
	if !b.Enabled || r.Stack == nil || r.Image == nil || len(r.Stack.Runs) == 0 {
		return nil, nil
	}
	if err := b.validate(h); err != nil {
		return nil, err
	}
	width := h.ExportWidthMM
	if width == 0 {
		width = 200
	}
	w, ih := r.Image.Bounds().Dx(), r.Image.Bounds().Dy()
	if w < 1 || ih < 1 {
		return nil, fmt.Errorf("border requires nonempty image dimensions")
	}
	height := width * float64(ih) / float64(w)
	v := &BorderView{Placement: b.Placement, WidthMM: b.WidthMM, HeightMM: b.HeightMM, ImageWidthMM: width, ImageHeightMM: height}
	if v.Placement == "" {
		v.Placement = "external"
	}
	if v.Placement == "internal" {
		spacing := h.MeshDetailMM
		if spacing == 0 {
			spacing = .2
		}
		// CreateTriangles shrinks proportionally using the short side. Refuse
		// an exhausted grid instead of HueForge's emergency 2x2 clamp.
		short := math.Min(width, height)
		if short-2*b.WidthMM < 2*spacing {
			return nil, fmt.Errorf("internal border leaves too little image area; reduce its width or mesh detail")
		}
		scale := (short - 2*b.WidthMM) / short
		v.ImageWidthMM *= scale
		v.ImageHeightMM *= scale
	}
	v.OuterWidthMM, v.OuterHeightMM = v.ImageWidthMM+2*b.WidthMM, v.ImageHeightMM+2*b.WidthMM
	last := r.Stack.Runs[len(r.Stack.Runs)-1]
	if v.HeightMM == 0 {
		v.HeightMM = h.Height(last.EndLayer)
	}
	// Match the installed shader's float32 subtract/divide/nearest-layer lookup.
	relative := (float32(v.HeightMM) - float32(h.FirstHeight())) / float32(h.LayerHeight)
	v.TopLayer = max(1, int(math.Floor(float64(relative+float32(.5))))+1)
	if v.TopLayer > 998 {
		return nil, fmt.Errorf("border exceeds the 998-layer HFP limit")
	}
	v.ExtraLayers = max(0, v.TopLayer-last.EndLayer)
	v.PrintHeightMM = math.Max(h.Height(last.EndLayer), v.HeightMM)
	v.FinalFilament = fmt.Sprintf("%s · %s · %s · TD %g mm", last.Filament.Brand, last.Filament.Name, last.Filament.Material, last.Filament.TD)
	v.VolumeMM3 = (v.OuterWidthMM*v.OuterHeightMM - v.ImageWidthMM*v.ImageHeightMM) * v.HeightMM
	// Evaluate from the distinct first layer, also supporting below-base
	// borders. Above the image, the last filament continues cumulatively.
	s := newOptics(r.Stack.Runs[0].Filament, h)
	pos := 0
	for layer := 1; layer <= v.TopLayer; layer++ {
		for pos+1 < len(r.Stack.Runs) && layer > r.Stack.Runs[pos].EndLayer {
			pos++
		}
		run := r.Stack.Runs[pos]
		thickness := h.LayerHeight
		if layer == 1 {
			thickness = h.FirstHeight()
		}
		v.TopRGB = s.layer(run.Filament, thickness, h, layer == run.StartLayer)
	}
	return v, nil
}
