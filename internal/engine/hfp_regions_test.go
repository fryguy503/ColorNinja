package engine

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func readSpotMask(t *testing.T, f hfpSpotFix) []bool {
	t.Helper()
	n := f.Footprint.Width * f.Footprint.Height
	mask := make([]bool, n)
	data, at, selected := f.Footprint.RLE, 0, false
	for len(data) > 0 {
		count, used := binary.Uvarint(data)
		if used <= 0 || count > uint64(n-at) {
			t.Fatal("invalid native RLE")
		}
		for end := at + int(count); at < end; at++ {
			mask[at] = selected
		}
		selected, data = !selected, data[used:]
	}
	if at != n {
		t.Fatal("truncated native footprint")
	}
	return mask
}

func TestRegionHFPInterchange(t *testing.T) {
	for _, tc := range []struct {
		name         string
		layers       []uint16
		groups       []RegionGroup
		fixes, baked int
	}{
		{"shift-islands", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "shift", Value: 1, Mask: []PixelSpan{{0, 6}}}}, 1, 0},
		{"assign-splits", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "assign", Value: 8, Mask: []PixelSpan{{0, 6}}}}, 1, 0},
		{"clamp-splits", []uint16{2, 4, 8, 2, 4, 8}, []RegionGroup{{Operation: "shift", Value: 2, Mask: []PixelSpan{{0, 6}}}}, 2, 0},
		{"cut", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "cut", Mask: []PixelSpan{{1, 1}, {4, 1}}}}, 1, 0},
		{"overlap", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "shift", Value: 1, Mask: []PixelSpan{{0, 6}}}, {Operation: "assign", Value: 8, Mask: []PixelSpan{{1, 1}, {4, 1}}}}, 2, 0},
		{"locked", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "shift", Value: 1, Locked: true, Mask: []PixelSpan{{0, 6}}}, {Operation: "assign", Value: 8, Mask: []PixelSpan{{1, 1}, {4, 1}}}}, 1, 0},
		{"restore", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "cut", Mask: []PixelSpan{{1, 1}, {4, 1}}}, {Operation: "restore", Mask: []PixelSpan{{1, 1}, {4, 1}}}}, 1, 0},
		{"partial-pixel", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "assign", Value: 8, Mask: []PixelSpan{{1, 1}}}}, 0, 1},
		{"hybrid", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "assign", Value: 8, Mask: []PixelSpan{{1, 1}}}, {Operation: "shift", Value: 1, Mask: []PixelSpan{{2, 1}, {5, 1}}}}, 1, 1},
		{"bake-merge-cascade", []uint16{2, 4, 6, 2, 4, 6}, []RegionGroup{{Operation: "assign", Value: 6, Mask: []PixelSpan{{1, 1}}}, {Operation: "shift", Value: 1, Mask: []PixelSpan{{2, 1}, {5, 1}}}}, 0, 2},
		{"transparent-edge", []uint16{0, 2, 6, 0, 2, 6}, []RegionGroup{{Operation: "shift", Value: 1, Mask: []PixelSpan{{1, 1}, {4, 1}}}}, 0, 1},
		{"asymmetric-orientation", []uint16{2, 2, 2, 4, 6, 8}, []RegionGroup{{Operation: "shift", Value: 1, Mask: []PixelSpan{{0, 3}}}}, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, opts := regionFixture(3, 2, tc.layers)
			d := newRegionDoc(base, tc.groups...)
			for i := range d.Groups {
				d.Groups[i].Name = tc.name
			}
			result, err := ApplyRegionDocument(context.Background(), base, base.Image, opts, d)
			if err != nil {
				t.Fatal(err)
			}
			input, fixes, info, err := regionHFPInput(context.Background(), result)
			if err != nil {
				t.Fatal(err)
			}
			if len(fixes) != tc.fixes || len(info.BakedGroups) != tc.baked {
				t.Fatalf("fixes=%d baked=%v", len(fixes), info.BakedGroups)
			}
			got := slices.Clone(input.LayerMap)
			occupied := make([]bool, len(got))
			for _, fix := range fixes {
				if len(fix.Regions) == 0 || !fix.Enabled {
					t.Fatal("native loader rejects seedless group")
				}
				mask := readSpotMask(t, fix)
				for j, selected := range mask {
					if !selected {
						continue
					}
					i := (1-j/3)*3 + j%3
					if occupied[i] {
						t.Fatal("overlapping native ownership")
					}
					occupied[i] = true
					v := int(got[i])
					if fix.FlattenMode != 0 {
						v = int(math.Round(fix.TargetLum*65535/float64(65535/7))) + opts.HueForge.BaseLayers()
					}
					v += fix.Delta
					if v < opts.HueForge.BaseLayers() {
						v = 0
					}
					got[i] = uint16(v)
				}
				for _, seed := range fix.Regions {
					j := int(seed.Y*2)*3 + int(seed.X*3)
					if !mask[j] {
						t.Fatal("seed outside bottom-up footprint")
					}
				}
			}
			if !slices.Equal(got, result.LayerMap) {
				t.Fatalf("double-applied or lost edits: %v != %v", got, result.LayerMap)
			}
			if !slices.Equal(base.LayerMap, tc.layers) {
				t.Fatal("export mutated baseline")
			}
			doc, err := hueForgeProject(context.Background(), result, tc.name+".png", ImageMetadata{})
			if err != nil {
				t.Fatal(err)
			}
			if doc["spotfix_version"] != 2 {
				t.Fatal("missing native SpotFix serialization")
			}
			// Optional fixtures for external HFP compatibility checks.
			if dest := os.Getenv("COLORNINJA_SPOTFIX_FIXTURES"); dest != "" {
				if err := os.MkdirAll(dest, 0755); err != nil {
					t.Fatal(err)
				}
				data, err := json.MarshalIndent(map[string]any{"hfp": doc, "inputLayers": input.LayerMap, "expectedLayers": result.LayerMap}, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dest, tc.name+".json"), data, 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRegionHFPCancelAndDocumentIsolation(t *testing.T) {
	base, opts := regionFixture(3, 2, []uint16{2, 4, 6, 2, 4, 6})
	d := newRegionDoc(base, RegionGroup{Operation: "shift", Value: 1, Mask: []PixelSpan{{0, 6}}})
	r, err := ApplyRegionDocument(context.Background(), base, base.Image, opts, d)
	if err != nil {
		t.Fatal(err)
	}
	d.Groups[0].Mask[0].Length = 1
	_, fixes, _, err := regionHFPInput(context.Background(), r)
	if err != nil || len(fixes) != 1 {
		t.Fatal("export replay inputs aliased caller", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hueForgeProject(ctx, r, "canceled.png", ImageMetadata{}); err == nil {
		t.Fatal("canceled export succeeded")
	}
}

func TestSpotFixRLELongRuns(t *testing.T) {
	for _, spans := range [][]PixelSpan{{{0, 260}}, {{129, 260}}, {{0, 1}, {1, 128}, {389, 123}}} {
		fix := hfpSpotFix{Footprint: hfpSpotFootprint{Width: 256, Height: 2, RLE: spotFixRLE(spans, 512)}}
		want, err := SpanMask(spans, 512)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(readSpotMask(t, fix), want) {
			t.Fatal("multibyte footprint differs")
		}
	}
}

func TestRegionHFPBorderAndOpticalReframes(t *testing.T) {
	for _, optical := range []string{FrontlitModel, BacklitModel} {
		base, opts := regionFixture(3, 2, []uint16{2, 4, 6, 2, 4, 6})
		opts.HueForge.OpticalModel = optical
		base.Stack.Options = opts.HueForge
		d := newRegionDoc(base, RegionGroup{Operation: "shift", Value: 1, Mask: []PixelSpan{{0, 6}}})
		r, err := ApplyRegionDocument(context.Background(), base, base.Image, opts, d)
		if err != nil {
			t.Fatal(err)
		}
		opts.HueForge.Border = BorderOptions{Enabled: true, WidthMM: 4, HeightMM: 4, Placement: "external"}
		r = ReframeResult(r, opts)
		doc, err := hueForgeProject(context.Background(), r, "framed.png", ImageMetadata{})
		if err != nil {
			t.Fatal(err)
		}
		fixes := doc["spot_fixes"].([]hfpSpotFix)
		if len(fixes) != 1 || fixes[0].Delta != 1 || doc["border_height"] != float64(4) {
			t.Fatal("reframe dropped groups or border")
		}
		if doc["colorninja"].(map[string]any)["rgbaSHA256"] != r.SHA256 {
			t.Fatal("final-result identity changed")
		}
	}
}

func TestRegionHFPDisabledEditsRemainInDocument(t *testing.T) {
	base, opts := regionFixture(3, 2, []uint16{2, 4, 6, 2, 4, 6})
	d := newRegionDoc(base, RegionGroup{Operation: "shift", Value: 1, Mask: []PixelSpan{{0, 6}}})
	d.Groups[0].Enabled = false
	r, err := ApplyRegionDocument(context.Background(), base, base.Image, opts, d)
	if err != nil {
		t.Fatal(err)
	}
	input, fixes, info, err := regionHFPInput(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 0 || !slices.Equal(input.LayerMap, base.LayerMap) || info.Document.Groups[0].Enabled {
		t.Fatal("disabled edit applied or lost")
	}
}
