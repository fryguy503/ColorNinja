package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"path/filepath"
	"slices"
	"strings"
)

// HFP is HueForge's JSON project format. Slider endpoints are inclusive.
// This export targets compatibility with version 0.9.4.3.
// IMAGE filaments belong only to the virtual mesh core, never the print stack.
type hfpFilament struct {
	Brand    string  `json:"Brand"`
	Color    string  `json:"Color"`
	Name     string  `json:"Name"`
	Owned    bool    `json:"Owned"`
	TD       float64 `json:"Transmissivity"`
	Material string  `json:"Type"`
	UUID     string  `json:"uuid"`
}

func hfpID(identity string) string {
	s := sha256.Sum256([]byte("ColorNinja HFP v1\x00" + identity))
	s[6] = (s[6] & 15) | 128 // UUID v8: application-defined SHA-256 identity.
	s[8] = (s[8] & 63) | 128
	return fmt.Sprintf("{%x-%x-%x-%x-%x}", s[:4], s[4:6], s[6:8], s[8:10], s[10:16])
}

// HueForge may round the maximum depth down when restoring a project. Reserve one
// unused layer of headroom; matching masks it out and the print schedule stays
// unchanged. This also survives saving and reopening the project in HueForge.
func hfpMaxDepth(h HueForgeOptions, layers int) float64 {
	return h.Height(layers + 1)
}

func hfpMaterial(f Filament) hfpFilament {
	id := f.UUID
	// An overridden color must not alias the library's unchanged definition.
	if id == "" || (f.LibraryRGB != nil && *f.LibraryRGB != f.RGB) {
		id = hfpID(fmt.Sprintf("%s|%s|%s|%s|%.17g|%s", f.UUID, f.Brand, f.Name, f.Material, f.TD, f.RGB.Hex()))
	}
	return hfpFilament{f.Brand, f.RGB.Hex(), f.Name, f.Owned, f.TD, f.Material, id}
}

