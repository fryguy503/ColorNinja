package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyMeshModesMigrateWithoutChangingSavedPixelsAndLayers(t *testing.T) {
	for _, mode := range []string{"color-pop", "combo", "color-aware"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			request := req(s, 1)
			request.Options.Mode, request.Options.Colors = "stack", 2
			request.Options.HueForge.MaxDepth = .8
			request.Options.HueForge.MeshMode = mode
			request.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
			p, err := s.Process(request)
			if err != nil {
				t.Fatal(err)
			}
			if p.Options.HueForge.MeshMode != "color-match" || !p.Result.StackView.HasMeshCore {
				t.Fatal("fresh desktop request retained a recomputed mesh")
			}
			// Create the legacy saved state, including its cached export view.
			s.resultRequest.Options = request.Options
			s.result = engine.ReframeResult(p.Result, request.Options)
			dir := t.TempDir()
			project := filepath.Join(dir, "old.colorninja")
			if err = s.SaveProject(project, request, false); err != nil {
				t.Fatal(err)
			}
			settings := s.Snapshot().Settings
			settings.Options = request.Options
			settings.Presets = []Preset{{Name: "Old", Options: request.Options}}
			raw, _ := json.Marshal(settings)
			config := filepath.Join(dir, "settings.json")
			if err = os.WriteFile(config, raw, 0600); err != nil {
				t.Fatal(err)
			}
			fresh := New(context.Background(), config)
			defer fresh.Shutdown()
			if fresh.settings.Options.HueForge.MeshMode != "color-match" || fresh.settings.Presets[0].Options.HueForge.MeshMode != "color-match" {
				t.Fatal("startup settings or preset retained stale mode")
			}
			profile := filepath.Join(dir, "old-profile.json")
			if err = SaveSettingsProfile(profile, "Old", request.Options, request.Filter, nil, false); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadProfile(profile)
			if err != nil || loaded.Options.HueForge.MeshMode != "color-match" {
				t.Fatal("profile retained stale mode", err)
			}
			snap, err := fresh.OpenProject(project)
			if err != nil || snap.Preview == nil {
				t.Fatal("cannot restore legacy preview", err)
			}
			got := snap.Preview.Result
			if snap.Settings.Options.HueForge.MeshMode != "color-match" || got.Stack.Options.MeshMode != "color-match" || !got.StackView.HasMeshCore {
				t.Fatal("project retained stale export mode")
			}
			if !bytes.Equal(got.Image.Pix, p.Result.Image.Pix) || !reflect.DeepEqual(got.LayerMap, p.Result.LayerMap) || !reflect.DeepEqual(got.Stack.Runs, p.Result.Stack.Runs) {
				t.Fatal("migration changed the planned print")
			}
			for _, studio := range []*Studio{s, fresh} {
				studio.settings.Preferences.ExportProfile = true
				preview := studio.Snapshot().Preview
				out := filepath.Join(t.TempDir(), "export.hfp")
				if err = studio.Export("hfp", out, preview.ID, preview.Revision, false); err != nil {
					t.Fatal(err)
				}
				raw, _ = os.ReadFile(out)
				var doc struct {
					Method int `json:"luminance_method"`
					Info   struct {
						Recomputed bool `json:"meshRecomputed"`
					} `json:"colorninja"`
				}
				if err = json.Unmarshal(raw, &doc); err != nil || doc.Method != 6 || doc.Info.Recomputed {
					t.Fatal("desktop export rebuilt heights", err)
				}
				companion, e := LoadProfile(ProfilePath(out))
				if e != nil || companion.Options.HueForge.MeshMode != "color-match" {
					t.Fatal("companion profile retained stale mesh mode", e)
				}
				reopened := New(context.Background(), filepath.Join(t.TempDir(), "settings.json"))
				saved, e := reopened.OpenProject(ProjectPath(out))
				reopened.Shutdown()
				if e != nil || saved.Preview == nil || saved.Preview.Result.Stack.Options.MeshMode != "color-match" || !reflect.DeepEqual(saved.Preview.Result.LayerMap, p.Result.LayerMap) {
					t.Fatal("companion project did not preserve the normalized plan", e)
				}
			}
			if s.result.Stack.Options.MeshMode != mode {
				t.Fatal("export mutated the cached old result")
			}
		})
	}
}
