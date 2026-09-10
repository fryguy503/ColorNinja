package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessingWorkerBudget(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		cpus, items            int
		work, minimum, scratch int64
		want                   int
	}{
		{"single CPU", 1, 100, 100000, 1, 1, 1},
		{"dual CPU", 2, 100, 100000, 1, 1, 2},
		{"quad CPU", 4, 100, 100000, 1, 1, 4},
		{"larger CPU reserves capacity", 8, 100, 100000, 1, 1, 7},
		{"large CPU capped", 128, 100, 100000, 1, 1, 16},
		{"few tasks", 32, 3, 100000, 1, 1, 3},
		{"tiny work", 32, 100, 100, 1000, 1, 1},
		{"work scaled", 32, 100, 4500, 1000, 1, 4},
		{"scratch capped", 32, 100, 100000, 1, 8 << 20, 4},
		{"large serial scratch", 32, 100, 100000, 1, 64 << 20, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workerCount(tc.cpus, tc.items, tc.work, tc.minimum, tc.scratch); got != tc.want {
				t.Fatalf("workers=%d want %d", got, tc.want)
			}
		})
	}
	o := DefaultOptions()
	o.HueForge.MaxDepth, o.HueForge.BeamWidth, o.HueForge.AutoDepth = o.HueForge.Height(4096), 512, true
	if got := workerCount(128, 512, 1<<30, 1, stackWorkerScratch(o, 256)); got != 1 {
		t.Fatalf("deep, wide search launched %d workers", got)
	}
}

