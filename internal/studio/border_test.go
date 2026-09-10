package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBorderProjectProfileAndPresetRoundTrip(t *testing.T) {
	s := fixture(t)
	r := req(s, 1)
	r.Options.Mode = "stack"
	r.Options.Colors = 2
	r.Options.HueForge.MaxDepth = .8
	r.Options.HueForge.Border = engine.BorderOptions{Enabled: true, Placement: "internal", WidthMM: 2, HeightMM: 4}
	r.LibraryPath, _ = filepath.Abs(filepath.Join("..", "engine", "testdata", "library.json"))
	p, err := s.Process(r)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	project := filepath.Join(dir, "frame.colorninja")
	profile := filepath.Join(dir, "frame.colorninja-profile.json")
	if err = s.SaveProject(project, r, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProfile(profile, "Frame", r, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePreset("Frame", r.Options); err != nil {
		t.Fatal(err)
	}
	settings := New(context.Background(), s.configPath)
	defer settings.Shutdown()
	if settings.Snapshot().Settings.Options.HueForge.Border != r.Options.HueForge.Border || settings.Snapshot().Settings.Presets[0].Options.HueForge.Border != r.Options.HueForge.Border {
		t.Fatal("preferences/preset lost border")
	}
	loaded, err := LoadProfile(profile)
	if err != nil || loaded.Options.HueForge.Border != r.Options.HueForge.Border {
		t.Fatal("profile lost border", err)
	}
	before := filepath.Join(dir, "before.hfp")
	if err = s.Export("hfp", before, p.ID, p.Revision, false); err != nil {
		t.Fatal(err)
	}
	fresh := New(context.Background(), filepath.Join(dir, "fresh.json"))
	defer fresh.Shutdown()
	snap, err := fresh.OpenProject(project)
	if err != nil || snap.Preview == nil {
		t.Fatal("reopen", err)
	}
	if snap.Settings.Options.HueForge.Border != r.Options.HueForge.Border || snap.Preview.Result.SurfaceView.Border == nil {
		t.Fatal("project lost border")
	}
	if !bytes.Equal(snap.Preview.Result.Image.Pix, p.Result.Image.Pix) {
		t.Fatal("reopen changed image")
	}
	after := filepath.Join(dir, "after.hfp")
	if err = fresh.Export("hfp", after, snap.Preview.ID, snap.Preview.Revision, false); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(before)
	b, _ := os.ReadFile(after)
	if !bytes.Equal(a, b) {
		t.Fatal("reopen changed HFP")
	}
}
