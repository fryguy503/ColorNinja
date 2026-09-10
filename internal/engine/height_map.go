package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"math"
	"strings"
)

// HeightMapOptions defines image interpretation independently from the optical
// model and HFP mesh encoding. Empty mode preserves all older Color Match plans.
type HeightMapOptions struct {
	Mode          string     `json:"mode"`
	StandardModel string     `json:"standardModel"`
	Mixing        float64    `json:"mixing"`
	FullRange     bool       `json:"fullRange"`
	Brightness    float64    `json:"brightness"`
	Gamma         float64    `json:"gamma"`
	Invert        bool       `json:"invert"`
	ChannelOrder  string     `json:"channelOrder"`
	ChannelShift  [3]int     `json:"channelShift"`
	Ignore        [3]bool    `json:"ignore"`
	BandWeights   [3]float64 `json:"bandWeights"`
	InvertBands   [3]bool    `json:"invertBands"`
	GapLayers     int        `json:"gapLayers"`
}

func DefaultHeightMapOptions() HeightMapOptions {
	return HeightMapOptions{Mixing: 100, FullRange: true, Gamma: 1, ChannelOrder: "rgb", BandWeights: [3]float64{1, 1, 1}, GapLayers: 1}
}

func (o Options) fixedHeights() bool {
	return o.Mode == "stack" && o.HeightMap.Mode != "" && o.HeightMap.Mode != "color-match"
}

func (h HeightMapOptions) validate(o Options) error {
	switch h.Mode {
	case "", "color-match", "standard", "combo", "max-channel", "scaled-max-channel", "color-aware":
	default:
		return fmt.Errorf("unknown height planning mode %q", h.Mode)
	}
	if !o.fixedHeights() {
		return nil
	}
	if h.StandardModel != "" && h.StandardModel != "rgb-weights" && h.StandardModel != "perceptual" {
		return fmt.Errorf("standard brightness model must be rgb-weights or perceptual")
	}
	if o.ColorPop.Enabled {
		return fmt.Errorf("choose Color Pop or another height workflow, not both")
	}
	if !o.HueForge.compatibleOptics() {
		return fmt.Errorf("height planning requires the HueForge optical model")
	}
	if !finite(h.Mixing) || h.Mixing < 0 || h.Mixing > 100 || !finite(h.Brightness) || h.Brightness < -100 || h.Brightness > 100 || !finite(h.Gamma) || h.Gamma < .1 || h.Gamma > 5 {
		return fmt.Errorf("height mixing must be 0–100, brightness -100–100, and gamma 0.1–5")
	}
	if h.GapLayers < 0 || h.GapLayers > 8 {
		return fmt.Errorf("height band gap must be 0–8 layers")
	}
	if h.Mode == "color-aware" {
		if len(h.ChannelOrder) != 3 || strings.Count(h.ChannelOrder, "r") != 1 || strings.Count(h.ChannelOrder, "g") != 1 || strings.Count(h.ChannelOrder, "b") != 1 {
			return fmt.Errorf("channel order must contain r, g, and b once each, from bottom to top")
		}
		if h.Ignore == [3]bool{true, true, true} {
			return fmt.Errorf("Color Aware needs at least one enabled channel")
		}
		for c := 0; c < 3; c++ {
			if h.ChannelShift[c] < -255 || h.ChannelShift[c] > 255 || !finite(h.BandWeights[c]) || h.BandWeights[c] <= 0 || h.BandWeights[c] > 100 {
				return fmt.Errorf("channel shifts must be -255–255 and band weights greater than zero, at most 100")
			}
		}
	}
	return nil
}

type HeightBand struct {
	Name          string  `json:"name"`
	Channel       int     `json:"channel"`
	StartLayer    int     `json:"startLayer"`
	EndLayer      int     `json:"endLayer"`
	PixelFraction float64 `json:"pixelFraction"`
}
type HeightMapInfo struct {
	Mode    string       `json:"mode"`
	Bands   []HeightBand `json:"bands"`
	Warning string       `json:"warning,omitempty"`
}

