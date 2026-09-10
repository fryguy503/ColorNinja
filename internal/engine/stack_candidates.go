package engine

import "fmt"

func diverseStacks(states []stackState, limit int) []stackState {
	out := []stackState{}
	seen := map[string]bool{}
	for _, s := range states {
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
	return out
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
