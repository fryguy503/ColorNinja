package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func beta6Fixture(t *testing.T) (*image.NRGBA, Library, Options) {
	t.Helper()
	src, err := LoadImage(context.Background(), "testdata/gradient.png")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/library.json")
	if err != nil {
		t.Fatal(err)
	}
	lib, err := ParseLibrary(raw, LibraryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	o.Mode = "stack"
	o.Colors = 3
	o.PreblurSigma = 0
	o.AnalysisMaxPixels = 1000
	o.HueForge.MaxDepth = .8
	o.HueForge.AnalysisColors = 4
	o.HueForge.BeamWidth = 6
	return src.Image, lib, o
}
func TestDownsampledCoherentMarksRetainContrast(t *testing.T) {
	o := DefaultOptions()
	o.Colors = 8
	o.TotalColors = true
	o.PreblurSigma = 0
	o.AnalysisMaxPixels = 65536
	for _, shape := range []string{"diagonal", "curve", "letter", "eyes"} {
		t.Run(shape, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, 2048, 2048))
			for i := range img.Pix {
				img.Pix[i] = 255
			}
			for y := 0; y < 2048; y++ {
				x := y
				switch shape {
				case "curve":
					x = 300 + int(90*math.Sin(float64(y)/100))
				case "letter":
					if y < 900 || y > 930 {
						continue
					}
					x = 900
				case "eyes":
					if y < 900 || y > 904 {
						continue
					}
					x = 900
				}
				img.SetNRGBA(x, y, color.NRGBA{A: 255})
				if shape == "letter" && y == 930 {
					for dx := 0; dx < 24; dx++ {
						img.SetNRGBA(x+dx, y, color.NRGBA{A: 255})
					}
				}
				if shape == "eyes" {
					for dx := 0; dx < 5; dx++ {
						img.SetNRGBA(x+dx, y, color.NRGBA{A: 255})
						img.SetNRGBA(x+30+dx, y, color.NRGBA{A: 255})
					}
				}
			}
			r, err := Process(context.Background(), img, o, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if r.AnalysisSize == r.SourceSize {
				t.Fatal("fixture was not downsampled")
			}
			darkest := 255
			for _, p := range r.Palette {
				darkest = min(darkest, int(p.RGB[0]))
			}
			if darkest > 16 {
				t.Fatalf("%s lost contrast: %v", shape, r.Palette)
			}
		})
	}
}
func TestProtectedColorsKeepBudgetAndActualFractions(t *testing.T) {
	img, lib, o := beta6Fixture(t)
	o.Mode = "standard"
	o.TotalColors = true
	o.Colors = 3
	o.ProtectedColors = "#010203,#01aBcD,#01ABCD"
	r, err := Process(context.Background(), img, o, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Palette) > 3 {
		t.Fatal("exceeded budget")
	}
	for _, c := range []RGB{{1, 2, 3}, {1, 171, 205}} {
		found := false
		for _, p := range r.Palette {
			found = found || p.RGB == c
		}
		if !found {
			t.Fatal("lost lock", c)
		}
	}
	o.Mode = "guided"
	o.ProtectedColors = "#FF0000"
	o.HueForge.RequiredFilaments = FilamentKey(lib.Filaments[0])
	r, err = Process(context.Background(), img, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.
	for _, p := range r.Palette {
		sum += p.Fraction
	}
	if math.Abs(sum-1) > 1e-8 {
		t.Fatal("analysis fractions", sum)
	}
	found := false
	for _, f := range r.Guidance.Selected {
		found = found || FilamentKey(f) == o.HueForge.RequiredFilaments
	}
	if !found {
		t.Fatal("required guided spool omitted")
	}
}
func TestRequiredStackEndsSurviveTrimmingAndExport(t *testing.T) {
	img, lib, o := beta6Fixture(t)
	o.PreserveDetails = false
	o.HueForge.SearchEffort = "refine"
	o.HueForge.BaseFilament = FilamentKey(lib.Filaments[0])
	o.HueForge.HighlightFilament = FilamentKey(lib.Filaments[1])
	o.HueForge.RequiredFilaments = FilamentKey(lib.Filaments[2])
	o.HueForge.MaxDepth = 1.12
	r, err := Process(context.Background(), img, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := []int{}
	for _, run := range r.Stack.Runs {
		for i, f := range lib.Filaments {
			if FilamentKey(f) == FilamentKey(run.Filament) {
				ids = append(ids, i)
			}
		}
	}
	if !completeConstraints(ids, lib, o) {
		t.Fatal("trimmed required spool", r.Stack.Runs)
	}
	maxLayer := uint16(0)
	for _, l := range r.LayerMap {
		maxLayer = max(maxLayer, l)
	}
	if int(maxLayer) < r.Stack.Runs[len(r.Stack.Runs)-1].StartLayer {
		t.Fatal("required final spool has no printed pixels")
	}
	if !strings.Contains(r.Stack.SearchMethod, "structural-moves") {
		t.Fatal("refinement incorrectly depends on detail preservation")
	}
	o.Colors = 2
	if _, err = Process(context.Background(), img, o, &lib, nil); err == nil {
		t.Fatal("accepted infeasible required budget")
	}
}
func TestRequiredRepeatedRGBRetainsLaterHeight(t *testing.T) {
	o := DefaultOptions()
	lib := Library{Filaments: []Filament{{UUID: "a", RGB: RGB{}, TD: .01}, {UUID: "b", RGB: RGB{}, TD: .02}}}
	s := rebuildStack([]int{0, 1}, []int{5, 3}, lib, o.HueForge)
	colors, layers, positions := uniqueStack(s)
	ctx := context.WithValue(context.Background(), stackConstraintsKey{}, stackConstraints{[]int{0, 1}, 1})
	if !enforceHeightConstraints(ctx, s, colors, layers, positions, []int{0}, []Vec{o.colorVector(RGB{})}, o, nil) || layers[0] < 6 || positions[0] != 2 {
		t.Fatal("required duplicate top discarded", layers, positions)
	}
}
func TestProcessorReusesOnlyCompatibleStages(t *testing.T) {
	src, lib, o := beta6Fixture(t)
	p := Processor{}
	first, err := p.Process(context.Background(), src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(first)
	geometry := o
	geometry.HueForge.ExportWidthMM = 123
	geometry.HueForge.MeshDetailMM = .5
	geometry.CalibrationNote = "Measured lot A"
	next, err := p.Process(context.Background(), src, geometry, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Reused, "rendered result") || next.SurfaceView.WidthMM != 123 || next.Stack.Options.ExportWidthMM != 123 {
		t.Fatal("geometry reuse failed", p.Reused)
	}
	still, _ := json.Marshal(first)
	if !bytes.Equal(original, still) {
		t.Fatal("mutated saved result")
	}
	variants := []Options{o, o, o, o}
	variants[0].HueForge.ReduceShowThrough = true
	variants[1].Mode = "guided"
	variants[2].Mode = "guided"
	variants[2].TrueBlack = false
	variants[3].Colors = 2
	for _, v := range variants {
		got, err := p.Process(context.Background(), src, v, &lib, nil)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := Process(context.Background(), src, v, &lib, nil)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(fresh)
		if !bytes.Equal(a, b) || !bytes.Equal(got.Image.Pix, fresh.Image.Pix) || !reflect.DeepEqual(got.LayerMap, fresh.LayerMap) {
			t.Fatalf("cache changed result for %+v", v)
		}
		if !slices.Contains(p.Reused, "smoothing") {
			t.Fatal("did not reuse unchanged smoothing")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = p.Process(canceled, src, o, &lib, nil); err != context.Canceled {
		t.Fatal("cache ignored cancellation", err)
	}
	other := image.NewNRGBA(src.Rect)
	copy(other.Pix, src.Pix)
	if _, err = p.Process(context.Background(), other, o, &lib, nil); err != nil {
		t.Fatal(err)
	}
	if len(p.Reused) != 0 {
		t.Fatal("reused stale document", p.Reused)
	}
}
func TestCalibrationSensitivityDoesNotChangePrint(t *testing.T) {
	src, lib, o := beta6Fixture(t)
	before, err := Process(context.Background(), src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.CalibrationNote = "Lot A, neutral lamp"
	o.HueForge.TDSensitivityPercent = 10
	after, err := Process(context.Background(), src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.SHA256 != before.SHA256 || !reflect.DeepEqual(after.LayerMap, before.LayerMap) {
		t.Fatal("sensitivity altered print")
	}
	if after.Calibration.Note != o.CalibrationNote || len(after.Calibration.Sensitivity) != after.Stack.UniqueFilaments {
		t.Fatal("missing provenance/sensitivity")
	}
	for _, s := range after.Calibration.Sensitivity {
		if !finite(s.MaxDeltaE76) || s.MaxDeltaE76 < 0 {
			t.Fatal("invalid sensitivity")
		}
	}
}
func TestSurfaceMetricsCountSingleColumnAndPartialCoverage(t *testing.T) {
	o := DefaultOptions()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 3))
	for i := 0; i < 3; i++ {
		img.SetNRGBA(0, i, color.NRGBA{A: 128})
	}
	r := &Result{Stack: &StackPlan{}, Image: img, LayerMap: []uint16{5, 6, 8}}
	v, err := BuildSurfaceView(context.Background(), r, o)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.MeanJumpMM-.12) > 1e-9 || math.Abs(v.P95JumpMM-.16) > 1e-9 || v.SolidifiedFraction != 1 {
		t.Fatal("incorrect column metrics", v)
	}
}
