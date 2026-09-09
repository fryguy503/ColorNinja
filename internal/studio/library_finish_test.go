package studio

import (
	"colorninja/internal/engine"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFinishFilterPersistsAndConstrainsBothFilamentModes(t *testing.T) {
	for _, mode := range []string{"guided", "stack"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			libraryPath := filepath.Join(t.TempDir(), "library.json")
			raw := []byte(`{"Filaments":[
				{"Name":"Silk Red","Color":"#FF0000","Type":"PLA","Transmissivity":2,"Owned":true},
				{"Name":"Red","Color":"#FF0000","Type":"PLA","Transmissivity":2,"Owned":true},
				{"Name":"White","Color":"#FFFFFF","Type":"PLA MATTE","Transmissivity":5,"Owned":true},
				{"Name":"Black","Color":"#000000","Type":"PLA","Transmissivity":0.3,"Owned":true}
			]}`)
			if err := os.WriteFile(libraryPath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			for _, enabled := range []bool{true, false} {
				filter := engine.LibraryFilter{AvoidSilkMetallic: enabled}
				lib, err := s.SetLibrary(libraryPath, filter)
				if err != nil {
					t.Fatal(err)
				}
				allowed := map[int]bool{}
				for _, f := range lib.Filaments {
					allowed[f.SourceIndex] = true
				}
				if allowed[0] == enabled || allowed[1] != enabled {
					t.Fatal("finish filter did not replace the silk duplicate with plain PLA", allowed)
				}
				fresh := New(context.Background(), s.configPath)
				if fresh.Snapshot().Settings.Filter.AvoidSilkMetallic != enabled {
					t.Fatal("finish preference was not saved by SetLibrary")
				}
				fresh.Shutdown()
				r := req(s, 1)
				r.Filter, r.LibraryPath = filter, libraryPath
				r.Options.Mode = mode
				r.Options.Colors = 2
				r.Options.AnalysisMaxPixels = 64
				r.Options.HueForge.AnalysisColors = 4
				r.Options.HueForge.BeamWidth = 4
				r.Options.HueForge.MaxRuns = 4
				r.Options.HueForge.MaxDepth = .72
				preview, err := s.Process(r)
				if err != nil {
					t.Fatal(err)
				}
				selected := []engine.Filament{}
				if mode == "guided" {
					selected = preview.Result.Guidance.Selected
				} else {
					for _, run := range preview.Result.Stack.Runs {
						selected = append(selected, run.Filament)
					}
				}
				if len(selected) == 0 {
					t.Fatal("no selected filaments")
				}
				for _, f := range selected {
					if !allowed[f.SourceIndex] {
						t.Fatalf("selected an excluded filament: %+v", f)
					}
				}
				project := filepath.Join(t.TempDir(), "finish.colorninja.json")
				if err := s.SaveProject(project, r, false); err != nil {
					t.Fatal(err)
				}
				reopened, err := s.OpenProject(project)
				if err != nil || reopened.Settings.Filter.AvoidSilkMetallic != enabled {
					t.Fatal("finish filter lost in project round-trip", err)
				}
			}
			unchanged, err := os.ReadFile(libraryPath)
			if err != nil || string(unchanged) != string(raw) {
				t.Fatal("library file was modified", err)
			}
		})
	}
}
