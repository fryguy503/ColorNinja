package studio

import (
	"archive/zip"
	"bytes"
	"colorninja/internal/engine"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func regionStudio(t *testing.T) (*Studio, *Preview) {
	t.Helper()
	s := fixture(t)
	request := req(s, 1)
	request.Options.Mode = "stack"
	request.Options.Colors = 2
	request.Options.HueForge.MaxDepth = .8
	request.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
	p, e := s.Process(request)
	if e != nil {
		t.Fatal(e)
	}
	return s, p
}
func regionAction(t *testing.T, s *Studio, state *RegionState, c RegionCommand) *RegionState {
	t.Helper()
	c.ID, c.Revision, c.Version, c.NewID = state.ID, state.Revision, state.Version, state.ID+1
	got, e := s.EditRegions(c)
	if e != nil {
		t.Fatal(c.Action, e)
	}
	return got
}

func TestRegionEditsColorPopAndBacklit(t *testing.T) {
	for _, style := range []string{"front", "back", "pop-front", "pop-back"} {
		t.Run(style, func(t *testing.T) {
			s := fixture(t)
			r := req(s, 1)
			r.Options.Mode, r.Options.Colors = "stack", 4
			r.Options.HueForge.MaxDepth = 1.6
			r.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
			r.Options.ColorPop.Enabled = style == "pop-front" || style == "pop-back"
			if style == "back" || style == "pop-back" {
				r.Options.HueForge.OpticalModel = engine.BacklitModel
			}
			p, err := s.Process(r)
			if err != nil {
				t.Fatal(err)
			}
			state, err := s.RegionEditor(p.ID, p.Revision)
			if err != nil {
				t.Fatal(err)
			}
			state = regionAction(t, s, state, RegionCommand{Action: "select", Selection: engine.RegionSelection{Tool: "rectangle", Scope: "pixels", Combine: "replace", Points: []engine.RegionPoint{{X: 1, Y: 1}, {X: 8, Y: 8}}}})
			state = regionAction(t, s, state, RegionCommand{Action: "shift", Value: 2})
			dir := t.TempDir()
			project := filepath.Join(dir, "edited.colorninja")
			if err = s.SaveProject(project, s.resultRequest, false); err != nil {
				t.Fatal(err)
			}
			reopened, err := s.OpenProject(project)
			if err != nil {
				t.Fatal(err)
			}
			if reopened.Preview.Result.SHA256 != state.Preview.Result.SHA256 || !slices.Equal(reopened.Preview.Result.LayerMap, state.Preview.Result.LayerMap) {
				t.Fatal("edited plan changed after reopening")
			}
			if r.Options.ColorPop.Enabled && (reopened.Preview.Result.ColorPop == nil || !bytes.Equal(reopened.Preview.Result.ColorPop.SelectionPNG, p.Result.ColorPop.SelectionPNG)) {
				t.Fatal("Color Pop selection was lost")
			}
			output := filepath.Join(dir, "edited.hfp")
			if err = s.Export("hfp", output, reopened.Preview.ID, reopened.Preview.Revision, false); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte("unique-rgb-keys-for-region-heights")) || !bytes.Contains(raw, []byte("regionEdits")) {
				t.Fatal("missing region height transport metadata")
			}
		})
	}
}
func TestRegionsStudioHistoryPersistenceAndExports(t *testing.T) {
	s, p := regionStudio(t)
	basePix := append([]byte(nil), p.Result.Image.Pix...)
	baseLayers := append([]uint16(nil), p.Result.LayerMap...)
	state, e := s.RegionEditor(p.ID, p.Revision)
	if e != nil {
		t.Fatal(e)
	}
	state = regionAction(t, s, state, RegionCommand{Action: "select", Selection: engine.RegionSelection{Tool: "rectangle", Scope: "pixels", Combine: "replace", Points: []engine.RegionPoint{{X: 0, Y: 0}, {X: 10, Y: 8}}}})
	if state.Pixels != 72 || !state.CanUndo {
		t.Fatal("selection/history", state)
	}
	state = regionAction(t, s, state, RegionCommand{Action: "assign", Value: p.Result.Stack.Options.BaseLayers()})
	edited := state.Preview
	if edited == nil || edited.ID == p.ID || edited.Result.RegionEdits == nil {
		t.Fatal("edit revision was not published")
	}
	if !bytes.Equal(p.Result.Image.Pix, basePix) || !slices.Equal(p.Result.LayerMap, baseLayers) {
		t.Fatal("baseline was mutated")
	}
	if e = s.Export("png", filepath.Join(t.TempDir(), "stale.png"), p.ID, p.Revision, false); e == nil {
		t.Fatal("old preview exported after edit")
	}
	stale := RegionCommand{ID: state.ID, Revision: state.Revision, Version: state.Version - 1, Action: "clear"}
	if _, e = s.EditRegions(stale); e == nil {
		t.Fatal("stale selection accepted")
	}
	state = regionAction(t, s, state, RegionCommand{Action: "undo"})
	if !bytes.Equal(state.Preview.Result.Image.Pix, basePix) {
		t.Fatal("undo failed")
	}
	state = regionAction(t, s, state, RegionCommand{Action: "redo"})
	if state.Preview.Result.SHA256 != edited.Result.SHA256 {
		t.Fatal("redo failed")
	}
	state = regionAction(t, s, state, RegionCommand{Action: "rename", GroupID: state.Groups[0].ID, Name: "Eyes and highlights"})
	request := s.resultRequest
	request.ID++
	request.Options.HueForge.ExportWidthMM = 150
	request.Options.HueForge.Border = engine.BorderOptions{Enabled: true, Placement: "external", WidthMM: 3, HeightMM: 4}
	reframed, e := s.Process(request)
	if e != nil {
		t.Fatal("display reframe", e)
	}
	if reframed.Result.SHA256 != edited.Result.SHA256 {
		t.Fatal("reframe lost edits")
	}
	if reframed.Result.SurfaceView == nil || reframed.Result.SurfaceView.WidthMM != 150 {
		t.Fatal("reframe left stale surface dimensions")
	}
	state, e = s.RegionEditor(reframed.ID, reframed.Revision)
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	project := filepath.Join(dir, "regions.colorninja")
	if e = s.SaveProject(project, s.resultRequest, false); e != nil {
		t.Fatal(e)
	}
	fresh := New(context.Background(), filepath.Join(dir, "fresh.json"))
	defer fresh.Shutdown()
	snap, e := fresh.OpenProject(project)
	if e != nil {
		t.Fatal("reopen", e)
	}
	if snap.Preview == nil || snap.Preview.Result.SHA256 != edited.Result.SHA256 || !slices.Equal(snap.Preview.Result.LayerMap, edited.Result.LayerMap) {
		t.Fatal("saved edit roundtrip")
	}
	fstate, e := fresh.RegionEditor(snap.Preview.ID, snap.Preview.Revision)
	if e != nil || len(fstate.Groups) != 1 || fstate.Groups[0].Name != "Eyes and highlights" {
		t.Fatal("group identity lost", e)
	}
	for _, kind := range []string{"png", "layers", "hfp", "palette"} {
		ext := map[string]string{"png": "png", "layers": "layers.png", "hfp": "hfp", "palette": "json"}[kind]
		if e = fresh.Export(kind, filepath.Join(dir, "edited."+ext), fstate.ID, fstate.Revision, false); e != nil {
			t.Fatal(kind, e)
		}
	}
	w := httptest.NewRecorder()
	fresh.ServeHTTP(w, httptest.NewRequest("GET", fstate.BaseURL, nil))
	if w.Code != 200 {
		t.Fatal("baseline preview unavailable")
	}
	fstate = regionAction(t, fresh, fstate, RegionCommand{Action: "enable", GroupID: fstate.Groups[0].ID})
	if !bytes.Equal(fstate.Preview.Result.Image.Pix, basePix) {
		t.Fatal("bypass after reopen did not restore baseline")
	}
	changed := fresh.resultRequest
	changed.Options.Colors++
	if _, e = fresh.Process(changed); e == nil {
		t.Fatal("regeneration silently discarded groups")
	}
	if e = fresh.SaveProject(filepath.Join(dir, "dirty.colorninja"), changed, false); e == nil {
		t.Fatal("dirty save silently discarded groups")
	}
	fstate = regionAction(t, fresh, fstate, RegionCommand{Action: "reset"})
	if len(fstate.Groups) != 0 {
		t.Fatal("reset retained groups")
	}
	if fstate.Preview.Result.SurfaceView.WidthMM != 150 {
		t.Fatal("reset lost reframed dimensions")
	}
	changed.ID = fstate.ID + 1
	if _, e = fresh.Process(changed); e != nil {
		t.Fatal("explicit reset did not permit replan", e)
	}
}

