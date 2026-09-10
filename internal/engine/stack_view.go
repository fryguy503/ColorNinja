package engine

import "sort"

type StackCoreLayer struct {
	Layer         int     `json:"layer"`
	Height        float64 `json:"height"`
	RunPosition   int     `json:"runPosition"`
	PredictedRGB  RGB     `json:"predictedRGB"`
	MeshRGB       *RGB    `json:"meshRGB,omitempty"`
	MeshEnabled   bool    `json:"meshEnabled"`
	PixelFraction float64 `json:"pixelFraction"`
}

type StackCoreView struct {
	Optimization *MeshOptimization `json:"optimization,omitempty"`
	MeshMode     string            `json:"meshMode"`
	MeshCore     string            `json:"meshCore"`
	HasMeshCore  bool              `json:"hasMeshCore"`
	Layers       []StackCoreLayer  `json:"layers"`
}

type meshColorBand struct {
	Start, End, TargetLayer int
	RGB                     RGB
}

// The visualizer and HFP writer share the same virtual Color Match core bands.
// Gaps carry the preceding target color, but only used heights are enabled.
func meshColorBands(used map[int]RGB, maxLayer int) []meshColorBand {
	layers := make([]int, 0, len(used))
	for layer := range used {
		layers = append(layers, layer)
	}
	sort.Ints(layers)
	bands := make([]meshColorBand, 0, len(layers))
	start := 1
	for i, layer := range layers {
		end := maxLayer
		if i+1 < len(layers) {
			end = layers[i+1] - 1
		}
		bands = append(bands, meshColorBand{start, end, layer, used[layer]})
		start = end + 1
	}
	return bands
}

func buildStackCoreView(r *Result) *StackCoreView {
	if r.Stack == nil || len(r.Stack.Runs) == 0 {
		return nil
	}
	h := r.Stack.Options
	v := &StackCoreView{MeshMode: h.MeshMode, MeshCore: h.meshCore()}
	if v.MeshMode == "" {
		v.MeshMode = "color-match"
	}
	v.HasMeshCore = v.MeshMode == "color-match"
	used, fractions := map[int]RGB{}, map[int]float64{}
	for _, p := range r.Palette {
		if p.PixelFraction > 0 {
			used[p.StackLayer] = p.RGB
			fractions[p.StackLayer] += p.PixelFraction
		}
	}
	if r.ColorPop != nil {
		v.MeshMode = "color-match"
		v.MeshCore = "legacy-flat"
		v.HasMeshCore = true
		used = colorPopMeshColors(r)
	}
	bands := meshColorBands(used, r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer)
	var compact *virtualMesh
	if v.HasMeshCore && v.MeshCore == "compact-blends" && len(used) > 0 {
		m := compactMesh(r, used)
		compact = &m
		v.Optimization = &m.info
	}
	band := 0
	for _, layer := range r.Stack.LayerColors {
		row := StackCoreLayer{Layer: layer.Layer, Height: layer.Height, RunPosition: layer.TopPosition, PredictedRGB: layer.RGB, PixelFraction: fractions[layer.Layer]}
		if v.HasMeshCore {
			_, row.MeshEnabled = used[layer.Layer]
			if compact != nil {
				row.MeshEnabled = layer.Layer >= h.BaseLayers()
				for _, disabled := range compact.disabled {
					if disabled == layer.Layer {
						row.MeshEnabled = false
					}
				}
				var rgb RGB
				for i, v := range compact.colors[layer.Layer] {
					rgb[i] = byteRound(float64(v) * 255)
				}
				row.MeshRGB = &rgb
			} else if v.MeshCore == "filament-blends" {
				color := layer.RGB
				row.MeshRGB = &color
			} else if len(bands) > 0 {
				for band+1 < len(bands) && layer.Layer > bands[band].End {
					band++
				}
				color := bands[band].RGB
				row.MeshRGB = &color
			}
		}
		v.Layers = append(v.Layers, row)
	}
	return v
}
