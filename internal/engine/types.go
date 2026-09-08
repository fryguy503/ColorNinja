// Package engine implements ColorNinja's deterministic image processing in Go.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"math"
)

type RGB [3]uint8

func (c RGB) Hex() string { return fmt.Sprintf("#%02X%02X%02X", c[0], c[1], c[2]) }

type Vec [3]float64

type Options struct {
	Colors             int             `json:"colors"`
	AnalysisMaxPixels  int             `json:"analysisMaxPixels"`
	NeutralChroma      float64         `json:"neutralChroma"`
	MinClusterFraction float64         `json:"minClusterFraction"`
	HistogramBits      int             `json:"histogramBits"`
	Iterations         int             `json:"iterations"`
	PreblurSigma       float64         `json:"preblurSigma"`
	Mode               string          `json:"mode"`
	GuidanceStrength   float64         `json:"guidanceStrength"`
	TrueBlack          bool            `json:"trueBlack"`
	PreserveDetails    bool            `json:"preserveDetails"`
	HueForge           HueForgeOptions `json:"hueforge"`
}
type HueForgeOptions struct {
	LayerHeight           float64 `json:"layerHeight"`
	BaseDepth             float64 `json:"baseDepth"`
	MaxDepth              float64 `json:"maxDepth"`
	AnalysisColors        int     `json:"analysisColors"`
	BeamWidth             int     `json:"beamWidth"`
	MaxPerceivedColors    int     `json:"maxPerceivedColors"`
	TDTransmission        float64 `json:"tdTransmission"`
	TDScale               float64 `json:"tdScale"`
	BaseTransmissionLimit float64 `json:"baseTransmissionLimit"`
}

func DefaultOptions() Options {
	return Options{Colors: 32, AnalysisMaxPixels: 6291456, NeutralChroma: 8,
		MinClusterFraction: .005, HistogramBits: 6, Iterations: 24,
		PreblurSigma: 1.5, Mode: "standard", GuidanceStrength: .8, TrueBlack: true, PreserveDetails: true,
		HueForge: HueForgeOptions{.08, .48, 2.24, 32, 24, 64, .05, .1, .1}}
}

