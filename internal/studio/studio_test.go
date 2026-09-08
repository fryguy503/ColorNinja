package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) *Studio {
	t.Helper()
	dir := t.TempDir()
	s := New(context.Background(), filepath.Join(dir, "settings.json"))
	path := filepath.Join("..", "engine", "testdata", "gradient.png")
	if _, e := s.LoadImage(path); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Shutdown)
	return s
}
func req(s *Studio, id uint64) Request {
	snap := s.Snapshot()
	o := engine.DefaultOptions()
	o.Colors = 4
	o.AnalysisMaxPixels = 1000
	return Request{ID: id, Revision: snap.Source.Revision, Options: o, Filter: engine.LibraryFilter{}}
}
func TestPreviewAndExportUseSamePixels(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	p, e := s.Process(r)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "export.png")
	if e = s.Export("png", path, p.ID, p.Revision, false); e != nil {
		t.Fatal(e)
	}
	loaded, e := engine.LoadImage(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(loaded.Image.Pix, p.Result.Image.Pix) {
		t.Fatal("export differs from preview")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", p.URL, nil))
	decoded, e := png.Decode(w.Body)
	if e != nil {
		t.Fatal(e)
	}
	if decoded.Bounds() != p.Result.Image.Bounds() {
		t.Fatal("preview dimensions changed")
	}
}

func TestPreviewColorCountsUseFullSourceAndActualResult(t *testing.T) {
	s := fixture(t)
	const sourceColors = 95 * 64 // 96x64 RGB gradient, with a fully transparent first column.
	if s.Snapshot().Source.UniqueColors != sourceColors {
		t.Fatal("original count differs from full-resolution fixture")
	}
	r := req(s, 1)
	r.Options.AnalysisMaxPixels = 64
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Source.UniqueColors != sourceColors {
		t.Fatal("analysis size changed the original count")
	}
	seen := map[engine.RGB]bool{}
	for i := 0; i < len(p.Result.Image.Pix); i += 4 {
		pixel := p.Result.Image.Pix[i : i+4]
		if pixel[3] > 0 {
			seen[engine.RGB{pixel[0], pixel[1], pixel[2]}] = true
		}
	}
	if p.Result.UniqueColors != len(seen) {
		t.Fatal("preview count differs from rendered pixels")
	}
	path := filepath.Join(t.TempDir(), "single-color.png")
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 128})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	snap, err := s.LoadImage(path)
	if err != nil || snap.Source.UniqueColors != 1 {
		t.Fatal("new source retained old count", snap.Source.UniqueColors, err)
	}
}
func TestOldPreviewCannotExportAfterNewImage(t *testing.T) {
	s := fixture(t)
	p, e := s.Process(req(s, 1))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.LoadImage(filepath.Join("..", "engine", "testdata", "gradient.png")); e != nil {
		t.Fatal(e)
	}
	if e = s.Export("png", filepath.Join(t.TempDir(), "stale.png"), p.ID, p.Revision, false); e == nil {
		t.Fatal("stale export accepted")
	}
	if _, e = s.Process(Request{ID: 2, Revision: p.Revision, Options: engine.DefaultOptions()}); e == nil {
		t.Fatal("old source accepted")
	}
}
func TestExportNeverOverwritesInputOrLibrary(t *testing.T) {
	s := fixture(t)
	p, e := s.Process(req(s, 1))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Export("png", s.Snapshot().Source.Path, p.ID, p.Revision, true); e == nil {
		t.Fatal("input overwrite accepted")
	}
}
func TestSupersededWorkerCannotPublish(t *testing.T) {
	s := fixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.Emit = func(name string, v any) {
		p := v.(Progress)
		if p.ID == 1 {
			once.Do(func() { close(started); <-release })
		}
	}
	first := make(chan error, 1)
	go func() { _, e := s.Process(req(s, 1)); first <- e }()
	<-started
	s.mu.RLock()
	firstJob := s.job
	s.mu.RUnlock()
	second := make(chan *Preview, 1)
	errCh := make(chan error, 1)
	go func() {
		p, e := s.Process(req(s, 2))
		if e != nil {
			errCh <- e
			return
		}
		second <- p
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.RLock()
		job := s.job
		s.mu.RUnlock()
		if job > firstJob {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("replacement did not start")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	if e := <-first; !IsCanceled(e) {
		t.Fatalf("first job: %v", e)
	}
	select {
	case e := <-errCh:
		t.Fatal(e)
	case p := <-second:
		if p.ID != 2 {
			t.Fatal(p.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not finish")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.resultRequest.ID != 2 {
		t.Fatal("old worker published")
	}
}
func TestCancelDoesNotPublish(t *testing.T) {
	s := fixture(t)
	s.Emit = func(name string, v any) { s.Cancel() }
	_, e := s.Process(req(s, 1))
	if !IsCanceled(e) {
		t.Fatal(e)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.result != nil {
		t.Fatal("canceled result published")
	}
}
func TestPresetsAndProjectRoundTrip(t *testing.T) {
	s := fixture(t)
	o := req(s, 1).Options
	o.Colors = 7
	if _, e := s.SavePreset("Seven", o); e != nil {
		t.Fatal(e)
	}
	fresh := New(context.Background(), s.configPath)
	if len(fresh.settings.Presets) != 1 || fresh.settings.Presets[0].Options.Colors != 7 {
		t.Fatal("preset lost")
	}
	p := filepath.Join(t.TempDir(), "project.colorninja.json")
	r := req(s, 1)
	r.Options = o
	if e := s.SaveProject(p, r, false); e != nil {
		t.Fatal(e)
	}
	snapshot, e := s.OpenProject(p)
	if e != nil {
		t.Fatal(e)
	}
	if snapshot.Settings.Options.Colors != 7 {
		t.Fatal("project settings lost")
	}
	if e = s.SaveProject(p, req(s, 3), false); e == nil {
		t.Fatal("project clobbered")
	}
}
func TestInvalidProjectLeavesDocumentIntact(t *testing.T) {
	s := fixture(t)
	before := s.Snapshot()
	p := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(p, []byte(`{"schemaVersion":99}`), 0600)
	if _, e := s.OpenProject(p); e == nil {
		t.Fatal("invalid project accepted")
	}
	if s.Snapshot().Source.Revision != before.Source.Revision {
		t.Fatal("document replaced")
	}
}
func TestMalformedSettingsRecover(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte("garbage"), 0600)
	s := New(context.Background(), p)
	if s.warning == "" || s.settings.Options.Validate() != nil {
		t.Fatal("invalid settings not recovered")
	}
}

func TestPreservationOptionsPersistInProjectsPresetsAndSettings(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	if !r.Options.TrueBlack || !r.Options.PreserveDetails {
		t.Fatal("new option must default on")
	}
	for _, enabled := range []bool{false, true} {
		r.Revision = s.Snapshot().Source.Revision
		r.Options.TrueBlack = enabled
		r.Options.PreserveDetails = enabled
		if _, e := s.Process(r); e != nil {
			t.Fatal(e)
		}
		if _, e := s.SavePreset("Saved black preference", r.Options); e != nil {
			t.Fatal(e)
		}
		project := filepath.Join(t.TempDir(), "black.colorninja.json")
		if e := s.SaveProject(project, r, false); e != nil {
			t.Fatal(e)
		}
		snapshot, e := s.OpenProject(project)
		if e != nil {
			t.Fatal(e)
		}
		if snapshot.Settings.Options.TrueBlack != enabled || snapshot.Settings.Options.PreserveDetails != enabled {
			t.Fatal("project lost option")
		}
		reopened := New(context.Background(), s.configPath)
		if reopened.settings.Options.TrueBlack != enabled || reopened.settings.Presets[0].Options.TrueBlack != enabled || reopened.settings.Options.PreserveDetails != enabled || reopened.settings.Presets[0].Options.PreserveDetails != enabled {
			t.Fatal("preferences or preset lost option")
		}
		reopened.Shutdown()
	}
}
func TestReportSeparatesGuidanceAndStack(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode = "guided"
	r.Options.Colors = 2
	r.LibraryPath = filepath.Join("..", "engine", "testdata", "library.json")
	r.Options.HueForge.MaxDepth = .8
	p, e := s.Process(r)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if e = s.Export("palette", path, p.ID, p.Revision, false); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(path)
	var report engine.Report
	if e = json.Unmarshal(raw, &report); e != nil {
		t.Fatal(e)
	}
	if report.Result.Stack != nil || report.Result.Guidance == nil || report.Result.Guidance.GlobalStackGuaranteed {
		t.Fatal("invalid guidance report")
	}
	if strings.Contains(string(raw), s.Snapshot().Source.Path) {
		t.Fatal("report includes absolute input path")
	}
}
