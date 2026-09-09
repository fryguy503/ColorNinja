package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type SettingsProfile struct {
	Format        string               `json:"format"`
	SchemaVersion int                  `json:"schemaVersion"`
	Name          string               `json:"name"`
	Options       engine.Options       `json:"options"`
	Filter        engine.LibraryFilter `json:"filter"`
	LibrarySHA256 string               `json:"librarySHA256,omitempty"`
}

func ProfilePath(output string) string { return output + ".colorninja-profile.json" }

func SaveSettingsProfile(path, name string, options engine.Options, filter engine.LibraryFilter, library []byte, overwrite bool) error {
	if err := options.Validate(); err != nil {
		return err
	}
	return writeProfile(path, makeProfile(name, Request{Options: options, Filter: filter}, library), overwrite)
}

func makeProfile(name string, req Request, library []byte) SettingsProfile {
	p := SettingsProfile{Format: "ColorNinja settings", SchemaVersion: 1, Name: name, Options: req.Options, Filter: req.Filter}
	if len(library) > 0 {
		p.LibrarySHA256 = fmt.Sprintf("%x", sha256.Sum256(library))
	}
	return p
}
func writeProfile(path string, p SettingsProfile, overwrite bool) error {
	return engine.AtomicWrite(path, overwrite, func(w io.Writer) error { enc := json.NewEncoder(w); enc.SetIndent("", "  "); return enc.Encode(p) })
}
func (s *Studio) SaveProfile(path, name string, req Request, overwrite bool) error {
	if err := req.Options.Validate(); err != nil {
		return err
	}
	s.mu.RLock()
	source, projectPath := s.source, s.projectPath
	s.mu.RUnlock()
	if err := engine.DistinctPaths(path, source.Path, req.LibraryPath, projectPath); err != nil {
		return err
	}
	var library []byte
	if req.LibraryPath != "" {
		var err error
		library, err = s.libraryBytes(req.LibraryPath)
		if err != nil && req.Options.Mode != "standard" {
			return err
		}
	}
	return writeProfile(path, makeProfile(name, req, library), overwrite)
}
func LoadProfile(path string) (SettingsProfile, error) {
	var p SettingsProfile
	raw, err := readBounded(path, 2<<20)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(bytes.TrimPrefix(raw, []byte{239, 187, 191}), &p); err != nil {
		return p, fmt.Errorf("invalid settings profile: %w", err)
	}
	if p.Format != "ColorNinja settings" || p.SchemaVersion != 1 {
		return p, fmt.Errorf("unsupported ColorNinja settings profile")
	}
	if err = p.Options.Validate(); err != nil {
		return p, err
	}
	return p, nil
}

// Both paths are checked before publishing anything. Individual files are
// written atomically; a second-file I/O failure reports the completed export.
func (s *Studio) Export(kind, path string, id, revision uint64, overwrite bool, overwriteProfile ...bool) error {
	s.mu.RLock()
	req, source, r, include := s.resultRequest, s.source, s.result, s.settings.Preferences.ExportProfile
	library, projectPath := s.resultLibrary, s.projectPath
	s.mu.RUnlock()
	if r == nil || req.ID != id || req.Revision != revision || source.Revision != revision {
		return fmt.Errorf("generate a current preview before exporting")
	}
	if kind != "project" {
		if err := engine.DistinctPaths(path, projectPath); err != nil {
			return err
		}
	}
	profilePath := ProfilePath(path)
	replaceProfile := len(overwriteProfile) > 0 && overwriteProfile[0]
	if include {
		if err := engine.DistinctPaths(profilePath, path, source.Path, req.LibraryPath, projectPath); err != nil {
			return err
		}
		if info, err := os.Stat(profilePath); err == nil {
			if info.IsDir() || !replaceProfile {
				return fmt.Errorf("settings profile already exists: %s", profilePath)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := s.exportSingle(kind, path, id, revision, overwrite); err != nil {
		return err
	}
	if include {
		if err := writeProfile(profilePath, makeProfile(source.Name, req, library), replaceProfile); err != nil {
			return fmt.Errorf("export saved to %s, but its settings profile could not be saved: %w", path, err)
		}
	}
	return nil
}

func (s *Studio) RenamePreset(oldName, name string) ([]Preset, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 60 {
		return nil, fmt.Errorf("preset name must contain 1–60 characters")
	}
	s.mu.Lock()
	p := append([]Preset{}, s.settings.Presets...)
	index := -1
	for i, item := range p {
		if item.Name == oldName {
			index = i
		} else if strings.EqualFold(item.Name, name) {
			s.mu.Unlock()
			return nil, fmt.Errorf("a preset with that name already exists")
		}
	}
	if index < 0 {
		s.mu.Unlock()
		return nil, fmt.Errorf("preset no longer exists")
	}
	p[index].Name = name
	s.settings.Presets = p
	s.mu.Unlock()
	return p, s.saveSettings()
}
