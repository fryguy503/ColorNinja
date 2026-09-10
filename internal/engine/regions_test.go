package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"slices"
	"testing"
)

func regionFixture(w, h int, layers []uint16) (*Result, Options) {
	o := DefaultOptions()
	o.Mode = "stack"
	o.HueForge.BaseDepth = o.HueForge.Height(2)
	o.HueForge.MaxDepth = o.HueForge.Height(8)
	r := &Result{Image: image.NewNRGBA(image.Rect(3, 5, 3+w, 5+h)), LayerMap: append([]uint16(nil), layers...), SourceSize: [2]int{w, h}, Stack: &StackPlan{Options: o.HueForge, Runs: []StackRun{{Position: 1, StartLayer: 1, EndLayer: 8, Layers: 8, Filament: Filament{RGB: RGB{255, 255, 255}, TD: 1, Name: "White"}}}}}
	for l := 1; l <= 8; l++ {
		r.Stack.LayerColors = append(r.Stack.LayerColors, LayerColor{Layer: l, Height: o.HueForge.Height(l), RGB: RGB{uint8(l * 25), uint8(l * 25), uint8(l * 25)}, TopPosition: 1})
	}
	for i, l := range layers {
		if l > 0 {
			c := r.Stack.LayerColors[l-1].RGB
			r.Image.SetNRGBA(3+i%w, 5+i/w, color.NRGBA{c[0], c[1], c[2], 255})
		}
	}
	r.SHA256 = fmt.Sprintf("%x", sha256.Sum256(r.Image.Pix))
	return r, o
}
func newRegionDoc(r *Result, groups ...RegionGroup) RegionDocument {
	for i := range groups {
		groups[i].ID = uint64(i + 1)
		groups[i].Enabled = true
	}
	return RegionDocument{1, r.SourceSize[0], r.SourceSize[1], r.SHA256, uint64(len(groups) + 1), groups}
}
func countMask(m []bool) int {
	n := 0
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}

