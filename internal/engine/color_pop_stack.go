package engine

import (
	"context"
	"fmt"
	"image"
	"math"
)

type popTarget struct {
	mean Vec
	mass float64
}
type popNode struct {
	parent   *popNode
	id, runs int
	used     []int
	current  opticalState
	score    float64
	order    int
}

// Color Pop assigns brightness to two disjoint layer bands first, then fits
// one physical stack to those fixed surface targets. The ordinary free-color
// stack search cannot preserve this separation, so its algorithm is untouched.
func planColorPop(ctx context.Context, src *image.NRGBA, mask []byte, info *ColorPopInfo, o Options, lib *Library, progress Reporter) (*Result, error) {
	if lib == nil || len(lib.Filaments) == 0 {
		return nil, fmt.Errorf("load a filament library with eligible filaments first")
	}
	if info.ColorFraction == 0 || info.GrayFraction == 0 {
		return nil, fmt.Errorf("Color Pop print planning needs both color and grayscale regions; adjust the selection or use Prepare image")
	}
	if o.Colors < 2 {
		return nil, fmt.Errorf("Color Pop print planning needs at least two filaments for color and grayscale")
	}
	if o.TrueBlack {
		v := trueBlackLibrary(*lib)
		lib = &v
	}
	h := o.HueForge
	count := h.MaxLayers() - h.BaseLayers() + 1 - o.ColorPop.GapLayers
	if count < 4 {
		return nil, fmt.Errorf("increase Color Pop thickness: each region needs at least two layers plus the boundary gap")
	}
	if h.MaxLayers() > 998 {
		return nil, fmt.Errorf("Color Pop supports at most 998 printable layers")
	}
	nc := max(2, min(count-2, int(math.Round(float64(count)*o.ColorPop.ColorPercent/100))))
	ng := count - nc
	start := h.BaseLayers()
	if o.ColorPop.GrayOnTop {
		info.ColorLayers = [2]int{start, start + nc - 1}
		info.GrayLayers = [2]int{start + nc + o.ColorPop.GapLayers, h.MaxLayers()}
	} else {
		info.GrayLayers = [2]int{start, start + ng - 1}
		info.ColorLayers = [2]int{start + ng + o.ColorPop.GapLayers, h.MaxLayers()}
	}
	info.GapLayers = o.ColorPop.GapLayers
	if h.MaxPerceivedColors < 2 {
		return nil, fmt.Errorf("Color Pop print planning needs at least two surface colors")
	}
	// Smooth/reduce separately before brightness assignment, keeping the original
	// mask immutable. This uses the same smoothing controls as Prepare image.
	analysis := o
	analysis.Mode = "standard"
	analysis.Colors = min(256, h.AnalysisColors)
	analysis.TotalColors = true
	flat, err := reduceColorPop(ctx, src, mask, info, analysis, progress)
	if err != nil {
		return nil, err
	}
	w, height := src.Bounds().Dx(), src.Bounds().Dy()
	mins, maxs := [2]int{255, 255}, [2]int{}
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			if src.Pix[i+3] == 0 {
				continue
			}
			region := int(mask[y*w+x])
			v := int(popGray(RGB{flat.Image.Pix[i], flat.Image.Pix[i+1], flat.Image.Pix[i+2]}))
			mins[region] = min(mins[region], v)
			maxs[region] = max(maxs[region], v)
		}
	}
	targets := make([]popTarget, h.MaxLayers()+1)
	layerMap := make([]uint16, w*height)
	budget := min(h.MaxPerceivedColors, count)
	colorLevels := max(1, min(nc, budget-1, int(math.Round(float64(budget)*float64(nc)/float64(count)))))
	grayLevels := min(ng, budget-colorLevels)
	colorLevels = min(nc, budget-grayLevels)
	bands := [2][2]int{info.GrayLayers, info.ColorLayers}
	levels := [2]int{grayLevels, colorLevels}
	total := 0.
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			if src.Pix[i+3] == 0 {
				continue
			}
			region := int(mask[y*w+x])
			rgb := RGB{flat.Image.Pix[i], flat.Image.Pix[i+1], flat.Image.Pix[i+2]}
			v := int(popGray(rgb))
			f := .5
			if maxs[region] > mins[region] {
				f = float64(v-mins[region]) / float64(maxs[region]-mins[region])
			}
			if levels[region] > 1 {
				f = math.Round(f*float64(levels[region]-1)) / float64(levels[region]-1)
			} else {
				f = .5
			}
			layer := bands[region][0] + int(math.Round(f*float64(bands[region][1]-bands[region][0])))
			layerMap[y*w+x] = uint16(layer)
			m := float64(src.Pix[i+3]) / 255
			total += m
			lab := o.colorVector(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]})
			targets[layer].mass += m
			for c := range lab {
				targets[layer].mean[c] += lab[c] * m
			}
		}
	}
	planned := h.BaseLayers()
	for layer := range targets {
		if targets[layer].mass > 0 {
			for c := range targets[layer].mean {
				targets[layer].mean[c] /= targets[layer].mass
			}
			targets[layer].mass /= total
			planned = layer
		}
	}
	return fitHeightStack(ctx, src, flat.AnalysisSize, layerMap, targets, planned, info, o, lib, progress)
}
