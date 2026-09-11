// Package studio owns document state and cancellable processing independently
// of the Wails host, so the desktop and integration tests use the same service.
package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"colorninja/internal/td1"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	Comparisons   []Comparison         `json:"comparisons,omitempty"`
	Preferences   Preferences          `json:"preferences"`
	SchemaVersion int                  `json:"schemaVersion"`
	Options       engine.Options       `json:"options"`
	LibraryPath   string               `json:"libraryPath"`
	Filter        engine.LibraryFilter `json:"filter"`
	Presets       []Preset             `json:"presets"`
	Recent        []string             `json:"recent"`
}
type Preferences struct {
	Advanced           bool `json:"advanced"`
	CheckOnStartup     bool `json:"checkOnStartup"`
	IncludePrereleases bool `json:"includePrereleases"`
	// Retain the Beta 2 preference key; the option now saves both companions.
	ExportProfile bool `json:"exportProfile"`
}
type Snapshot struct {
	Source   Source          `json:"source"`
	Settings Settings        `json:"settings"`
	Library  *engine.Library `json:"library"`
	Warning  string          `json:"warning"`
	Preview  *Preview        `json:"preview,omitempty"`
}
type Request struct {
	ID          uint64               `json:"id"`
	Revision    uint64               `json:"revision"`
	Options     engine.Options       `json:"options"`
	LibraryPath string               `json:"libraryPath"`
	Filter      engine.LibraryFilter `json:"filter"`
}
type Preview struct {
	ReusedStages []string       `json:"reusedStages,omitempty"`
	ID           uint64         `json:"id"`
	Revision     uint64         `json:"revision"`
	URL          string         `json:"url"`
	Result       *engine.Result `json:"result"`
	Options      engine.Options `json:"options"`
	Seconds      float64        `json:"seconds"`
	Warning      string         `json:"warning,omitempty"`
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
	TD1             *td1.Manager
	regions         *regionSession
	processor       engine.Processor
	ctx             context.Context
	mu              sync.RWMutex
	worker          sync.Mutex
	settingsMu      sync.Mutex
	libraryMu       sync.Mutex
	deviceMu        sync.Mutex
	recoveryPlan    *td1.RecoveryPlan
	cancel          context.CancelFunc
	loadCancel      context.CancelFunc
	loadSerial      uint64
	job             uint64
	revision        uint64
	image           *image.NRGBA
	source          Source
	settings        Settings
	result          *engine.Result
	resultRequest   Request
	resultLibrary   []byte
	embeddedLibrary []byte
	projectPath     string
	resultJob       uint64
	configPath      string
	warning         string
	Emit            func(string, any)
}

