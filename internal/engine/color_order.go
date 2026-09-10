package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

type colorOrderKey struct{}
type colorOrderSource struct {
	groups []string
	areas  []float64
}
type ColorOrderGroup struct {
	Key            string  `json:"key"`
	RGB            RGB     `json:"rgb"`
	SourceFraction float64 `json:"sourceFraction"`
	MeanHeight     float64 `json:"meanHeightMm"`
}
type ColorOrderReport struct {
	Groups          []ColorOrderGroup `json:"groups"`
	Requested       string            `json:"requested"`
	Weight          float64           `json:"weight"`
	OrderedFraction float64           `json:"orderedFraction"`
	Message         string            `json:"message"`
}

func colorOrderGroup(c RGB) string {
	f := layerColorFamily(c)
	if f != "neutral" {
		return f
	}
	switch l := ToLab(c)[0]; {
	case l < 25:
		return "black"
	case l > 75:
		return "white"
	default:
		return "gray"
	}
}

func (h HueForgeOptions) validateColorOrder() error {
	if !finite(h.ColorOrderWeight) || h.ColorOrderWeight < 0 || h.ColorOrderWeight > 100 {
		return fmt.Errorf("color order weight must be between 0 and 100")
	}
	if h.ColorOrder == "" {
		return nil
	}
	seen := map[string]bool{}
	for _, key := range strings.Split(h.ColorOrder, ",") {
		switch key {
		case "black", "gray", "white", "red", "yellow", "green", "cyan", "blue", "purple":
		default:
			return fmt.Errorf("unknown color order group %q", key)
		}
		if seen[key] {
			return fmt.Errorf("duplicate color order group %q", key)
		}
		seen[key] = true
	}
	if len(seen) < 2 {
		return fmt.Errorf("color order needs at least two groups")
	}
	return nil
}

func (o Options) customColorOrder() bool {
	return o.Mode == "stack" && !o.ColorPop.Enabled && !o.fixedHeights() && o.HueForge.ColorOrder != "" && o.HueForge.ColorOrderWeight > 0
}

func orderSource(p []PaletteEntry) colorOrderSource {
	s := colorOrderSource{groups: make([]string, len(p)), areas: paletteAreas(p)}
	for i, c := range p {
		s.groups[i] = colorOrderGroup(c.RGB)
	}
	return s
}

// Penalize inversions of actual source-to-output height assignments. Each
// requested pair has equal influence, so a small accent is not overwhelmed by
// background coverage. Extra height beyond one layer earns no further reward.
func colorOrderPenalty(s colorOrderSource, assigned []float64, o Options) (float64, float64) {
	if !o.customColorOrder() || len(assigned) != len(s.groups) {
		return 0, 0
	}
	mass := map[string]float64{}
	for i, key := range s.groups {
		mass[key] += s.areas[i]
	}
	order := strings.Split(o.HueForge.ColorOrder, ",")
	penalty, ordered, pairs := 0., 0., 0.
	for a, lower := range order {
		for _, upper := range order[a+1:] {
			if mass[lower] == 0 || mass[upper] == 0 {
				continue
			}
			pairs++
			for i, key := range s.groups {
				if key != lower {
					continue
				}
				for j, other := range s.groups {
					if other != upper {
						continue
					}
					weight := s.areas[i] * s.areas[j] / (mass[lower] * mass[upper])
					d := math.Max(0, assigned[i]+o.HueForge.LayerHeight-assigned[j])
					penalty += weight * d * d
					if d < 1e-8 {
						ordered += weight
					}
				}
			}
		}
	}
	if pairs == 0 {
		return 0, 0
	}
	return 128 * o.HueForge.ColorOrderWeight / 100 * penalty / pairs, ordered / pairs
}

func colorOrderReport(p, out []PaletteEntry, o Options) *ColorOrderReport {
	if o.Mode != "stack" || o.ColorPop.Enabled || o.fixedHeights() {
		return nil
	}
	selected, heights := make([]RGB, len(out)), make([]int, len(out))
	for i, c := range out {
		selected[i], heights[i] = c.RGB, c.StackLayer
	}
	target, _ := targets(p, o)
	assigned := preferenceHeights(selected, heights, target, o)
	s := orderSource(p)
	groups := map[string]*ColorOrderGroup{}
	colors := map[string][3]float64{}
	for i, key := range s.groups {
		if groups[key] == nil {
			groups[key] = &ColorOrderGroup{Key: key}
		}
		g := groups[key]
		g.SourceFraction += s.areas[i]
		g.MeanHeight += s.areas[i] * assigned[i]
		c := colors[key]
		for k := range c {
			c[k] += s.areas[i] * float64(p[i].RGB[k])
		}
		colors[key] = c
	}
	r := &ColorOrderReport{Groups: []ColorOrderGroup{}, Message: "Drag colors into your preferred order, from bottom to top."}
	for key, g := range groups {
		if g.SourceFraction <= 0 {
			continue
		}
		g.MeanHeight /= g.SourceFraction
		for k, v := range colors[key] {
			g.RGB[k] = uint8(math.Round(v / g.SourceFraction))
		}
		r.Groups = append(r.Groups, *g)
	}
	sort.Slice(r.Groups, func(i, j int) bool {
		if r.Groups[i].MeanHeight != r.Groups[j].MeanHeight {
			return r.Groups[i].MeanHeight < r.Groups[j].MeanHeight
		}
		return r.Groups[i].Key < r.Groups[j].Key
	})
	if o.customColorOrder() {
		r.Requested, r.Weight = o.HueForge.ColorOrder, o.HueForge.ColorOrderWeight
		_, r.OrderedFraction = colorOrderPenalty(s, assigned, o)
		r.Message = "Preferred order applied to source color heights."
		if r.OrderedFraction < .999 {
			r.Message = "Some colors overlap in height. Color accuracy, available blends, and filament constraints limit the preferred order."
		}
	}
	return r
}

// Supply a complete alternative stack with the highest requested color near
// the top. This is only a search seed; final scoring still enforces all pins
// and the baseline color limits.
func colorOrderSeed(ctx context.Context, palette []PaletteEntry, o Options) *layerPreference {
	if !o.customColorOrder() {
		return nil
	}
	order := strings.Split(o.HueForge.ColorOrder, ",")
	for i := len(order) - 1; i >= 0; i-- {
		best := -1
		for j, p := range palette {
			if colorOrderGroup(p.RGB) == order[i] && (best < 0 || p.Fraction > palette[best].Fraction) {
				best = j
			}
		}
		if best >= 0 {
			return &layerPreference{family: order[i], representative: palette[best].RGB}
		}
	}
	return nil
}