func hueForgeProject(ctx context.Context, r *Result, sourceName string, meta ImageMetadata) (map[string]any, error) {
	if r == nil || r.Stack == nil || r.Image == nil || len(r.Stack.Runs) == 0 {
		return nil, fmt.Errorf("HFP export requires a current global stack preview")
	}
	h := r.Stack.Options
	if err := h.Validate(); err != nil {
		return nil, err
	}
	if !h.frontlit() {
		return nil, fmt.Errorf("HFP export requires the HueForge Front Lit optical model; refresh the preview with that model")
	}
	if h.MaxLayers() > 998 {
		return nil, fmt.Errorf("HFP export supports at most 998 planned layers plus import headroom")
	}
	for _, height := range []float64{h.FirstHeight(), h.LayerHeight} {
		if height < .01 || math.Abs(height*100-math.Round(height*100)) > 1e-8 {
			return nil, fmt.Errorf("HFP export requires first and regular layer heights in 0.01 mm increments to survive HueForge's controls")
		}
	}
	w, height := r.Image.Bounds().Dx(), r.Image.Bounds().Dy()
	if w < 1 || height < 1 || len(r.LayerMap) != w*height {
		return nil, fmt.Errorf("stack image and layer map dimensions disagree")
	}
	mode := h.MeshMode
	if mode == "" {
		mode = "color-match"
	}
	core := h.MeshCore
	if core == "" {
		core = "planned-colors"
	}
	width := h.ExportWidthMM
	if width == 0 {
		width = 200
	}
	detail := h.MeshDetailMM
	if detail == 0 {
		detail = .2
	}
	maxLayer := r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer
	maxDepth := hfpMaxDepth(h, max(maxLayer, h.BaseLayers()+1))
	filaments := make([]hfpFilament, 0, len(r.Stack.Runs))
	sliders := make([]int, 0, len(r.Stack.Runs))
	for _, run := range r.Stack.Runs {
		filaments = append(filaments, hfpMaterial(run.Filament))
		sliders = append(sliders, run.EndLayer)
	}
	// Preserve the simplified regions. A physical mesh has binary coverage;
	// HueForge otherwise premultiplies partial alpha before color matching.
	img := image.NewNRGBA(image.Rect(0, 0, w, height))
	used := map[int]RGB{}
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*w + x
			c := r.Image.NRGBAAt(x+r.Image.Rect.Min.X, y+r.Image.Rect.Min.Y)
			if c.A != 0 {
				layer := int(r.LayerMap[i])
				if layer < h.BaseLayers() || layer > maxLayer {
					return nil, fmt.Errorf("invalid printable layer %d", layer)
				}
				rgb := RGB{c.R, c.G, c.B}
				if prior, ok := used[layer]; ok && prior != rgb {
					return nil, fmt.Errorf("multiple colors assigned to layer %d", layer)
				}
				used[layer] = rgb
				c.A = 255
			}
			img.SetNRGBA(x, y, c)
		}
	}
	if len(used) == 0 {
		return nil, fmt.Errorf("HFP export needs visible image content")
	}
	var pngData bytes.Buffer
	if err := WritePNG(ctx, &pngData, img, meta.DPI); err != nil {
		return nil, err
	}
	light := 1
	if h.LightPreset == "neutral-white" || h.LightPreset == "" {
		light = 2
	}
	if h.LightPreset == "warm-white" {
		light = 0
	}
	doc := map[string]any{
		"version": "0.9.4.3", "image": strings.TrimSuffix(filepath.Base(sourceName), filepath.Ext(sourceName)) + "-colorninja.png", "image_binary": pngData.Bytes(),
		"width_in_mm": width, "height_in_mm": width * float64(height) / float64(w), "min_detail": detail,
		"layer_height": h.LayerHeight, "base_layer_height": h.FirstHeight(), "min_depth": h.BaseDepth, "max_depth": maxDepth,
		"filament_set": filaments, "slider_values": sliders,
		"luminance_method": map[string]int{"color-match": 6, "combo": 2, "color-aware": 4, "color-pop": 5}[mode],
		"depth_mode":       2, "lighting_visualizer": 0, "light_temperature": light, "light_intensity": 20,
		"brightness_compensation_name": "Standard", "luminance_weight": 50, "luminance_offset": 0, "luminance_offset_max": 1000,
		"luminance_power": 1, "luminance_factor": 1, "bw_tolerance": 0, "gs_threshold": 0,
		"color_match_method": 5, "legacy_lab": false, "srgb_linearize": false,
		"edit_image": false, "mesh_style_edit": false, "flat_image_scaling": true,
		"median": 0, "smoothing": 0, "flatten": false, "full_range": true, "negative": true, "reverse_litho": true,
		"borderless": true, "border_width": 0, "border_height": maxDepth, "transparency": true,
		"red_shift": 0, "green_shift": 0, "blue_shift": 0, "ignore_red": false, "ignore_green": false, "ignore_blue": false,
		"colorninja": map[string]any{"schemaVersion": 1, "meshCore": core, "opticalModel": h.OpticalModel, "rgbaSHA256": r.SHA256, "librarySHA256": r.Stack.LibrarySHA256, "alphaPolicy": "nonzero alpha becomes solid mesh", "meshRecomputed": mode != "color-match"},
	}
	if mode == "color-match" {
		if core == "filament-blends" {
			doc["match_filament_set"], doc["match_slider_values"] = filaments, sliders
		} else {
			doc["color_match_method"] = 0 // RGB matching prioritizes exact color matches.
			bands := meshColorBands(used, maxLayer)
			mesh := make([]hfpFilament, 0, len(bands))
			ends := make([]int, 0, len(bands))
			for _, band := range bands {
				c := band.RGB
				mesh = append(mesh, hfpFilament{"", c.Hex(), c.Hex(), false, .01, "IMAGE", hfpID(fmt.Sprintf("mesh|%d|%s", band.TargetLayer, c.Hex()))})
				ends = append(ends, band.End)
			}
			doc["match_filament_set"], doc["match_slider_values"] = mesh, ends
		}
		// Restrict matching to heights actually selected by the optimizer.
		disabled := []int{}
		for layer := 0; layer <= h.MaxLayers()+1; layer++ {
			if _, ok := used[layer]; !ok {
				disabled = append(disabled, layer)
			}
		}
		doc["disabled_match_layers"] = disabled
	}
	// Filament Painting stores filament arrays in reverse order, while
	// slider endpoints remain ascending.
	slices.Reverse(filaments)
	if mesh, ok := doc["match_filament_set"].([]hfpFilament); ok && core != "filament-blends" {
		slices.Reverse(mesh)
	}
	return doc, nil
}

func SaveHFP(ctx context.Context, path string, r *Result, sourceName string, meta ImageMetadata, overwrite bool) error {
	if !strings.EqualFold(filepath.Ext(path), ".hfp") {
		return fmt.Errorf("HueForge project filename must end in .hfp")
	}
	doc, err := hueForgeProject(ctx, r, sourceName, meta)
	if err != nil {
		return err
	}
	return AtomicWrite(path, overwrite, func(w io.Writer) error {
		e := json.NewEncoder(checkedWriter{ctx, w})
		e.SetIndent("", "  ")
		return e.Encode(doc)
	})
}