// Each region has its own measured brightness range. Transparent pixels never
// affect normalization, channel populations, or material targets.
func heightSample(rgb RGB, h HeightMapOptions) (int, float64) {
	v := [3]float64{float64(rgb[0]), float64(rgb[1]), float64(rgb[2])}
	region := 0
	if h.Mode == "color-aware" {
		for c := range v {
			if h.Ignore[c] {
				v[c] = 0
			}
		}
		// Native shifts run B, G, R. Overflow changes the other channels
		// rather than clipping the selected channel, including negative shifts.
		for _, c := range []int{2, 1, 0} {
			if h.Ignore[c] {
				continue
			}
			n := int(v[c]) + h.ChannelShift[c]
			if n >= 0 && n < 256 {
				v[c] = float64(n)
			} else {
				for other := range v {
					if other != c {
						v[other] = max(0, min(255, v[other]-float64(h.ChannelShift[c])))
					}
				}
			}
		}
		region = 2
		if v[1] < v[0] && v[2] <= v[0] {
			region = 0
		} else if v[2] < v[1] && v[0] <= v[1] {
			region = 1
		}
		// Neutral ties and ignored winners belong to the first enabled band.
		// This also makes a single-channel workflow use only that channel.
		if h.Ignore[region] || max(v[0], v[1], v[2])-min(v[0], v[1], v[2]) <= 8 {
			for _, name := range h.ChannelOrder {
				c := strings.IndexRune("rgb", name)
				if c >= 0 && !h.Ignore[c] {
					region = c
					break
				}
			}
		}
	}
	weights := [3]float64{.3, .55, .15}
	if h.Mode == "color-aware" {
		weights = [3]float64{.33, .34, .33}
		for c := range weights {
			if h.Ignore[c] {
				weights[c] = 0
			}
		}
	}
	standard := (v[1]*weights[1] + v[0]*weights[0] + v[2]*weights[2]) / ((weights[0] + weights[1] + weights[2]) * 256)
	if h.StandardModel == "perceptual" {
		r, g, b := linear(v[0]), linear(v[1]), linear(v[2])
		l := math.Pow(max(0, .5363325363*g+.4122214708*r+.0514459929*b), 1/2.4)
		m := math.Pow(max(0, .2119034982*r+.6806995451*g+.1073969566*b), 1/2.4)
		s := math.Pow(max(0, .2817188376*g+.0883024619*r+.6299787005*b), 1/2.4)
		standard = max(0, min(1, l*.2104542553+m*.793617785-s*.0040720468))
	}
	maximum := max(v[0], v[1], v[2]) / 256
	value := standard
	switch h.Mode {
	case "max-channel":
		value = maximum
	case "scaled-max-channel":
		// HueForge 0.9.4.3 uses the channel mean with a <=32 black cutoff.
		// This is intentionally not a generic smoothing or gamma operation.
		value = (v[0] + v[1] + v[2]) / 256 * float64(float32(1.0/3))
		if max(v[0], v[1], v[2]) <= 32 {
			value = 0
		}
	case "combo":
		mixing := float32(h.Mixing / 100)
		value = standard*float64(mixing) + maximum*float64(1-mixing)
	}
	return region, value
}

func processHeightMap(ctx context.Context, src *image.NRGBA, o Options, lib *Library, progress Reporter) (*Result, error) {
	if lib == nil || len(lib.Filaments) == 0 {
		return nil, fmt.Errorf("load a filament library with eligible filaments first")
	}
	// Normalize nonzero-origin subimages once; every full-pixel scan uses one
	// coordinate convention and can be cancelled between rows.
	normalized := image.NewNRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(normalized, normalized.Bounds(), src, src.Bounds().Min, draw.Src)
	working, err := detailSmoothWithTolerance(ctx, normalized, o.PreblurSigma, o.SmoothingColorSigma, progress)
	if err != nil {
		return nil, err
	}
	if o.TrueBlack {
		copy := trueBlackLibrary(*lib)
		lib = &copy
	}
	r, err := chooseHeightDepth(ctx, o, progress, func(candidate Options, reporter Reporter) (*Result, error) {
		return planHeightImage(ctx, normalized, working, candidate, lib, reporter)
	})
	if err != nil {
		return nil, err
	}
	return finishHeightResult(ctx, r, o, progress)
}

