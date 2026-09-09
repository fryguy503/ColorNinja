package engine

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestCompactMeshRetainsMatchesAndPhysicalCore(t *testing.T) {
	for _, light := range []string{"hueforge-default", "neutral-white", "warm-white"} {
		r := hfpFixture(t)
		r.Stack.Options.LightPreset = light
		r.Stack.Options.MeshCore = "legacy-flat"
		before, err := hueForgeProject(context.Background(), r, "test.png", ImageMetadata{})
		if err != nil {
			t.Fatal(err)
		}
		r.Stack.Options.MeshCore = DefaultOptions().HueForge.MeshCore
		after, err := hueForgeProject(context.Background(), r, "test.png", ImageMetadata{})
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"filament_set", "slider_values", "image_binary"} {
			if !reflect.DeepEqual(before[key], after[key]) {
				t.Fatal("mesh optimization changed physical output", key)
			}
		}
		used := map[int]RGB{}
		for _, p := range r.Palette {
			used[p.StackLayer] = p.RGB
		}
		m := compactMesh(r, used)
		if len(m.disabled) >= len(before["disabled_match_layers"].([]int)) {
			t.Fatal("did not remove redundant masks")
		}
		if m.info.MaxTD <= .1 {
			t.Fatal("did not introduce useful virtual TDs")
		}
		disabled := map[int]bool{}
		for _, l := range m.disabled {
			disabled[l] = true
		}
		for target, rgb := range used {
			winner, best := -1, math.Inf(1)
			for l := r.Stack.Options.BaseLayers(); l < len(m.colors); l++ {
				if disabled[l] {
					continue
				}
				d := distance(toOKLab(rgb), meshLab(m.colors[l]))
				if m.method == 0 {
					if meshRGBDistance(m.colors[l], rgb) <= 1e-10 {
						winner = l
						break
					}
				} else if d < best {
					winner, best = l, d
				}
			}
			if winner != target {
				t.Fatalf("%s moved %s from %d to %d", light, rgb.Hex(), target, winner)
			}
		}
	}
}

func TestSavedMeshCoreDefaultsUpgradeWithoutChangingFilamentChoice(t *testing.T) {
	for _, tc := range []struct{ json, want string }{
		{`{}`, "compact-blends"},
		{`{"hueforge":{}}`, "compact-blends"},
		{`{"hueforge":{"meshCore":""}}`, "compact-blends"},
		{`{"hueforge":{"meshCore":"planned-colors"}}`, "compact-blends"},
		{`{"hueforge":{"meshCore":"filament-blends"}}`, "filament-blends"},
		{`{"hueforge":{"meshCore":"legacy-flat"}}`, "legacy-flat"},
	} {
		for _, initial := range []Options{{}, DefaultOptions()} {
			o := initial
			if err := json.Unmarshal([]byte(tc.json), &o); err != nil {
				t.Fatal(err)
			}
			if o.HueForge.MeshCore != tc.want {
				t.Fatalf("saved options %s resolved to %q, want %q", tc.json, o.HueForge.MeshCore, tc.want)
			}
		}
	}
}
func TestCompactMeshFitsHigherTDsAcrossGaps(t *testing.T) {
	r := hfpFixture(t)
	r.Stack.Options.MeshCore = "compact-blends"
	used := map[int]RGB{5: {255, 255, 255}, 11: {255, 100, 100}}
	m := compactMesh(r, used)
	if m.method != 0 || len(m.disabled) != 0 || len(m.filaments) != 2 {
		t.Fatal("expected an unmasked fitted gradient", m.info)
	}
	if m.colors[6] == m.colors[11] || meshRGBDistance(m.colors[11], used[11]) > 1e-10 {
		t.Fatal("no intermediate blends or missed endpoint")
	}
}
