package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// The editor preserves every source record and unknown HueForge field. Engine
// eligibility/deduplication must never become a lossy library save operation.
type FilamentEntry struct {
	Index          int                  `json:"index"`
	UUID           string               `json:"uuid"`
	Brand          string               `json:"brand"`
	Name           string               `json:"name"`
	Material       string               `json:"material"`
	Color          string               `json:"color"`
	SecondaryColor string               `json:"secondaryColor"`
	TD             float64              `json:"td"`
	Owned          bool                 `json:"owned"`
	Tags           []string             `json:"tags"`
	SourceURL      string               `json:"sourceURL"`
	Measurement    *FilamentMeasurement `json:"measurement,omitempty"`
}
type FilamentMeasurement struct {
	TD         float64 `json:"td"`
	Color      string  `json:"color"`
	Serial     string  `json:"serial"`
	CapturedAt string  `json:"capturedAt"`
}
type FilamentCatalog struct {
	Path       string               `json:"path"`
	SHA256     string               `json:"sha256"`
	Managed    bool                 `json:"managed"`
	Entries    []FilamentEntry      `json:"entries"`
	Library    *engine.Library      `json:"library"`
	Filter     engine.LibraryFilter `json:"filter"`
	Warning    string               `json:"warning"`
	KeyChanges map[string]string    `json:"keyChanges,omitempty"`
}
type FilamentEdit struct {
	Path   string        `json:"path"`
	SHA256 string        `json:"sha256"`
	Entry  FilamentEntry `json:"entry"`
	Delete bool          `json:"delete"`
}
type libraryDocument struct {
	root    any
	records []any
	key     string
}