func planHeightImage(ctx context.Context, src, working *image.NRGBA, o Options, lib *Library, progress Reporter) (*Result, error) {
	h := o.HueForge
	if h.MaxLayers() > 998 {
		return nil, fmt.Errorf("height planning supports at most 998 layers")
	}
	mins, maxs, mass := [3]float64{1, 1, 1}, [3]float64{}, [3]float64{}
	w, height := src.Bounds().Dx(), src.Bounds().Dy()
	total := 0.
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*working.Stride + x*4
			if working.Pix[i+3] == 0 {
				continue
			}
			region, value := heightSample(RGB{working.Pix[i], working.Pix[i+1], working.Pix[i+2]}, o.HeightMap)
			m := float64(working.Pix[i+3]) / 255
			mass[region] += m
			total += m
			mins[region] = min(mins[region], value)
			maxs[region] = max(maxs[region], value)
		}
	}
	if total == 0 {
		return nil, fmt.Errorf("height planning needs visible image content")
	}
	channels := []int{0}
	if o.HeightMap.Mode == "color-aware" {
		channels = nil
		for _, name := range o.HeightMap.ChannelOrder {
			c := strings.IndexRune("rgb", name)
			if mass[c] > 0 {
				channels = append(channels, c)
			}
		}
	}
	count := h.MaxLayers() - h.BaseLayers() + 1 - o.HeightMap.GapLayers*(len(channels)-1)
	if count < 2*len(channels) || h.MaxPerceivedColors < len(channels) {
		return nil, fmt.Errorf("increase thickness or surface color budget: each nonempty band needs two layers and one surface color")
	}
	lengths, levels := [3]int{}, [3]int{}
	for _, c := range channels {
		lengths[c], levels[c] = 2, 1
	}
	// Largest deficit apportionment preserves the shared layer/color budgets.
	allocate := func(values *[3]int, budget int, weights [3]float64, caps [3]int) {
		sum, weight := 0, 0.
		for _, c := range channels {
			sum += values[c]
			weight += weights[c]
		}
		for ; sum < budget; sum++ {
			best, deficit := -1, math.Inf(-1)
			for _, c := range channels {
				if values[c] >= caps[c] {
					continue
				}
				d := float64(budget)*weights[c]/weight - float64(values[c])
				if d > deficit {
					best, deficit = c, d
				}
			}
			if best < 0 {
				break
			}
			values[best]++
		}
	}
	weights := o.HeightMap.BandWeights
	if o.HeightMap.Mode != "color-aware" {
		weights = [3]float64{1, 1, 1}
	}
	allocate(&lengths, count, weights, [3]int{count, count, count})
	allocate(&levels, min(count, h.MaxPerceivedColors), weights, lengths)
	info := &HeightMapInfo{Mode: o.HeightMap.Mode}
	starts := [3]int{}
	start := h.BaseLayers()
	for _, c := range channels {
		starts[c] = start
		name := "Brightness"
		if o.HeightMap.Mode == "color-aware" {
			name = [3]string{"Red", "Green", "Blue"}[c]
		}
		info.Bands = append(info.Bands, HeightBand{name, c, start, start + lengths[c] - 1, mass[c] / total})
		start += lengths[c] + o.HeightMap.GapLayers
	}
	if o.HeightMap.Mode == "color-aware" && len(channels) < 3 {
		info.Warning = "Empty color bands are omitted; visible channels share the available height."
	}
	layers := make([]uint16, w*height)
	targets := make([]popTarget, h.MaxLayers()+1)
	planned := h.BaseLayers()
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*working.Stride + x*4
			if working.Pix[i+3] == 0 {
				continue
			}
			region, value := heightSample(RGB{working.Pix[i], working.Pix[i+1], working.Pix[i+2]}, o.HeightMap)
			if o.HeightMap.FullRange {
				if maxs[region] > mins[region] {
					value = (value - mins[region]) / (maxs[region] - mins[region])
				} else {
					value = .5
				}
			}
			value = math.Pow(max(0, min(1, value+o.HeightMap.Brightness/100)), 1/o.HeightMap.Gamma)
			if o.HeightMap.Invert != (o.HeightMap.Mode == "color-aware" && o.HeightMap.InvertBands[region]) {
				value = 1 - value
			}
			if levels[region] > 1 {
				value = math.Round(value*float64(levels[region]-1)) / float64(levels[region]-1)
			} else {
				value = .5
			}
			layer := starts[region] + int(math.Round(value*float64(lengths[region]-1)))
			layers[y*w+x] = uint16(layer)
			planned = max(planned, layer)
			m := float64(src.Pix[i+3]) / 255
			v := o.colorVector(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]})
			targets[layer].mass += m
			for c := range v {
				targets[layer].mean[c] += v[c] * m
			}
		}
	}
	for i := range targets {
		if targets[i].mass > 0 {
			for c := range targets[i].mean {
				targets[i].mean[c] /= targets[i].mass
			}
			targets[i].mass /= total
		}
	}
	r, err := fitHeightStack(ctx, src, [2]int{w, height}, layers, targets, planned, nil, o, lib, progress)
	if err != nil {
		return nil, err
	}
	r.HeightMap = info
	return r, nil
}

func finishHeightResult(ctx context.Context, r *Result, o Options, progress Reporter) (*Result, error) {
	var err error
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
	return r, report(ctx, progress, "Preview ready", 1)
}
