package engine

import (
	"fmt"
	"math"
	"sort"
)

type MeshOptimization struct {
	Strategy               string  `json:"strategy"`
	Entries                int     `json:"entries"`
	OriginalEntries        int     `json:"originalEntries"`
	DisabledLayers         int     `json:"disabledLayers"`
	OriginalDisabledLayers int     `json:"originalDisabledLayers"`
	MinTD                  float64 `json:"minTD"`
	MaxTD                  float64 `json:"maxTD"`
}
type virtualMesh struct {
	filaments []hfpFilament
	ends      []int
	colors    [][3]float32
	disabled  []int
	method    int
	info      MeshOptimization
}

func meshRGBDistance(a [3]float32, b RGB) float64 {
	d := 0.
	for i, v := range a {
		x := float64(v - float32(b[i])/255)
		d += x * x
	}
	return d
}
func meshLab(c [3]float32) Vec {
	var v Vec
	for i, x := range c {
		v[i] = linear(float64(x) * 255)
	}
	return okLabLinear(v)
}
func meshSimulate(fs []hfpFilament, ends []int, h HueForgeOptions) [][3]float32 {
	out := make([][3]float32, ends[len(ends)-1]+1)
	s := newFrontlit(h.light())
	pos := 0
	for layer := 1; layer < len(out); layer++ {
		newRun := layer == 1
		if pos+1 < len(ends) && layer > ends[pos] {
			pos++
			newRun = true
		}
		rgb, _ := ParseRGB(fs[pos].Color)
		depth := h.LayerHeight
		if layer == 1 {
			depth = h.FirstHeight()
		}
		s.step(Filament{RGB: rgb, TD: fs[pos].TD}, depth, newRun)
		out[layer] = s.rgb()
	}
	return out
}

// Keep only masks that would otherwise move a used color away from its chosen
// height. Later equal colors lose the first-match tie naturally; no blanket
// disabling of unused or '=' layers is necessary.
func meshMasks(colors [][3]float32, used map[int]RGB, h HueForgeOptions, method int) ([]int, bool) {
	disabled := map[int]bool{}
	for target, rgb := range used {
		if target >= len(colors) {
			return nil, false
		}
		targetLab := toOKLab(rgb)
		score := distance(targetLab, meshLab(colors[target]))
		if method == 0 && meshRGBDistance(colors[target], rgb) > 1e-10 {
			return nil, false
		}
		for layer := h.BaseLayers(); layer < len(colors); layer++ {
			if layer == target {
				continue
			}
			conflict := false
			if method == 0 {
				conflict = layer < target && meshRGBDistance(colors[layer], rgb) <= 1e-10
			} else {
				d := distance(targetLab, meshLab(colors[layer]))
				// A conservative guard covers float32 shader/C++ conversion differences.
				conflict = d < score+1e-4
				if layer > target && colors[layer] == colors[target] {
					conflict = false
				}
			}
			if conflict {
				if _, active := used[layer]; active {
					return nil, false
				}
				disabled[layer] = true
			}
		}
	}
	out := []int{}
	for l := range disabled {
		out = append(out, l)
	}
	sort.Ints(out)
	return out, true
}

// Build virtual IMAGE runs with real blending TDs. Try the physical blend path
// as a compact representation and a fitted image-color path as a fallback.
// Neither candidate modifies the physical Color Core, image, or layer map.
func compactMesh(r *Result, used map[int]RGB) virtualMesh {
	h := r.Stack.Options
	last := r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer
	targets := []int{}
	for l := range used {
		targets = append(targets, l)
	}
	sort.Ints(targets)
	originalMasks := h.MaxLayers() + 2 - len(used)
	finish := func(m virtualMesh, strategy string) virtualMesh {
		m.info = MeshOptimization{Strategy: strategy, Entries: len(m.filaments), OriginalEntries: len(used), DisabledLayers: len(m.disabled), OriginalDisabledLayers: originalMasks, MinTD: math.Inf(1)}
		for _, f := range m.filaments {
			m.info.MinTD = math.Min(m.info.MinTD, f.TD)
			m.info.MaxTD = math.Max(m.info.MaxTD, f.TD)
		}
		return m
	}
	fitted := virtualMesh{method: 0}
	state := newFrontlit(h.light())
	start := 1
	for i, target := range targets {
		rgb := used[target]
		end := target
		if i == len(targets)-1 {
			end = last
		}
		simulate := func(td float64, stop int) frontlitState {
			s := state
			for l := start; l <= stop; l++ {
				depth := h.LayerHeight
				if l == 1 {
					depth = h.FirstHeight()
				}
				s.step(Filament{RGB: rgb, TD: td}, depth, l == start)
			}
			return s
		}
		// Maximum TD that reaches this exact image color at its selected height.
		// Round down to hundredths for editable, reproducible HueForge controls.
		low, high := .01, 1000.
		for n := 0; n < 28; n++ {
			mid := (low + high) / 2
			s := simulate(mid, target)
			if meshRGBDistance(s.rgb(), rgb) <= 1e-12 {
				low = mid
			} else {
				high = mid
			}
		}
		td := math.Max(.01, math.Floor(low*100)/100)
		if td > .01 {
			td = math.Max(.01, td-.01)
		}
		fitted.filaments = append(fitted.filaments, hfpFilament{"", rgb.Hex(), fmt.Sprintf("Image %s", rgb.Hex()), false, td, "IMAGE", hfpID(fmt.Sprintf("blended-mesh|%d|%s|%.2f", target, rgb.Hex(), td))})
		fitted.ends = append(fitted.ends, end)
		state = simulate(td, end)
		start = end + 1
	}
	fitted.colors = meshSimulate(fitted.filaments, fitted.ends, h)
	var ok bool
	fitted.disabled, ok = meshMasks(fitted.colors, used, h, 0)
	strategy := "fitted image-color TDs"
	if !ok {
		// Defensive exact-color fallback for unusual geometry or rounding cases.
		fitted = virtualMesh{method: 0}
		for _, b := range meshColorBands(used, last) {
			fitted.filaments = append(fitted.filaments, hfpFilament{"", b.RGB.Hex(), b.RGB.Hex(), false, .01, "IMAGE", hfpID(fmt.Sprintf("mesh|%d|%s", b.TargetLayer, b.RGB.Hex()))})
			fitted.ends = append(fitted.ends, b.End)
		}
		fitted.colors = meshSimulate(fitted.filaments, fitted.ends, h)
		fitted.disabled, _ = meshMasks(fitted.colors, used, h, 0)
		strategy = "flat image-color fallback to retain planned heights"
	}
	best := finish(fitted, strategy)
	blend := virtualMesh{method: 5}
	for i, run := range r.Stack.Runs {
		f := hfpMaterial(run.Filament)
		f.Brand = ""
		f.Name = "Image blend " + run.Filament.Name
		f.Material = "IMAGE"
		f.Owned = false
		f.UUID = hfpID(fmt.Sprintf("blend-mesh|%d|%s|%.17g", i, f.Color, f.TD))
		blend.filaments = append(blend.filaments, f)
		blend.ends = append(blend.ends, run.EndLayer)
	}
	blend.colors = meshSimulate(blend.filaments, blend.ends, h)
	blend.disabled, ok = meshMasks(blend.colors, used, h, 5)
	if ok && (len(blend.disabled) < len(best.disabled) || len(blend.disabled) == len(best.disabled) && len(blend.filaments) < len(best.filaments)) {
		best = finish(blend, "compact IMAGE blend path")
	}
	return best
}
