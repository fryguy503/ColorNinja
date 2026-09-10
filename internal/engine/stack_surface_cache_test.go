package engine

import (
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
)

// Frozen pre-optimization scorer: exact arithmetic is the compatibility oracle.
func TestStackSurfaceCacheMatchesOriginal(t *testing.T) {
	rng := rand.New(rand.NewPCG(73, 113))
	for _, count := range []int{39, 80} {
		for _, legacy := range []bool{false, true} {
			o := DefaultOptions()
			o.Mode, o.LegacyColorPipeline, o.ColorPriority = "stack", legacy, "vivid"
			s := stackState{}
			for i := 0; i < count; i++ {
				rgb := RGB{uint8(rng.IntN(256)), uint8(rng.IntN(256)), uint8(rng.IntN(256))}
				if i > 0 && i%4 == 0 {
					rgb = s.rgbs[i-1]
				}
				s.rgbs, s.layers = append(s.rgbs, rgb), append(s.layers, i+3)
			}
			target := o.colorVectors(s.rgbs[:32])
			boundaries := []stackBoundary{}
			for a := range target {
				for b := a + 1; b < len(target); b++ {
					boundaries = append(boundaries, stackBoundary{a, b, float64(rng.IntN(99)+1) / 10000, float64(rng.IntN(4))})
				}
			}
			s.surfaceCache = newStackSurfaceCache(s)
			for trial := 0; trial < 32; trial++ {
				selected, heights := []RGB{}, []int{}
				for j := 0; j < 12; j++ {
					id := rng.IntN(count)
					selected, heights = append(selected, s.rgbs[id]), append(heights, s.layers[id])
				}
				want := referenceStackSurface(s, selected, heights, target, o, boundaries)
				for repeat := 0; repeat < 2; repeat++ {
					got := stackSurface(s, selected, heights, target, o, boundaries)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("layers=%d legacy=%t trial=%d: %+v != %+v", count, legacy, trial, got, want)
					}
				}
			}
			if count > 64 {
				for a := range s.rgbs {
					for b := range s.rgbs {
						s.surfaceCache.interval(s, s.rgbs[a], s.rgbs[b], s.layers[a], s.layers[b])
					}
				}
				if len(s.surfaceCache.intervals) != 4096 {
					t.Fatal("sparse cache did not respect its cap", len(s.surfaceCache.intervals))
				}
				selected := []RGB{s.rgbs[count-1], s.rgbs[0]}
				heights := []int{s.layers[count-1], s.layers[0]}
				if got, want := stackSurface(s, selected, heights, target, o, boundaries), referenceStackSurface(s, selected, heights, target, o, boundaries); got != want {
					t.Fatal("full cache changed scoring", got, want)
				}
			}
		}
	}
}

func TestLinearByteTableMatchesTransferFunction(t *testing.T) {
	for i := 0; i < 256; i++ {
		c := RGB{uint8(i), uint8(255 - i), uint8((i * 73) % 256)}
		got := LinearRGB(c)
		for j := range got {
			if got[j] != linear(float64(c[j])) {
				t.Fatalf("channel %d changed", c[j])
			}
		}
	}
}

func BenchmarkStackSurfaceReuse(b *testing.B) {
	o, lib := allocationFixture()
	o.HueForge.ReduceShowThrough = true
	s := rebuildStack([]int{0, 1, 2, 1}, []int{5, 12, 10, 12}, lib, o.HueForge)
	colors, layers, _ := uniqueStack(s)
	target := o.colorVectors(colors)
	edges := []stackBoundary{}
	for a := range target {
		for c := a + 1; c < len(target); c++ {
			edges = append(edges, stackBoundary{a, c, .001, 1})
		}
	}
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprintf("cached-%t", cached), func(b *testing.B) {
			s.surfaceCache = newStackSurfaceCache(s)
			b.ReportAllocs()
			for b.Loop() {
				if cached {
					stackSurface(s, colors, layers, target, o, edges)
				} else {
					referenceStackSurface(s, colors, layers, target, o, edges)
				}
			}
		})
	}
}

func referenceStackSurface(s stackState, selected []RGB, layers []int, target []Vec, o Options, boundaries []stackBoundary) StackSurface {
	stats := StackSurface{BoundaryPairs: len(boundaries)}
	if len(boundaries) == 0 {
		return stats
	}
	vectors := o.colorVectors(selected)
	assigned := make([]int, len(target))
	for i, t := range target {
		best := math.Inf(1)
		for j, c := range vectors {
			if d := distance(t, c); d < best {
				best, assigned[i] = d, j
			}
		}
	}
	// Use unweighted Oklab for the geometric color detour even if the source
	// matching prioritizes chroma or uses the legacy Lab pipeline.
	path := make([]Vec, len(s.rgbs))
	for i, c := range s.rgbs {
		path[i] = toOKLab(c)
	}
	for _, edge := range boundaries {
		a, b := assigned[edge.a], assigned[edge.b]
		lo, hi := min(layers[a], layers[b]), max(layers[a], layers[b])
		jump := float64(hi-lo) * o.HueForge.LayerHeight
		stats.MeanHeightJumpMM += edge.weight * jump
		start, end := toOKLab(selected[a]), toOKLab(selected[b])
		detour, count := 0., 0
		for i, layer := range s.layers {
			if layer > lo && layer < hi {
				detour += colorSegmentDistance(path[i], start, end)
				count++
			}
		}
		// Score integrated color deviation through the vertical interval.
		// An average lets extra opaque endpoint-colored layers dilute the
		// offending bands, rewarding padding without improving appearance.
		integratedDetour := detour * o.HueForge.LayerHeight
		if count > 0 {
			stats.RMSColorDetour += edge.weight * detour / float64(count)
		}
		// One mm of boundary relief costs four working-space color units.
		// Detours have a smaller influence so exact color fidelity still matters.
		scale := edge.geometryScale
		if scale == 0 {
			scale = 1
		}
		stats.Penalty += edge.weight * (16*jump*jump*scale + .1*integratedDetour)
	}
	stats.RMSColorDetour = math.Sqrt(stats.RMSColorDetour)
	return stats
}
