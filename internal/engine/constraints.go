package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
)

// Stable across library reordering. UUID-less inventory uses its full optical
// identity, not a display name or a position in the JSON file.
func FilamentKey(f Filament) string {
	if f.UUID != "" {
		return strings.ToLower(strings.Trim(f.UUID, "{}"))
	}
	rgb := f.RGB
	if f.LibraryRGB != nil {
		rgb = *f.LibraryRGB
	}
	s := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%.17g", f.Brand, f.Name, f.Material, rgb.Hex(), f.TD)))
	return fmt.Sprintf("%x", s[:16])
}

func keys(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func optimizationMetric(o Options, boundaries bool) string {
	unit := "priority-weighted Oklab RMS (100x Oklab coordinates)"
	if o.LegacyColorPipeline {
		unit = "priority-weighted CIELAB RMS"
	}
	if boundaries {
		unit += " with physical-scale boundary penalty"
	}
	if o.materialOptimization() {
		unit += " with area-weighted material penalty"
	}
	if o.layerOptimization() {
		unit += " with source-color layer preference"
	}
	return unit
}
func constraintIDs(lib Library, o Options) (required []int, base, top int, err error) {
	base, top = -1, -1
	lookup := map[string]int{}
	for i, f := range lib.Filaments {
		lookup[FilamentKey(f)] = i
	}
	find := func(key string) (int, error) {
		id, ok := lookup[key]
		if !ok {
			return -1, fmt.Errorf("a required filament is missing or filtered out; clear its constraint or restore that spool")
		}
		return id, nil
	}
	for _, key := range keys(o.HueForge.RequiredFilaments) {
		id, e := find(key)
		if e != nil {
			return nil, -1, -1, e
		}
		required = append(required, id)
	}
	for i, key := range []string{o.HueForge.BaseFilament, o.HueForge.HighlightFilament} {
		if key == "" {
			continue
		}
		id, e := find(key)
		if e != nil {
			return nil, -1, -1, e
		}
		if i == 0 {
			base = id
		} else {
			top = id
		}
		if !contains(required, id) {
			required = append(required, id)
		}
	}
	if len(required) > o.Colors {
		return nil, -1, -1, fmt.Errorf("%d required spools exceed the %d-filament budget", len(required), o.Colors)
	}
	return required, base, top, nil
}
func completeConstraints(ids []int, lib Library, o Options) bool {
	required, base, top, err := constraintIDs(lib, o)
	if err != nil || len(ids) == 0 {
		return false
	}
	if base >= 0 && ids[0] != base {
		return false
	}
	if top >= 0 && ids[len(ids)-1] != top {
		return false
	}
	if top >= 0 && o.HueForge.HighlightOnlyAtTop && contains(ids[:len(ids)-1], top) {
		return false
	}
	for _, id := range required {
		if !contains(ids, id) {
			return false
		}
	}
	return true
}
func protectedRGB(o Options) []RGB {
	out := []RGB{}
	seen := map[RGB]bool{}
	for _, s := range keys(o.ProtectedColors) {
		if c, err := ParseRGB(s); err == nil && !seen[c] {
			out = append(out, c)
			seen[c] = true
		}
	}
	return out
}

type stackConstraintsKey struct{}
type stackConstraints struct {
	required []int
	top      int
}

// Required spools must occur below a selected output height, otherwise trimming
// would silently remove them from the exported, physically used schedule.
func enforceHeightConstraints(ctx context.Context, s stackState, colors []RGB, layers, positions, ids []int, target []Vec, o Options, boundaries []stackBoundary) bool {
	c, ok := ctx.Value(stackConstraintsKey{}).(stackConstraints)
	if !ok || len(c.required) == 0 {
		return true
	}
	floor, start := 1, 1
	seen := map[int]bool{}
	for pos, id := range s.indices {
		if contains(c.required, id) && !seen[id] || c.top == id && pos == len(s.indices)-1 {
			floor = max(floor, start)
		}
		seen[id] = true
		start += s.runs[pos]
	}
	for _, id := range c.required {
		if !seen[id] {
			return false
		}
	}
	selected, heights := make([]RGB, len(ids)), make([]int, len(ids))
	for i, id := range ids {
		if layers[id] >= floor {
			return true
		}
		selected[i], heights[i] = colors[id], layers[id]
	}
	best, winner, sample := math.Inf(1), -1, -1
	for p, id := range ids {
		for j, rgb := range s.rgbs {
			if rgb != colors[id] || s.layers[j] < floor {
				continue
			}
			heights[p] = s.layers[j]
			score := stackGeometryPenalty(ctx, s, selected, heights, target, o, boundaries)
			if score < best {
				best, winner, sample = score, id, j
			}
		}
		heights[p] = layers[id]
	}
	if winner < 0 {
		return false
	}
	layers[winner] = s.layers[sample]
	if positions != nil {
		positions[winner] = s.positions[sample]
	}
	return true
}
func validateProtectedColors(o Options) error {
	if len(o.ProtectedColors) > 2048 {
		return fmt.Errorf("too many protected colors")
	}
	seen := map[RGB]bool{}
	for _, s := range keys(o.ProtectedColors) {
		c, err := ParseRGB(s)
		if err != nil {
			return fmt.Errorf("protected colors must be comma-separated RGB hex colors")
		}
		seen[c] = true
	}
	budget := o.Colors
	if o.Mode == "standard" && !o.TotalColors {
		budget *= 2
	}
	if o.Mode != "standard" {
		budget = o.HueForge.AnalysisColors * 2
	}
	if len(seen) > min(32, budget) {
		return fmt.Errorf("protected colors must fit the analysis/color budget (at most 32)")
	}
	return nil
}

// Reserve exact palette slots without changing the requested ceiling. In
// filament modes these are protected analysis targets, not invented print RGBs.
func protectPalette(p []PaletteEntry, o Options) []PaletteEntry {
	locks := protectedRGB(o)
	if len(locks) == 0 {
		return p
	}
	target := append([]PaletteEntry(nil), p...)
	budget := o.Colors
	if !o.TotalColors || o.Mode != "standard" {
		budget *= 2
	}
	fixed := map[RGB]bool{}
	for _, c := range locks {
		fixed[c] = true
	}
	for _, c := range locks {
		found := false
		for _, v := range p {
			if v.RGB == c {
				found = true
				break
			}
		}
		if found {
			continue
		}
		v := entry(c, 0, o.NeutralChroma)
		if len(p) < budget {
			p = append(p, v)
			continue
		}
		best, pos := math.Inf(1), -1
		for i, old := range p {
			if fixed[old.RGB] {
				continue
			}
			p[i] = v
			rgb := []RGB{}
			for _, a := range p {
				rgb = append(rgb, a.RGB)
			}
			score := paletteRMS76(rgb, target, o)
			p[i] = old
			if score < best {
				best, pos = score, i
			}
		}
		if pos >= 0 {
			p[pos] = v
		}
	}
	vectors := make([]Vec, len(p))
	for i, v := range p {
		vectors[i] = o.colorVector(v.RGB)
	}
	fractions := sourceFractions(vectors, target, o)
	for i := range p {
		p[i].Fraction = fractions[i]
	}
	return p
}
