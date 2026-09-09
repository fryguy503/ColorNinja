package engine

import (
	"context"
	"math"
)

const autoDepthTolerance = .01

type DepthSelection struct {
	HardMaximum      float64 `json:"hardMaximum"`
	PrintableMaximum float64 `json:"printableMaximum"`
	ComparedDepths   int     `json:"comparedDepths"`
	BestScore        float64 `json:"bestScore"`
	SelectedScore    float64 `json:"selectedScore"`
	TolerancePercent float64 `json:"tolerancePercent"`
	ScoreMetric      string  `json:"scoreMetric"`
}

// Keep the best complete stack at each printable depth. The ordinary beam still
// expands full-budget terminals, so shorter endings do not crowd them out.
type depthCandidates map[int]stackState

func stackLayers(s stackState) int {
	n := 0
	for _, run := range s.runs {
		n += run
	}
	return n
}

func depthStateLess(a, b stackState) bool {
	if a.score != b.score {
		return a.score < b.score
	}
	if len(a.indices) != len(b.indices) {
		return len(a.indices) < len(b.indices)
	}
	return stateLess(a, b)
}

func (c depthCandidates) record(s stackState) {
	layer := stackLayers(s)
	if prior, ok := c[layer]; ok && !depthStateLess(s, prior) {
		return
	}
	// Search buffers grow and runs are updated in place; retain an owned snapshot.
	s.indices = append([]int(nil), s.indices...)
	s.runs = append([]int(nil), s.runs...)
	s.rgbs = append([]RGB(nil), s.rgbs...)
	s.layers = append([]int(nil), s.layers...)
	s.positions = append([]int(nil), s.positions...)
	s.distances = append([]float64(nil), s.distances...)
	c[layer] = s
}

func (c depthCandidates) consider(ctx context.Context, s stackState, target []Vec, weights []float64, o Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if prior, ok := c[stackLayers(s)]; ok && dot(weights, s.distances) > prior.score*prior.score+1e-12 {
		return nil
	}
	var err error
	s.score, err = stateScore(ctx, s, target, weights, o)
	if err == nil {
		c.record(s)
	}
	return err
}

func (c depthCandidates) choose() (stackState, float64) {
	bestScore := math.Inf(1)
	for _, s := range c {
		bestScore = math.Min(bestScore, s.score)
	}
	limit := bestScore*(1+autoDepthTolerance) + 1e-9
	var best stackState
	for layer, s := range c {
		if s.score > limit {
			continue
		}
		if len(best.runs) == 0 || layer < stackLayers(best) || (layer == stackLayers(best) && depthStateLess(s, best)) {
			best = s
		}
	}
	return best, bestScore
}
