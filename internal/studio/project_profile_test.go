package studio

import (
	"archive/zip"
	"bytes"
	"colorninja/internal/engine"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPortableProjectRestoresExactPixelsAndStackWithoutOriginalFiles(t *testing.T) {
	for _, mode := range []string{"standard", "guided", "stack"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			s := New(context.Background(), filepath.Join(dir, "settings.json"))
			defer s.Shutdown()
			img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					img.SetNRGBA(x, y, color.NRGBA{uint8(x * 17), uint8(y * 17), 77, uint8((x + y) * 8)})
				}
			}
			input, libpath := filepath.Join(dir, "source.png"), filepath.Join(dir, "filaments.json")
			if err := engine.AtomicWrite(input, false, func(w io.Writer) error { return engine.WritePNG(context.Background(), w, img, [2]float64{300, 300}) }); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join("..", "engine", "testdata", "library.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(libpath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = s.LoadImage(input); err != nil {
				t.Fatal(err)
			}
			r := req(s, 7)
			r.Options.Mode = mode
			r.LibraryPath = libpath
			r.Options.ColorPriority = "distinctive"
			r.Options.HueForge.MaxDepth = 1.04
			r.Options.HueForge.AutoDepth = true
			r.Options.HueForge.MaxRuns = 6
			r.Filter.AvoidSilkMetallic = true
			preview, err := s.Process(r)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "portable.colorninja")
			// Saving must use the exact inventory used to render, even if the disk library changed.
			if err = os.WriteFile(libpath, []byte("changed after processing"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = s.SaveProject(path, r, false); err != nil {
				t.Fatal(err)
			}
			os.Remove(input)
			os.Remove(libpath)
			fresh := New(context.Background(), filepath.Join(dir, "fresh", "settings.json"))
			defer fresh.Shutdown()
			snap, err := fresh.OpenProject(path)
			if err != nil {
				t.Fatal(err)
			}
			if snap.Preview == nil || snap.Preview.Result.SHA256 != preview.Result.SHA256 || !bytes.Equal(snap.Preview.Result.Image.Pix, preview.Result.Image.Pix) || !bytes.Equal(fresh.image.Pix, s.image.Pix) {
				t.Fatal("exact source or saved preview lost")
			}
			if !reflect.DeepEqual(snap.Preview.Result.LayerMap, preview.Result.LayerMap) || !reflect.DeepEqual(snap.Preview.Result.Stack, preview.Result.Stack) || !reflect.DeepEqual(snap.Preview.Result.StackView, preview.Result.StackView) || snap.Settings.Options != r.Options || !reflect.DeepEqual(snap.Settings.Filter, r.Filter) {
				t.Fatal("project settings or stack changed")
			}
			if !reflect.DeepEqual(snap.Source.Metadata, s.source.Metadata) {
				t.Fatal("metadata lost")
			}
			if len(snap.Settings.Recent) != 0 {
				t.Fatal("embedded image added a nonexistent recent path")
			}
			rebuilt, err := fresh.Process(Request{ID: 9, Revision: snap.Source.Revision, Options: snap.Settings.Options, LibraryPath: snap.Settings.LibraryPath, Filter: snap.Settings.Filter})
			if err != nil {
				t.Fatal(err)
			}
			if rebuilt.Result.SHA256 != preview.Result.SHA256 {
				t.Fatal("embedded source/inventory did not reproduce output")
			}
			if mode == "stack" {
				if err = fresh.Export("hfp", filepath.Join(dir, "restored.hfp"), rebuilt.ID, rebuilt.Revision, false); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestProjectSavesChangedSettingsWithoutStaleResult(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	if _, err := s.Process(r); err != nil {
		t.Fatal(err)
	}
	r.Options.Colors = 2
	path := filepath.Join(t.TempDir(), "unrendered.colorninja")
	if err := s.SaveProject(path, r, false); err != nil {
		t.Fatal(err)
	}
	snap, err := s.OpenProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Preview != nil || snap.Settings.Options.Colors != 2 {
		t.Fatal("stale result restored for changed settings")
	}
}

func TestCorruptPortableProjectLeavesCurrentDocumentIntact(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "valid.colorninja")
	if err = s.SaveProject(path, r, false); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, kind := range []string{"checksum", "traversal", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			bad := filepath.Join(t.TempDir(), "bad.colorninja")
			err := engine.AtomicWrite(bad, false, func(w io.Writer) error {
				out := zip.NewWriter(w)
				for _, entry := range z.File {
					r, e := entry.Open()
					if e != nil {
						return e
					}
					b, e := io.ReadAll(r)
					r.Close()
					if e != nil {
						return e
					}
					name := entry.Name
					if name == "source.png" {
						if kind == "checksum" {
							b[len(b)/2] ^= 1
						}
						if kind == "traversal" {
							name = "../source.png"
						}
					}
					v, e := out.Create(name)
					if e != nil {
						return e
					}
					if _, e = v.Write(b); e != nil {
						return e
					}
					if kind == "duplicate" && name == "source.png" {
						v, e = out.Create(name)
						if e != nil {
							return e
						}
						v.Write(b)
					}
				}
				return out.Close()
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.OpenProject(bad); err == nil {
				t.Fatal("corrupt project accepted")
			}
			if s.source.Revision != p.Revision || s.result.SHA256 != p.Result.SHA256 {
				t.Fatal("active document lost on invalid project")
			}
		})
	}
}

func TestLegacyProjectRemainsReadable(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	path := filepath.Join(t.TempDir(), "legacy.colorninja.json")
	b, err := json.Marshal(Document{1, s.source.Path, false, r.Options, "", r.Filter})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := s.OpenProject(path)
	if err != nil || snap.Settings.Options != r.Options {
		t.Fatal("legacy project failed", err)
	}
	// A malformed legacy source may name a project; it must be decoded only as
	// an image, rather than recursively reopening projects.
	b, _ = json.Marshal(Document{1, path, false, r.Options, "", r.Filter})
	os.WriteFile(path, b, 0600)
	if _, err = s.OpenProject(path); err == nil {
		t.Fatal("recursive project source accepted")
	}
}

func TestEveryExportCanWriteExactSettingsProfileAndProtectExistingFiles(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode = "stack"
	r.LibraryPath = filepath.Join("..", "engine", "testdata", "library.json")
	r.Options.HueForge.MaxDepth = .72
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	prefs := s.settings.Preferences
	prefs.ExportProfile = true
	if _, err = s.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	for kind, ext := range map[string]string{"png": ".png", "palette": ".json", "layers": ".png", "hfp": ".hfp", "project": ".colorninja"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), kind+ext)
			sidecar := ProfilePath(path)
			if err := os.WriteFile(sidecar, []byte("keep this"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.Export(kind, path, p.ID, p.Revision, false); err == nil {
				t.Fatal("existing profile replaced")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("primary written before collision check")
			}
			if err := s.Export(kind, path, p.ID, p.Revision, false, true); err != nil {
				t.Fatal(err)
			}
			profile, err := LoadProfile(sidecar)
			if err != nil || profile.Options != r.Options || !reflect.DeepEqual(profile.Filter, r.Filter) || profile.LibrarySHA256 != p.Result.Stack.LibrarySHA256 {
				t.Fatal("profile settings differ from exported result", err)
			}
			if err = s.Export(kind, path, p.ID, p.Revision, false, true); err == nil {
				t.Fatal("primary overwritten without approval")
			}
		})
	}
	prefs.ExportProfile = false
	s.SavePreferences(prefs)
	path := filepath.Join(t.TempDir(), "plain.png")
	if err = s.Export("png", path, p.ID, p.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(ProfilePath(path)); !os.IsNotExist(err) {
		t.Fatal("unchecked option wrote profile")
	}
}

func TestPresetRenameAndProfileValidation(t *testing.T) {
	s := fixture(t)
	o := req(s, 1).Options
	s.SavePreset("Original", o)
	s.SavePreset("Other", o)
	if _, err := s.RenamePreset("Original", "other"); err == nil {
		t.Fatal("rename silently replaced another preset")
	}
	if _, err := s.RenamePreset("Original", "Favorite"); err != nil {
		t.Fatal(err)
	}
	fresh := New(context.Background(), s.configPath)
	defer fresh.Shutdown()
	if fresh.settings.Presets[0].Name != "Favorite" || fresh.settings.Presets[0].Options != o {
		t.Fatal("renamed preset was not persisted")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte(`{"format":"ColorNinja settings","schemaVersion":99}`), 0600)
	if _, err := LoadProfile(path); err == nil {
		t.Fatal("unsupported profile accepted")
	}
}
