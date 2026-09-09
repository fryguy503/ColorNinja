package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
)

type GuidedColor struct {
	SourceRGB    RGB     `json:"sourceRGB"`
	RGB          RGB     `json:"rgb"`
	ReferenceRGB RGB     `json:"referenceRGB"`
	Kind         string  `json:"referenceKind"`
	Positions    []int   `json:"filamentPositions"`
	TopLayers    int     `json:"topLayers"`
	Fraction     float64 `json:"analysisFraction"`
}
type GuidancePlan struct {
	OptimizationScore     float64         `json:"optimizationScore"`
	OptimizationMetric    string          `json:"optimizationMetric"`
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
	return cachedGuidanceCandidates(ctx, indices, lib, o)
}
func guidanceCandidatesUncached(ctx context.Context, indices []int, lib Library, o HueForgeOptions) ([]guidanceCandidate, error) {
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
		v := baseOptics(lib.Filaments[id], o)
		add(v.RGB(o), "filament", []int{id}, 0)
		if o.frontlit() {
			for layer := 1; layer <= o.TransitionLayers(); layer++ {
				add(v.step(lib.Filaments[id], o, false), "single-filament-run", []int{id}, layer)
			}
		}
	}
	for _, bottom := range indices {
		for _, top := range indices {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if bottom == top {
				continue
			}
			v := baseOptics(lib.Filaments[bottom], o)
			for layer := 1; layer <= o.TransitionLayers(); layer++ {
				add(v.step(lib.Filaments[top], o, layer == 1), "pairwise-perceived-hue", []int{bottom, top}, layer)
			}
		}
	}
	return out, nil
}
func candidateScore(ctx context.Context, candidates []guidanceCandidate, target []Vec, weights []float64, o Options) (float64, error) {
	if o.HueForge.frontlit() {
		records := guidedRecords(candidates, target, weights, o)
		vectors := make([]Vec, len(records))
		for i, r := range records {
			vectors[i] = o.colorVector(r.rgb)
		}
		_, _, rms, err := selectReachable(ctx, vectors, target, weights, o.HueForge.MaxPerceivedColors, o.selectionFraction())
		return rms * rms, err
	}
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
	required, _, _, constraintErr := constraintIDs(lib, o)
	if constraintErr != nil {
		return nil, nil, constraintErr
	}
	target, weights := targets(palette, o)
	cache := map[string]float64{}
	score := func(ids []int) (float64, error) {
		for _, id := range required {
			if !contains(ids, id) {
				ids = append(append([]int{}, ids...), id)
			}
		}
		if len(ids) > o.Colors {
			return math.Inf(1), nil
		}
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
	for _, id := range required {
		if !contains(selected, id) {
			selected = append(selected, id)
		}
	}
	// A useful pair can have two individually poor solid colors. Evaluate
	// pairs together before the greedy additions, using the actual guided
	// output (strength, byte rounding, and palette cap included).
	if o.HueForge.frontlit() && o.Colors >= 2 && o.GuidanceStrength > 0 {
		for i := range lib.Filaments {
			for j := i + 1; j < len(lib.Filaments); j++ {
				v, e := score([]int{i, j})
				if e != nil {
					return nil, nil, e
				}
				if v < value-1e-12 {
					selected = []int{i, j}
					for _, id := range required {
						if !contains(selected, id) {
							selected = append(selected, id)
						}
					}
					value = v
				}
			}
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
			if contains(required, selected[pos]) {
				continue
			}
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
	records := guidedRecords(c, target, weights, o)
	guidedLabs := make([]Vec, len(records))
	for i, r := range records {
		guidedLabs[i] = o.colorVector(r.rgb)
	}
	ids, masses, rms, err := selectReachable(ctx, guidedLabs, target, weights, o.HueForge.MaxPerceivedColors, o.selectionFraction())
	if err != nil {
		return nil, nil, err
	}
	if o.prioritizeColors() || o.ProtectedColors != "" {
		selected := make([]Vec, len(ids))
		for i, id := range ids {
			selected[i] = guidedLabs[id]
		}
		masses = sourceFractions(selected, palette, o)
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
	plan.OptimizationScore, plan.OptimizationMetric = rms, optimizationMetric(o, false)
	if o.HueForge.frontlit() {
		plan.Model = FrontlitModel + "-pairwise-guide"
	}
	for _, id := range selected {
		if used[id] || contains(required, id) {
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
		plan.Colors = append(plan.Colors, GuidedColor{SourceRGB: palette[r.target].RGB, RGB: r.rgb, ReferenceRGB: ref.rgb, Kind: ref.kind, Positions: fp, TopLayers: ref.layers, Fraction: masses[p]})
	}
	{
		actual := make([]RGB, len(out))
		for i, p := range out {
			actual[i] = p.RGB
		}
		plan.RMS = paletteRMS76(actual, palette, o)
	}
	return out, plan, nil
}

type guidedRecord struct {
	rgb               RGB
	target, reference int
}

func guidedRecords(c []guidanceCandidate, target []Vec, weights []float64, o Options) []guidedRecord {
	labs := make([]Vec, len(c))
	for i, v := range c {
		labs[i] = o.colorVector(v.rgb)
	}
	records := []guidedRecord{}
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
				records[old] = guidedRecord{rgb, i, pos}
			}
		} else {
			seen[rgb] = len(records)
			records = append(records, guidedRecord{rgb, i, pos})
		}
	}
	return records
}
