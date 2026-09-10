package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// Preferences concern the heights assigned to source colors, not the last
// occurrence of a spool. In particular, an early red followed by a red return
// does not satisfy a preference if the source reds still match the early run.
type layerPreferenceKey struct{}
type layerFamiliesKey struct{}
type layerColorLimitsKey struct{}
type layerSearchBarrierKey struct{}
type layerPreference struct {
	family               string
	preferred, reference []float64
	representative       RGB
}

type LayerPreferenceReport struct {
	Family              string  `json:"family"`
	SourceFraction      float64 `json:"sourceFraction"`
	PreferredMeanHeight float64 `json:"preferredMeanHeightMm"`
	ReferenceMeanHeight float64 `json:"referenceMeanHeightMm"`
	RaisedFraction      float64 `json:"raisedFraction"`
	Satisfied           bool    `json:"satisfied"`
	Message             string  `json:"message"`
}

func (o Options) layerOptimization() bool {
	return o.Mode == "stack" && !o.ColorPop.Enabled && !o.fixedHeights() && (o.customColorOrder() || o.HueForge.LayerPreference != "")
}

func validLayerPreference(s string) bool {
	switch s {
	case "", "auto", "red", "yellow", "green", "cyan", "blue", "purple":
		return true
	}
	return false
}

// Group shades before measuring rarity: a tiny pale green within a large
// green family is not an independent foreground accent. These broad hue
// groups are a starting preference, not semantic foreground detection.
func layerColorFamily(c RGB) string {
	lab := ToLab(c)
	if math.Hypot(lab[1], lab[2]) < 12 {
		return "neutral"
	}
	r, g, b := float64(c[0]), float64(c[1]), float64(c[2])
	hi, lo := max(r, g, b), min(r, g, b)
	if hi-lo < 12 {
		return "neutral"
	}
	hue := 0.
	switch hi {
	case r:
		hue = 60 * math.Mod((g-b)/(hi-lo), 6)
	case g:
		hue = 60 * ((b-r)/(hi-lo) + 2)
	case b:
		hue = 60 * ((r-g)/(hi-lo) + 4)
	}
	if hue < 0 {
		hue += 360
	}
	switch {
	case hue < 35 || hue >= 330:
		return "red"
	case hue < 80:
		return "yellow"
	case hue < 165:
		return "green"
	case hue < 210:
		return "cyan"
	case hue < 270:
		return "blue"
	default:
		return "purple"
	}
}

func imageLayerPreference(ctx context.Context, p []PaletteEntry, o Options) *layerPreference {
	if !o.layerOptimization() || o.customColorOrder() {
		return nil
	}
	areas := paletteAreas(p)
	families, totals := make([]string, len(p)), map[string]float64{}
	for i, c := range p {
		families[i] = layerColorFamily(c.RGB)
		totals[families[i]] += areas[i]
	}
	family := o.HueForge.LayerPreference
	regions, _ := ctx.Value(layerRegionsKey{}).(map[string]layerRegion)
	if family == "auto" {
		family = ""
		// Ignore sub-0.5% noise. Prefer a coherent, compact color region smaller
		// than the dominant background; do not infer depth from hue alone.
		largest := 0.
		for _, f := range []string{"red", "yellow", "green", "cyan", "blue", "purple"} {
			a := totals[f]
			maximum := .10
			if regions != nil {
				maximum = .25
			}
			if a < .005 || a > maximum {
				continue
			}
			dominant := 0.
			for other, area := range totals {
				if other != f {
					dominant = math.Max(dominant, area)
				}
			}
			if dominant < 1.5*a {
				continue
			}
			score := a
			if regions != nil {
				r, ok := regions[f]
				if !ok || r.compactness > .35 || r.coherence < .2 {
					continue
				}
				score *= r.coherence / math.Max(.01, r.compactness)
			}
			if score > largest {
				largest, family = score, f
			}
		}
	}
	if family == "" || totals[family] == 0 {
		return nil
	}
	pref := &layerPreference{family: family, preferred: make([]float64, len(p)), reference: make([]float64, len(p))}
	largest, referenceTotal := 0., 0.
	for i, c := range p {
		if families[i] == family {
			pref.preferred[i] = areas[i]
			lab := toOKLab(c.RGB)
			distinctiveness := areas[i] * (lab[1]*lab[1] + lab[2]*lab[2])
			if distinctiveness > largest {
				largest, pref.representative = distinctiveness, c.RGB
			}
		} else if totals[families[i]] >= .15 {
			weight := areas[i]
			if r, ok := regions[family]; ok {
				// Weight surrounding dominant families, retaining a small global
				// reference so a thin shared outline cannot dictate all ordering.
				contact, total := r.contacts[families[i]], 0.
				for _, n := range r.contacts {
					total += n
				}
				if total > 0 {
					weight *= .2 + .8*contact/total
				}
			}
			pref.reference[i] = weight
			referenceTotal += weight
		}
	}
	if referenceTotal == 0 {
		return nil
	}
	return pref
}

