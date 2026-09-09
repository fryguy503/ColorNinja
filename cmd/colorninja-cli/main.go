// colorninja-cli exposes the same Go engine used by the desktop app.
package main

import (
	"colorninja/internal/engine"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

type stringsFlag []string

func (f *stringsFlag) String() string     { return strings.Join(*f, ",") }
func (f *stringsFlag) Set(v string) error { *f = append(*f, v); return nil }
func run() error {
	o := engine.DefaultOptions()
	var output, report, layerMap, library, optionsPath, hfp string
	var includeUnowned, allowSecondary, avoidSilkMetallic, stack, full, force, quiet bool
	var materials stringsFlag
	colors := 0
	chunkPixels := 131072
	f := flag.NewFlagSet("ColorNinja", flag.ContinueOnError)
	f.StringVar(&output, "o", "", "output PNG")
	f.StringVar(&output, "output", "", "output PNG")
	f.IntVar(&colors, "colors", 0, "color budget per population, or maximum filament anchors")
	f.BoolVar(&o.TotalColors, "total-colors", false, "use --colors as a total limit including neutrals in standard mode")
	f.StringVar(&o.ColorPriority, "color-priority", "balanced", "palette priority: balanced, distinctive, or vivid; priority modes share the full color budget")
	f.Float64Var(&o.SmoothingColorSigma, "smoothing-color-tolerance", 0, "edge-aware smoothing color tolerance (0 = original 5; stronger texture smoothing: 10; maximum 25)")
	f.IntVar(&o.AnalysisMaxPixels, "analysis-max-pixels", o.AnalysisMaxPixels, "palette analysis pixel ceiling (0 = every pixel)")
	f.BoolVar(&full, "full-analysis", false, "analyze every source pixel")
	f.Float64Var(&o.PreblurSigma, "preblur-sigma", o.PreblurSigma, "edge-aware smoothing radius in source pixels (0 disables smoothing)")
	f.BoolVar(&o.LegacyColorPipeline, "legacy-color-pipeline", false, "compatibility: use CIELAB matching and palette-discovery-only blur")
	f.Float64Var(&o.NeutralChroma, "neutral-chroma", o.NeutralChroma, "achromatic threshold in Lab")
	minimum := o.MinClusterFraction * 100
	f.Float64Var(&minimum, "min-cluster-percent", minimum, "minimum cluster population percentage")
	f.IntVar(&o.HistogramBits, "histogram-bits", o.HistogramBits, "RGB histogram precision (3–7)")
	f.IntVar(&o.Iterations, "iterations", o.Iterations, "clustering iteration limit")
	f.IntVar(&chunkPixels, "chunk-pixels", chunkPixels, "compatibility flag; Go mapping uses bounded parallel rows")
	f.StringVar(&report, "palette-json", "", "save palette, settings, metrics, and provenance")
	f.StringVar(&library, "hueforge-library", "", "HueForge filament library JSON")
	f.StringVar(&library, "filament-library", "", "alias for --hueforge-library")
	f.BoolVar(&includeUnowned, "hueforge-include-unowned", false, "allow unowned filaments")
	f.Var(&materials, "hueforge-type", "allow material type (repeatable)")
	f.BoolVar(&allowSecondary, "hueforge-allow-secondary", false, "use primary color of dual-color filaments")
	f.BoolVar(&avoidSilkMetallic, "hueforge-avoid-silk-metallic", false, "exclude silk, metallic, pearl, Elixir, and Starlight finishes by material, name, or tags")
	f.BoolVar(&stack, "hueforge-stack", false, "plan one global filament stack")
	f.StringVar(&layerMap, "hueforge-height-map", "", "write 16-bit layer-index PNG (stack mode)")
	f.StringVar(&hfp, "hueforge-project", "", "write a self-contained .hfp project (Front Lit stack mode)")
	f.IntVar(&o.HueForge.MaxRuns, "hueforge-max-runs", 0, "maximum contiguous filament runs, allowing returns; 0 keeps one run per filament")
	f.StringVar(&o.HueForge.MeshMode, "hueforge-mesh-mode", "color-match", "HFP mesh mode: color-match, combo, color-aware, color-pop (other modes rebuild heights in HueForge)")
	f.StringVar(&o.HueForge.MeshCore, "hueforge-mesh-core", "planned-colors", "HFP Color Match mesh core: planned-colors or filament-blends")
	f.Float64Var(&o.HueForge.ExportWidthMM, "hueforge-width-mm", 200, "HFP width in mm; aspect ratio is retained")
	f.Float64Var(&o.HueForge.MeshDetailMM, "hueforge-mesh-detail-mm", .2, "HFP mesh detail spacing in mm")
	f.Float64Var(&o.GuidanceStrength, "hueforge-guidance-strength", o.GuidanceStrength, "pull toward filament-derived hues (0–1)")
	f.BoolVar(&o.TrueBlack, "true-black", o.TrueBlack, "use #000000 for black filaments (set --true-black=false for library colors)")
	f.BoolVar(&o.PreserveDetails, "preserve-details", o.PreserveDetails, "protect small shapes and coherent color groups; smoothing works with either setting")
	f.Float64Var(&o.HueForge.LayerHeight, "hueforge-layer-height", o.HueForge.LayerHeight, "layer height in mm")
	f.Float64Var(&o.HueForge.FirstLayerHeight, "hueforge-first-layer-height", o.HueForge.FirstLayerHeight, "first layer height in mm (0 uses regular height)")
	f.StringVar(&o.HueForge.OpticalModel, "hueforge-optical-model", o.HueForge.OpticalModel, "hueforge-0.9.4.3-frontlit-v1 or legacy-exponential")
	f.StringVar(&o.HueForge.LightPreset, "hueforge-light", o.HueForge.LightPreset, "hueforge-default, neutral-white, or warm-white")
	f.Float64Var(&o.HueForge.BaseDepth, "hueforge-base-depth", o.HueForge.BaseDepth, "base depth in mm")
	f.Float64Var(&o.HueForge.MaxDepth, "hueforge-max-depth", o.HueForge.MaxDepth, "maximum total depth in mm")
	f.BoolVar(&o.HueForge.AutoDepth, "hueforge-auto-depth", false, "choose the thinnest Front Lit stack within 1% of the best color score found, below --hueforge-max-depth")
	f.IntVar(&o.HueForge.AnalysisColors, "hueforge-analysis-colors", o.HueForge.AnalysisColors, "analysis colors per population")
	f.IntVar(&o.HueForge.MaxPerceivedColors, "hueforge-max-perceived-colors", o.HueForge.MaxPerceivedColors, "maximum output colors")
	f.IntVar(&o.HueForge.BeamWidth, "hueforge-beam-width", o.HueForge.BeamWidth, "stack search beam width")
	f.Float64Var(&o.HueForge.TDTransmission, "hueforge-td-transmission", o.HueForge.TDTransmission, "transmission at one scaled TD")
	f.Float64Var(&o.HueForge.TDScale, "hueforge-td-scale", o.HueForge.TDScale, "front-lit TD scale")
	f.Float64Var(&o.HueForge.BaseTransmissionLimit, "hueforge-base-transmission-limit", o.HueForge.BaseTransmissionLimit, "maximum base transmission")
	f.BoolVar(&force, "force", false, "replace existing outputs")
	f.BoolVar(&quiet, "quiet", false, "suppress summary")
	f.StringVar(&optionsPath, "options-json", "", "load a Go Options JSON object (overrides processing flags)")
	f.Usage = func() {
		fmt.Fprintln(f.Output(), "ColorNinja Go · perceptual image reduction\nUsage: colorninja-cli input.png -o output.png [options]")
		f.PrintDefaults()
	}
	args := os.Args[1:]
	input := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		input = args[0]
		args = args[1:]
	}
	if e := f.Parse(args); e != nil {
		if e == flag.ErrHelp {
			return nil
		}
		return e
	}
	if input == "" && len(f.Args()) == 1 {
		input = f.Args()[0]
	} else if len(f.Args()) != 0 {
		return fmt.Errorf("provide one input image; place options before or after that first argument")
	}
	if input == "" {
		f.Usage()
		return fmt.Errorf("input image is required")
	}
	o.MinClusterFraction = minimum / 100
	if full {
		o.AnalysisMaxPixels = 0
	}
	if library != "" {
		o.Mode = "guided"
		if stack {
			o.Mode = "stack"
		}
	} else if stack || layerMap != "" || includeUnowned || allowSecondary || avoidSilkMetallic || len(materials) > 0 {
		return fmt.Errorf("filament options require --hueforge-library")
	}
	if colors == 0 {
		if library != "" {
			o.Colors = 4
		}
	} else {
		o.Colors = colors
	}
	if chunkPixels < 1 {
		return fmt.Errorf("chunk-pixels must be positive")
	}
	if optionsPath != "" {
		raw, e := os.ReadFile(optionsPath)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &o); e != nil {
			return e
		}
	}
	if layerMap != "" && o.Mode != "stack" {
		return fmt.Errorf("height map requires explicit --hueforge-stack")
	}
	if hfp != "" && (o.Mode != "stack" || o.HueForge.OpticalModel != engine.FrontlitModel) {
		return fmt.Errorf("HFP export requires Front Lit stack mode")
	}
	if hfp != "" && !strings.EqualFold(filepath.Ext(hfp), ".hfp") {
		return fmt.Errorf("HueForge project must end in .hfp")
	}
	if e := o.Validate(); e != nil {
		return e
	}
	if output == "" {
		output = strings.TrimSuffix(input, filepath.Ext(input)) + "-colorninja.png"
	}
	if e := engine.DistinctPaths(input, output, report, layerMap, library, optionsPath, hfp); e != nil {
		return e
	}
	for _, path := range []string{output, layerMap} {
		if path != "" && !strings.EqualFold(filepath.Ext(path), ".png") {
			return fmt.Errorf("PNG output must end in .png: %s", path)
		}
	}
	if !force {
		for _, path := range []string{output, report, layerMap, hfp} {
			if path != "" {
				if _, e := os.Stat(path); e == nil {
					return fmt.Errorf("output already exists: %s (use --force to replace)", path)
				}
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	start := time.Now()
	source, e := engine.LoadImage(ctx, input)
	if e != nil {
		return e
	}
	var lib *engine.Library
	if o.Mode != "standard" {
		l, e := engine.LoadLibrary(library, engine.LibraryFilter{IncludeUnowned: includeUnowned, MaterialTypes: materials, AllowSecondary: allowSecondary, AvoidSilkMetallic: avoidSilkMetallic})
		if e != nil {
			return e
		}
		lib = &l
	}
	result, e := engine.Process(ctx, source.Image, o, lib, nil)
	if e != nil {
		return e
	}
	if e = engine.SavePNG(ctx, output, result, source.Metadata, force); e != nil {
		return e
	}
	if report != "" {
		if e = engine.SaveReport(report, input, result, o, source.Metadata, force); e != nil {
			return fmt.Errorf("PNG saved to %s; palette report failed: %w", output, e)
		}
	}
	if layerMap != "" {
		if e = engine.SaveLayerMap(ctx, layerMap, result, force); e != nil {
			return fmt.Errorf("PNG saved to %s; layer map failed: %w", output, e)
		}
	}
	if hfp != "" {
		if e = engine.SaveHFP(ctx, hfp, result, input, source.Metadata, force); e != nil {
			return fmt.Errorf("PNG saved to %s; HFP export failed: %w", output, e)
		}
	}
	if !quiet {
		if hfp != "" {
			fmt.Printf("HFP: %s (mesh mode %s)\n", hfp, o.HueForge.MeshMode)
		}
		fmt.Printf("Saved %s · %d × %d · %d colors · %.2f s\n", output, result.SourceSize[0], result.SourceSize[1], len(result.Palette), time.Since(start).Seconds())
		fmt.Printf("Mean ΔE76 %.4f · RMS %.4f · RGBA SHA256 %s\n", result.Quality.Mean, result.Quality.RMS, result.SHA256)
		if result.Guidance != nil {
			fmt.Printf("Guided by %d owned/eligible filaments; no global stack promised.\n", len(result.Guidance.Selected))
		}
		if result.Stack != nil {
			fmt.Printf("Stack: %d filaments · %d runs · %.2f mm total\n", result.Stack.UniqueFilaments, len(result.Stack.Runs), result.Stack.PlannedDepth)
		}
	}
	for _, warning := range source.Metadata.Warnings {
		fmt.Fprintln(os.Stderr, "Warning:", warning)
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Error:", e)
		os.Exit(1)
	}
}
