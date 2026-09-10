package engine

import (
	"context"
	"math"
)

func (o Options) prioritizeColors() bool {
	return o.ColorPriority == "distinctive" || o.ColorPriority == "vivid"
}

func (o Options) chromaPriority() float64 {
	if o.ColorPriority == "vivid" {
		return 3
	}
	if o.ColorPriority == "distinctive" {
		return 2
	}
	return 1
}

// Priority changes selection weights, never the image's measured area or
// saturation. Nearby hues share support so a heavily shaded background cannot
// earn an independent rarity bonus for every one of its shades.
func priorityWeights(pts []point, o Options) []float64 {
	const families = 12
	var hueMass [families]float64
	total := 0.
	hue := func(v Vec) (int, int, float64) {
		h := math.Atan2(v[2], v[1]) * families / (2 * math.Pi)
		if h < 0 {
			h += families
		}
		i := int(h) % families
		return i, (i + 1) % families, h - math.Floor(h)
	}
	for _, p := range pts {
		total += p.mass
		if math.Hypot(p.lab[1], p.lab[2])/o.chromaPriority() >= 4 {
			i, j, t := hue(p.lab)
			hueMass[i] += p.mass * (1 - t)
			hueMass[j] += p.mass * t
		}
	}
	out := make([]float64, len(pts))
	for i, p := range pts {
		boost := 1.
		chroma := math.Hypot(p.lab[1], p.lab[2]) / o.chromaPriority()
		// Do not promote isolated rare bins just because their RGB is vivid.
		if chroma >= 4 && (p.detail || p.mass >= total*.0005) {
			a, b, t := hue(p.lab)
			family := hueMass[a]*(1-t) + hueMass[b]*t
			exponent, saturation, cap := .45, .6, 8.
			if o.ColorPriority == "vivid" {
				exponent, saturation, cap = .6, 1.8, 12
			}
			boost = math.Min(cap, math.Pow(total/math.Max(family, 1e-15), exponent))
			boost *= 1 + saturation*math.Min(1, chroma/25)
		}
		// Light/dark anchors remain important even in an image with many hues.
		if p.lab[0] < 18 || p.lab[0] > 88 {
			boost = math.Max(boost, 1.35)
		}
		out[i] = p.mass * boost
	}
	return out
}

func priorityCluster(ctx context.Context, pts []point, o Options, budget int) ([]Vec, []float64, error) {
	if len(pts) == 0 {
		return nil, nil, nil
	}
	weighted := append([]point(nil), pts...)
	weights := priorityWeights(pts, o)
	for i := range weighted {
		weighted[i].mass = weights[i]
	}
	options := o
	options.Colors = budget
	centers, masses, err := cluster(ctx, weighted, options)
	if err != nil {
		return nil, nil, err
	}
	separation := 4.
	if o.ColorPriority == "vivid" {
		separation = 5.5
	}
	// Consolidate nearly interchangeable shades after weighted fitting. The
	// requested count is a ceiling; a simple image need not fill every slot.
	for len(centers) > 1 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if err := anchorNeutralCenters(ctx, pts, centers, o); err != nil {
			return nil, nil, err
		}
		_, _, masses, err = assign(ctx, weighted, centers)
		if err != nil {
			return nil, nil, err
		}
		a, b, nearest := -1, -1, separation*separation
		for i := range centers {
			for j := i + 1; j < len(centers); j++ {
				if d := distance(centers[i], centers[j]); d < nearest {
					a, b, nearest = i, j, d
				}
			}
		}
		if a < 0 {
			break
		}
		mass := masses[a] + masses[b]
		for c := range centers[a] {
			centers[a][c] = (centers[a][c]*masses[a] + centers[b][c]*masses[b]) / math.Max(mass, 1e-15)
		}
		centers = append(centers[:b], centers[b+1:]...)
		centers, masses, err = refine(ctx, weighted, centers, min(8, o.Iterations))
		if err != nil {
			return nil, nil, err
		}
	}
	// Actual alpha-weighted pixel area, not importance, is returned to reports.
	_, _, masses, err = assign(ctx, pts, centers)
	return centers, masses, err
}

// A mostly neutral shape should not inherit a color cast from a small vivid
// region that received a larger selection weight. Recenter such a cluster on
// its neutral source pixels, using their actual alpha-weighted area.
func anchorNeutralCenters(ctx context.Context, pts []point, centers []Vec, o Options) error {
	labels, _, masses, err := assign(ctx, pts, centers)
	if err != nil {
		return err
	}
	neutral := make([]float64, len(centers))
	sums := make([]Vec, len(centers))
	for i, p := range pts {
		if math.Hypot(p.lab[1], p.lab[2])/o.chromaPriority() >= 4 {
			continue
		}
		j := labels[i]
		neutral[j] += p.mass
		for c := range p.lab {
			sums[j][c] += p.mass * p.lab[c]
		}
	}
	for j := range centers {
		if neutral[j] > 0 && neutral[j] >= .75*masses[j] {
			for c := range centers[j] {
				centers[j][c] = sums[j][c] / neutral[j]
			}
		}
	}
	return ctx.Err()
}
