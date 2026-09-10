package studio

import (
	"archive/zip"
	"bytes"
	"colorninja/internal/engine"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
)

const projectLibraryPath = "embedded:project-filaments"
const maxLibraryBytes = 32 << 20

// SaveResultProject lets batch workflows preserve the same already-rendered
// document as the desktop without processing it a second time.
func SaveResultProject(ctx context.Context, path, input string, source *engine.LoadedImage, result *engine.Result, options engine.Options, filter engine.LibraryFilter, library []byte, overwrite bool) error {
	req := Request{ID: 1, Revision: 1, Options: options, Filter: filter}
	s := &Studio{ctx: ctx, image: source.Image, source: Source{Name: filepath.Base(input), Path: input, Revision: 1, Metadata: source.Metadata}, result: result, resultRequest: req, resultLibrary: library}
	return s.SaveProject(path, req, overwrite)
}

type projectManifest struct {
	Format        string               `json:"format"`
	SchemaVersion int                  `json:"schemaVersion"`
	Name          string               `json:"name"`
	Metadata      engine.ImageMetadata `json:"metadata"`
	Options       engine.Options       `json:"options"`
	Filter        engine.LibraryFilter `json:"filter"`
	Files         map[string]string    `json:"files"`
}

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(raw)) > limit {
		err = fmt.Errorf("file exceeds the supported size limit")
	}
	return raw, err
}

func (s *Studio) libraryBytes(path string) ([]byte, error) {
	if path == projectLibraryPath {
		s.mu.RLock()
		raw := s.embeddedLibrary
		s.mu.RUnlock()
		if len(raw) == 0 {
			return nil, fmt.Errorf("open the project again to load its embedded filaments")
		}
		return raw, nil
	}
	return readBounded(path, maxLibraryBytes)
}
func (s *Studio) loadLibrary(path string, filter engine.LibraryFilter) (engine.Library, error) {
	raw, err := s.libraryBytes(path)
	if err != nil {
		return engine.Library{}, err
	}
	return engine.ParseLibrary(raw, filter)
}

func sameSettings(a, b Request) bool {
	af, bf := a.Filter, b.Filter
	return a.Revision == b.Revision && a.Options == b.Options && a.LibraryPath == b.LibraryPath && af.IncludeUnowned == bf.IncludeUnowned && af.AllowSecondary == bf.AllowSecondary && af.AvoidSilkMetallic == bf.AvoidSilkMetallic && slices.Equal(af.MaterialTypes, bf.MaterialTypes) && slices.Equal(af.ExcludedIDs, bf.ExcludedIDs)
}

func (s *Studio) writeProject(w io.Writer, req Request) error {
	if err := req.Options.Validate(); err != nil {
		return err
	}
	s.mu.RLock()
	src, source := s.image, s.source
	regions := s.regions
	var result *engine.Result
	var library []byte
	if s.result != nil && sameSettings(req, s.resultRequest) {
		result, library = s.result, s.resultLibrary
	}
	s.mu.RUnlock()
	if regions != nil && len(regions.Document.Groups) > 0 && result == nil {
		return fmt.Errorf("restore the edited preview's settings or reset its region edits before saving new settings")
	}
	if src == nil || source.Revision != req.Revision {
		return fmt.Errorf("image changed; save again")
	}
	if library == nil && req.LibraryPath != "" {
		var err error
		library, err = s.libraryBytes(req.LibraryPath)
		if err != nil && req.Options.Mode != "standard" {
			return err
		}
	}
	if len(library) > maxLibraryBytes {
		return fmt.Errorf("embedded filament library exceeds the 32 MB limit")
	}
	if req.Options.Mode != "standard" {
		if _, err := engine.ParseLibrary(library, req.Filter); err != nil {
			return err
		}
	}
	m := projectManifest{"ColorNinja project", 2, source.Name, source.Metadata, req.Options, req.Filter, map[string]string{}}
	if regions != nil && len(regions.Document.Groups) > 0 {
		m.SchemaVersion = 3
	}
	z := zip.NewWriter(w)
	entry := func(name string, fn func(io.Writer) error) error {
		out, err := z.Create(name)
		if err != nil {
			return err
		}
		h := sha256.New()
		if err = fn(io.MultiWriter(out, h)); err != nil {
			return err
		}
		m.Files[name] = fmt.Sprintf("%x", h.Sum(nil))
		return nil
	}
	if err := entry("source.png", func(w io.Writer) error { return engine.WritePNG(s.ctx, w, src, source.Metadata.DPI) }); err != nil {
		return err
	}
	if len(library) > 0 {
		if err := entry("filaments.json", func(w io.Writer) error { _, err := w.Write(library); return err }); err != nil {
			return err
		}
	}
	if result != nil {
		if m.SchemaVersion == 3 {
			base := engine.ReframeResult(regions.Base, req.Options)
			if err := entry("region-base.png", func(w io.Writer) error { return engine.WritePNG(s.ctx, w, regions.Base.Image, source.Metadata.DPI) }); err != nil {
				return err
			}
			if err := entry("region-base.json", func(w io.Writer) error { return json.NewEncoder(w).Encode(base) }); err != nil {
				return err
			}
			if err := entry("region-base-layers.bin", func(w io.Writer) error { return binary.Write(w, binary.LittleEndian, regions.Base.LayerMap) }); err != nil {
				return err
			}
			if err := entry("region-edits.json", func(w io.Writer) error { return json.NewEncoder(w).Encode(regions.Document) }); err != nil {
				return err
			}
		}
		if err := entry("result.png", func(w io.Writer) error { return engine.WritePNG(s.ctx, w, result.Image, source.Metadata.DPI) }); err != nil {
			return err
		}
		if err := entry("result.json", func(w io.Writer) error { return json.NewEncoder(w).Encode(result) }); err != nil {
			return err
		}
		if result.Stack != nil {
			if err := entry("layers.bin", func(w io.Writer) error { return binary.Write(w, binary.LittleEndian, result.LayerMap) }); err != nil {
				return err
			}
		}
	}
	out, err := z.Create("project.json")
	if err != nil {
		return err
	}
	if err = json.NewEncoder(out).Encode(m); err != nil {
		return err
	}
	return z.Close()
}