func TestRegionConnectivityAndLasso(t *testing.T) {
	ctx := context.Background()
	r, _ := regionFixture(5, 3, []uint16{2, 4, 2, 4, 2, 2, 2, 2, 2, 2, 2, 4, 2, 4, 0})
	x, e := BuildRegionIndex(ctx, r, r)
	if e != nil {
		t.Fatal(e)
	}
	if len(x.Areas) != 6 {
		t.Fatal("separate equal-height islands merged", x.Areas)
	}
	s := RegionSelection{Tool: "rectangle", Scope: "inside", Points: []RegionPoint{{.5, -.5}, {4.5, 3}}}
	m, e := x.Select(ctx, s)
	if e != nil || countMask(m) != 4 {
		t.Fatal("whole-region lasso must exclude clipped background", countMask(m), e)
	}
	s.Scope = "touch"
	m, e = x.Select(ctx, s)
	if e != nil || countMask(m) != 14 {
		t.Fatal("touching region must expand to full background", countMask(m), e)
	}
	s.Scope = "pixels"
	s.Points = []RegionPoint{{1, 0}, {4, 2}}
	m, e = x.Select(ctx, s)
	if e != nil || countMask(m) != 6 {
		t.Fatal("pixel selection", countMask(m), e)
	}
	s = RegionSelection{Tool: "click", Points: []RegionPoint{{1, 0}}}
	m, e = x.Select(ctx, s)
	if e != nil || countMask(m) != 1 {
		t.Fatal("click island", e)
	}
	s.Below = 2
	m, e = x.Select(ctx, s)
	if e != nil || countMask(m) != 14 {
		t.Fatal("layer tolerance", e)
	}
	d, _ := regionFixture(2, 2, []uint16{2, 0, 0, 2})
	ix, _ := BuildRegionIndex(ctx, d, d)
	if len(ix.Areas) != 3 {
		t.Fatal("diagonal connection merged")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = x.Select(ctx, RegionSelection{Tool: "lasso", Scope: "inside", Points: []RegionPoint{{0, 0}, {5, 0}, {5, 3}, {0, 3}}}); e == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestRegionMaskAlgebraAndRefinement(t *testing.T) {
	a := []bool{true, true, false, false}
	b := []bool{false, true, true, false}
	for op, want := range map[string][]bool{"add": {true, true, true, false}, "subtract": {true, false, false, false}, "intersect": {false, true, false, false}, "replace": b} {
		got, e := CombineRegionMasks(a, b, op)
		if e != nil || !slices.Equal(got, want) {
			t.Fatal(op, got, e)
		}
	}
	r, _ := regionFixture(5, 5, slices.Repeat([]uint16{2}, 25))
	x, _ := BuildRegionIndex(context.Background(), r, r)
	m := make([]bool, 25)
	m[12] = true
	g, e := x.Refine(context.Background(), m, "grow", 1, r, 0)
	if e != nil || countMask(g) != 5 {
		t.Fatal("grow", e)
	}
	got, e := x.Refine(context.Background(), g, "shrink", 1, r, 0)
	if e != nil || !slices.Equal(got, m) {
		t.Fatal("shrink", e)
	}
	ring := []bool{false, false, false, false, false, false, true, true, true, false, false, true, false, true, false, false, true, true, true, false, false, false, false, false, false}
	got, e = x.Refine(context.Background(), ring, "fill", 0, r, 0)
	if e != nil || countMask(got) != 9 {
		t.Fatal("fill", e)
	}
	sp := MaskSpans(ring)
	decoded, e := SpanMask(sp, 25)
	if e != nil || !slices.Equal(decoded, ring) {
		t.Fatal("span roundtrip")
	}
	for _, bad := range [][]PixelSpan{{{24, 2}}, {{-1, 1}}, {{1, 0}}, {{2, 4}, {3, 1}}} {
		if ValidateSpans(bad, 25) == nil {
			t.Fatal("invalid footprint accepted", bad)
		}
	}
}
func TestRegionReplayAssignmentReliefProtectionCutAndRestore(t *testing.T) {
	r, o := regionFixture(4, 2, []uint16{2, 4, 6, 8, 2, 4, 6, 0})
	original := append([]byte(nil), r.Image.Pix...)
	originalLayers := append([]uint16(nil), r.LayerMap...)
	d := newRegionDoc(r, RegionGroup{Operation: "shift", Value: 1, Mask: []PixelSpan{{0, 4}}})
	got, e := ApplyRegionDocument(context.Background(), r, r.Image, o, d)
	if e != nil {
		t.Fatal(e)
	}
	if !slices.Equal(got.LayerMap, []uint16{3, 5, 7, 8, 2, 4, 6, 0}) {
		t.Fatal("shift lost relief/clamping", got.LayerMap)
	}
	d = newRegionDoc(r, RegionGroup{Operation: "assign", Value: 6, Locked: true, Mask: []PixelSpan{{0, 2}}}, RegionGroup{Operation: "assign", Value: 8, Mask: []PixelSpan{{1, 3}}})
	got, e = ApplyRegionDocument(context.Background(), r, r.Image, o, d)
	if e != nil || !slices.Equal(got.LayerMap, []uint16{6, 6, 8, 8, 2, 4, 6, 0}) {
		t.Fatal("overlap/protection", got, e)
	}
	d = newRegionDoc(r, RegionGroup{Operation: "cut", Mask: []PixelSpan{{0, 2}}})
	cut, e := ApplyRegionDocument(context.Background(), r, r.Image, o, d)
	if e != nil || cut.LayerMap[0] != 0 || cut.Image.Pix[3] != 0 {
		t.Fatal("cut coverage", e)
	}
	d.Groups = append(d.Groups, RegionGroup{ID: 2, Enabled: true, Operation: "restore", Mask: []PixelSpan{{0, 2}}})
	d.NextID = 3
	got, e = ApplyRegionDocument(context.Background(), r, r.Image, o, d)
	if e != nil || !bytes.Equal(got.Image.Pix, original) || !slices.Equal(got.LayerMap, originalLayers) {
		t.Fatal("restore exactness", e)
	}
	if !bytes.Equal(r.Image.Pix, original) || !slices.Equal(r.LayerMap, originalLayers) {
		t.Fatal("cached baseline mutated")
	}
	if cut.SHA256 == r.SHA256 || cut.SurfaceView.VolumeMM3 >= got.SurfaceView.VolumeMM3 {
		t.Fatal("derived result was not updated")
	}
	if cut.StackView.MeshCore != "legacy-flat" {
		t.Fatal("regional heights require unique export keys")
	}
}
func TestRegionValidationAndQuantizedSmoothing(t *testing.T) {
	r, o := regionFixture(3, 3, []uint16{2, 2, 2, 2, 8, 2, 2, 2, 2})
	d := newRegionDoc(r, RegionGroup{Operation: "smooth", Value: 1, Mask: []PixelSpan{{4, 1}}})
	out, e := ApplyRegionDocument(context.Background(), r, r.Image, o, d)
	if e != nil || out.LayerMap[4] != 3 {
		t.Fatal("quantized smoothing", out, e)
	}
	for _, g := range []RegionGroup{{Operation: "assign", Value: 1, Mask: []PixelSpan{{0, 1}}}, {Operation: "cut", Mask: []PixelSpan{{0, 9}}}, {Operation: "smooth", Value: 100, Mask: []PixelSpan{{0, 1}}}} {
		if _, e = ApplyRegionDocument(context.Background(), r, r.Image, o, newRegionDoc(r, g)); e == nil {
			t.Fatal("invalid edit accepted", g)
		}
	}
	d.BaseSHA256 = "wrong"
	if _, e = ApplyRegionDocument(context.Background(), r, r.Image, o, d); e == nil {
		t.Fatal("stale baseline accepted")
	}
}
func TestRegionHFPDuplicateColorsKeepDistinctHeights(t *testing.T) {
	r := hfpFixture(t)
	o := DefaultOptions()
	o.Mode = "stack"
	o.HueForge = r.Stack.Options
	// Use two printable layers with identical predicted RGB, a legitimate
	// optical plateau. The transport image must still encode both heights.
	lo := r.Stack.Options.BaseLayers()
	hi := r.Stack.Runs[len(r.Stack.Runs)-1].EndLayer
	for i := range r.Stack.LayerColors {
		if r.Stack.LayerColors[i].Layer == hi {
			for _, c := range r.Stack.LayerColors {
				if c.Layer == lo {
					r.Stack.LayerColors[i].RGB = c.RGB
				}
			}
		}
	}
	d := newRegionDoc(r, RegionGroup{Operation: "assign", Value: lo, Mask: []PixelSpan{{0, len(r.LayerMap)}}}, RegionGroup{Operation: "assign", Value: hi, Mask: []PixelSpan{{0, 1}}})
	got, e := ApplyRegionDocument(context.Background(), r, r.Image, o, d)
	if e != nil {
		t.Fatal(e)
	}
	transport, e := colorPopMeshResult(context.Background(), got)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Equal(transport.Image.Pix[:3], transport.Image.Pix[4:7]) {
		t.Fatal("distinct heights collapsed to the same export RGB")
	}
	if _, e = hueForgeProject(context.Background(), got, "regions", ImageMetadata{}); e != nil {
		t.Fatal(e)
	}
}

func BenchmarkRegionIndexAndLasso(b *testing.B) {
	for _, size := range []int{1024, 4096} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			layers := make([]uint16, size*size)
			for i := range layers {
				layers[i] = uint16(2 + ((i%size)/16+(i/size)/16)%2)
			}
			r, _ := regionFixture(size, size, layers)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				x, err := BuildRegionIndex(context.Background(), r, r)
				if err != nil {
					b.Fatal(err)
				}
				_, err = x.Select(context.Background(), RegionSelection{Tool: "lasso", Scope: "inside", Points: []RegionPoint{{50, 50}, {float64(size - 50), 100}, {float64(size - 100), float64(size - 50)}, {100, float64(size - 100)}}})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
