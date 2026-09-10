package engine

import (
	"context"
	"math"
)

// Material uses source area, never the salience weights used for color fitting.
// The search estimate uses analysis groups; SurfaceView measures the final map.
type stackAreaKey struct{}

func (o Options) materialOptimization() bool {
	return o.Mode == "stack" && !o.ColorPop.Enabled && o.HueForge.OptimizeMaterial
}

func paletteAreas(p []PaletteEntry) []float64 {
	areas, total := make([]float64, len(p)), 0.
	for i, v := range p {
		areas[i] = math.Max(0, v.Fraction)
		total += areas[i]
	}
	if total > 0 {
		for i := range areas {
			areas[i] /= total
		}
	}
	return areas
}

func stackGeometryPenalty(ctx context.Context, s stackState, selected []RGB, heights []int, target []Vec, o Options, boundaries []stackBoundary) float64 {
	penalty := stackSurface(s, selected, heights, target, o, boundaries).Penalty + layerPreferencePenalty(ctx, selected, heights, target, o)
	if !o.materialOptimization() {
		return penalty
	}
	areas, ok := ctx.Value(stackAreaKey{}).([]float64)
	if !ok || len(areas) != len(target) || len(selected) == 0 {
		return penalty
	}
	vectors := o.colorVectors(selected)
	meanAboveBase := 0.
	for i, t := range target {
		best, id := math.Inf(1), 0
		for j, v := range vectors {
			if d := distance(t, v); d < best {
				best, id = d, j
			}
		}
		meanAboveBase += areas[i] * math.Max(0, o.HueForge.Height(heights[id])-o.HueForge.BaseDepth)
	}
	// Four working color units for 1 mm of average thickness above the fixed
	// foundation. Nonnegative, so existing color-only lower-bound pruning holds.
	// The separate color-fidelity guard limits the permitted color tradeoff.
	return penalty + 16*meanAboveBase*meanAboveBase
}

type RunMaterial struct {
	Position              int     `json:"position"`
	VolumeMM3             float64 `json:"volumeMm3"`
	BuriedVolumeMM3       float64 `json:"buriedVolumeMm3"`
	VisibleAreaFraction   float64 `json:"visibleAreaFraction"`
	StartCoverageFraction float64 `json:"startCoverageFraction"`
}

// Integrate occupied layer area, including the distinct first-layer thickness.
// Partial alpha becomes solid in HFP. No mesh resampling, purge, infill, or
// slicer extrusion is modeled, and buried material may be optically necessary.
func surfaceMaterial(v *SurfaceView, tops []float64, visible float64, plan *StackPlan, h HueForgeOptions) {
	if visible == 0 {
		return
	}
	occupied := make([]float64, len(tops))
	for layer := len(tops) - 1; layer >= 1; layer-- {
		occupied[layer] = tops[layer]
		if layer+1 < len(tops) {
			occupied[layer] += occupied[layer+1]
		}
	}
	pixelArea := v.PixelMM * v.PixelMM
	for _, run := range plan.Runs {
		m := RunMaterial{Position: run.Position}
		if run.StartLayer < 1 || run.StartLayer >= len(tops) || run.EndLayer >= len(tops) {
			continue
		}
		m.StartCoverageFraction = occupied[run.StartLayer] / visible
		for layer := run.StartLayer; layer <= run.EndLayer; layer++ {
			thickness := h.Height(layer) - h.Height(layer-1)
			m.VolumeMM3 += occupied[layer] * pixelArea * thickness
			m.VisibleAreaFraction += tops[layer] / visible
			if run.EndLayer+1 < len(tops) {
				m.BuriedVolumeMM3 += occupied[run.EndLayer+1] * pixelArea * thickness
			}
		}
		v.VolumeMM3 += m.VolumeMM3
		v.MaterialRuns = append(v.MaterialRuns, m)
	}
	v.MeanThicknessMM = v.VolumeMM3 / (visible * pixelArea)
}