func preferenceHeights(selected []RGB, heights []int, target []Vec, o Options) []float64 {
	out, vectors := make([]float64, len(target)), o.colorVectors(selected)
	for i, t := range target {
		best, id := math.Inf(1), 0
		for j, v := range vectors {
			if d := distance(t, v); d < best {
				best, id = d, j
			}
		}
		out[i] = o.HueForge.Height(heights[id])
	}
	return out
}

func preferencePenalty(pref *layerPreference, heights []float64, o Options) (float64, *LayerPreferenceReport) {
	r := &LayerPreferenceReport{Family: pref.family}
	a, b, penalty := 0., 0., 0.
	for i, w := range pref.preferred {
		a += w
		r.PreferredMeanHeight += w * heights[i]
	}
	for i, w := range pref.reference {
		b += w
		r.ReferenceMeanHeight += w * heights[i]
	}
	if a == 0 || b == 0 {
		return 0, r
	}
	r.SourceFraction, r.PreferredMeanHeight, r.ReferenceMeanHeight = a, r.PreferredMeanHeight/a, r.ReferenceMeanHeight/b
	for i, w := range pref.preferred {
		if w == 0 {
			continue
		}
		for j, v := range pref.reference {
			if v == 0 {
				continue
			}
			weight := w * v / (a * b)
			gap := math.Max(0, heights[j]+o.HueForge.LayerHeight-heights[i])
			penalty += weight * gap * gap
			if heights[i] > heights[j]+1e-8 {
				r.RaisedFraction += weight
			}
		}
	}
	r.Satisfied = r.RaisedFraction >= .9-1e-8
	if r.Satisfied {
		r.Message = fmt.Sprintf("%s shades raised above %.0f%% of dominant-color comparisons.", pref.family, r.RaisedFraction*100)
	} else {
		r.Message = fmt.Sprintf("%s shades raised above %.0f%% of surrounding dominant-color comparisons. Placement is limited by color accuracy and the available stack; the accent is still partly recessed.", pref.family, r.RaisedFraction*100)
	}
	// A directional appearance cost. Once the accent clears the reference,
	// additional height has no reward; show-through still discourages cliffs.
	return 64 * penalty, r
}

func layerPreferencePenalty(ctx context.Context, selected []RGB, heights []int, target []Vec, o Options) float64 {
	pref, _ := ctx.Value(layerPreferenceKey{}).(*layerPreference)
	if !o.layerOptimization() || len(selected) == 0 {
		return 0
	}
	assigned := preferenceHeights(selected, heights, target, o)
	source, _ := ctx.Value(colorOrderKey{}).(colorOrderSource)
	penalty, _ := colorOrderPenalty(source, assigned, o)
	if pref != nil {
		penalty, _ = preferencePenalty(pref, assigned, o)
	}
	families, _ := ctx.Value(layerFamiliesKey{}).([]string)
	areas, _ := ctx.Value(stackAreaKey{}).([]float64)
	// Keep dominant chromatic families reasonably compact in height. A small
	// pale shade should not float far above the rest of its foliage family.
	if len(families) == len(target) && len(areas) == len(target) {
		for _, family := range []string{"red", "yellow", "green", "cyan", "blue", "purple"} {
			area, mean := 0., 0.
			for i, f := range families {
				if f == family {
					area += areas[i]
					mean += areas[i] * assigned[i]
				}
			}
			if area < .15 {
				continue
			}
			mean /= area
			for i, f := range families {
				if f == family {
					d := assigned[i] - mean
					penalty += 4 * areas[i] * d * d / area
				}
			}
		}
	}
	return penalty
}

