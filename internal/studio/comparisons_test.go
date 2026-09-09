package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"path/filepath"
	"testing"
)

func TestComparisonsPreserveActiveResultAndPersistFour(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode = "stack"
	r.Options.Colors = 3
	r.Options.PreblurSigma = 0
	r.Options.HueForge.MaxDepth = .8
	r.Options.HueForge.AnalysisColors = 4
	r.Options.HueForge.BeamWidth = 4
	r.LibraryPath = filepath.Join("..", "engine", "testdata", "library.json")
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	r.ID++
	alternatives, err := s.ComparePlans(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(alternatives) != 4 {
		t.Fatal("missing alternatives", len(alternatives))
	}
	if s.result != p.Result || s.resultRequest.ID != p.ID {
		t.Fatal("comparison replaced active result")
	}
	path := filepath.Join(t.TempDir(), "active.png")
	if err = s.Export("png", path, p.ID, p.Revision, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := engine.LoadImage(context.Background(), path)
	if err != nil || !bytes.Equal(loaded.Image.Pix, p.Result.Image.Pix) {
		t.Fatal("export differs after comparison", err)
	}
	for i := 0; i < 5; i++ {
		if _, err = s.CaptureComparison(r); err != nil {
			t.Fatal(err)
		}
	}
	reopened := New(context.Background(), s.configPath)
	defer reopened.Shutdown()
	if len(reopened.Snapshot().Settings.Comparisons) != 4 {
		t.Fatal("comparisons did not persist")
	}
	if err = reopened.ClearComparisons(); err != nil {
		t.Fatal(err)
	}
	again := New(context.Background(), s.configPath)
	defer again.Shutdown()
	if len(again.Snapshot().Settings.Comparisons) != 0 {
		t.Fatal("clear did not persist")
	}
	r.Options.Colors++
	if _, err = s.CaptureComparison(r); err == nil {
		t.Fatal("saved stale result")
	}
	r.Revision++
	if _, err = s.ComparePlans(r); err == nil {
		t.Fatal("compared stale document")
	}
}
func TestSpoolExclusionComparisonUsesExactFilter(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode = "guided"
	r.Options.Colors = 2
	r.Options.HueForge.MaxDepth = .8
	r.Options.HueForge.AnalysisColors = 4
	r.LibraryPath = filepath.Join("..", "engine", "testdata", "library.json")
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	f := p.Result.Guidance.Selected[0]
	out, err := s.CompareWithout(r, engine.FilamentKey(f))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || len(out[1].Filter.ExcludedIDs) != 1 || out[1].Filter.ExcludedIDs[0] != f.SourceIndex {
		t.Fatal("wrong exclusion", out)
	}
	if s.result != p.Result || len(s.settings.Filter.ExcludedIDs) != 0 {
		t.Fatal("comparison changed active settings")
	}
	r.Options.HueForge.RequiredFilaments = engine.FilamentKey(f)
	if _, err = s.CompareWithout(r, engine.FilamentKey(f)); err == nil {
		t.Fatal("silently excluded a required spool")
	}
}

func TestCanceledComparisonCannotReplaceOrSaveResult(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Options.Mode = "stack"
	r.LibraryPath = filepath.Join("..", "engine", "testdata", "library.json")
	s.Emit = func(name string, v any) {
		if name == "progress" {
			s.Cancel()
		}
	}
	if _, err = s.ComparePlans(r); err != context.Canceled {
		t.Fatal("comparison did not cancel", err)
	}
	if s.result != p.Result || len(s.settings.Comparisons) != 0 {
		t.Fatal("canceled work changed saved state")
	}
}
