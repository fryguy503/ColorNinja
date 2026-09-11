package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const editableLibrary = `{"OtherRoot":{"version":7},"Filaments":[{"Brand":"Acme","Name":"Red","Type":"PLA","Color":"#FF0011","Transmissivity":4.2,"Owned":true,"CustomField":{"keep":true}},{"Brand":"Acme","Name":"Spare Red","Type":"PLA","Color":"#FF0011","Transmissivity":4.2,"Owned":false},{"Brand":"Acme","Name":"Dual","Type":"PETG","Color":"#123456","Secondary_Color":"#654321","Transmissivity":8,"Owned":true}]}`

func editorFixture(t *testing.T) (*Studio, string, FilamentCatalog) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "original.json")
	if e := os.WriteFile(p, []byte(editableLibrary), 0600); e != nil {
		t.Fatal(e)
	}
	s := New(context.Background(), filepath.Join(dir, "config", "settings.json"))
	t.Cleanup(s.Shutdown)
	c, e := s.FilamentCatalog(p)
	if e != nil {
		t.Fatal(e)
	}
	return s, p, c
}
func TestFilamentEditPreservesOriginalAndUnfilteredRecords(t *testing.T) {
	s, p, c := editorFixture(t)
	if len(c.Entries) != 3 || len(c.Library.Filaments) != 1 {
		t.Fatal(c)
	}
	entry := c.Entries[0]
	entry.TD = 5.6
	entry.SourceURL = "https://3dfilamentprofiles.com/filament/details/123"
	entry.Measurement = &FilamentMeasurement{TD: 5.6, Color: "#F01234", Serial: "TD1TEST", CapturedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	oldKey := engine.FilamentKey(c.Library.Filaments[0])
	s.settings.Options.HueForge.RequiredFilaments = oldKey
	s.settings.Options.HueForge.BaseFilament = oldKey
	next, e := s.EditFilament(FilamentEdit{Path: p, SHA256: c.SHA256, Entry: entry})
	if e != nil {
		t.Fatal(e)
	}
	if next.Path == p || !next.Managed || len(next.Entries) != 3 || next.Entries[0].Color != "#FF0011" || next.Entries[0].TD != 5.6 || next.Entries[0].Measurement.Serial != "TD1TEST" {
		t.Fatal(next)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != editableLibrary {
		t.Fatal("source library changed")
	}
	saved, _ := os.ReadFile(next.Path)
	var doc map[string]any
	if e = json.Unmarshal(saved, &doc); e != nil {
		t.Fatal(e)
	}
	record := doc["Filaments"].([]any)[0].(map[string]any)
	if doc["OtherRoot"] == nil || record["CustomField"] == nil || len(record["ColorNinjaTDHistory"].([]any)) != 1 {
		t.Fatal(doc)
	}
	if next.KeyChanges[oldKey] != next.Entries[0].UUID || s.settings.Options.HueForge.BaseFilament != next.Entries[0].UUID {
		t.Fatal("constraint did not follow changed identity")
	}
	entry = next.Entries[0]
	entry.Name = "Measured red"
	after, e := s.EditFilament(FilamentEdit{Path: next.Path, SHA256: next.SHA256, Entry: entry})
	if e != nil || after.Path != next.Path {
		t.Fatal(after, e)
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(after.Path), "backups", "*.bak"))
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	backup, _ := os.ReadFile(backups[0])
	if !bytes.Equal(saved, backup) {
		t.Fatal("backup differs from pre-edit library")
	}
	if _, e = s.EditFilament(FilamentEdit{Path: next.Path, SHA256: next.SHA256, Entry: entry}); e == nil {
		t.Fatal("stale edit overwrote newer library")
	}
}
func TestEmptyLibraryAndUnknownTDRemainEditable(t *testing.T) {
	s := New(context.Background(), filepath.Join(t.TempDir(), "settings.json"))
	defer s.Shutdown()
	c, e := s.FilamentCatalog("")
	if e != nil || len(c.Entries) != 0 || c.Library.Filaments == nil {
		t.Fatal(c, e)
	}
	entry := FilamentEntry{Index: -1, Brand: "New", Name: "Blue", Material: "PLA", Color: "#123456", Owned: true, Tags: []string{}}
	next, e := s.EditFilament(FilamentEdit{SHA256: c.SHA256, Entry: entry})
	if e != nil || len(next.Entries) != 1 || len(next.Library.Filaments) != 0 || next.Warning == "" {
		t.Fatal(next, e)
	}
	entry = next.Entries[0]
	entry.TD = 3
	next, e = s.EditFilament(FilamentEdit{Path: next.Path, SHA256: next.SHA256, Entry: entry})
	if e != nil || len(next.Library.Filaments) != 1 {
		t.Fatal(next, e)
	}
	entry = next.Entries[0]
	next, e = s.EditFilament(FilamentEdit{Path: next.Path, SHA256: next.SHA256, Entry: entry, Delete: true})
	if e != nil || len(next.Entries) != 0 {
		t.Fatal(next, e)
	}
}
func TestDeleteAdjustsExcludedIndicesAndInvalidatesPreview(t *testing.T) {
	s, p, c := editorFixture(t)
	s.settings.Filter.ExcludedIDs = []int{1, 2}
	s.result = &engine.Result{}
	s.resultLibrary = []byte("old")
	next, e := s.EditFilament(FilamentEdit{Path: p, SHA256: c.SHA256, Entry: c.Entries[0], Delete: true})
	if e != nil {
		t.Fatal(e)
	}
	if len(next.Filter.ExcludedIDs) != 2 || next.Filter.ExcludedIDs[0] != 0 || next.Filter.ExcludedIDs[1] != 1 || s.result != nil || s.resultLibrary != nil {
		t.Fatal(next)
	}
	if next.KeyChanges[engine.FilamentKey(c.Library.Filaments[0])] != "" {
		t.Fatal("deleted constraint retained")
	}
}
func TestLibraryExportRetainsMetadataAndRejectsSourceOverwrite(t *testing.T) {
	s, p, _ := editorFixture(t)
	dest := filepath.Join(t.TempDir(), "export.json")
	if e := s.ExportFilamentLibrary(p, dest, false); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(dest)
	if string(raw) != editableLibrary {
		t.Fatal("lossy export")
	}
	if e := s.ExportFilamentLibrary(p, p, true); e == nil {
		t.Fatal("source overwritten")
	}
	if e := s.ExportFilamentLibrary(p, dest, false); e == nil {
		t.Fatal("existing export overwritten")
	}
}
func TestLibraryRejectedEditDoesNotWrite(t *testing.T) {
	s, p, c := editorFixture(t)
	entry := c.Entries[0]
	entry.TD = -1
	if _, e := s.EditFilament(FilamentEdit{Path: p, SHA256: c.SHA256, Entry: entry}); e == nil {
		t.Fatal("invalid TD accepted")
	}
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(s.configPath), "libraries", "*"))
	if len(paths) > 0 {
		t.Fatal("invalid edit created a library")
	}
	if _, e := parseLibraryDocument([]byte(`{"Unknown":[]}`)); e == nil {
		t.Fatal("unsupported schema accepted")
	}
}
