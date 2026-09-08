// Package studio owns document state and cancellable processing independently
// of the Wails host, so the desktop and integration tests use the same service.
package studio

import (
	"colorninja/internal/engine"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Source struct {
	Name         string               `json:"name"`
	Path         string               `json:"path"`
	Width        int                  `json:"width"`
	Height       int                  `json:"height"`
	UniqueColors int                  `json:"uniqueColors"`
	URL          string               `json:"url"`
	Revision     uint64               `json:"revision"`
	Demo         bool                 `json:"demo"`
	Metadata     engine.ImageMetadata `json:"metadata"`
}
type Preset struct {
	Name    string         `json:"name"`
	Options engine.Options `json:"options"`
}
type Settings struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Options       engine.Options       `json:"options"`
	LibraryPath   string               `json:"libraryPath"`
	Filter        engine.LibraryFilter `json:"filter"`
	Presets       []Preset             `json:"presets"`
	Recent        []string             `json:"recent"`
}
type Snapshot struct {
	Source   Source          `json:"source"`
	Settings Settings        `json:"settings"`
	Library  *engine.Library `json:"library"`
	Warning  string          `json:"warning"`
}
type Request struct {
	ID          uint64               `json:"id"`
	Revision    uint64               `json:"revision"`
	Options     engine.Options       `json:"options"`
	LibraryPath string               `json:"libraryPath"`
	Filter      engine.LibraryFilter `json:"filter"`
}
type Preview struct {
	ID       uint64         `json:"id"`
	Revision uint64         `json:"revision"`
	URL      string         `json:"url"`
	Result   *engine.Result `json:"result"`
	Options  engine.Options `json:"options"`
	Seconds  float64        `json:"seconds"`
	Warning  string         `json:"warning,omitempty"`
}
type Progress struct {
	ID uint64 `json:"id"`
	engine.Progress
}
type Document struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Source        string               `json:"source"`
	Demo          bool                 `json:"demo"`
	Options       engine.Options       `json:"options"`
	LibraryPath   string               `json:"libraryPath"`
	Filter        engine.LibraryFilter `json:"filter"`
}
type Studio struct {
	ctx           context.Context
	mu            sync.RWMutex
	worker        sync.Mutex
	settingsMu    sync.Mutex
	cancel        context.CancelFunc
	loadCancel    context.CancelFunc
	loadSerial    uint64
	job           uint64
	revision      uint64
	image         *image.NRGBA
	source        Source
	settings      Settings
	result        *engine.Result
	resultRequest Request
	resultJob     uint64
	configPath    string
	warning       string
	Emit          func(string, any)
}