func TestRegionCompanionsAndCorruptFootprints(t *testing.T) {
	s, p := regionStudio(t)
	state, e := s.RegionEditor(p.ID, p.Revision)
	if e != nil {
		t.Fatal(e)
	}
	state = regionAction(t, s, state, RegionCommand{Action: "select", Selection: engine.RegionSelection{Tool: "click", Points: []engine.RegionPoint{{X: 3, Y: 3}}, Combine: "replace"}})
	state = regionAction(t, s, state, RegionCommand{Action: "cut"})
	s.settings.Preferences.ExportProfile = true
	dir := t.TempDir()
	output := filepath.Join(dir, "result.png")
	if e = s.Export("png", output, state.ID, state.Revision, false); e != nil {
		t.Fatal(e)
	}
	fresh := New(context.Background(), filepath.Join(dir, "fresh.json"))
	defer fresh.Shutdown()
	snap, e := fresh.OpenProject(ProjectPath(output))
	if e != nil || snap.Preview.Result.SHA256 != state.Preview.Result.SHA256 {
		t.Fatal("companion lost edits", e)
	}
	// Recompute the ZIP checksum after deliberate corruption: schema validation,
	// not only transport hashing, must reject an out-of-bounds footprint.
	z, e := zip.OpenReader(ProjectPath(output))
	if e != nil {
		t.Fatal(e)
	}
	entries := map[string][]byte{}
	for _, f := range z.File {
		r, _ := f.Open()
		entries[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	z.Close()
	var doc engine.RegionDocument
	if e = json.Unmarshal(entries["region-edits.json"], &doc); e != nil {
		t.Fatal(e)
	}
	doc.Groups[0].Mask = []engine.PixelSpan{{Start: doc.Width * doc.Height, Length: 1}}
	entries["region-edits.json"], _ = json.Marshal(doc)
	var manifest projectManifest
	json.Unmarshal(entries["project.json"], &manifest)
	manifest.Files["region-edits.json"] = fmtHash(entries["region-edits.json"])
	entries["project.json"], _ = json.Marshal(manifest)
	bad := filepath.Join(dir, "bad.colorninja")
	file, _ := os.Create(bad)
	writer := zip.NewWriter(file)
	for name, b := range entries {
		w, _ := writer.Create(name)
		w.Write(b)
	}
	writer.Close()
	file.Close()
	previous := fresh.Snapshot().Preview
	if _, e = fresh.OpenProject(bad); e == nil {
		t.Fatal("invalid footprint accepted")
	}
	if fresh.Snapshot().Preview.Result.SHA256 != previous.Result.SHA256 {
		t.Fatal("failed open damaged active result")
	}
}
func fmtHash(b []byte) string {
	sum := sha256.Sum256(b)
	const digits = "0123456789abcdef"
	out := make([]byte, 64)
	for i, v := range sum {
		out[i*2], out[i*2+1] = digits[v>>4], digits[v&15]
	}
	return string(out)
}

func TestRegionGroupProtectionAndSplit(t *testing.T) {
	s, p := regionStudio(t)
	state, e := s.RegionEditor(p.ID, p.Revision)
	if e != nil {
		t.Fatal(e)
	}
	for i, point := range []engine.RegionPoint{{X: 2, Y: 2}, {X: 20, Y: 10}} {
		combine := "replace"
		if i > 0 {
			combine = "add"
		}
		state = regionAction(t, s, state, RegionCommand{Action: "select", Selection: engine.RegionSelection{Tool: "brush", Scope: "pixels", Radius: 1, Combine: combine, Points: []engine.RegionPoint{point}}})
	}
	state = regionAction(t, s, state, RegionCommand{Action: "assign", Value: p.Result.Stack.Options.BaseLayers()})
	sha := state.Preview.Result.SHA256
	state = regionAction(t, s, state, RegionCommand{Action: "split", GroupID: state.Groups[0].ID})
	if len(state.Groups) != 2 || state.Preview.Result.SHA256 != sha {
		t.Fatal("split changed pixels or failed", state.Groups)
	}
	state = regionAction(t, s, state, RegionCommand{Action: "lock", GroupID: state.Groups[0].ID})
	state = regionAction(t, s, state, RegionCommand{Action: "recall", GroupID: state.Groups[0].ID})
	if _, e = s.EditRegions(RegionCommand{ID: state.ID, Revision: state.Revision, Version: state.Version, NewID: state.ID + 1, Action: "cut"}); e == nil {
		t.Fatal("protected pixels could be cut")
	}
	if _, e = s.UseDemo(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RegionEditor(state.ID, state.Revision); e == nil {
		t.Fatal("old document editor remained active")
	}
}