func layerFamilyErrors(colors []RGB, target []Vec, areas []float64, families []string, o Options) map[string]float64 {
	out, mass := map[string]float64{}, map[string]float64{}
	vectors := o.colorVectors(colors)
	for i, t := range target {
		best := math.Inf(1)
		for _, v := range vectors {
			best = math.Min(best, distance(t, v))
		}
		out[families[i]] += areas[i] * best
		mass[families[i]] += areas[i]
	}
	for family, v := range out {
		if mass[family] > 0 {
			out[family] = math.Sqrt(v / mass[family])
		}
	}
	return out
}

func layerColorsAllowed(ctx context.Context, colors []RGB, target []Vec, o Options) bool {
	return layerColorExcess(ctx, colors, target, o) == 0
}

func layerColorExcess(ctx context.Context, colors []RGB, target []Vec, o Options) float64 {
	limits, _ := ctx.Value(layerColorLimitsKey{}).(map[string]float64)
	if !o.layerOptimization() || limits == nil {
		return 0
	}
	areas, _ := ctx.Value(stackAreaKey{}).([]float64)
	families, _ := ctx.Value(layerFamiliesKey{}).([]string)
	if len(areas) != len(target) || len(families) != len(target) {
		return 0
	}
	excess := 0.
	for f, v := range layerFamilyErrors(colors, target, areas, families, o) {
		if v > limits[f]+1e-8 {
			d := v - limits[f]
			excess += d * d
		}
	}
	return excess
}

// A constrained candidate search can cross the bad intermediate blends that
// trap local swaps. This is a seed, not a user pin: it must pass the same color
// guard and actual-height objective as every other candidate.
func preferenceSeeds(ctx context.Context, palette []PaletteEntry, lib Library, o Options, progress Reporter) ([]*StackPlan, error) {
	pref, _ := ctx.Value(layerPreferenceKey{}).(*layerPreference)
	custom := o.customColorOrder()
	if custom {
		pref = colorOrderSeed(ctx, palette, o)
	}
	if pref == nil || o.HueForge.HighlightFilament != "" {
		return nil, nil
	}
	type candidate struct {
		id       int
		distance float64
	}
	candidates := []candidate{}
	for i, f := range lib.Filaments {
		family := layerColorFamily(f.RGB)
		if custom {
			family = colorOrderGroup(f.RGB)
		}
		if family != pref.family || FilamentKey(f) == o.HueForge.BaseFilament {
			continue
		}
		candidates = append(candidates, candidate{i, distance(o.colorVector(pref.representative), o.colorVector(f.RGB))})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].distance < candidates[j].distance })
	plans := []*StackPlan{}
	for _, candidate := range candidates[:min(3, len(candidates))] {
		seed := o
		seed.HueForge.ColorOrder = ""
		seed.HueForge.LayerPreference, seed.HueForge.OptimizeMaterial, seed.HueForge.ReduceShowThrough = "", false, false
		// Ordering needs complete blend exploration even in Normal preview.
		// Requiring the user to select Deeper refinement would make this
		// automatic control fall back to the same recessed accent again.
		seed.HueForge.SearchEffort = "refine"
		seed.HueForge.HighlightFilament, seed.HueForge.HighlightOnlyAtTop = FilamentKey(lib.Filaments[candidate.id]), true
		searchCtx := context.WithValue(ctx, stackColorLimitKey{}, struct{}{})
		_, plan, err := planStack(searchCtx, palette, lib, seed, progress)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil {
			plans = append(plans, plan)
		} // optional seeds may conflict with a budget
	}
	return plans, nil
}