func New(ctx context.Context, configPath string) *Studio {
	o := engine.DefaultOptions()
	o.Colors = 8
	o.TotalColors = true
	s := &Studio{ctx: ctx, configPath: configPath, settings: Settings{SchemaVersion: 1, Options: o, Presets: []Preset{}, Recent: []string{}, Filter: engine.LibraryFilter{MaterialTypes: []string{}, ExcludedIDs: []int{}}}}
	s.settings.Preferences.CheckOnStartup = true
	s.TD1 = td1.New()
	if raw, e := os.ReadFile(configPath); e == nil {
		saved := Settings{Preferences: s.settings.Preferences}
		if e = json.Unmarshal(raw, &saved); e == nil && saved.SchemaVersion == 1 && saved.Options.Validate() == nil {
			s.settings = saved
		} else {
			s.warning = "Saved settings could not be read. Defaults have been restored."
		}
	} else if !os.IsNotExist(e) {
		s.warning = "Saved settings could not be opened: " + e.Error()
	}
	s.settings.Options = workflowOptions(s.settings.Options)
	for i := range s.settings.Presets {
		s.settings.Presets[i].Options = workflowOptions(s.settings.Presets[i].Options)
	}
	if s.settings.LibraryPath == "" {
		homeDir, _ := os.UserHomeDir()
		s.settings.LibraryPath = detectHueForgeLibrary(runtime.GOOS, homeDir, os.Getenv("APPDATA"))
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
	var preview *Preview
	if s.result != nil {
		req := s.resultRequest
		preview = &Preview{ID: req.ID, Revision: req.Revision, URL: fmt.Sprintf("/media/result/%d.png", s.resultJob), Result: s.result, Options: req.Options}
	}
	s.mu.RUnlock()
	var lib *engine.Library
	if settings.LibraryPath != "" {
		l, e := s.loadLibrary(settings.LibraryPath, settings.Filter)
		if e == nil {
			lib = &l
		} else {
			warning = e.Error()
		}
	}
	return Snapshot{source, settings, lib, warning, preview}
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
	s.regions = nil
	s.resultLibrary = nil
	s.projectPath = ""
	s.source = Source{Name: filepath.Base(path), Path: path,
		Width: loaded.Image.Bounds().Dx(), Height: loaded.Image.Bounds().Dy(),
		UniqueColors: uniqueColors, URL: fmt.Sprintf("/media/source/%d.png", s.revision),
		Revision: s.revision, Demo: demo, Metadata: loaded.Metadata}
	if demo {
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
	if strings.HasSuffix(strings.ToLower(path), ".colorninja") || strings.HasSuffix(strings.ToLower(path), ".colorninja.json") {
		return s.OpenProject(path)
	}
	return s.loadSourceImage(path)
}
func (s *Studio) loadSourceImage(path string) (Snapshot, error) {
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
	lib, e := s.loadLibrary(path, filter)
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
	if s.TD1 != nil {
		s.TD1.Disconnect()
	}
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
	req.Options = workflowOptions(req.Options)
	s.mu.Lock()
	if req.Revision != s.revision || s.image == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("image changed; refresh the preview")
	}
	if s.regions != nil && len(s.regions.Document.Groups) > 0 {
		if !engine.SameRegionPlanOptions(req.Options, s.resultRequest.Options) || req.LibraryPath != s.resultRequest.LibraryPath || !sameRegionFilter(req.Filter, s.resultRequest.Filter) {
			s.mu.Unlock()
			return nil, fmt.Errorf("this result has region edits; reset them in Region Edit before generating a different stack")
		}
		if s.cancel != nil {
			s.cancel()
		}
		ctx, cancel := context.WithCancel(s.ctx)
		s.cancel = cancel
		s.job++
		job, previous := s.job, s.result
		s.mu.Unlock()
		defer cancel()
		s.worker.Lock()
		defer s.worker.Unlock()
		r := engine.ReframeResult(previous, req.Options)
		view, err := engine.BuildSurfaceView(ctx, r, req.Options)
		if err != nil {
			return nil, err
		}
		r.SurfaceView = view
		s.mu.Lock()
		if s.job != job || s.revision != req.Revision {
			s.mu.Unlock()
			return nil, context.Canceled
		}
		s.result, s.resultRequest, s.resultJob = r, req, job
		s.settings.Options = req.Options
		p := &Preview{ID: req.ID, Revision: req.Revision, URL: fmt.Sprintf("/media/result/%d.png", job), Result: r, Options: req.Options}
		s.mu.Unlock()
		if err = s.saveSettings(); err != nil {
			p.Warning = "Preview ready, but preferences could not be saved: " + err.Error()
		}
		return p, nil
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
	var libraryRaw []byte
	if req.LibraryPath != "" {
		var err error
		libraryRaw, err = s.libraryBytes(req.LibraryPath)
		if err != nil && req.Options.Mode != "standard" {
			return nil, err
		}
	}
	if req.Options.Mode != "standard" {
		l, e := engine.ParseLibrary(libraryRaw, req.Filter)
		if e != nil {
			return nil, e
		}
		library = &l
	}
	last := time.Time{}
	r, e := s.processor.Process(ctx, src, req.Options, library, func(p engine.Progress) {
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
	s.regions = nil
	s.resultLibrary = libraryRaw
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
	return &Preview{ID: req.ID, Revision: req.Revision, URL: fmt.Sprintf("/media/result/%d.png", job), Result: r, Options: req.Options, Seconds: time.Since(start).Seconds(), Warning: warning, ReusedStages: append([]string(nil), s.processor.Reused...)}, nil
}
func (s *Studio) saveSettings() error {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	if settings.LibraryPath == projectLibraryPath {
		settings.LibraryPath = ""
	}
	return engine.AtomicWrite(s.configPath, true, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(settings)
	})
}
func (s *Studio) SavePreferences(p Preferences) (Preferences, error) {
	s.mu.Lock()
	s.settings.Preferences = p
	s.mu.Unlock()
	return p, s.saveSettings()
}
func (s *Studio) SavePreset(name string, o engine.Options) ([]Preset, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 60 {
		return nil, fmt.Errorf("preset name must contain 1â€“60 characters")
	}
	if e := o.Validate(); e != nil {
		return nil, e
	}
	o = workflowOptions(o)
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
func (s *Studio) openLegacyProject(path string) (Snapshot, error) {
	raw, e := readBounded(path, 2<<20)
	if e != nil {
		return Snapshot{}, e
	}
	var d struct {
		Document
		Format string          `json:"format"`
		Engine string          `json:"engine"`
		Result json.RawMessage `json:"result"`
	}
	if e = json.Unmarshal(bytes.TrimPrefix(raw, []byte{239, 187, 191}), &d); e != nil {
		return Snapshot{}, fmt.Errorf("invalid ColorNinja project: %w", e)
	}
	if d.Format == "ColorNinja settings" {
		return Snapshot{}, fmt.Errorf("this is a settings profile; use Presets & profiles > Load profile to apply it to an image. To restore the full project, open the .colorninja file")
	}
	if d.Engine != "" || d.Result != nil {
		return Snapshot{}, fmt.Errorf("this is a palette report; open the .colorninja file to restore your project")
	}
	if d.Format != "" {
		return Snapshot{}, fmt.Errorf("this file is not a supported ColorNinja project; open a .colorninja file")
	}
	if d.SchemaVersion != 1 {
		return Snapshot{}, fmt.Errorf("unsupported project version")
	}
	if !d.Demo && strings.TrimSpace(d.Source) == "" {
		return Snapshot{}, fmt.Errorf("this older project is missing its source image path; open a .colorninja project or reopen the original image")
	}
	if e = d.Options.Validate(); e != nil {
		return Snapshot{}, e
	}
	if d.Options.Mode != "standard" && strings.TrimSpace(d.LibraryPath) == "" {
		return Snapshot{}, fmt.Errorf("this older project is missing its filament library path; reopen the original image, select a library, and save a .colorninja project")
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
		_, e = s.loadSourceImage(d.Source)
	}
	if e != nil {
		return Snapshot{}, e
	}
	s.mu.Lock()
	s.settings.Options = workflowOptions(d.Options)
	s.projectPath = path
	s.settings.LibraryPath = d.LibraryPath
	s.settings.Filter = d.Filter
	s.mu.Unlock()
	if e = s.saveSettings(); e != nil {
		return Snapshot{}, e
	}
	return s.Snapshot(), nil
}
func (s *Studio) exportSingle(kind, path string, id, revision uint64, overwrite bool) error {
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
	case "project":
		return s.SaveProject(path, req, overwrite)
	case "png":
		return engine.SavePNG(s.ctx, path, r, source.Metadata, overwrite)
	case "palette":
		return engine.SaveReport(path, source.Path, r, req.Options, source.Metadata, overwrite)
	case "layers":
		return engine.SaveLayerMap(s.ctx, path, r, overwrite)
	case "hfp":
		return engine.SaveHFP(s.ctx, path, r, source.Name, source.Metadata, overwrite)
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
	} else if s.regions != nil && r.URL.Path == fmt.Sprintf("/media/region-base/%d/%s.png", s.revision, s.regions.Base.SHA256) {
		img = s.regions.Base.Image
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
