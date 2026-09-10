package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPausedChannelProjectDropsCachedPreviewWithoutChangingFile(t *testing.T) {
	s := fixture(t)
	o := engine.DefaultOptions()
	o.Mode, o.Colors, o.HeightMap.Mode = "stack", 4, "combo"
	o.HueForge.MaxDepth = 1.44
	raw, err := os.ReadFile(filepath.Join("..", "engine", "testdata", "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	lib, err := engine.ParseLibrary(raw, engine.LibraryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Process(context.Background(), s.image, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "old-channel.colorninja")
	if err = SaveResultProject(context.Background(), path, "old.png", &engine.LoadedImage{Image: s.image}, result, o, engine.LibraryFilter{}, raw, false); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	snap, err := s.OpenProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Preview != nil || snap.Settings.Options.HeightMap.Mode != "" || snap.Settings.Options.Mode != "stack" || !strings.Contains(snap.Warning, "temporarily disabled") {
		t.Fatal("unsafe channel project migration", snap.Warning)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("opening changed the saved project")
	}
	r := req(s, 10)
	r.Options = snap.Settings.Options
	r.LibraryPath = snap.Settings.LibraryPath
	preview, err := s.Process(r)
	if err != nil || preview.Result.HeightMap != nil || preview.Result.Stack.ColorOrder == nil {
		t.Fatal("migrated project cannot render Color Match", err)
	}
}

func TestPausedHeightComparisonsUseColorMatchAndRetainBacklit(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode = "stack"
	r.Options.Colors = 4
	r.Options.HeightMap.Mode = "combo"
	r.Options.HeightMap.Mixing = 37
	r.Options.HeightMap.Gamma = 1.3
	r.Options.HueForge.OpticalModel = engine.BacklitModel
	r.Options.HueForge.TDScale = 1.2
	r.Options.HueForge.MaxDepth = 1.12
	r.Options.HueForge.MaxRuns = 8
	r.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
	original, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := s.ComparePlans(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) < 4 {
		t.Fatal("missing fixed-height comparisons")
	}
	for _, c := range candidates {
		if c.Options.HeightMap.Mode != "" || c.Options.HeightMap.Mixing != 37 || c.Options.HeightMap.Gamma != 1.3 || c.Options.HueForge.OpticalModel != engine.BacklitModel {
			t.Fatal("comparison changed interpretation", c.Name)
		}
	}
	if s.Snapshot().Preview.Result.SHA256 != original.Result.SHA256 {
		t.Fatal("comparison replaced active preview")
	}
}

func TestAllHeightWorkflowsPortable(t *testing.T) {
	for _, mode := range []string{"standard", "combo", "max-channel", "scaled-max-channel", "color-aware", "color-pop", "color-match"} {
		for _, model := range []string{engine.FrontlitModel, engine.BacklitModel} {
			t.Run(mode+"/"+model, func(t *testing.T) {
				s := fixture(t)
				r := req(s, 1)
				r.Options.Mode = "stack"
				r.Options.Colors = 4
				r.Options.HueForge.MaxDepth = 1.44
				r.Options.HueForge.MaxRuns = 8
				r.Options.HueForge.OpticalModel = model
				r.Options.HueForge.TDScale = 1.2
				r.Options.HueForge.MeshMode = "combo"
				if mode == "color-match" {
					r.Options.HueForge.ColorOrder = "black,green,red,white"
					r.Options.HueForge.ColorOrderWeight = 75
				}
				if mode == "color-pop" {
					r.Options.ColorPop.Enabled = true
				} else {
					r.Options.HeightMap.Mode = mode
				}
				r.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
				preview, err := s.Process(r)
				if err != nil {
					t.Fatal(err)
				}
				if preview.Options.HeightMap.Mode != "" || preview.Result.HeightMap != nil {
					t.Fatal("paused channel workflow remained active")
				}
				r.Options = preview.Options
				dir := t.TempDir()
				project := filepath.Join(dir, "portable.colorninja")
				if err = s.SaveProject(project, r, false); err != nil {
					t.Fatal(err)
				}
				profile := filepath.Join(dir, "profile.json")
				if err = s.SaveProfile(profile, "height workflow", r, false); err != nil {
					t.Fatal(err)
				}
				loaded, err := LoadProfile(profile)
				if err != nil || loaded.Options != r.Options {
					t.Fatal("profile changed settings", err)
				}
				fresh := New(context.Background(), filepath.Join(dir, "fresh.json"))
				defer fresh.Shutdown()
				snap, err := fresh.OpenProject(project)
				if err != nil || snap.Preview == nil {
					t.Fatal("reopen", err)
				}
				got := snap.Preview.Result
				if !bytes.Equal(got.Image.Pix, preview.Result.Image.Pix) || !reflect.DeepEqual(got.LayerMap, preview.Result.LayerMap) || !reflect.DeepEqual(got.HeightMap, preview.Result.HeightMap) {
					t.Fatal("portable result drift")
				}
				before := filepath.Join(dir, "before.hfp")
				after := filepath.Join(dir, "after.hfp")
				if err = s.Export("hfp", before, preview.ID, preview.Revision, false); err != nil {
					t.Fatal(err)
				}
				if err = fresh.Export("hfp", after, snap.Preview.ID, snap.Preview.Revision, false); err != nil {
					t.Fatal(err)
				}
				a, _ := os.ReadFile(before)
				b, _ := os.ReadFile(after)
				if !bytes.Equal(a, b) {
					t.Fatal("reopened HFP differs")
				}
				request := Request{ID: 2, Revision: snap.Source.Revision, Options: snap.Settings.Options, Filter: snap.Settings.Filter, LibraryPath: snap.Settings.LibraryPath}
				rendered, err := fresh.Process(request)
				if err != nil {
					t.Fatal(err)
				}
				if rendered.Result.SHA256 != preview.Result.SHA256 || !reflect.DeepEqual(rendered.Result.LayerMap, preview.Result.LayerMap) {
					t.Fatal("embedded rerender differs")
				}
			})
		}
	}
}
