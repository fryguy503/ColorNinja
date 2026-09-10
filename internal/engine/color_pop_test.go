package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"testing"
)

func popFixture() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			v := uint8(30 + y*10)
			c := color.NRGBA{v, v, v, 255}
			switch x / 10 {
			case 0:
				c = color.NRGBA{v, 10, 15, 255}
			case 1:
				c = color.NRGBA{12, v, 30, 255}
			case 2:
				c = color.NRGBA{10, 35, v, 255}
			}
			im.SetNRGBA(x, y, c)
		}
	}
	im.SetNRGBA(3, 3, color.NRGBA{255, 0, 0, 0})
	im.SetNRGBA(13, 3, color.NRGBA{0, 255, 0, 67})
	return im
}
func popOptions() Options {
	o := DefaultOptions()
	o.ColorPop.Enabled = true
	o.ColorPop.Selection = "selected"
	o.ColorPop.Colors = "#F01020"
	o.ColorPop.HueTolerance = 20
	o.Colors = 8
	o.TotalColors = true
	o.PreblurSigma = 0
	return o
}
func TestColorPopSelectionAndReduction(t *testing.T) {
	ctx := context.Background()
	src := popFixture()
	before := append([]byte(nil), src.Pix...)
	o := popOptions()
	r, err := Process(ctx, src, o, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.ColorPop == nil || len(r.ColorPop.SelectionPNG) == 0 || r.UniqueColors > 8 || r.UniqueColors < 3 {
		t.Fatal("missing separation or wrong color budget", r.UniqueColors)
	}
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			a, b := src.NRGBAAt(x, y), r.Image.NRGBAAt(x, y)
			if a.A != b.A {
				t.Fatal("alpha changed")
			}
			if a.A == 0 {
				continue
			}
			if x >= 10 && (b.R != b.G || b.G != b.B) {
				t.Fatal("unselected region tinted", x, y, b)
			}
			if x < 10 && b.R <= b.G {
				t.Fatal("accent swallowed", b)
			}
		}
	}
	if !bytes.Equal(before, src.Pix) {
		t.Fatal("source mutated")
	}
	if math.Abs(r.ColorPop.ColorFraction-.25) > .01 {
		t.Fatal("coverage includes invisible RGB", r.ColorPop.ColorFraction)
	}
	// A tight two-color budget must reserve a color for the small accent.
	o.Colors = 2
	r, err = Process(ctx, src, o, nil, nil)
	if err != nil || r.UniqueColors != 2 {
		t.Fatal("two-region budget", err)
	}
	o.Colors = 1
	if _, err = Process(ctx, src, o, nil, nil); err == nil {
		t.Fatal("one color silently erased a region")
	}
	// Empty selection is a valid, fully grayscale preparation state.
	o.Colors = 8
	o.ColorPop.Colors = ""
	r, err = Process(ctx, src, o, nil, nil)
	if err != nil || r.ColorPop.ColorFraction != 0 || r.ColorPop.Warning == "" {
		t.Fatal("empty selection", err)
	}
}
func TestColorPopHueWrapToleranceAndLegacy(t *testing.T) {
	ctx := context.Background()
	o := popOptions()
	o.ColorPop.Colors = "#FF0010"
	o.ColorPop.HueTolerance = 8
	im := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	for i, c := range []color.NRGBA{{255, 10, 0, 255}, {255, 0, 10, 255}, {122, 120, 118, 255}, {0, 240, 0, 255}} {
		im.SetNRGBA(i, 0, c)
	}
	_, mask, _, err := prepareColorPop(ctx, im, o, nil)
	if err != nil || !bytes.Equal(mask, []byte{1, 1, 0, 0}) {
		t.Fatal(mask, err)
	}
	o.ColorPop.Selection = "existing"
	_, mask, _, err = prepareColorPop(ctx, im, o, nil)
	if err != nil || !bytes.Equal(mask, []byte{1, 1, 0, 1}) {
		t.Fatal(mask, err)
	}
	o.ColorPop.Enabled = false
	a, err := Process(ctx, im, o, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.ColorPop = ColorPopOptions{}
	b, err := Process(ctx, im, o, nil, nil)
	if err != nil || a.SHA256 != b.SHA256 {
		t.Fatal("disabled treatment changed the reducer")
	}
	raw, _ := json.Marshal(o)
	var old map[string]json.RawMessage
	json.Unmarshal(raw, &old)
	delete(old, "colorPop")
	raw, _ = json.Marshal(old)
	o = popOptions()
	if err = json.Unmarshal(raw, &o); err != nil || o.ColorPop.Enabled || o.ColorPop.HueTolerance != 25 {
		t.Fatal("old project inherited Color Pop", err)
	}
	for _, bad := range []string{"#12345G", "#12345", "#1 2233", "#FF0000,red"} {
		o = popOptions()
		o.ColorPop.Colors = bad
		if o.Validate() == nil {
			t.Fatal("accepted invalid sample", bad)
		}
	}
}
func popLibrary() Library {
	return Library{SHA256: "fixture", Filaments: []Filament{{Name: "Black", RGB: RGB{}, TD: .3}, {Name: "White", RGB: RGB{255, 255, 255}, TD: 2}, {Name: "Red", RGB: RGB{230, 20, 15}, TD: 2}, {Name: "Blue", RGB: RGB{0, 0, 255}, TD: 2}, {Name: "Gray", RGB: RGB{128, 128, 128}, TD: 2}}}
}
func TestColorPopStackBandsExportsAndCache(t *testing.T) {
	ctx := context.Background()
	o := popOptions()
	o.Mode = "stack"
	o.Colors = 3
	o.HueForge.BeamWidth = 12
	o.HueForge.MaxPerceivedColors = 8
	lib := popLibrary()
	src := popFixture()
	p := &Processor{}
	r, err := p.Process(ctx, src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Stack == nil || r.Stack.UniqueFilaments > 3 || len(r.Stack.Runs) > 8 {
		t.Fatal("invalid physical budget")
	}
	check := func(r *Result) {
		t.Helper()
		for y := 0; y < 20; y++ {
			for x := 0; x < 40; x++ {
				if src.NRGBAAt(x, y).A == 0 {
					continue
				}
				band := r.ColorPop.GrayLayers
				if x < 10 {
					band = r.ColorPop.ColorLayers
				}
				layer := int(r.LayerMap[y*40+x])
				if layer < band[0] || layer > band[1] {
					t.Fatal("crossed region boundary", x, layer, band)
				}
			}
		}
	}
	check(r)
	for _, run := range r.Stack.Runs {
		if run.StartLayer <= r.ColorPop.GrayLayers[1] {
			lab := ToLab(run.Filament.RGB)
			if math.Hypot(lab[1], lab[2]) > 12 {
				t.Fatal("chromatic spool used in grayscale band", run.Filament)
			}
		}
	}
	accent := r.Image.NRGBAAt(5, 18)
	if accent.R <= accent.G || accent.R <= accent.B {
		t.Fatal("lower band consumed the accent's filament budget", accent)
	}
	d, err := hueForgeProject(ctx, r, "fixture.png", ImageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if d["luminance_method"] != 6 || d["color_match_method"] != 0 {
		t.Fatal("HFP did not preserve exact band targets")
	}
	if len(r.StackView.Layers) != r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer {
		t.Fatal("invalid stack view")
	}
	old := r.LayerMap[0]
	o.ColorPop.GrayOnTop = true
	flipped, err := p.Process(ctx, src, o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	check(flipped)
	if old == flipped.LayerMap[0] {
		t.Fatal("stale band geometry reused")
	}
	o.HueForge.ExportWidthMM = 140
	cached, err := p.Process(ctx, src, o, &lib, nil)
	if err != nil || cached.SurfaceView.WidthMM != 140 {
		t.Fatal("reframed export", err)
	}
	cancel, c := context.WithCancel(ctx)
	c()
	if _, err = p.Process(cancel, src, o, &lib, nil); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestColorPopDuplicateMeshKeys(t *testing.T) {
	o := popOptions()
	o.Mode = "stack"
	lib := popLibrary()
	r, err := Process(context.Background(), popFixture(), o, &lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Repeated neutral colors must remain distinct surface heights in the HFP.
	for i := range r.Palette {
		r.Palette[i].RGB = RGB{255, 255, 255}
	}
	keys := colorPopMeshColors(r)
	seen := map[RGB]bool{}
	for _, c := range keys {
		if seen[c] {
			t.Fatal("duplicate virtual key")
		}
		seen[c] = true
	}
	encoded, err := colorPopMeshResult(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if encoded.Stack == r.Stack || encoded.Image == r.Image {
		t.Fatal("export mutated original")
	}
	if _, err = hueForgeProject(context.Background(), r, "duplicate.png", ImageMetadata{}); err != nil {
		t.Fatal(err)
	}
}
