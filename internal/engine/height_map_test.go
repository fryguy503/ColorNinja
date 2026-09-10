package engine

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"os"
	"reflect"
	"testing"
)

func TestHueForgeBrightnessReference(t *testing.T) {
	raw, err := os.ReadFile("testdata/height-brightness-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Mode, Model string
			Mixing      float64
			RGB         RGB
			Brightness  float64
		}
	}
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Cases) != 656 {
		t.Fatal("incomplete reference fixture")
	}
	for _, c := range ref.Cases {
		h := DefaultHeightMapOptions()
		h.Mode = c.Mode
		h.StandardModel = c.Model
		h.Mixing = c.Mixing
		_, v := heightSample(c.RGB, h)
		if math.Abs(v-c.Brightness) > 1e-12 {
			t.Fatalf("%s/%s %v mixing %v: got %.17g, native %.17g", c.Mode, c.Model, c.RGB, c.Mixing, v, c.Brightness)
		}
	}
}

func heightOptions(mode string) Options {
	o := DefaultOptions()
	o.Mode, o.HeightMap.Mode = "stack", mode
	o.PreblurSigma, o.Colors = 0, 4
	o.HueForge.BeamWidth = 12
	o.HueForge.MaxRuns = 8
	return o
}

func TestBrightnessWorkflowsAndExports(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 32, 2))
	for x := 0; x < 32; x++ {
		v := uint8(x * 255 / 31)
		for y := 0; y < 2; y++ {
			im.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	im.SetNRGBA(15, 1, color.NRGBA{255, 0, 0, 0})
	lib := popLibrary()
	for _, mode := range []string{"standard", "combo", "max-channel", "scaled-max-channel"} {
		t.Run(mode, func(t *testing.T) {
			o := heightOptions(mode)
			r, err := Process(context.Background(), im, o, &lib, nil)
			if err != nil {
				t.Fatal(err)
			}
			if r.HeightMap == nil || len(r.HeightMap.Bands) != 1 || r.ColorPop != nil {
				t.Fatal("wrong workflow metadata")
			}
			for x := 1; x < 32; x++ {
				if r.LayerMap[x] < r.LayerMap[x-1] {
					t.Fatal("brightness inverted", x)
				}
			}
			if int(r.LayerMap[0]) != o.HueForge.BaseLayers() || int(r.LayerMap[31]) != o.HueForge.MaxLayers() {
				t.Fatal("lost full brightness range")
			}
			if r.LayerMap[47] != 0 || r.Image.NRGBAAt(15, 1).A != 0 {
				t.Fatal("lost transparency")
			}
			if len(r.Palette) > o.HueForge.MaxPerceivedColors {
				t.Fatal("surface budget exceeded")
			}
			d, err := hueForgeProject(context.Background(), r, "gray.png", ImageMetadata{})
			if err != nil {
				t.Fatal(err)
			}
			if d["luminance_method"] != 6 || d["colorninja"].(map[string]any)["heightMap"] == nil {
				t.Fatal("missing height-preserving export")
			}
			o.HeightMap.Invert = true
			reversed, err := Process(context.Background(), im, o, &lib, nil)
			if err != nil {
				t.Fatal(err)
			}
			if reversed.LayerMap[0] != r.LayerMap[31] || reversed.LayerMap[31] != r.LayerMap[0] {
				t.Fatal("invert did not reverse heights")
			}
		})
	}
}

func TestColorAwareOrdersAndBudgets(t *testing.T) {
	lib := popLibrary()
	src := popFixture()
	for _, order := range []string{"rgb", "rbg", "grb", "gbr", "brg", "bgr"} {
		t.Run(order, func(t *testing.T) {
			o := heightOptions("color-aware")
			o.HeightMap.ChannelOrder = order
			r, err := Process(context.Background(), src, o, &lib, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.HeightMap.Bands) != 3 {
				t.Fatal("missing channel", r.HeightMap)
			}
			bands := map[int]HeightBand{}
			for i, b := range r.HeightMap.Bands {
				if "rgb"[b.Channel] != order[i] {
					t.Fatal("wrong order")
				}
				if i > 0 && b.StartLayer-r.HeightMap.Bands[i-1].EndLayer != o.HeightMap.GapLayers+1 {
					t.Fatal("wrong band gap")
				}
				bands[b.Channel] = b
			}
			// At y=0 the nominal blue patch is actually green-dominant (35 > 30).
			for y := 1; y < 20; y++ {
				for x := 0; x < 30; x++ {
					p := src.NRGBAAt(x, y)
					if p.A == 0 {
						continue
					}
					b := bands[x/10]
					l := int(r.LayerMap[y*40+x])
					if l < b.StartLayer || l > b.EndLayer {
						t.Fatal("pixel escaped its channel band", x, y, l, b)
					}
				}
			}
			if len(r.Palette) > o.HueForge.MaxPerceivedColors {
				t.Fatal("shared budget exceeded")
			}
			if _, err := hueForgeProject(context.Background(), r, "rgb.png", ImageMetadata{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHeightSettingsCacheAndCompatibility(t *testing.T) {
	o := heightOptions("combo")
	lib := popLibrary()
	src := popFixture()
	p := &Processor{}
	a, err := p.Process(context.Background(), src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.HeightMap.Mixing = 0
	b, err := p.Process(context.Background(), src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(a.LayerMap, b.LayerMap) {
		t.Fatal("height settings did not invalidate cache")
	}
	fresh, err := Process(context.Background(), src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fresh.LayerMap, b.LayerMap) || fresh.SHA256 != b.SHA256 {
		t.Fatal("cached/fresh mismatch")
	}
	var old Options
	if err := json.Unmarshal([]byte(`{"mode":"standard","colors":8}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.HeightMap != DefaultHeightMapOptions() || old.fixedHeights() {
		t.Fatal("old settings activated height planning")
	}
	raw, _ := json.Marshal(o)
	var restored Options
	if err := json.Unmarshal(raw, &restored); err != nil || restored.HeightMap != o.HeightMap {
		t.Fatal("height settings did not round trip", err)
	}
	cancel, stop := context.WithCancel(context.Background())
	stop()
	if _, err := p.Process(cancel, src, o, &lib, nil); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestHeightValidationAndEmptyChannels(t *testing.T) {
	o := heightOptions("color-aware")
	for _, bad := range []string{"rgg", "xyz", "rg"} {
		o.HeightMap.ChannelOrder = bad
		if o.Validate() == nil {
			t.Fatal("invalid channel order", bad)
		}
	}
	o.HeightMap = DefaultHeightMapOptions()
	o.HeightMap.Mode = "color-aware"
	o.HeightMap.Ignore = [3]bool{true, true, true}
	if o.Validate() == nil {
		t.Fatal("accepted all channels ignored")
	}
	o.HeightMap.Ignore = [3]bool{false, true, true}
	lib := popLibrary()
	r, err := Process(context.Background(), popFixture(), o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.HeightMap.Bands) != 1 || r.HeightMap.Bands[0].Channel != 0 {
		t.Fatal("empty bands consumed height")
	}
}

func TestHeightAutoDepthAndSubimages(t *testing.T) {
	o := heightOptions("standard")
	o.HueForge.MaxDepth = 1.04
	o.HueForge.AutoDepth = true
	lib := popLibrary()
	src := popFixture().SubImage(image.Rect(2, 2, 35, 18)).(*image.NRGBA)
	r, err := Process(context.Background(), src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.SourceSize != [2]int{33, 16} || r.Stack.DepthSelection == nil || r.Stack.PlannedDepth > o.HueForge.MaxDepth {
		t.Fatal("invalid selected depth or dimensions")
	}
	if _, err := hueForgeProject(context.Background(), r, "cropped.png", ImageMetadata{}); err != nil {
		t.Fatal(err)
	}
}