func New(ctx context.Context, configPath string) *Studio {
	o := engine.DefaultOptions()
	o.Colors = 8
	s := &Studio{ctx: ctx, configPath: configPath, settings: Settings{SchemaVersion: 1, Options: o, Presets: []Preset{}, Recent: []string{}, Filter: engine.LibraryFilter{MaterialTypes: []string{}, ExcludedIDs: []int{}}}}
	if raw, e := os.ReadFile(configPath); e == nil {
		var saved Settings
		if e = json.Unmarshal(raw, &saved); e == nil && saved.SchemaVersion == 1 && saved.Options.Validate() == nil {
			s.settings = saved
		} else {
			s.warning = "Saved settings could not be read. Defaults have been restored."
		}
	} else if !os.IsNotExist(e) {
		s.warning = "Saved settings could not be opened: " + e.Error()
	}
	if s.settings.LibraryPath == "" {
		candidate := filepath.Join(os.Getenv("APPDATA"), "HueForge", "Filaments", "personal_library.json")
		if _, e := os.Stat(candidate); e == nil {
			s.settings.LibraryPath = candidate
		}
	}
	return s
}
func (s *Studio) emit(name string, v any) {
	if s.Emit != nil {
		s.Emit(name, v)
	}
}
func (s *Studio) Initialize() (Snapshot, error) {
	s.mu.RLock()
	loaded := s.image != nil
	s.mu.RUnlock()
	if !loaded {
		return s.UseDemo()
	}
	return s.Snapshot(), nil
}
func (s *Studio) Snapshot() Snapshot {
	s.mu.RLock()
	settings := s.settings
	source := s.source
	warning := s.warning
	s.mu.RUnlock()
	var lib *engine.Library
	if settings.LibraryPath != "" {
		l, e := engine.LoadLibrary(settings.LibraryPath, settings.Filter)
		if e == nil {
			lib = &l
		} else {
			warning = e.Error()
		}
	}
	return Snapshot{source, settings, lib, warning}
}
func (s *Studio) beginLoad() (context.Context, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadCancel != nil {
		s.loadCancel()
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.job++
	s.loadSerial++
	ctx, cancel := context.WithCancel(s.ctx)
	s.loadCancel = cancel
	return ctx, s.loadSerial
}
func (s *Studio) installImage(ctx context.Context, loaded *engine.LoadedImage, path string, demo bool, serial uint64) error {
	uniqueColors, err := engine.CountUniqueColors(ctx, loaded.Image)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if serial != s.loadSerial {
		return context.Canceled
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.job++
	s.revision++
	s.image = loaded.Image
	s.result = nil
	s.source = Source{Name: filepath.Base(path), Path: path,
		Width: loaded.Image.Bounds().Dx(), Height: loaded.Image.Bounds().Dy(),
		UniqueColors: uniqueColors, URL: fmt.Sprintf("/media/source/%d.png", s.revision),
		Revision: s.revision, Demo: demo, Metadata: loaded.Metadata}
	if demo {
		s.source.Name = "Painted dunes"
		s.source.Path = ""
	}
	if !demo {
		recent := []string{path}
		for _, p := range s.settings.Recent {
			if p != path && len(recent) < 8 {
				recent = append(recent, p)
			}
		}
		s.settings.Recent = recent
	}
	return nil
}
func (s *Studio) UseDemo() (Snapshot, error) {
	ctx, serial := s.beginLoad()
	img := engine.SampleImage()
	if e := ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	e := s.installImage(ctx, &engine.LoadedImage{Image: img, Metadata: engine.ImageMetadata{Format: "sample", ColorProfile: "sRGB", Warnings: []string{}}}, "Painted dunes", true, serial)
	if e != nil {
		return Snapshot{}, e
	}
	return s.Snapshot(), nil
}
func (s *Studio) LoadImage(path string) (Snapshot, error) {
	if strings.TrimSpace(path) == "" {
		return Snapshot{}, fmt.Errorf("select an image")
	}
	absolute, e := filepath.Abs(path)
	if e != nil {
		return Snapshot{}, e
	}
	ctx, serial := s.beginLoad()
	loaded, e := engine.LoadImage(ctx, absolute)
	if e != nil {
		return Snapshot{}, e
	}
	if e = s.installImage(ctx, loaded, absolute, false, serial); e != nil {
		return Snapshot{}, e
	}
	if e = s.saveSettings(); e != nil {
		s.mu.Lock()
		s.warning = e.Error()
		s.mu.Unlock()
	}
	return s.Snapshot(), nil
}
func (s *Studio) SetLibrary(path string, filter engine.LibraryFilter) (*engine.Library, error) {
	lib, e := engine.LoadLibrary(path, filter)
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	s.settings.LibraryPath = path
	s.settings.Filter = filter
	s.mu.Unlock()
	if e = s.saveSettings(); e != nil {
		return &lib, e
	}
	return &lib, nil
}
func (s *Studio) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.job++
}
func (s *Studio) Shutdown() {
	s.Cancel()
	s.mu.Lock()
	if s.loadCancel != nil {
		s.loadCancel()
	}
	s.mu.Unlock()
}
func (s *Studio) Process(req Request) (*Preview, error) {
	if e := req.Options.Validate(); e != nil {
		return nil, e
	}
	s.mu.Lock()
	if req.Revision != s.revision || s.image == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("image changed; refresh the preview")
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.cancel = cancel
	s.job++
	job := s.job
	src := s.image
	s.mu.Unlock()
	defer cancel()
	// At most one engine job can hold its analysis and result buffers at a time.
	s.worker.Lock()
	defer s.worker.Unlock()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	start := time.Now()
	var library *engine.Library
	if req.Options.Mode != "standard" {
		l, e := engine.LoadLibrary(req.LibraryPath, req.Filter)
		if e != nil {
			return nil, e
		}
		library = &l
	}
	last := time.Time{}
	r, e := engine.Process(ctx, src, req.Options, library, func(p engine.Progress) {
		if time.Since(last) > 50*time.Millisecond || p.Fraction == 1 {
			last = time.Now()
			s.emit("progress", Progress{req.ID, p})
		}
	})
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	if s.job != job || s.revision != req.Revision {
		s.mu.Unlock()
		return nil, context.Canceled
	}
	s.result = r
	s.resultRequest = req
	s.resultJob = job
	s.settings.Options = req.Options
	s.settings.LibraryPath = req.LibraryPath
	s.settings.Filter = req.Filter
	s.mu.Unlock()
	warning := ""
	if e = s.saveSettings(); e != nil {
		warning = "Preview ready, but preferences could not be saved: " + e.Error()
	}
	return &Preview{req.ID, req.Revision, fmt.Sprintf("/media/result/%d.png", job), r, req.Options, time.Since(start).Seconds(), warning}, nil
}
func (s *Studio) saveSettings() error {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	return engine.AtomicWrite(s.configPath, true, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(settings)
	})
}
func (s *Studio) SavePreset(name string, o engine.Options) ([]Preset, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 60 {
		return nil, fmt.Errorf("preset name must contain 1–60 characters")
	}
	if e := o.Validate(); e != nil {
		return nil, e
	}
	s.mu.Lock()
	p := append([]Preset{}, s.settings.Presets...)
	found := false
	for i := range p {
		if strings.EqualFold(p[i].Name, name) {
			p[i] = Preset{name, o}
			found = true
			break
		}
	}
	if !found {
		if len(p) >= 50 {
			s.mu.Unlock()
			return nil, fmt.Errorf("maximum of 50 custom presets reached")
		}
		p = append(p, Preset{name, o})
	}
	s.settings.Presets = p
	s.mu.Unlock()
	return p, s.saveSettings()
}
func (s *Studio) DeletePreset(name string) ([]Preset, error) {
	s.mu.Lock()
	p := []Preset{}
	for _, v := range s.settings.Presets {
		if v.Name != name {
			p = append(p, v)
		}
	}
	s.settings.Presets = p
	s.mu.Unlock()
	return p, s.saveSettings()
}
func (s *Studio) SaveProject(path string, req Request, overwrite bool) error {
	if e := req.Options.Validate(); e != nil {
		return e
	}
	s.mu.RLock()
	source := s.source
	s.mu.RUnlock()
	if req.Revision != source.Revision {
		return fmt.Errorf("image changed; save again")
	}
	if e := engine.DistinctPaths(path, source.Path, req.LibraryPath); e != nil {
		return e
	}
	doc := Document{1, source.Path, source.Demo, req.Options, req.LibraryPath, req.Filter}
	return engine.AtomicWrite(path, overwrite, func(w io.Writer) error { enc := json.NewEncoder(w); enc.SetIndent("", "  "); return enc.Encode(doc) })
}
func (s *Studio) OpenProject(path string) (Snapshot, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return Snapshot{}, e
	}
	var d Document
	if e = json.Unmarshal(raw, &d); e != nil {
		return Snapshot{}, fmt.Errorf("invalid ColorNinja project: %w", e)
	}
	if d.SchemaVersion != 1 {
		return Snapshot{}, fmt.Errorf("unsupported project version")
	}
	if e = d.Options.Validate(); e != nil {
		return Snapshot{}, e
	}
	if d.LibraryPath != "" && !filepath.IsAbs(d.LibraryPath) {
		d.LibraryPath = filepath.Join(filepath.Dir(path), d.LibraryPath)
	}
	if !d.Demo && !filepath.IsAbs(d.Source) {
		d.Source = filepath.Join(filepath.Dir(path), d.Source)
	}
	if d.Options.Mode != "standard" {
		if _, e = engine.LoadLibrary(d.LibraryPath, d.Filter); e != nil {
			return Snapshot{}, e
		}
	}
	if d.Demo {
		_, e = s.UseDemo()
	} else {
		_, e = s.LoadImage(d.Source)
	}
	if e != nil {
		return Snapshot{}, e
	}
	s.mu.Lock()
	s.settings.Options = d.Options
	s.settings.LibraryPath = d.LibraryPath
	s.settings.Filter = d.Filter
	s.mu.Unlock()
	if e = s.saveSettings(); e != nil {
		return Snapshot{}, e
	}
	return s.Snapshot(), nil
}
func (s *Studio) Export(kind, path string, id, revision uint64, overwrite bool) error {
	s.mu.RLock()
	r := s.result
	req := s.resultRequest
	source := s.source
	s.mu.RUnlock()
	if r == nil || req.ID != id || req.Revision != revision || source.Revision != revision {
		return fmt.Errorf("generate a current preview before exporting")
	}
	if e := engine.DistinctPaths(path, source.Path, req.LibraryPath); e != nil {
		return e
	}
	switch kind {
	case "png":
		return engine.SavePNG(s.ctx, path, r, source.Metadata, overwrite)
	case "palette":
		return engine.SaveReport(path, source.Path, r, req.Options, source.Metadata, overwrite)
	case "layers":
		return engine.SaveLayerMap(s.ctx, path, r, overwrite)
	default:
		return fmt.Errorf("unknown export type")
	}
}
func (s *Studio) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	s.mu.RLock()
	var img *image.NRGBA
	var dpi [2]float64
	if r.URL.Path == s.source.URL {
		img = s.image
		dpi = s.source.Metadata.DPI
	} else if s.result != nil && r.URL.Path == fmt.Sprintf("/media/result/%d.png", s.resultJob) {
		img = s.result.Image
	}
	s.mu.RUnlock()
	if img == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == "GET" {
		_ = engine.WritePNG(r.Context(), w, img, dpi)
	}
}
func IsCanceled(e error) bool { return errors.Is(e, context.Canceled) }
