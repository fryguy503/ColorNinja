package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
)

type GuidedColor struct {
	RGB          RGB     `json:"rgb"`
	ReferenceRGB RGB     `json:"referenceRGB"`
	Kind         string  `json:"referenceKind"`
	Positions    []int   `json:"filamentPositions"`
	TopLayers    int     `json:"topLayers"`
	Fraction     float64 `json:"analysisFraction"`
}
type GuidancePlan struct {
	Model                 string          `json:"model"`
	GlobalStackGuaranteed bool            `json:"globalStackGuaranteed"`
	Options               HueForgeOptions `json:"options"`
	Strength              float64         `json:"strength"`
	Requested             int             `json:"requestedFilaments"`
	Eligible              int             `json:"eligibleFilaments"`
	Selected              []Filament      `json:"selectedFilaments"`
	CandidateCount        int             `json:"candidateColorCount"`
	Colors                []GuidedColor   `json:"colors"`
	RMS                   float64         `json:"weightedRmsDeltaE76"`
	LibrarySHA256         string          `json:"librarySHA256"`
}
type guidanceCandidate struct {
	rgb     RGB
	kind    string
	indices []int
	layers  int
}

func guidanceCandidates(ctx context.Context, indices []int, lib Library, o HueForgeOptions) ([]guidanceCandidate, error) {
	indices = sorted(indices)
	seen := map[RGB]bool{}
	out := []guidanceCandidate{}
	add := func(rgb RGB, kind string, ids []int, layers int) {
		if !seen[rgb] {
			seen[rgb] = true
			out = append(out, guidanceCandidate{rgb, kind, ids, layers})
		}
	}
	for _, id := range indices {
		add(lib.Filaments[id].RGB, "filament", []int{id}, 0)
	}
	for _, bottom := range indices {
		for _, top := range indices {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if bottom == top {
				continue
			}
			v := LinearRGB(lib.Filaments[bottom].RGB)
			for layer := 1; layer <= o.TransitionLayers(); layer++ {
				v = blend(v, lib.Filaments[top], o)
				add(FromLinear(v), "pairwise-perceived-hue", []int{bottom, top}, layer)
			}
		}
	}
	return out, nil
}
func candidateScore(ctx context.Context, candidates []guidanceCandidate, target []Vec, weights []float64, o Options) (float64, error) {
	labs := make([]Vec, len(candidates))
	for i, c := range candidates {
		labs[i] = o.colorVector(c.rgb)
	}
	relevant := map[int]bool{}
	total := 0.
	for i, t := range target {
		best, pos := math.Inf(1), 0
		for j, c := range labs {
			if d := distance(t, c); d < best {
				best = d
				pos = j
			}
		}
		relevant[pos] = true
		total += weights[i] * best
	}
	if len(relevant) <= o.HueForge.MaxPerceivedColors {
		return total, nil
	}
	ids := []int{}
	for id := range relevant {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	subset := []Vec{}
	for _, id := range ids {
		subset = append(subset, labs[id])
	}
	_, _, rms, err := selectReachable(ctx, subset, target, weights, o.HueForge.MaxPerceivedColors, o.MinClusterFraction)
	return rms * rms, err
}
func guide(ctx context.Context, palette []PaletteEntry, lib Library, o Options, progress Reporter) ([]PaletteEntry, *GuidancePlan, error) {
	target, weights := targets(palette, o)
	cache := map[string]float64{}
	score := func(ids []int) (float64, error) {
		ids = sorted(ids)
		key := fmt.Sprint(ids)
		if v, ok := cache[key]; ok {
			return v, nil
		}
		c, err := guidanceCandidates(ctx, ids, lib, o.HueForge)
		if err != nil {
			return 0, err
		}
		v, err := candidateScore(ctx, c, target, weights, o)
		if err == nil {
			cache[key] = v
		}
		return v, err
	}
	selected := []int{0}
	value := math.Inf(1)
	for i := range lib.Filaments {
		v, e := score([]int{i})
		if e != nil {
			return nil, nil, e
		}
		if v < value {
			value = v
			selected[0] = i
		}
	}
	for len(selected) < min(o.Colors, len(lib.Filaments)) {
		if e := report(ctx, progress, "Selecting owned filaments", .35+.18*float64(len(selected))/float64(o.Colors)); e != nil {
			return nil, nil, e
		}
		addition := -1
		best := value
		for i := range lib.Filaments {
			if contains(selected, i) {
				continue
			}
			v, e := score(append(append([]int{}, selected...), i))
			if e != nil {
				return nil, nil, e
			}
			if v < best-1e-12 {
				best = v
				addition = i
			}
		}
		if addition < 0 {
			break
		}
		selected = sorted(append(selected, addition))
		value = best
	}
	for pass := 0; pass < 3; pass++ {
		best := value
		replacement := selected
		for pos := range selected {
			for i := range lib.Filaments {
				if contains(selected, i) {
					continue
				}
				trial := append([]int{}, selected...)
				trial[pos] = i
				trial = sorted(trial)
				v, e := score(trial)
				if e != nil {
					return nil, nil, e
				}
				if v < best-1e-12 {
					best = v
					replacement = trial
				}
			}
		}
		if best >= value-1e-12 {
			break
		}
		selected = replacement
		value = best
	}
	c, err := guidanceCandidates(ctx, selected, lib, o.HueForge)
	if err != nil {
		return nil, nil, err
	}
	labs := make([]Vec, len(c))
	for i, v := range c {
		labs[i] = o.colorVector(v.rgb)
	}
	type record struct {
		rgb               RGB
		target, reference int
	}
	records := []record{}
	seen := map[RGB]int{}
	for i, t := range target {
		best, pos := math.Inf(1), 0
		for j, v := range labs {
			if d := distance(t, v); d < best {
				best = d
				pos = j
			}
		}
		v := t
		for k := range v {
			v[k] = (1-o.GuidanceStrength)*t[k] + o.GuidanceStrength*labs[pos][k]
		}
		rgb := o.colorRGB(v)
		// A target assigned to a pure-black filament stays exact black instead
		// of retaining a gray/tinted residue from partial guidance. Zero guidance
		// still preserves analyzed source colors, and pairwise hues remain modeled.
		if o.TrueBlack && o.GuidanceStrength > 0 && c[pos].kind == "filament" && c[pos].rgb == (RGB{}) {
			rgb = RGB{}
		}
		if old, ok := seen[rgb]; ok {
			if weights[i] > weights[records[old].target]+1e-15 {
				records[old] = record{rgb, i, pos}
			}
		} else {
			seen[rgb] = len(records)
			records = append(records, record{rgb, i, pos})
		}
	}
	guidedLabs := make([]Vec, len(records))
	for i, r := range records {
		guidedLabs[i] = o.colorVector(r.rgb)
	}
	ids, masses, rms, err := selectReachable(ctx, guidedLabs, target, weights, o.HueForge.MaxPerceivedColors, o.MinClusterFraction)
	if err != nil {
		return nil, nil, err
	}
	order := make([]int, len(ids))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return masses[order[i]] > masses[order[j]] })
	used := map[int]bool{}
	for _, id := range ids {
		for _, f := range c[records[id].reference].indices {
			used[f] = true
		}
	}
	reported := []int{}
	positions := map[int]int{}
	plan := &GuidancePlan{Model: "inventory-guided-pairwise-td-hues-v1", Options: o.HueForge, Strength: o.GuidanceStrength, Requested: o.Colors, Eligible: len(lib.Filaments), RMS: rms, LibrarySHA256: lib.SHA256}
	for _, id := range selected {
		if used[id] {
			reported = append(reported, id)
			positions[id] = len(reported)
			plan.Selected = append(plan.Selected, lib.Filaments[id])
		}
	}
	actual, e := guidanceCandidates(ctx, reported, lib, o.HueForge)
	if e != nil {
		return nil, nil, e
	}
	plan.CandidateCount = len(actual)
	out := []PaletteEntry{}
	for _, p := range order {
		id := ids[p]
		r := records[id]
		ref := c[r.reference]
		out = append(out, entry(r.rgb, masses[p], o.NeutralChroma))
		fp := []int{}
		for _, id := range ref.indices {
			fp = append(fp, positions[id])
		}
		plan.Colors = append(plan.Colors, GuidedColor{r.rgb, ref.rgb, ref.kind, fp, ref.layers, masses[p]})
	}
	if o.PreserveDetails {
		actual := make([]RGB, len(out))
		for i, p := range out {
			actual[i] = p.RGB
		}
		plan.RMS = paletteRMS76(actual, palette, o)
	}
	return out, plan, nil
}
