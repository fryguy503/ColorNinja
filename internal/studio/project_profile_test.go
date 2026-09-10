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
	"strings"
	"testing"
)

func TestBeta5MeshCoreUpgradesAcrossSavedWorkflows(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode, r.Options.Colors = "stack", 2
	r.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
	r.Options.HueForge.MaxDepth = .8
	r.Options.HueForge.MeshCore = "legacy-flat"
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	// Serialize the Beta 5 option name and cached flat Mesh Core, as an existing
	// user's project/preferences/profile would contain them.
	r.Options.HueForge.MeshCore = "planned-colors"
	p.Result.Stack.Options.MeshCore = "planned-colors"
	p.Result.StackView.MeshCore = "planned-colors"
	s.resultRequest.Options = r.Options
	dir := t.TempDir()
	project := filepath.Join(dir, "beta5.colorninja")
	if err = s.SaveProject(project, r, false); err != nil {
		t.Fatal(err)
	}
	saved := s.Snapshot().Settings
	saved.Options = r.Options
	saved.Presets = []Preset{{Name: "Beta 5", Options: r.Options}}
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(dir, "settings.json")
	if err = os.WriteFile(settings, raw, 0600); err != nil {
		t.Fatal(err)
	}
	fresh := New(context.Background(), settings)
	defer fresh.Shutdown()
	loaded := fresh.Snapshot().Settings
	if loaded.Options.HueForge.MeshCore != "compact-blends" || len(loaded.Presets) != 1 || loaded.Presets[0].Options.HueForge.MeshCore != "compact-blends" {
		t.Fatal("saved default or preset did not upgrade")
	}
	profile := filepath.Join(dir, "beta5.colorninja-profile.json")
	if err = s.SaveProfile(profile, "Beta 5", r, false); err != nil {
		t.Fatal(err)
	}
	loadedProfile, err := LoadProfile(profile)
	if err != nil || loadedProfile.Options.HueForge.MeshCore != "compact-blends" {
		t.Fatal("saved profile did not upgrade", err)
	}
	snap, err := fresh.OpenProject(project)
	if err != nil || snap.Preview == nil {
		t.Fatal("could not restore old project", err)
	}
	got := snap.Preview.Result
	if !bytes.Equal(got.Image.Pix, p.Result.Image.Pix) || !reflect.DeepEqual(got.LayerMap, p.Result.LayerMap) || !reflect.DeepEqual(got.Stack.Runs, p.Result.Stack.Runs) {
		t.Fatal("Mesh Core migration changed saved pixels, print heights, or physical spools")
	}
	if got.StackView == nil || got.StackView.MeshCore != "compact-blends" || got.StackView.Optimization == nil || got.StackView.Optimization.MaxTD <= .1 {
		t.Fatal("cached flat Mesh Core display was not refreshed")
	}
	export := filepath.Join(dir, "upgraded.hfp")
	if err = fresh.Export("hfp", export, snap.Preview.ID, snap.Preview.Revision, false); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(export)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Disabled   []int `json:"disabled_match_layers"`
		ColorNinja struct {
			MeshCore     string                  `json:"meshCore"`
			Optimization engine.MeshOptimization `json:"meshOptimization"`
		} `json:"colorninja"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.ColorNinja.MeshCore != "compact-blends" || doc.ColorNinja.Optimization != *got.StackView.Optimization || len(doc.Disabled) >= doc.ColorNinja.Optimization.OriginalDisabledLayers {
		t.Fatal("reopened project exported old disables or disagreed with the inspector")
	}
}

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
			r.Options.HueForge.ReduceShowThrough = true
			r.Options.HueForge.OptimizeMaterial = true
			r.Options.HueForge.LayerPreference = "auto"
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
			prefs := s.settings.Preferences
			prefs.ExportProfile = true
			if _, err = s.SavePreferences(prefs); err != nil {
				t.Fatal(err)
			}
			// The same export checkbox must include the portable document.
			exportPath := filepath.Join(dir, "export.png")
			if err = s.Export("png", exportPath, preview.ID, preview.Revision, false); err != nil {
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
			companion := New(context.Background(), filepath.Join(dir, "companion", "settings.json"))
			defer companion.Shutdown()
			restored, err := companion.OpenProject(ProjectPath(exportPath))
			if err != nil {
				t.Fatal("export companion could not reopen without the original files", err)
			}
			if restored.Preview == nil || !bytes.Equal(restored.Preview.Result.Image.Pix, preview.Result.Image.Pix) || !bytes.Equal(companion.image.Pix, s.image.Pix) || !reflect.DeepEqual(restored.Preview.Result.Stack, preview.Result.Stack) || !reflect.DeepEqual(restored.Preview.Result.LayerMap, preview.Result.LayerMap) || restored.Settings.Options != r.Options || !reflect.DeepEqual(restored.Settings.Filter, r.Filter) || !reflect.DeepEqual(restored.Source.Metadata, s.source.Metadata) {
				t.Fatal("export companion did not preserve the full document")
			}
			regenerated, err := companion.Process(Request{ID: 10, Revision: restored.Source.Revision, Options: restored.Settings.Options, LibraryPath: restored.Settings.LibraryPath, Filter: restored.Settings.Filter})
			if err != nil || regenerated.Result.SHA256 != preview.Result.SHA256 {
				t.Fatal("export companion could not reproduce the saved result", err)
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

func TestEveryExportCanWriteProjectAndProfileAndProtectExistingFiles(t *testing.T) {
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
			project := path
			if kind == "project" {
				if _, err := os.Stat(ProjectPath(path)); !os.IsNotExist(err) {
					t.Fatal("project export wrote a redundant project companion")
				}
			} else {
				project = ProjectPath(path)
			}
			fresh := New(context.Background(), filepath.Join(t.TempDir(), "settings.json"))
			defer fresh.Shutdown()
			snap, err := fresh.OpenProject(project)
			if err != nil || snap.Preview == nil || snap.Preview.Result.SHA256 != p.Result.SHA256 || snap.Settings.Options != r.Options {
				t.Fatal("export did not include a reopenable project with the exact result", err)
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
	if _, err = os.Stat(ProjectPath(path)); !os.IsNotExist(err) {
		t.Fatal("unchecked option wrote project")
	}
}

func TestProjectCompanionNeedsSeparateOverwriteApproval(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	prefs := s.settings.Preferences
	prefs.ExportProfile = true
	if _, err = s.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "image.png")
		project := ProjectPath(path)
		if directory {
			err = os.Mkdir(project, 0700)
		} else {
			err = os.WriteFile(project, []byte("keep this project"), 0600)
		}
		if err != nil {
			t.Fatal(err)
		}
		// Approval to overwrite a profile must never approve replacing a project.
		if err = s.Export("png", path, p.ID, p.Revision, false, true, directory); err == nil {
			t.Fatal("existing project companion was not protected")
		}
		for _, unwritten := range []string{path, ProfilePath(path)} {
			if _, err = os.Stat(unwritten); !os.IsNotExist(err) {
				t.Fatal("export wrote files before checking its project companion")
			}
		}
		if !directory {
			raw, err := os.ReadFile(project)
			if err != nil || string(raw) != "keep this project" {
				t.Fatal("project companion was changed without approval", err)
			}
			if err = s.Export("png", path, p.ID, p.Revision, false, false, true); err != nil {
				t.Fatal("approved project replacement failed", err)
			}
			fresh := New(context.Background(), filepath.Join(t.TempDir(), "settings.json"))
			defer fresh.Shutdown()
			if _, err = fresh.OpenProject(project); err != nil {
				t.Fatal("approved companion is not a valid project", err)
			}
		}
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

func TestOpenProjectIdentifiesExportedJSONWithoutChangingDocument(t *testing.T) {
	for _, mode := range []string{"standard", "guided", "stack"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			r := req(s, 1)
			r.Options.Mode = mode
			r.Options.HueForge.MaxDepth = .72
			r.LibraryPath = filepath.Join("..", "engine", "testdata", "library.json")
			p, err := s.Process(r)
			if err != nil {
				t.Fatal(err)
			}
			prefs := s.settings.Preferences
			prefs.ExportProfile = true
			if _, err = s.SavePreferences(prefs); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "export-palette.json")
			if err = s.Export("palette", path, p.ID, p.Revision, false); err != nil {
				t.Fatal(err)
			}
			before := s.Snapshot()
			for file, want := range map[string]string{path: "palette report", ProfilePath(path): "Load profile"} {
				if _, err = s.OpenProject(file); err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("opening %s should identify its type and recovery action, got %v", filepath.Base(file), err)
				}
				if after := s.Snapshot(); !reflect.DeepEqual(after, before) {
					t.Fatal("opening a non-project changed the current document")
				}
			}
			if profile, err := LoadProfile(ProfilePath(path)); err != nil || profile.Options != r.Options {
				t.Fatal("exported profile no longer loads through the profile workflow", err)
			}
		})
	}
}

func TestLegacyProjectRejectsMissingInputsBeforeLoading(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	if _, err := s.Process(r); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot()
	for _, field := range []string{"source image", "filament library"} {
		d := Document{1, s.source.Path, false, r.Options, "", r.Filter}
		if field == "source image" {
			d.Source = ""
		} else {
			d.Options.Mode = "stack"
		}
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "legacy.json")
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = s.OpenProject(path); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("missing %s should be explained, got %v", field, err)
		}
		if after := s.Snapshot(); !reflect.DeepEqual(after, before) {
			t.Fatal("invalid legacy project changed the current document")
		}
	}
}
