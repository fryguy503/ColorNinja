package studio

import (
	"colorninja/internal/td1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type td1BackupManifest struct {
	Serial string            `json:"serial"`
	Files  map[string]string `json:"files"`
}

func (s *Studio) readTD1Backup(dir, serial string) ([]td1.UpdateFile, error) {
	root, e := filepath.EvalSymlinks(filepath.Join(filepath.Dir(s.configPath), "td1", serial))
	if e != nil {
		return nil, e
	}
	dir, e = filepath.EvalSymlinks(dir)
	if e != nil {
		return nil, e
	}
	root, _ = filepath.Abs(root)
	dir, _ = filepath.Abs(dir)
	rel, e := filepath.Rel(root, dir)
	if e != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("select a backup folder for the connected device")
	}
	raw, e := readBounded(filepath.Join(dir, "backup.json"), 16384)
	if e != nil {
		return nil, fmt.Errorf("backup is incomplete: %w", e)
	}
	var manifest td1BackupManifest
	if e = json.Unmarshal(raw, &manifest); e != nil || manifest.Serial != serial {
		return nil, fmt.Errorf("backup does not belong to connected device")
	}
	if manifest.Files["settings.py"] == "" || manifest.Files["pins.py"] == "" {
		return nil, fmt.Errorf("backup is incomplete")
	}
	files := []td1.UpdateFile{}
	for _, name := range []string{"license.bin", "settings.py", "pins.py", "lib/rgbOffset.py"} {
		hash, ok := manifest.Files[name]
		if !ok {
			continue
		}
		file := filepath.Join(dir, filepath.FromSlash(name))
		resolved, e := filepath.EvalSymlinks(file)
		if e != nil {
			return nil, e
		}
		if !strings.EqualFold(resolved, file) {
			return nil, fmt.Errorf("backup file resolves outside its original path")
		}
		raw, e := readBounded(file, 1<<20)
		if e != nil {
			return nil, e
		}
		if len(raw) == 0 || fmt.Sprintf("%x", sha256.Sum256(raw)) != hash {
			return nil, fmt.Errorf("backup checksum failed for %s", name)
		}
		if name == "license.bin" && len(raw) != 256 {
			return nil, fmt.Errorf("backup license size is invalid")
		}
		files = append(files, td1.UpdateFile{Name: name, Size: len(raw), Data: raw})
	}
	return files, nil
}
