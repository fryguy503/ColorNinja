package main

import (
	"colorninja/internal/engine"
	"colorninja/internal/studio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIProjectAndSettingsProfilesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join("..", "..", "internal", "engine", "testdata")
	output, project, report := filepath.Join(dir, "first.png"), filepath.Join(dir, "saved.colorninja"), filepath.Join(dir, "report.json")
	args := []string{filepath.Join(base, "gradient.png"), "-o", output, "--colorninja-project", project, "--export-profile", "--palette-json", report, "--colors", "3", "--total-colors", "--color-priority", "vivid", "--quiet"}
	if err := argsRun(t, args...); err != nil {
		t.Fatal(err)
	}
	p, err := studio.LoadProfile(studio.ProfilePath(output))
	if err != nil || p.Options.ColorPriority != "vivid" || p.Options.Colors != 3 {
		t.Fatal("profile lost settings", err)
	}
	for _, path := range []string{project, report} {
		if _, err := studio.LoadProfile(studio.ProfilePath(path)); err != nil {
			t.Fatal(err)
		}
	}
	s := studio.New(context.Background(), filepath.Join(dir, "settings.json"))
	defer s.Shutdown()
	snap, err := s.OpenProject(project)
	if err != nil || snap.Preview == nil {
		t.Fatal("CLI project failed to open", err)
	}
	second := filepath.Join(dir, "second.png")
	if err = argsRun(t, filepath.Join(base, "gradient.png"), "-o", second, "--settings-profile", studio.ProfilePath(output), "--quiet"); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(output)
	b, _ := os.ReadFile(second)
	if string(a) != string(b) {
		t.Fatal("profile did not reproduce exported image")
	}
	blocked := filepath.Join(dir, "blocked.png")
	os.WriteFile(studio.ProfilePath(blocked), []byte("keep"), 0600)
	if err = argsRun(t, filepath.Join(base, "gradient.png"), "-o", blocked, "--export-profile", "--quiet"); err == nil {
		t.Fatal("profile collision accepted")
	}
	if _, err = os.Stat(blocked); !os.IsNotExist(err) {
		t.Fatal("image exported before profile collision check")
	}
}

func argsRun(t *testing.T, args ...string) error {
	t.Helper()
	old := os.Args
	os.Args = append([]string{"colorninja-cli"}, args...)
	defer func() { os.Args = old }()
	return run()
}
func TestCLIStandardExportAndNoClobber(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join("..", "..", "internal", "engine", "testdata", "gradient.png")
	out, report := filepath.Join(dir, "image.png"), filepath.Join(dir, "palette.json")
	args := []string{src, "-o", out, "--colors", "4", "--palette-json", report, "--quiet"}
	if e := argsRun(t, args...); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(report)
	if e != nil {
		t.Fatal(e)
	}
	var r engine.Report
	if e = json.Unmarshal(raw, &r); e != nil || r.Result.SourceSize != [2]int{96, 64} {
		t.Fatal(e, r)
	}
	if e = argsRun(t, args...); e == nil || !strings.Contains(e.Error(), "already exists") {
		t.Fatal(e)
	}
}
func TestCLIStackLayerExport(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join("..", "..", "internal", "engine", "testdata")
	out, layers := filepath.Join(dir, "stack.png"), filepath.Join(dir, "layers.png")
	if e := argsRun(t, filepath.Join(base, "gradient.png"), "-o", out, "--hueforge-library", filepath.Join(base, "library.json"), "--hueforge-stack", "--hueforge-max-depth", "0.8", "--colors", "2", "--hueforge-height-map", layers, "--quiet"); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(layers); e != nil {
		t.Fatal(e)
	}
}
func TestCLIRejectsInvalidModeAndInputOverwrite(t *testing.T) {
	src := filepath.Join("..", "..", "internal", "engine", "testdata", "gradient.png")
	if e := argsRun(t, src, "-o", src, "--force"); e == nil {
		t.Fatal("input overwrite accepted")
	}
	if e := argsRun(t, src, "--hueforge-height-map", "layers.png"); e == nil {
		t.Fatal("accepted height map without stack mode")
	}
}

func TestCLIPreservationFlags(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join("..", "..", "internal", "engine", "testdata")
	for _, enabled := range []bool{true, false} {
		output := filepath.Join(dir, "black.png")
		report := filepath.Join(dir, "black.json")
		args := []string{filepath.Join(base, "gradient.png"), "-o", output, "--palette-json", report, "--force", "--quiet"}
		if !enabled {
			args = append(args, "--true-black=false", "--preserve-details=false")
		}
		if e := argsRun(t, args...); e != nil {
			t.Fatal(e)
		}
		raw, e := os.ReadFile(report)
		if e != nil {
			t.Fatal(e)
		}
		var saved engine.Report
		if e = json.Unmarshal(raw, &saved); e != nil {
			t.Fatal(e)
		}
		if saved.Options.TrueBlack != enabled || saved.Options.PreserveDetails != enabled {
			t.Fatal("CLI did not preserve processing preferences")
		}
	}
}
