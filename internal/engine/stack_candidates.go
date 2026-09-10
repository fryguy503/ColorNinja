package engine

import (
	"fmt"
	"strconv"
)

func stackScheduleKey(ids, runs []int) string {
	key := make([]byte, 0, len(ids)*12)
	for i, id := range ids {
		key = strconv.AppendInt(key, int64(id), 10)
		key = append(key, ':')
		key = strconv.AppendInt(key, int64(runs[i]), 10)
		key = append(key, ',')
	}
	return string(key)
}

// Enumerate small moves exactly and sample the full donor range in deep
// stacks. Always include one layer and the entire legal block so refinement
// can cross flat or temporarily worse intermediate allocations.
func layerTransfers(available int, effort string) []int {
	limit := 16
	if effort == "refine" {
		limit = 32
	}
	out := make([]int, 0, min(available, limit))
	for i := 1; i <= min(available, limit); i++ {
		n := i
		if available > limit && i > limit/2 {
			n = limit/2 + (available-limit/2)*(i-limit/2)/(limit-limit/2)
		}
		out = append(out, n)
	}
	return out
}

func diverseStacks(states []stackState, limit int) []stackState {
	if limit <= 0 {
		return nil
	}
	out := []stackState{}
	seen := map[string]bool{}
	for _, s := range states {
		if !finite(s.score) {
			continue
		}
		key := fmt.Sprint(s.indices)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	// Preserve the original distinct-order winners, then retain at most one
	// separated allocation per order. Nearby swap heights do not all deserve
	// a slot, but a different substrate thickness can open a new blend basin.
	for _, winner := range out[:len(out):len(out)] {
		var alternative stackState
		separation := 1
		for _, s := range states {
			if !finite(s.score) || !sameInts(s.indices, winner.indices) {
				continue
			}
			a, b, d := 0, 0, 0
			for i := 0; i+1 < len(s.runs); i++ {
				a, b = a+s.runs[i], b+winner.runs[i]
				d = max(d, max(a-b, b-a))
			}
			if d > separation {
				alternative, separation = s, d
			}
		}
		if separation > 1 {
			out = append(out, alternative)
		}
	}
	return out
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Move a whole filament run with its thickness. Swapping identities alone
// leaves the old thicknesses behind and can miss a useful later accent band.
func relocateRuns(s stackState, o Options, try func([]int, []int) error) error {
	for from := range s.indices {
		for to := range s.indices {
			if from == to {
				continue
			}
			ids, runs := append([]int(nil), s.indices...), append([]int(nil), s.runs...)
			id, count := ids[from], runs[from]
			ids, runs = append(ids[:from], ids[from+1:]...), append(runs[:from], runs[from+1:]...)
			ids, runs = append(ids, 0), append(runs, 0)
			copy(ids[to+1:], ids[to:len(ids)-1])
			copy(runs[to+1:], runs[to:len(runs)-1])
			ids[to], runs[to] = id, count
			if runs[0] < o.HueForge.BaseLayers() {
				continue
			}
			if err := try(ids, runs); err != nil {
				return err
			}
		}
	}
	return nil
}

// Structural moves conserve depth and respect the unique-spool/run budgets.
func structuralMoves(s stackState, lib Library, o Options, try func([]int, []int) error) error {
	for p := 1; p < len(s.indices); p++ {
		for _, neighbor := range []int{p - 1, p + 1} {
			if neighbor >= len(s.indices) {
				continue
			}
			ids, runs := append([]int(nil), s.indices...), append([]int(nil), s.runs...)
			runs[neighbor] += runs[p]
			ids = append(ids[:p], ids[p+1:]...)
			runs = append(runs[:p], runs[p+1:]...)
			for i := 1; i < len(ids); {
				if ids[i] == ids[i-1] {
					runs[i-1] += runs[i]
					ids = append(ids[:i], ids[i+1:]...)
					runs = append(runs[:i], runs[i+1:]...)
				} else {
					i++
				}
			}
			if err := try(ids, runs); err != nil {
				return err
			}
		}
	}
	for p, n := range s.runs {
		minimum := 1
		if p == 0 {
			minimum = o.HueForge.BaseLayers()
		}
		if n <= minimum {
			continue
		}
		for id := range lib.Filaments {
			if id == s.indices[p] {
				continue
			}
			// Insert a one-layer run after each donor; transfers refine its depth.
			ids, runs := append([]int(nil), s.indices[:p+1]...), append([]int(nil), s.runs[:p+1]...)
			runs[p]--
			ids = append(ids, id)
			runs = append(runs, 1)
			ids = append(ids, s.indices[p+1:]...)
			runs = append(runs, s.runs[p+1:]...)
			if err := try(ids, runs); err != nil {
				return err
			}
			if o.HueForge.MaxRuns > 0 && n >= minimum+2 {
				ids = append([]int(nil), s.indices[:p+1]...)
				runs = append([]int(nil), s.runs[:p+1]...)
				runs[p]--
				last := runs[p] - minimum
				runs[p] = minimum
				ids = append(ids, id, s.indices[p])
				runs = append(runs, 1, last)
				ids = append(ids, s.indices[p+1:]...)
				runs = append(runs, s.runs[p+1:]...)
				if err := try(ids, runs); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