// Older projects, presets, and preferences lack the new preservation options.
// Enable them when decoding a fresh Options value, but respect explicit false and the
// CLI's preinitialized options when an options file only overrides some fields.
func (o *Options) UnmarshalJSON(data []byte) error {
	type plainOptions Options
	value := plainOptions(*o)
	if *o == (Options{}) {
		value.TrueBlack = true
		value.PreserveDetails = true
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*o = Options(value)
	return nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (o Options) Validate() error {
	if o.Colors < 1 || o.Colors > 256 {
		return fmt.Errorf("color budget must be between 1 and 256")
	}
	if o.AnalysisMaxPixels < 0 {
		return fmt.Errorf("analysis pixel limit cannot be negative")
	}
	if !finite(o.NeutralChroma) || o.NeutralChroma < 0 {
		return fmt.Errorf("neutral threshold must be finite and non-negative")
	}
	if !finite(o.MinClusterFraction) || o.MinClusterFraction < 0 || o.MinClusterFraction >= 1 {
		return fmt.Errorf("minimum cluster fraction must be between 0 and 1 (exclusive)")
	}
	if o.HistogramBits < 3 || o.HistogramBits > 7 {
		return fmt.Errorf("histogram precision must be between 3 and 7 bits")
	}
	if o.Iterations < 1 || o.Iterations > 1000 {
		return fmt.Errorf("iterations must be between 1 and 1000")
	}
	if !finite(o.PreblurSigma) || o.PreblurSigma < 0 || o.PreblurSigma > 100 {
		return fmt.Errorf("pre-blur must be between 0 and 100")
	}
	if o.Mode != "standard" && o.Mode != "guided" && o.Mode != "stack" {
		return fmt.Errorf("unknown processing mode %q", o.Mode)
	}
	if !finite(o.GuidanceStrength) || o.GuidanceStrength < 0 || o.GuidanceStrength > 1 {
		return fmt.Errorf("guidance strength must be between 0 and 1")
	}
	if o.Mode != "standard" {
		return o.HueForge.Validate()
	}
	return nil
}
func (o HueForgeOptions) BaseLayers() int       { return int(math.Round(o.BaseDepth / o.LayerHeight)) }
func (o HueForgeOptions) MaxLayers() int        { return int(math.Round(o.MaxDepth / o.LayerHeight)) }
func (o HueForgeOptions) TransitionLayers() int { return o.MaxLayers() - o.BaseLayers() }
func (o HueForgeOptions) Validate() error {
	for _, v := range []float64{o.LayerHeight, o.BaseDepth, o.MaxDepth, o.TDScale} {
		if !finite(v) || v <= 0 {
			return fmt.Errorf("layer height, depths, and TD scale must be finite and positive")
		}
	}
	b, m := o.BaseDepth/o.LayerHeight, o.MaxDepth/o.LayerHeight
	if !finite(b) || !finite(m) || m > 4096 || b < 1 || math.Abs(b-math.Round(b)) > 1e-8 || math.Abs(m-math.Round(m)) > 1e-8 || math.Round(m) <= math.Round(b) {
		return fmt.Errorf("depths must be exact layer-height multiples with at least one base layer and one top layer, at most 4096 layers")
	}
	if o.AnalysisColors < 1 || o.AnalysisColors > 256 || o.MaxPerceivedColors < 1 || o.MaxPerceivedColors > 256 || o.BeamWidth < 1 || o.BeamWidth > 512 {
		return fmt.Errorf("analysis/output colors must be 1–256 and beam width 1–512")
	}
	if !finite(o.TDTransmission) || o.TDTransmission <= 0 || o.TDTransmission >= 1 || !finite(o.BaseTransmissionLimit) || o.BaseTransmissionLimit <= 0 || o.BaseTransmissionLimit > 1 {
		return fmt.Errorf("invalid transmission settings")
	}
	return nil
}

type PaletteEntry struct {
	RGB           RGB     `json:"rgb"`
	Hex           string  `json:"hex"`
	Population    string  `json:"population"`
	Fraction      float64 `json:"analysisFraction"`
	PixelFraction float64 `json:"pixelFraction"`
	StackLayer    int     `json:"stackLayer,omitempty"`
	StackHeight   float64 `json:"stackHeight,omitempty"`
	TopPosition   int     `json:"topPosition,omitempty"`
}

func entry(c RGB, f float64, neutral float64) PaletteEntry {
	l := ToLab(c)
	pop := "chromatic"
	if math.Hypot(l[1], l[2]) < neutral {
		pop = "achromatic"
	}
	return PaletteEntry{RGB: c, Hex: c.Hex(), Population: pop, Fraction: f}
}

type Quality struct {
	Mean float64 `json:"meanDeltaE76"`
	RMS  float64 `json:"rmsDeltaE76"`
	Max  float64 `json:"maxDeltaE76"`
}
type Result struct {
	Image        *image.NRGBA   `json:"-"`
	Palette      []PaletteEntry `json:"palette"`
	SourceSize   [2]int         `json:"sourceSize"`
	AnalysisSize [2]int         `json:"analysisSize"`
	UniqueColors int            `json:"uniqueColors"`
	Quality      Quality        `json:"quality"`
	SHA256       string         `json:"rgbaSHA256"`
	Guidance     *GuidancePlan  `json:"guidance,omitempty"`
	Stack        *StackPlan     `json:"stack,omitempty"`
	LayerMap     []uint16       `json:"-"`
}
type Progress struct {
	Stage    string  `json:"stage"`
	Fraction float64 `json:"fraction"`
}
type Reporter func(Progress)

func report(ctx context.Context, fn Reporter, stage string, fraction float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if fn != nil {
		fn(Progress{stage, fraction})
	}
	return nil
}
