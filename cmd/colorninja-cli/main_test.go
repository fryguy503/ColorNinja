package main

import (
	"colorninja/internal/engine"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
