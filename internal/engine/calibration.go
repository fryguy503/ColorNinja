package engine

import (
	"context"
	"math"
)

type FilamentSensitivity struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	MaxDeltaE76 float64 `json:"maxDeltaE76"`
}
type CalibrationInfo struct {
	Note              string                `json:"note,omitempty"`
	TrueBlackOverride bool                  `json:"trueBlackOverride"`
	VariationPercent  float64               `json:"variationPercent"`
	Sensitivity       []FilamentSensitivity `json:"sensitivity,omitempty"`
	Truncated         bool                  `json:"truncated,omitempty"`
}

func calibrationInfo(ctx context.Context, r *Result, o Options) (*CalibrationInfo, error) {
	if o.Mode == "standard" {
		return nil, nil
	}
	c := &CalibrationInfo{Note: o.CalibrationNote, VariationPercent: o.HueForge.TDSensitivityPercent}
	if r.Guidance != nil {
		for _, f := range r.Guidance.Selected {
			c.TrueBlackOverride = c.TrueBlackOverride || f.LibraryRGB != nil
		}
	}
	if r.Stack == nil {
		return c, nil
	}
	lib := Library{}
	ids, runs := []int{}, []int{}
	index := map[string]int{}
	for _, run := range r.Stack.Runs {
		f := run.Filament
		c.TrueBlackOverride = c.TrueBlackOverride || f.LibraryRGB != nil
		key := FilamentKey(f)
		id, ok := index[key]
		if !ok {
			id = len(lib.Filaments)
			index[key] = id
			lib.Filaments = append(lib.Filaments, f)
		}
		ids = append(ids, id)
		runs = append(runs, run.Layers)
	}
	if c.VariationPercent == 0 {
		return c, nil
	}
	for id, f := range lib.Filaments {
		if id >= 32 {
			c.Truncated = true
			break
		}
		entry := FilamentSensitivity{Key: FilamentKey(f), Name: f.Name}
		for _, direction := range []float64{-1, 1} {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			lib.Filaments[id].TD = f.TD * (1 + direction*c.VariationPercent/100)
			s := rebuildStack(ids, runs, lib, o.HueForge)
			colors := map[int]RGB{}
			for i, l := range s.layers {
				colors[l] = s.rgbs[i]
			}
			for _, p := range r.Palette {
				if p.PixelFraction > 0 {
					entry.MaxDeltaE76 = math.Max(entry.MaxDeltaE76, math.Sqrt(distance(ToLab(p.RGB), ToLab(colors[p.StackLayer]))))
				}
			}
		}
		lib.Filaments[id] = f
		c.Sensitivity = append(c.Sensitivity, entry)
	}
	return c, nil
}
