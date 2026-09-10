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
	HeightMap           HeightMapOptions `json:"heightMap"`
	ColorPop            ColorPopOptions  `json:"colorPop"`
	Colors              int              `json:"colors"`
	TotalColors         bool             `json:"totalColors"`
	ColorPriority       string           `json:"colorPriority"`
	AnalysisMaxPixels   int              `json:"analysisMaxPixels"`
	NeutralChroma       float64          `json:"neutralChroma"`
	MinClusterFraction  float64          `json:"minClusterFraction"`
	HistogramBits       int              `json:"histogramBits"`
	Iterations          int              `json:"iterations"`
	PreblurSigma        float64          `json:"preblurSigma"`
	SmoothingColorSigma float64          `json:"smoothingColorSigma"`
	LegacyColorPipeline bool             `json:"legacyColorPipeline"`
	Mode                string           `json:"mode"`
	GuidanceStrength    float64          `json:"guidanceStrength"`
	TrueBlack           bool             `json:"trueBlack"`
	PreserveDetails     bool             `json:"preserveDetails"`
	ProtectedColors     string           `json:"protectedColors,omitempty"`
	CalibrationNote     string           `json:"calibrationNote,omitempty"`
	HueForge            HueForgeOptions  `json:"hueforge"`
}
type HueForgeOptions struct {
	Border                BorderOptions `json:"border"`
	OpticalModel          string        `json:"opticalModel"`
	FirstLayerHeight      float64       `json:"firstLayerHeight"`
	LightPreset           string        `json:"lightPreset"`
	LayerHeight           float64       `json:"layerHeight"`
	BaseDepth             float64       `json:"baseDepth"`
	MaxDepth              float64       `json:"maxDepth"`
	AutoDepth             bool          `json:"autoDepth"`
	ReduceShowThrough     bool          `json:"reduceShowThrough"`
	OptimizeMaterial      bool          `json:"optimizeMaterial"`
	LayerPreference       string        `json:"layerPreference"`
	ColorOrder            string        `json:"colorOrder,omitempty"`
	ColorOrderWeight      float64       `json:"colorOrderWeight"`
	SearchEffort          string        `json:"searchEffort,omitempty"`
	RequiredFilaments     string        `json:"requiredFilaments,omitempty"`
	BaseFilament          string        `json:"baseFilament,omitempty"`
	HighlightFilament     string        `json:"highlightFilament,omitempty"`
	HighlightOnlyAtTop    bool          `json:"highlightOnlyAtTop"`
	SurfaceColorTolerance float64       `json:"surfaceColorTolerance"`
	DepthTolerance        float64       `json:"depthTolerance"`
	TDSensitivityPercent  float64       `json:"tdSensitivityPercent"`
	AnalysisColors        int           `json:"analysisColors"`
	BeamWidth             int           `json:"beamWidth"`
	MaxRuns               int           `json:"maxRuns"`
	MeshMode              string        `json:"meshMode"`
	MeshCore              string        `json:"meshCore"`
	ExportWidthMM         float64       `json:"exportWidthMm"`
	MeshDetailMM          float64       `json:"meshDetailMm"`
	MaxPerceivedColors    int           `json:"maxPerceivedColors"`
	TDTransmission        float64       `json:"tdTransmission"`
	TDScale               float64       `json:"tdScale"`
	BaseTransmissionLimit float64       `json:"baseTransmissionLimit"`
}

func DefaultOptions() Options {
	return Options{HeightMap: DefaultHeightMapOptions(), ColorPop: DefaultColorPopOptions(), Colors: 32, AnalysisMaxPixels: 6291456, NeutralChroma: 8,
		MinClusterFraction: .005, HistogramBits: 6, Iterations: 24,
		PreblurSigma: 1.5, Mode: "standard", GuidanceStrength: .8, TrueBlack: true, PreserveDetails: true,
		HueForge: HueForgeOptions{Border: DefaultBorderOptions(), ColorOrderWeight: 50, MeshCore: "compact-blends", SearchEffort: "preview", SurfaceColorTolerance: 5, DepthTolerance: 1, OpticalModel: FrontlitModel, FirstLayerHeight: .16, LightPreset: "hueforge-default", LayerHeight: .08, BaseDepth: .48, MaxDepth: 2.24, AnalysisColors: 32, BeamWidth: 24, MaxPerceivedColors: 64, TDTransmission: .05, TDScale: .1, BaseTransmissionLimit: .1}}
}