func (s *Studio) SaveProject(path string, req Request, overwrite bool) error {
	s.mu.RLock()
	source := s.source
	s.mu.RUnlock()
	if err := engine.DistinctPaths(path, source.Path, req.LibraryPath); err != nil {
		return err
	}
	return engine.AtomicWrite(path, overwrite, func(w io.Writer) error { return s.writeProject(w, req) })
}

func projectPNG(raw []byte) (*image.NRGBA, error) {
	c, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if c.Width < 1 || c.Height < 1 || int64(c.Width)*int64(c.Height) > engine.MaxImagePixels {
		return nil, fmt.Errorf("project image exceeds supported dimensions")
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if n, ok := img.(*image.NRGBA); ok {
		return n, nil
	}
	n := image.NewNRGBA(img.Bounds())
	draw.Draw(n, n.Bounds(), img, img.Bounds().Min, draw.Src)
	return n, nil
}

func (s *Studio) OpenProject(path string) (Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	var magic [4]byte
	_, err = f.Read(magic[:])
	f.Close()
	if err != nil {
		return Snapshot{}, err
	}
	if string(magic[:]) != "PK\x03\x04" {
		return s.openLegacyProject(path)
	}
	ctx, serial := s.beginLoad()
	z, err := zip.OpenReader(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer z.Close()
	limits := map[string]uint64{"project.json": 1 << 20, "source.png": 512 << 20, "result.png": 512 << 20, "result.json": 32 << 20, "filaments.json": maxLibraryBytes, "layers.bin": 2 * engine.MaxImagePixels}
	limits["region-base.png"] = 128 << 20
	limits["region-base.json"] = 32 << 20
	limits["region-base-layers.bin"] = 2 * engine.RegionMaxPixels
	limits["region-edits.json"] = 64 << 20
	entries := map[string]*zip.File{}
	for _, f := range z.File {
		limit, ok := limits[f.Name]
		if !ok || entries[f.Name] != nil || f.UncompressedSize64 > limit {
			return Snapshot{}, fmt.Errorf("invalid or oversized project entry: %s", f.Name)
		}
		entries[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f := entries[name]
		if f == nil {
			return nil, fmt.Errorf("project is missing %s", name)
		}
		r, e := f.Open()
		if e != nil {
			return nil, e
		}
		defer r.Close()
		b, e := io.ReadAll(io.LimitReader(r, int64(limits[name])+1))
		if uint64(len(b)) > limits[name] {
			return nil, fmt.Errorf("oversized project entry")
		}
		return b, e
	}
	raw, err := read("project.json")
	if err != nil {
		return Snapshot{}, err
	}
	var m projectManifest
	if err = json.Unmarshal(raw, &m); err != nil {
		return Snapshot{}, err
	}
	if m.Format != "ColorNinja project" || (m.SchemaVersion != 2 && m.SchemaVersion != 3) {
		return Snapshot{}, fmt.Errorf("unsupported ColorNinja project version")
	}
	if err = m.Options.Validate(); err != nil {
		return Snapshot{}, err
	}
	if len(m.Files) != len(entries)-1 {
		return Snapshot{}, fmt.Errorf("project entry manifest is incomplete")
	}
	checked := func(name string) ([]byte, error) {
		b, e := read(name)
		if e == nil && fmt.Sprintf("%x", sha256.Sum256(b)) != m.Files[name] {
			e = fmt.Errorf("project checksum mismatch: %s", name)
		}
		return b, e
	}
	raw, err = checked("source.png")
	if err != nil {
		return Snapshot{}, err
	}
	src, err := projectPNG(raw)
	if err != nil {
		return Snapshot{}, err
	}
	var library []byte
	if entries["filaments.json"] != nil {
		library, err = checked("filaments.json")
		if err != nil {
			return Snapshot{}, err
		}
	}
	if m.Options.Mode != "standard" {
		if _, err = engine.ParseLibrary(library, m.Filter); err != nil {
			return Snapshot{}, err
		}
	}
	var result *engine.Result
	if entries["result.json"] != nil {
		raw, err = checked("result.json")
		if err != nil {
			return Snapshot{}, err
		}
		result = &engine.Result{}
		if err = json.Unmarshal(raw, result); err != nil {
			return Snapshot{}, err
		}
		raw, err = checked("result.png")
		if err != nil {
			return Snapshot{}, err
		}
		result.Image, err = projectPNG(raw)
		if err != nil {
			return Snapshot{}, err
		}
		if result.Image.Bounds() != src.Bounds() || result.SourceSize != [2]int{src.Bounds().Dx(), src.Bounds().Dy()} || fmt.Sprintf("%x", sha256.Sum256(result.Image.Pix)) != result.SHA256 {
			return Snapshot{}, fmt.Errorf("saved result does not match the project image")
		}
		if result.Stack != nil {
			raw, err = checked("layers.bin")
			if err != nil {
				return Snapshot{}, err
			}
			if len(raw) != 2*src.Bounds().Dx()*src.Bounds().Dy() {
				return Snapshot{}, fmt.Errorf("invalid project layer dimensions")
			}
			result.LayerMap = make([]uint16, len(raw)/2)
			if err = binary.Read(bytes.NewReader(raw), binary.LittleEndian, result.LayerMap); err != nil {
				return Snapshot{}, err
			}
			if err = validateProjectStack(result, m.Options); err != nil {
				return Snapshot{}, err
			}
		} else if entries["layers.bin"] != nil {
			return Snapshot{}, fmt.Errorf("layer data requires a saved stack")
		}
		if (m.Options.Mode == "stack") != (result.Stack != nil) {
			return Snapshot{}, fmt.Errorf("saved workflow and stack disagree")
		}
	} else if entries["result.png"] != nil || entries["layers.bin"] != nil {
		return Snapshot{}, fmt.Errorf("incomplete saved result")
	}
	var regions *regionSession
	if m.SchemaVersion == 3 {
		if result == nil || result.RegionEdits == nil {
			return Snapshot{}, fmt.Errorf("edited project is missing its result")
		}
		base := &engine.Result{}
		raw, err = checked("region-base.json")
		if err != nil {
			return Snapshot{}, err
		}
		if err = json.Unmarshal(raw, base); err != nil {
			return Snapshot{}, err
		}
		raw, err = checked("region-base.png")
		if err != nil {
			return Snapshot{}, err
		}
		base.Image, err = projectPNG(raw)
		if err != nil {
			return Snapshot{}, err
		}
		if base.RegionEdits != nil || base.Image.Bounds() != src.Bounds() || fmt.Sprintf("%x", sha256.Sum256(base.Image.Pix)) != base.SHA256 {
			return Snapshot{}, fmt.Errorf("invalid region baseline image")
		}
		raw, err = checked("region-base-layers.bin")
		if err != nil {
			return Snapshot{}, err
		}
		if len(raw) != 2*src.Bounds().Dx()*src.Bounds().Dy() {
			return Snapshot{}, fmt.Errorf("invalid region baseline dimensions")
		}
		base.LayerMap = make([]uint16, len(raw)/2)
		if err = binary.Read(bytes.NewReader(raw), binary.LittleEndian, base.LayerMap); err != nil {
			return Snapshot{}, err
		}
		if err = validateProjectStack(base, m.Options); err != nil {
			return Snapshot{}, err
		}
		var doc engine.RegionDocument
		raw, err = checked("region-edits.json")
		if err != nil {
			return Snapshot{}, err
		}
		if err = json.Unmarshal(raw, &doc); err != nil {
			return Snapshot{}, err
		}
		if err = doc.Validate(base); err != nil {
			return Snapshot{}, err
		}
		replayed, e := engine.ApplyRegionDocument(ctx, base, src, m.Options, doc)
		if e != nil {
			return Snapshot{}, e
		}
		if replayed.SHA256 != result.SHA256 || !slices.Equal(replayed.LayerMap, result.LayerMap) {
			return Snapshot{}, fmt.Errorf("saved region edits and result disagree")
		}
		result = replayed
		index, e := engine.BuildRegionIndex(ctx, result, base)
		if e != nil {
			return Snapshot{}, e
		}
		regions = &regionSession{Base: base, Document: doc, Index: index, Mask: make([]bool, len(base.LayerMap)), Version: 1}
	} else {
		if result != nil && result.RegionEdits != nil {
			return Snapshot{}, fmt.Errorf("region edits require a version 3 project")
		}
		for _, name := range []string{"region-base.png", "region-base.json", "region-base-layers.bin", "region-edits.json"} {
			if entries[name] != nil {
				return Snapshot{}, fmt.Errorf("unexpected region data in a legacy project")
			}
		}
	}
	// Validate old options and the saved image/layers before migrating the
	// export choice. The cached preview's print plan remains intact.
	paused := m.Options.HeightMap.Mode != "" && m.Options.HeightMap.Mode != "color-match"
	m.Options = workflowOptions(m.Options)
	if paused {
		result = nil
		regions = nil
	} // Never display cached channel heights as a Color Match preview.
	if result != nil && result.Stack != nil {
		// Saved Mesh Core displays may describe Beta 5's opaque TD bands. Rebuild
		// this derived export view after validating the exact saved pixels/heights.
		result = engine.ReframeResult(result, m.Options)
		result.SurfaceView, err = engine.BuildSurfaceView(ctx, result, m.Options)
		if err != nil {
			return Snapshot{}, err
		}
	}
	// Finish validation before replacing the active document.
	if err = s.installImage(ctx, &engine.LoadedImage{Image: src, Metadata: m.Metadata}, filepath.Base(m.Name), true, serial); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	if serial != s.loadSerial {
		s.mu.Unlock()
		return Snapshot{}, context.Canceled
	}
	s.source.Path = ""
	s.source.Name, s.source.Demo = filepath.Base(m.Name), false
	s.warning = ""
	if paused {
		s.warning = "Channel workflows are temporarily disabled. This project is now set to Color Match; generate a new preview."
	}
	s.projectPath = path
	s.embeddedLibrary = library
	s.settings.LibraryPath = ""
	if len(library) > 0 {
		s.settings.LibraryPath = projectLibraryPath
	}
	s.settings.Options, s.settings.Filter = m.Options, m.Filter
	s.result, s.resultLibrary = result, library
	s.regions = regions
	s.resultJob = s.job
	s.resultRequest = Request{s.job, s.revision, m.Options, s.settings.LibraryPath, m.Filter}
	s.mu.Unlock()
	if err = s.saveSettings(); err != nil {
		s.mu.Lock()
		s.warning = err.Error()
		s.mu.Unlock()
	}
	return s.Snapshot(), nil
}

func validateProjectStack(r *engine.Result, o engine.Options) error {
	p := r.Stack
	if p.Options != o.HueForge || p.Options.Validate() != nil || len(p.Runs) == 0 {
		return fmt.Errorf("invalid saved stack options")
	}
	last := 0
	for i, run := range p.Runs {
		if run.Position != i+1 || run.Filament.TD <= 0 || run.StartLayer != last+1 || run.Layers < 1 || run.EndLayer != last+run.Layers || run.EndLayer > p.Options.MaxLayers() {
			return fmt.Errorf("invalid saved stack runs")
		}
		last = run.EndLayer
	}
	if r.StackView != nil {
		if len(r.StackView.Layers) != last {
			return fmt.Errorf("saved stack map has invalid dimensions")
		}
		for i, row := range r.StackView.Layers {
			if row.Layer != i+1 || row.RunPosition < 1 || row.RunPosition > len(p.Runs) {
				return fmt.Errorf("invalid saved stack map")
			}
		}
	}
	colors := map[int]engine.RGB{}
	for _, c := range p.LayerColors {
		colors[c.Layer] = c.RGB
	}
	for i, layer := range r.LayerMap {
		if r.Image.Pix[i*4+3] == 0 {
			continue
		}
		c, ok := colors[int(layer)]
		if !ok || int(layer) < p.Options.BaseLayers() || int(layer) > last || c != (engine.RGB{r.Image.Pix[i*4], r.Image.Pix[i*4+1], r.Image.Pix[i*4+2]}) {
			return fmt.Errorf("saved pixels and stack layers disagree")
		}
	}
	return nil
}
