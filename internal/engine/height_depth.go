package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// Each trial recomputes its bands and refits the complete stack. Comparing the
// final image's CIELAB error includes within-height target variance, which a
// score of only each height's mean color would incorrectly omit.
func chooseHeightDepth(ctx context.Context, o Options, progress Reporter, plan func(Options, Reporter) (*Result, error)) (*Result, error) {
	if !o.HueForge.AutoDepth {
		return plan(o, progress)
	}
	maximum := o.HueForge.MaxLayers()
	minimum := o.HueForge.BaseLayers() + 1
	if o.ColorPop.Enabled {
		minimum = o.HueForge.BaseLayers() + 3 + o.ColorPop.GapLayers
	}
	if minimum > maximum {
		return nil, fmt.Errorf("maximum depth leaves insufficient room for height bands")
	}
	limit := 24
	if o.HueForge.SearchEffort == "refine" {
		limit = 48
	}
	depths := map[int]bool{minimum: true, maximum: true}
	for i := 0; i < min(limit, maximum-minimum+1); i++ {
		depths[minimum+int(math.Round(float64(i)*float64(maximum-minimum)/float64(max(1, min(limit, maximum-minimum+1)-1))))] = true
	}
	ordered := []int{}
	for n := range depths {
		ordered = append(ordered, n)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ordered)))
	type candidate struct {
		layers int
		depth  float64
		score  float64
	}
	candidates := []candidate{}
	bestScore := math.Inf(1)
	var lastErr error
	for i, n := range ordered {
		if err := report(ctx, progress, fmt.Sprintf("Comparing height plans · depth %d of %d", i+1, len(ordered)), .15+.7*float64(i)/float64(len(ordered))); err != nil {
			return nil, err
		}
		trial := o
		trial.HueForge.AutoDepth = false
		trial.HueForge.MaxDepth = trial.HueForge.Height(n)
		r, err := plan(trial, nil)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			continue
		}
		score := r.Quality.RMS
		// Retain scalars only: dozens of full-resolution images would exceed the
		// memory budget. Rebuild the deterministic selected plan once at the end.
		candidates = append(candidates, candidate{n, r.Stack.PlannedDepth, score})
		bestScore = min(bestScore, score)
	}
	if len(candidates) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("no depth satisfies the height plan: %w", lastErr)
		}
		return nil, fmt.Errorf("no usable height plan")
	}
	selectedLayers := 0
	selectedDepth := math.Inf(1)
	selectedScore := math.Inf(1)
	for _, c := range candidates {
		if c.score > bestScore*(1+o.HueForge.DepthTolerance/100)+1e-9 {
			continue
		}
		if c.depth < selectedDepth || (c.depth == selectedDepth && c.score < selectedScore) {
			selectedLayers, selectedDepth, selectedScore = c.layers, c.depth, c.score
		}
	}
	trial := o
	trial.HueForge.AutoDepth = false
	trial.HueForge.MaxDepth = trial.HueForge.Height(selectedLayers)
	selected, err := plan(trial, nil)
	if err != nil {
		return nil, err
	}
	selected.Stack.Options = o.HueForge
	selected.Stack.DepthSelection = &DepthSelection{HardMaximum: o.HueForge.MaxDepth, PrintableMaximum: o.HueForge.Height(maximum), ComparedDepths: len(candidates), BestScore: bestScore, SelectedScore: selectedScore, TolerancePercent: o.HueForge.DepthTolerance, ScoreMetric: "full-image RMS CIELAB Delta E76"}
	return selected, nil
}