func TestParallelRangesOwnershipAndFailure(t *testing.T) {
	for _, workers := range []int{1, 2, 7, 16} {
		seen := make([]int, 103)
		err := parallelRanges(context.Background(), len(seen), workers, func(ctx context.Context, slot, lo, hi int) error {
			for i := lo; i < hi; i++ {
				seen[i]++
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for i, n := range seen {
			if n != 1 {
				t.Fatalf("workers=%d index=%d processed %d times", workers, i, n)
			}
		}
	}
	sentinel := errors.New("worker failed")
	var active atomic.Int32
	err := parallelRanges(context.Background(), 32, 4, func(ctx context.Context, slot, lo, hi int) error {
		active.Add(1)
		defer active.Add(-1)
		if slot == 2 {
			return sentinel
		}
		<-ctx.Done()
		return ctx.Err()
	})
	if err != sentinel || active.Load() != 0 {
		t.Fatal("error lost or worker still running", err, active.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := parallelRanges(ctx, 20, 4, func(context.Context, int, int, int) error {
		t.Error("started canceled work")
		return nil
	}); err != context.Canceled {
		t.Fatal(err)
	}

	// Cancel after all four callbacks have started, and verify return joins
	// every callback rather than letting a superseded job write to reused data.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 4)
	done := make(chan error, 1)
	go func() {
		done <- parallelRanges(ctx, 100, 4, func(ctx context.Context, slot, lo, hi int) error {
			active.Add(1)
			defer active.Add(-1)
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled || active.Load() != 0 {
			t.Fatal("canceled workers still active", err, active.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not join")
	}
}

func TestParallelClusteringMatchesSerialArithmetic(t *testing.T) {
	prior := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prior)
	pts := make([]point, 20000)
	centers := make([]Vec, 17)
	for i := range pts {
		pts[i] = point{lab: Vec{float64(i % 101), float64((i*73)%201) - 100, float64((i*113)%201) - 100}, mass: math.Ldexp(float64(i%7+1), -(i % 32))}
	}
	for i := range centers {
		centers[i] = pts[i*117].lab
	}
	centers[16] = centers[0] // An exact tie must select the first center.
	labels, distances, masses := make([]int, len(pts)), make([]float64, len(pts)), make([]float64, len(centers))
	for i, p := range pts {
		distances[i] = math.Inf(1)
		for j, c := range centers {
			if d := distance(p.lab, c); d < distances[i] {
				labels[i], distances[i] = j, d
			}
		}
		masses[labels[i]] += p.mass
	}
	var fitted []Vec
	var fittedMasses []float64
	for _, cpus := range []int{1, 2, 4, 8, 32} {
		runtime.GOMAXPROCS(cpus)
		gotLabels, gotDistances, gotMasses, err := assign(context.Background(), pts, centers)
		if err != nil || !reflect.DeepEqual(labels, gotLabels) || !reflect.DeepEqual(distances, gotDistances) || !reflect.DeepEqual(masses, gotMasses) {
			t.Fatalf("assignment arithmetic changed at %d CPUs: %v", cpus, err)
		}
		gotCenters, gotMasses, err := refine(context.Background(), pts, append([]Vec(nil), centers...), 8)
		if err != nil {
			t.Fatal(err)
		}
		if fitted == nil {
			fitted, fittedMasses = gotCenters, gotMasses
		} else if !reflect.DeepEqual(fitted, gotCenters) || !reflect.DeepEqual(fittedMasses, gotMasses) {
			t.Fatalf("weighted centroids changed at %d CPUs", cpus)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := assign(ctx, pts, centers); err != context.Canceled {
		t.Fatal("lost assignment cancellation", err)
	}
}

// Compare complete pixels, palette/metrics, physical layers, geometry, and
// reports, including cache reuse. Never run these tests with t.Parallel: CPU
// limits are process-wide. Logging a digest also allows before/after audits.
func TestProcessingParallelEquivalence(t *testing.T) {
	prior := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prior)
	for _, mode := range []string{"standard", "vivid", "guided", "stack", "auto-depth", "geometry", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			o := testOptions()
			o.TotalColors, o.PreblurSigma, o.AnalysisMaxPixels = true, 1, 0
			o.HueForge.MaxDepth, o.HueForge.BeamWidth = 1.28, 24
			lib := testLibrary()
			for i := 4; i < 12; i++ {
				lib.Filaments = append(lib.Filaments, Filament{UUID: fmt.Sprint(i), RGB: RGB{uint8(i * 73), uint8(i * 37), uint8(i * 113)}, TD: .3 + float64(i%7)/2})
			}
			switch mode {
			case "standard":
			case "vivid":
				o.ColorPriority = "vivid"
			case "guided":
				o.Mode = "guided"
			case "stack":
				o.Mode = "stack"
			case "auto-depth":
				o.Mode, o.HueForge.AutoDepth, o.HueForge.MaxRuns = "stack", true, 4
				o.HueForge.SearchEffort = "refine"
			case "geometry":
				o.Mode, o.HueForge.ReduceShowThrough, o.HueForge.OptimizeMaterial = "stack", true, true
				o.HueForge.BaseFilament = FilamentKey(lib.Filaments[0])
				o.HueForge.RequiredFilaments = FilamentKey(lib.Filaments[1])
			case "legacy":
				o.Mode, o.LegacyColorPipeline, o.PreserveDetails = "stack", true, false
				o.HueForge.OpticalModel, o.HueForge.FirstLayerHeight = LegacyModel, 0
			}
			img := gradient(192, 128)
			var reference *Result
			for _, cpus := range []int{1, 2, 4, 8, 32} {
				runtime.GOMAXPROCS(cpus)
				p := Processor{}
				var reporting atomic.Int32
				result, err := p.Process(context.Background(), img, o, &lib, func(Progress) {
					if reporting.Add(1) != 1 {
						t.Error("concurrent progress callbacks")
					}
					runtime.Gosched()
					reporting.Add(-1)
				})
				if err != nil {
					t.Fatal(err)
				}
				if reference == nil {
					reference = result
					raw, err := json.Marshal(struct {
						Result *Result
						Pixels []byte
						Layers []uint16
					}{result, result.Image.Pix, result.LayerMap})
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("baseline %s %x", mode, sha256.Sum256(raw))
				} else if !reflect.DeepEqual(reference, result) {
					t.Fatalf("output changed with GOMAXPROCS=%d", cpus)
				}
				reused, err := p.Process(context.Background(), img, o, &lib, nil)
				if err != nil || !reflect.DeepEqual(result, reused) {
					t.Fatal("cached result changed", err)
				}
			}
		})
	}
}

func TestParallelPlanningCancellation(t *testing.T) {
	prior := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(prior)
	o, lib := allocationFixture()
	o.HueForge.MaxDepth, o.HueForge.AutoDepth = o.HueForge.Height(100), true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		announced := false
		_, _, err := planStack(ctx, []PaletteEntry{entry(RGB{120, 150, 180}, 1, 8)}, lib, o, func(Progress) {
			if !announced {
				close(started)
				announced = true
			}
		})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal("lost cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("planning did not cancel promptly")
	}
}
