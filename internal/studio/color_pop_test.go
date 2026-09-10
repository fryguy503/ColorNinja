package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"path/filepath"
	"testing"
)

func TestColorPopDemoAndPortableProject(t *testing.T) {
	s := fixture(t)
	before := s.Snapshot().Source.Revision
	snap, err := s.UseColorPopDemo()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source.Revision <= before || !snap.Source.Demo || !snap.Settings.Options.ColorPop.Enabled || snap.Settings.Options.ColorPop.Selection != "selected" {
		t.Fatal("demo did not install its workflow")
	}
	if snap.Source.UniqueColors < 1000 || snap.Source.Name == "Painted dunes" {
		t.Fatal("wrong demo source")
	}
	request := Request{ID: 1, Revision: snap.Source.Revision, Options: snap.Settings.Options, Filter: engine.LibraryFilter{}}
	request.Options.AnalysisMaxPixels = 10000
	p, err := s.Process(request)
	if err != nil {
		t.Fatal(err)
	}
	if p.Result.ColorPop.ColorFraction < .05 || p.Result.ColorPop.ColorFraction > .4 {
		t.Fatal("demo did not isolate an accent")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "color-pop.colorninja")
	if err = s.SaveProject(path, request, false); err != nil {
		t.Fatal(err)
	}
	other := New(context.Background(), filepath.Join(dir, "settings.json"))
	defer other.Shutdown()
	reopened, err := other.OpenProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Settings.Options != request.Options || reopened.Preview == nil || reopened.Preview.Result.SHA256 != p.Result.SHA256 || !bytes.Equal(reopened.Preview.Result.ColorPop.SelectionPNG, p.Result.ColorPop.SelectionPNG) {
		t.Fatal("project lost Color Pop settings, selection or pixels")
	}
	if err = other.Export("png", filepath.Join(dir, "result.png"), reopened.Preview.ID, reopened.Preview.Revision, false); err != nil {
		t.Fatal(err)
	}
}

func TestColorPopStackProject(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode = "stack"
	r.Options.ColorPop = engine.DefaultColorPopOptions()
	r.Options.ColorPop.Enabled = true
	r.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
	r.Options.HueForge.MaxDepth = 1.6
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "stack.colorninja")
	if err = s.SaveProject(path, r, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := s.OpenProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Preview.Result.SHA256 != p.Result.SHA256 {
		t.Fatal("stack pixels changed")
	}
	if err = s.Export("hfp", filepath.Join(t.TempDir(), "stack.hfp"), reopened.Preview.ID, reopened.Preview.Revision, false); err != nil {
		t.Fatal(err)
	}
	comparisonRequest := r
	comparisonRequest.Revision = reopened.Source.Revision
	comparisonRequest.LibraryPath = reopened.Settings.LibraryPath
	alternatives, err := s.ComparePlans(comparisonRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(alternatives) < 3 || alternatives[0].Name != "Current bands" || alternatives[1].Options.ColorPop.ColorPercent != r.Options.ColorPop.ColorPercent+10 || alternatives[len(alternatives)-1].Options.ColorPop.GrayOnTop == r.Options.ColorPop.GrayOnTop {
		t.Fatal("comparison did not vary Color Pop geometry", alternatives)
	}
	if s.Snapshot().Preview.Result.SHA256 != p.Result.SHA256 {
		t.Fatal("comparison replaced the current result")
	}
}
