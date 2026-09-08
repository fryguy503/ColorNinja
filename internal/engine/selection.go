package engine

import (
	"context"
	"math"
	"sort"
)

func targets(p []PaletteEntry, o Options) ([]Vec, []float64) {
	labs := make([]Vec, len(p))
	weights := make([]float64, len(p))
	sum := 0.
	for i, e := range p {
		labs[i] = o.colorVector(e.RGB)
		weights[i] = math.Max(0, e.Fraction)
		sum += weights[i]
	}
	for i := range weights {
		if sum == 0 {
			weights[i] = 1 / float64(len(p))
		} else {
			weights[i] /= sum
		}
	}
	return labs, weights
}
func contains(a []int, n int) bool {
	for _, v := range a {
		if v == n {
			return true
		}
	}
	return false
}
func sorted(a []int) []int { b := append([]int{}, a...); sort.Ints(b); return b }
func lexLess(a, b []int) bool {
	for i := 0; i < min(len(a), len(b)); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
func dot(a, b []float64) float64 {
	v := 0.
	for i := range a {
		v += a[i] * b[i]
	}
	return v
}
func labColors(a []RGB) []Vec {
	b := make([]Vec, len(a))
	for i, c := range a {
		b[i] = ToLab(c)
	}
	return b
}

// selectReachable chooses fixed candidate colors; it never invents unreachable
// cluster centroids. Small capped sets are exhaustive; larger sets use
// deterministic multistart greedy selection with local swaps and repeated culls.
func selectReachable(ctx context.Context, candidates, target []Vec, weights []float64, limit int, minFraction float64) ([]int, []float64, float64, error) {
	n := len(candidates)
	limit = min(limit, n)
	d := make([][]float64, len(target))
	for i, t := range target {
		d[i] = make([]float64, n)
		for j, c := range candidates {
			d[i][j] = distance(t, c)
		}
	}
	score := func(ids []int) float64 {
		s := 0.
		for i := range target {
			b := math.Inf(1)
			for _, j := range ids {
				b = math.Min(b, d[i][j])
			}
			s += weights[i] * b
		}
		return s
	}
	masses := func(ids []int) []float64 {
		m := make([]float64, len(ids))
		for i := range target {
			best, pos := math.Inf(1), 0
			for j, id := range ids {
				if d[i][id] < best {
					best = d[i][id]
					pos = j
				}
			}
			m[pos] += weights[i]
		}
		return m
	}
	combinations := 1.
	for k := 1; k <= limit; k++ {
		combinations *= float64(n-limit+k) / float64(k)
		if combinations > 50000 {
			break
		}
	}
	var selected []int
	if limit == n {
		for i := 0; i < n; i++ {
			selected = append(selected, i)
		}
	} else if limit <= 4 && combinations <= 50000 {
		best := math.Inf(1)
		var visit func([]int, int)
		visit = func(ids []int, start int) {
			if ctx.Err() != nil {
				return
			}
			if len(ids) == limit {
				if s := score(ids); s < best-1e-12 {
					best = s
					selected = append([]int{}, ids...)
				}
				return
			}
			for j := start; j <= n-(limit-len(ids)); j++ {
				visit(append(ids, j), j+1)
			}
		}
		visit(nil, 0)
	} else {
		best := math.Inf(1)
		for start := 0; start < n; start++ {
			if e := ctx.Err(); e != nil {
				return nil, nil, 0, e
			}
			trial := []int{start}
			value := score(trial)
			for len(trial) < limit {
				addition := -1
				next := value
				for j := 0; j < n; j++ {
					if contains(trial, j) {
						continue
					}
					s := score(append(append([]int{}, trial...), j))
					if s < next-1e-12 {
						addition = j
						next = s
					}
				}
				if addition < 0 {
					break
				}
				trial = append(trial, addition)
				value = next
			}
			for {
				if e := ctx.Err(); e != nil {
					return nil, nil, 0, e
				}
				replacement := trial
				next := value
				for p := range trial {
					for j := 0; j < n; j++ {
						if contains(trial, j) {
							continue
						}
						t := append([]int{}, trial...)
						t[p] = j
						t = sorted(t)
						if s := score(t); s < next-1e-12 {
							replacement = t
							next = s
						}
					}
				}
				if next >= value-1e-12 {
					break
				}
				trial = replacement
				value = next
			}
			trial = sorted(trial)
			if value < best-1e-12 || (math.Abs(value-best) <= 1e-12 && (selected == nil || lexLess(trial, selected))) {
				best = value
				selected = trial
			}
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, 0, e
	}
	removed := map[int]bool{}
	cull := func() {
		for len(selected) > 1 {
			selected = sorted(selected)
			m := masses(selected)
			keep := []int{}
			largest := 0
			for i, v := range m {
				if v > m[largest] {
					largest = i
				}
				if v > 1e-15 && v >= minFraction {
					keep = append(keep, selected[i])
				}
			}
			if len(keep) == len(selected) {
				break
			}
			if len(keep) == 0 {
				keep = append(keep, selected[largest])
			}
			for _, v := range selected {
				if !contains(keep, v) {
					removed[v] = true
				}
			}
			selected = keep
		}
	}
	cull()
	for len(selected) < limit {
		if e := ctx.Err(); e != nil {
			return nil, nil, 0, e
		}
		best := score(selected)
		addition := -1
		for j := 0; j < n; j++ {
			if contains(selected, j) || removed[j] {
				continue
			}
			t := sorted(append(append([]int{}, selected...), j))
			m := masses(t)
			pos := sort.SearchInts(t, j)
			if m[pos] < math.Max(minFraction, 1e-15) {
				continue
			}
			if s := score(t); s < best-1e-12 {
				addition = j
				best = s
			}
		}
		if addition < 0 {
			break
		}
		selected = append(selected, addition)
	}
	cull()
	selected = sorted(selected)
	return selected, masses(selected), math.Sqrt(score(selected)), ctx.Err()
}