// Older projects, presets, and preferences lack the new preservation options.
// Enable them when decoding a fresh Options value, but respect explicit false and the
// CLI's preinitialized options when an options file only overrides some fields.
func (o *Options) UnmarshalJSON(data []byte) error {
	type plainOptions Options
	value := plainOptions(*o)
	value.ColorPop = DefaultColorPopOptions()
	value.HeightMap = DefaultHeightMapOptions()
	if *o == (Options{}) {
		value.TrueBlack = true
		value.PreserveDetails = true
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*o = Options(value)
	o.HueForge.MeshCore = o.HueForge.meshCore()
	return nil
}

// Missing model/first-layer fields identify a saved legacy calculation. Keep
// its optical model and layer geometry when decoding into initialized defaults.
func (o *HueForgeOptions) UnmarshalJSON(data []byte) error {
	type plain HueForgeOptions
	v := plain(*o)
	v.OpticalModel = LegacyModel
	v.FirstLayerHeight = 0
	v.LightPreset = ""
	v.MaxRuns = 0
	v.AutoDepth = false
	v.ReduceShowThrough = false
	v.OptimizeMaterial = false
	v.LayerPreference = ""
	v.ColorOrder, v.ColorOrderWeight = "", 50
	v.HighlightOnlyAtTop = false
	v.Border = DefaultBorderOptions()
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	// Dev.1 exposed material reduction as the layer-order control. Upgrade that
	// saved choice, but respect an explicit new preference (including off).
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, present := fields["layerPreference"]; !present && v.OptimizeMaterial {
		v.LayerPreference, v.OptimizeMaterial = "auto", false
		// The replacement is an appearance preset. Start it with the normal
		// 5% allowance; explicit new-format tolerances remain authoritative.
		v.SurfaceColorTolerance = max(5, v.SurfaceColorTolerance)
	}
	*o = HueForgeOptions(v)
	o.MeshCore = o.meshCore()
	return nil
}

// Beta 5's default named the goal (planned heights), not a user preference for
// opaque IMAGE entries. Upgrade that choice in saved settings/projects too.
// The old 0.01 TD implementation remains an explicit compatibility option.
func (o HueForgeOptions) meshCore() string {
	if o.MeshCore == "" || o.MeshCore == "planned-colors" {
		return "compact-blends"
	}
	return o.MeshCore
}
func (o Options) selectionFraction() float64 {
	// Analysis has already culled noise. Do not erase surviving small details
	// a second time while choosing constrained colors.
	if o.PreserveDetails || o.prioritizeColors() || o.ProtectedColors != "" {
		return 0
	}
	return o.MinClusterFraction
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (o Options) Validate() error {
	if err := o.HueForge.validateColorOrder(); err != nil {
		return err
	}
	if err := o.HeightMap.validate(o); err != nil {
		return err
	}
	if !validLayerPreference(o.HueForge.LayerPreference) {
		return fmt.Errorf("layer preference must be auto, red, yellow, green, cyan, blue, purple, or empty")
	}
	if err := o.ColorPop.validate(o); err != nil {
		return err
	}
	if len(o.CalibrationNote) > 1000 {
		return fmt.Errorf("calibration note must be at most 1000 bytes")
	}
	if err := validateProtectedColors(o); err != nil {
		return err
	}
	if o.ColorPriority != "" && o.ColorPriority != "balanced" && o.ColorPriority != "distinctive" && o.ColorPriority != "vivid" {
		return fmt.Errorf("color priority must be balanced, distinctive, or vivid")
	}
	if o.prioritizeColors() && o.LegacyColorPipeline {
		return fmt.Errorf("color priority requires the current color pipeline; disable legacy color matching")
	}
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
	if !finite(o.SmoothingColorSigma) || o.SmoothingColorSigma < 0 || o.SmoothingColorSigma > 25 {
		return fmt.Errorf("smoothing color tolerance must be between 0 and 25; 0 uses the original tolerance of 5")
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
func (o HueForgeOptions) frontlit() bool         { return o.OpticalModel == FrontlitModel }
func (o HueForgeOptions) backlit() bool          { return o.OpticalModel == BacklitModel }
func (o HueForgeOptions) compatibleOptics() bool { return o.frontlit() || o.backlit() }
func (o HueForgeOptions) SupportsHFP() bool      { return o.compatibleOptics() }
func (o HueForgeOptions) FirstHeight() float64 {
	if o.FirstLayerHeight > 0 {
		return o.FirstLayerHeight
	}
	return o.LayerHeight
}
func (o HueForgeOptions) Height(layer int) float64 {
	if layer <= 0 {
		return 0
	}
	return o.FirstHeight() + float64(layer-1)*o.LayerHeight
}
func (o HueForgeOptions) layerCount(depth float64) float64 {
	return 1 + (depth-o.FirstHeight())/o.LayerHeight
}
func (o HueForgeOptions) BaseLayers() int { return int(math.Round(o.layerCount(o.BaseDepth))) }
func (o HueForgeOptions) MaxLayers() int {
	if o.AutoDepth {
		// Never round a requested physical ceiling up to a thicker layer.
		return int(math.Floor(o.layerCount(o.MaxDepth) + 1e-9))
	}
	return int(math.Round(o.layerCount(o.MaxDepth)))
}
func (o HueForgeOptions) TransitionLayers() int { return o.MaxLayers() - o.BaseLayers() }
func (o HueForgeOptions) Validate() error {
	if err := o.Border.validate(o); err != nil {
		return err
	}
	if err := o.validateColorOrder(); err != nil {
		return err
	}
	if !finite(o.TDSensitivityPercent) || o.TDSensitivityPercent < 0 || o.TDSensitivityPercent > 25 {
		return fmt.Errorf("TD sensitivity must be between 0 and 25 percent")
	}
	if o.SearchEffort != "" && o.SearchEffort != "preview" && o.SearchEffort != "refine" {
		return fmt.Errorf("search effort must be preview or refine")
	}
	if !finite(o.SurfaceColorTolerance) || o.SurfaceColorTolerance < 0 || o.SurfaceColorTolerance > 50 || !finite(o.DepthTolerance) || o.DepthTolerance < 0 || o.DepthTolerance > 50 {
		return fmt.Errorf("color and depth tolerances must be between 0 and 50 percent")
	}
	if len(o.RequiredFilaments) > 8192 || len(o.BaseFilament) > 256 || len(o.HighlightFilament) > 256 {
		return fmt.Errorf("filament constraints are too long")
	}
	if o.AutoDepth && !o.compatibleOptics() {
		return fmt.Errorf("automatic depth requires a HueForge optical model")
	}
	if o.MeshMode != "" && o.MeshMode != "color-match" && o.MeshMode != "combo" && o.MeshMode != "color-aware" && o.MeshMode != "color-pop" {
		return fmt.Errorf("unknown HueForge mesh mode %q", o.MeshMode)
	}
	if o.meshCore() != "legacy-flat" && o.meshCore() != "filament-blends" && o.meshCore() != "compact-blends" {
		return fmt.Errorf("unknown HueForge mesh core %q", o.MeshCore)
	}
	if !finite(o.ExportWidthMM) || o.ExportWidthMM < 0 || o.ExportWidthMM > 2000 || !finite(o.MeshDetailMM) || o.MeshDetailMM < 0 || o.MeshDetailMM > 10 {
		return fmt.Errorf("HFP width must be 0–2000 mm and mesh detail 0–10 mm; 0 uses defaults")
	}
	if o.MaxRuns < 0 || o.MaxRuns > 64 {
		return fmt.Errorf("maximum filament runs must be 0–64; 0 keeps one run per filament")
	}
	if o.LightPreset != "" && o.LightPreset != "hueforge-default" && o.LightPreset != "neutral-white" && o.LightPreset != "warm-white" {
		return fmt.Errorf("unknown light preset %q", o.LightPreset)
	}
	if o.OpticalModel != "" && o.OpticalModel != LegacyModel && !o.compatibleOptics() {
		return fmt.Errorf("unknown optical model %q", o.OpticalModel)
	}
	if !finite(o.FirstLayerHeight) || o.FirstLayerHeight < 0 {
		return fmt.Errorf("first layer height must be finite and non-negative")
	}
	for _, v := range []float64{o.LayerHeight, o.BaseDepth, o.MaxDepth, o.TDScale} {
		if !finite(v) || v <= 0 {
			return fmt.Errorf("layer height, depths, and TD scale must be finite and positive")
		}
	}
	b, m := o.layerCount(o.BaseDepth), o.layerCount(o.MaxDepth)
	if !finite(b) || !finite(m) || m > 4096 || b < 1 || math.Abs(b-math.Round(b)) > 1e-8 || (!o.AutoDepth && math.Abs(m-math.Round(m)) > 1e-8) || o.MaxLayers() <= o.BaseLayers() {
		return fmt.Errorf("depths must equal first layer height plus whole regular layers, with at least one base layer and one top layer, at most 4096 layers")
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
	RegionEdits  *RegionSummary   `json:"regionEdits,omitempty"`
	HeightMap    *HeightMapInfo   `json:"heightMap,omitempty"`
	ColorPop     *ColorPopInfo    `json:"colorPop,omitempty"`
	Calibration  *CalibrationInfo `json:"calibration,omitempty"`
	SurfaceView  *SurfaceView     `json:"surfaceView,omitempty"`
	Image        *image.NRGBA     `json:"-"`
	Palette      []PaletteEntry   `json:"palette"`
	SourceSize   [2]int           `json:"sourceSize"`
	AnalysisSize [2]int           `json:"analysisSize"`
	UniqueColors int              `json:"uniqueColors"`
	Quality      Quality          `json:"quality"`
	SHA256       string           `json:"rgbaSHA256"`
	Guidance     *GuidancePlan    `json:"guidance,omitempty"`
	Stack        *StackPlan       `json:"stack,omitempty"`
	StackView    *StackCoreView   `json:"stackView,omitempty"`
	LayerMap     []uint16         `json:"-"`
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