func parseLibraryDocument(raw []byte) (libraryDocument, error) {
	var d libraryDocument
	if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte{239, 187, 191}), &d.root); err != nil {
		return d, err
	}
	switch root := d.root.(type) {
	case []any:
		d.records = root
	case map[string]any:
		for _, k := range []string{"Filaments", "filaments"} {
			if r, ok := root[k].([]any); ok {
				d.records, d.key = r, k
				break
			}
		}
	}
	if d.records == nil {
		return d, fmt.Errorf("library must contain a Filaments array")
	}
	return d, nil
}
func (d libraryDocument) bytes() ([]byte, error) {
	if d.key == "" {
		return json.MarshalIndent(d.records, "", "  ")
	}
	d.root.(map[string]any)[d.key] = d.records
	return json.MarshalIndent(d.root, "", "  ")
}
func entryText(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
func entryFromRecord(index int, r map[string]any) FilamentEntry {
	td, _ := strconv.ParseFloat(entryText(r["Transmissivity"]), 64)
	if math.IsNaN(td) || math.IsInf(td, 0) {
		td = 0
	}
	e := FilamentEntry{Index: index, UUID: entryText(r["uuid"]), Brand: entryText(r["Brand"]), Name: entryText(r["Name"]), Material: entryText(r["Type"]), Color: entryText(r["Color"]), SecondaryColor: entryText(r["Secondary_Color"]), TD: td, Tags: []string{}, SourceURL: entryText(r["ColorNinjaSourceURL"])}
	e.Owned = strings.EqualFold(entryText(r["Owned"]), "true") || entryText(r["Owned"]) == "1" || strings.EqualFold(entryText(r["Owned"]), "yes")
	switch tags := r["Tags"].(type) {
	case string:
		if tags != "" {
			e.Tags = append(e.Tags, tags)
		}
	case []any:
		for _, tag := range tags {
			e.Tags = append(e.Tags, entryText(tag))
		}
	}
	if raw, err := json.Marshal(r["ColorNinjaMeasurement"]); err == nil {
		_ = json.Unmarshal(raw, &e.Measurement)
	}
	return e
}
func (s *Studio) managedLibrary(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	dir, err := filepath.Abs(filepath.Join(filepath.Dir(s.configPath), "libraries"))
	if err != nil {
		return false
	}
	if !strings.HasPrefix(filepath.Base(abs), "library-") || filepath.Ext(abs) != ".json" {
		return false
	}
	// Compare directory identities so system aliases and Windows short names
	// still identify our library. A linked file itself is never managed.
	file, err := os.Lstat(abs)
	if err != nil || !file.Mode().IsRegular() {
		return false
	}
	parent, err := os.Stat(filepath.Dir(abs))
	if err != nil {
		return false
	}
	managed, err := os.Stat(dir)
	return err == nil && os.SameFile(parent, managed)
}
func (s *Studio) catalogBytes(path string) ([]byte, error) {
	if path == "" {
		return []byte(`{"Filaments":[]}`), nil
	}
	return s.libraryBytes(path)
}
func (s *Studio) FilamentCatalog(path string) (FilamentCatalog, error) {
	raw, err := s.catalogBytes(path)
	if err != nil {
		return FilamentCatalog{}, err
	}
	return s.makeCatalog(path, raw)
}
func (s *Studio) makeCatalog(path string, raw []byte) (FilamentCatalog, error) {
	d, err := parseLibraryDocument(raw)
	if err != nil {
		return FilamentCatalog{}, err
	}
	s.mu.RLock()
	filter := s.settings.Filter
	s.mu.RUnlock()
	c := FilamentCatalog{Path: path, SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Managed: s.managedLibrary(path), Entries: []FilamentEntry{}, Filter: filter}
	for i, r := range d.records {
		if record, ok := r.(map[string]any); ok {
			c.Entries = append(c.Entries, entryFromRecord(i, record))
		}
	}
	lib, err := engine.ParseLibrary(raw, filter)
	c.Library = &lib
	if lib.Filaments == nil {
		lib.Filaments = []engine.Filament{}
	}
	if err != nil {
		c.Warning = err.Error()
	}
	return c, nil
}
func validateEntry(e *FilamentEntry) error {
	e.Brand = strings.TrimSpace(e.Brand)
	e.Name = strings.TrimSpace(e.Name)
	e.Material = strings.TrimSpace(e.Material)
	if e.Brand == "" || e.Name == "" || e.Material == "" {
		return fmt.Errorf("brand, name, and material are required")
	}
	for _, v := range []string{e.Brand, e.Name, e.Material, e.SourceURL} {
		if len(v) > 2048 {
			return fmt.Errorf("filament text is too long")
		}
	}
	if len(e.Color) != 7 && len(e.Color) != 6 {
		return fmt.Errorf("color must contain six hex digits")
	}
	rgb, err := engine.ParseRGB(e.Color)
	if err != nil {
		return fmt.Errorf("invalid filament color")
	}
	e.Color = rgb.Hex()
	if e.SecondaryColor != "" {
		rgb, err = engine.ParseRGB(e.SecondaryColor)
		if err != nil {
			return fmt.Errorf("invalid secondary color")
		}
		e.SecondaryColor = rgb.Hex()
	}
	if math.IsNaN(e.TD) || math.IsInf(e.TD, 0) || e.TD < 0 || e.TD > 1000 {
		return fmt.Errorf("TD must be between 0 and 1000 mm; use 0 for unknown")
	}
	if e.Measurement != nil {
		m := e.Measurement
		if m.TD <= 0 || m.TD > 1000 || math.IsNaN(m.TD) || math.IsInf(m.TD, 0) {
			return fmt.Errorf("invalid TD measurement")
		}
		if _, err := time.Parse(time.RFC3339Nano, m.CapturedAt); err != nil {
			return fmt.Errorf("invalid measurement timestamp")
		}
		if _, err := engine.ParseRGB(m.Color); err != nil {
			return fmt.Errorf("invalid measured color")
		}
	}
	if len(e.Tags) > 128 {
		return fmt.Errorf("at most 128 tags are supported")
	}
	tags := []string{}
	for _, tag := range e.Tags {
		tag = strings.TrimSpace(tag)
		if len(tag) > 256 {
			return fmt.Errorf("filament tag is too long")
		}
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	e.Tags = tags
	return nil
}
func (s *Studio) EditFilament(edit FilamentEdit) (FilamentCatalog, error) {
	s.libraryMu.Lock()
	defer s.libraryMu.Unlock()
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.worker.Lock()
	defer s.worker.Unlock()
	raw, err := s.catalogBytes(edit.Path)
	if err != nil {
		return FilamentCatalog{}, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != edit.SHA256 {
		return FilamentCatalog{}, fmt.Errorf("library changed since it was opened; reload before saving")
	}
	d, err := parseLibraryDocument(raw)
	if err != nil {
		return FilamentCatalog{}, err
	}
	s.mu.RLock()
	oldFilter := s.settings.Filter
	s.mu.RUnlock()
	oldLibrary, _ := engine.ParseLibrary(raw, oldFilter)
	e := edit.Entry
	if e.Index < -1 || e.Index >= len(d.records) || (edit.Delete && e.Index < 0) {
		return FilamentCatalog{}, fmt.Errorf("filament no longer exists; reload the library")
	}
	if !edit.Delete {
		if err = validateEntry(&e); err != nil {
			return FilamentCatalog{}, err
		}
	}
	s.mu.Lock()
	if s.regions != nil && len(s.regions.Document.Groups) > 0 {
		s.mu.Unlock()
		return FilamentCatalog{}, fmt.Errorf("save your project and reset region edits before changing its filament library")
	}
	s.mu.Unlock()
	if edit.Delete {
		d.records = append(d.records[:e.Index], d.records[e.Index+1:]...)
	} else {
		r := map[string]any{}
		if e.Index >= 0 {
			if old, ok := d.records[e.Index].(map[string]any); ok {
				r = old
			}
		}
		if entryText(r["uuid"]) == "" {
			r["uuid"] = uuid.NewString()
		}
		if previous, ok := r["Transmissivity"]; ok && entryText(previous) != strconv.FormatFloat(e.TD, 'f', -1, 64) {
			history, _ := r["ColorNinjaTDHistory"].([]any)
			history = append(history, map[string]any{"td": previous, "measurement": r["ColorNinjaMeasurement"], "changedAt": time.Now().UTC().Format(time.RFC3339Nano)})
			if len(history) > 100 {
				history = history[len(history)-100:]
			}
			r["ColorNinjaTDHistory"] = history
		}
		r["Brand"] = e.Brand
		r["Name"] = e.Name
		r["Type"] = e.Material
		r["Color"] = e.Color
		r["Secondary_Color"] = e.SecondaryColor
		r["Transmissivity"] = e.TD
		r["Owned"] = e.Owned
		r["Tags"] = e.Tags
		r["ColorNinjaSourceURL"] = e.SourceURL
		if e.Measurement != nil && e.Measurement.TD == e.TD {
			r["ColorNinjaMeasurement"] = e.Measurement
		} else {
			delete(r, "ColorNinjaMeasurement")
		}
		if e.Index < 0 {
			d.records = append(d.records, r)
		} else {
			d.records[e.Index] = r
		}
	}
	next, err := d.bytes()
	if err != nil {
		return FilamentCatalog{}, err
	}
	if len(next) > 32<<20 {
		return FilamentCatalog{}, fmt.Errorf("library exceeds 32 MiB")
	}
	path := edit.Path
	managed := s.managedLibrary(path)
	if !managed {
		path = filepath.Join(filepath.Dir(s.configPath), "libraries", "library-"+uuid.NewString()+".json")
	}
	if managed {
		backup := filepath.Join(filepath.Dir(path), "backups", filepath.Base(path)+"-"+uuid.NewString()+".bak")
		if err = engine.AtomicWrite(backup, false, func(w io.Writer) error { _, e := w.Write(raw); return e }); err != nil {
			return FilamentCatalog{}, fmt.Errorf("backup library: %w", err)
		}
	}
	if err = engine.AtomicWrite(path, managed, func(w io.Writer) error { _, e := w.Write(next); return e }); err != nil {
		return FilamentCatalog{}, err
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.job++
	s.result = nil
	s.resultLibrary = nil
	s.settings.LibraryPath = path
	if edit.Delete {
		ids := []int{}
		for _, id := range s.settings.Filter.ExcludedIDs {
			if id < e.Index {
				ids = append(ids, id)
			} else if id > e.Index {
				ids = append(ids, id-1)
			}
		}
		s.settings.Filter.ExcludedIDs = ids
	}
	s.mu.Unlock()
	c, err := s.makeCatalog(path, next)
	if err != nil {
		return c, err
	}
	c.KeyChanges = map[string]string{}
	keysByIndex := map[int]string{}
	for _, f := range c.Library.Filaments {
		keysByIndex[f.SourceIndex] = engine.FilamentKey(f)
	}
	for _, old := range oldLibrary.Filaments {
		index := old.SourceIndex
		key := ""
		if edit.Delete && index == e.Index {
			c.KeyChanges[engine.FilamentKey(old)] = ""
			continue
		}
		if edit.Delete && index > e.Index {
			index--
		}
		key = keysByIndex[index]
		if key != engine.FilamentKey(old) {
			c.KeyChanges[engine.FilamentKey(old)] = key
		}
	}
	s.mu.Lock()
	hf := &s.settings.Options.HueForge
	hf.RequiredFilaments = remapFilamentKeys(hf.RequiredFilaments, c.KeyChanges)
	hf.BaseFilament = remapFilamentKeys(hf.BaseFilament, c.KeyChanges)
	hf.HighlightFilament = remapFilamentKeys(hf.HighlightFilament, c.KeyChanges)
	s.mu.Unlock()
	if err = s.saveSettings(); err != nil {
		c.Warning = "Library saved, but preferences could not be saved: " + err.Error()
	}
	return c, nil
}

func remapFilamentKeys(keys string, changes map[string]string) string {
	result := []string{}
	for _, key := range strings.Split(keys, ",") {
		key = strings.TrimSpace(key)
		if next, ok := changes[key]; ok {
			key = next
		}
		if key != "" {
			result = append(result, key)
		}
	}
	return strings.Join(result, ",")
}

func (s *Studio) ExportFilamentLibrary(path, destination string, overwrite bool) error {
	raw, err := s.catalogBytes(path)
	if err != nil {
		return err
	}
	if _, err = parseLibraryDocument(raw); err != nil {
		return err
	}
	if err = engine.DistinctPaths(destination, path, s.configPath); err != nil {
		return err
	}
	return engine.AtomicWrite(destination, overwrite, func(w io.Writer) error { _, e := w.Write(raw); return e })
}
