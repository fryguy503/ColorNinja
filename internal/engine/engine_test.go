package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func gradient(w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / max(1, w-1)), uint8(y * 255 / max(1, h-1)), uint8((x + y) * 255 / max(1, w+h-2)), uint8(x * 255 / max(1, w-1))})
		}
	}
	return out
}
func solid(c RGB) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, 5, 4))
	for i := 0; i < len(out.Pix); i += 4 {
		copy(out.Pix[i:i+3], c[:])
		out.Pix[i+3] = 255
	}
	return out
}
func testOptions() Options {
	o := DefaultOptions()
	o.Colors = 4
	o.AnalysisMaxPixels = 1000
	o.PreblurSigma = 0
	o.HistogramBits = 5
	o.Iterations = 12
	o.MinClusterFraction = 0
	o.HueForge.MaxDepth = .8
	o.HueForge.AnalysisColors = 8
	o.HueForge.BeamWidth = 6
	return o
}
func testLibrary() Library {
	lib := Library{SHA256: "fixture"}
	for i, c := range []RGB{{0, 0, 0}, {255, 255, 255}, {255, 0, 0}, {0, 0, 255}} {
		lib.Filaments = append(lib.Filaments, Filament{Name: []string{"Black", "White", "Red", "Blue"}[i], RGB: c, Hex: c.Hex(), TD: 1, Material: "PLA", UUID: fmt.Sprint(i), Owned: true, SourceIndex: i})
	}
	return lib
}
func process(t *testing.T, img *image.NRGBA, o Options, lib *Library) *Result {
	t.Helper()
	r, e := Process(context.Background(), img, o, lib, nil)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestColorRoundTrip(t *testing.T) {
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 17 {
				c := RGB{uint8(r), uint8(g), uint8(b)}
				back := FromLab(ToLab(c))
				for i := range c {
					if math.Abs(float64(c[i])-float64(back[i])) > 1 {
						t.Fatalf("%v -> %v", c, back)
					}
				}
			}
		}
	}
}
func TestKnownLabValues(t *testing.T) {
	for _, tt := range []struct {
		rgb RGB
		lab Vec
	}{{RGB{255, 255, 255}, Vec{100, 0, 0}}, {RGB{}, Vec{}}, {RGB{255, 0, 0}, Vec{53.2408, 80.0925, 67.2032}}, {RGB{0, 255, 0}, Vec{87.7347, -86.1827, 83.1793}}} {
		got := ToLab(tt.rgb)
		if math.Sqrt(distance(got, tt.lab)) > .01 {
			t.Fatalf("%v: %v", tt.rgb, got)
		}
	}
}
func TestAnalysisSizeCeiling(t *testing.T) {
	for _, v := range [][3]int{{4000, 3000, 6291456}, {100000, 1, 1000}, {1, 100000, 1000}, {57, 73, 11}, {12, 12, 1}} {
		w, h := AnalysisSize(v[0], v[1], v[2])
		if w < 1 || h < 1 || w*h > v[2] {
			t.Fatal(v, w, h)
		}
	}
}
func TestValidationRejectsInvalid(t *testing.T) {
	cases := []func(*Options){func(o *Options) { o.Colors = 0 }, func(o *Options) { o.Colors = 257 }, func(o *Options) { o.AnalysisMaxPixels = -1 }, func(o *Options) { o.PreblurSigma = math.NaN() }, func(o *Options) { o.NeutralChroma = math.Inf(1) }, func(o *Options) { o.MinClusterFraction = 1 }, func(o *Options) { o.HistogramBits = 8 }, func(o *Options) { o.Iterations = 0 }, func(o *Options) { o.Mode = "other" }, func(o *Options) { o.GuidanceStrength = math.NaN() }, func(o *Options) { o.Mode = "stack"; o.HueForge.MaxDepth = .79 }, func(o *Options) { o.Mode = "guided"; o.HueForge.TDScale = 0 }, func(o *Options) { o.Mode = "stack"; o.HueForge.TDTransmission = 1 }, func(o *Options) { o.Mode = "stack"; o.HueForge.LayerHeight = math.SmallestNonzeroFloat64 }}
	for i, f := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			o := DefaultOptions()
			f(&o)
			if o.Validate() == nil {
				t.Fatal("accepted invalid options")
			}
		})
	}
}
func TestAlphaDimensionsPaletteAndDeterminism(t *testing.T) {
	src := gradient(96, 64)
	o := testOptions()
	a, b := process(t, src, o, nil), process(t, src, o, nil)
	if a.SHA256 != b.SHA256 || !reflect.DeepEqual(a.Palette, b.Palette) {
		t.Fatal("nondeterministic")
	}
	if a.SourceSize != [2]int{96, 64} || a.Image.Bounds() != src.Bounds() {
		t.Fatal("resized output")
	}
	palette := map[RGB]bool{}
	for _, p := range a.Palette {
		palette[p.RGB] = true
	}
	for i := 0; i < len(src.Pix); i += 4 {
		if src.Pix[i+3] != a.Image.Pix[i+3] {
			t.Fatal("alpha changed")
		}
		if !palette[RGB{a.Image.Pix[i], a.Image.Pix[i+1], a.Image.Pix[i+2]}] {
			t.Fatal("off-palette color")
		}
	}
}
func TestTransparentImage(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 9, 7))
	r := process(t, src, testOptions(), nil)
	if len(r.Palette) != 1 || r.Quality.RMS != 0 {
		t.Fatal("transparent image")
	}
	for _, v := range r.Image.Pix {
		if v != 0 {
			t.Fatal("hidden color")
		}
	}
}
func TestHiddenRGBDoesNotContaminateBlur(t *testing.T) {
	a := gradient(20, 20)
	b := image.NewNRGBA(a.Bounds())
	copy(b.Pix, a.Pix)
	for i := 0; i < len(a.Pix); i += 4 {
		if i%12 == 0 {
			a.Pix[i+3] = 0
			b.Pix[i], b.Pix[i+1], b.Pix[i+2], b.Pix[i+3] = 255, 0, 255, 0
		}
	}
	o := testOptions()
	o.PreblurSigma = 2
	if process(t, a, o, nil).SHA256 != process(t, b, o, nil).SHA256 {
		t.Fatal("invisible RGB changed visible output")
	}
}
func TestClusterCullInvariant(t *testing.T) {
	pts := []point{{Vec{0, 0, 0}, 100}, {Vec{10, 0, 0}, 1}, {Vec{50, 0, 0}, 10}, {Vec{100, 0, 0}, 1}}
	o := testOptions()
	o.MinClusterFraction = .1
	_, m, e := cluster(context.Background(), pts, o)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range m {
		if len(m) > 1 && v < 112*.1 {
			t.Fatal("unculled cluster", m)
		}
	}
}
func TestCancellationDuringProcessing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, e := Process(ctx, gradient(160, 120), testOptions(), nil, func(p Progress) {
		if p.Fraction >= .65 {
			cancel()
		}
	})
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("%v", e)
	}
}
func TestCanceledLargeBlur(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, e := analysisBlur(ctx, gradient(100, 100), 100)
	if !errors.Is(e, context.Canceled) || time.Since(start) > time.Second {
		t.Fatal(e)
	}
}
func TestLibraryFiltersAndBOM(t *testing.T) {
	raw := []byte(`{"Filaments":[{"Name":"Red","Color":"#FF0000","Owned":true,"Type":"PLA","Transmissivity":1},{"Color":"#FF0000","Owned":true,"Type":"PLA","Transmissivity":1},{"Color":"#0000FF","Owned":false,"Type":"PLA","Transmissivity":1},{"Color":"#00FF00","Type":"PLA","Transmissivity":1},{"Color":"#00FF00","Owned":"garbage","Type":"PLA","Transmissivity":1},{"Color":"#00FF00","Owned":true,"Type":"PETG","Transmissivity":1},{"Color":"#00FFFF","Secondary_Color":"#FF0000","Owned":true,"Type":"PLA","Transmissivity":1},{"Color":"bad","Owned":true,"Type":"PLA","Transmissivity":1}]}`)
	lib, e := ParseLibrary(append([]byte{239, 187, 191}, raw...), LibraryFilter{MaterialTypes: []string{"pla"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(lib.Filaments) != 1 || lib.SkippedDuplicate != 1 || lib.SkippedUnowned != 1 || lib.SkippedInvalid != 3 || lib.SkippedFiltered != 1 || lib.SkippedSecondary != 1 {
		t.Fatalf("%+v", lib)
	}
}
func TestEmptyLibraryFailsClosed(t *testing.T) {
	for _, raw := range []string{`{}`, `{"Filaments":[]}`, `{"Filaments":[{"Color":"#FF0000","Transmissivity":1}]}`, `null`, `[12]`} {
		if _, e := ParseLibrary([]byte(raw), LibraryFilter{}); e == nil {
			t.Fatal(raw)
		}
	}
}
func TestExactLayerTransmission(t *testing.T) {
	o := DefaultOptions().HueForge
	o.LayerHeight = .5
	o.TDScale = 1
	o.TDTransmission = .25
	f := Filament{RGB: RGB{255, 0, 0}, TD: 1}
	first := blend(Vec{}, f, o)
	second := blend(first, f, o)
	if first[0] != .5 || second[0] != .75 || first[1] != 0 {
		t.Fatal(first, second)
	}
}
func TestReachableExactSubset(t *testing.T) {
	c := labColors([]RGB{{0, 0, 0}, {225, 225, 225}, {248, 248, 248}})
	ids, m, rms, e := selectReachable(context.Background(), c, []Vec{c[0], c[2]}, []float64{.5, .5}, 2, 0)
	if e != nil || !reflect.DeepEqual(ids, []int{0, 2}) || !reflect.DeepEqual(m, []float64{.5, .5}) || rms != 0 {
		t.Fatal(ids, m, rms, e)
	}
}
func TestGuidanceFavorsRelevantOwnedColor(t *testing.T) {
	lib := testLibrary()
	o := testOptions()
	o.Mode = "guided"
	o.Colors = 1
	o.GuidanceStrength = 1
	r := process(t, solid(RGB{240, 35, 25}), o, &lib)
	if r.Guidance == nil || r.Stack != nil || r.LayerMap != nil || r.Guidance.Selected[0].Name != "Red" || r.Palette[0].RGB != (RGB{255, 0, 0}) {
		t.Fatalf("%+v", r)
	}
	if r.Guidance.GlobalStackGuaranteed {
		t.Fatal("guided plan promised global stack")
	}
}
func TestGuidanceZeroPreservesAnalyzedColors(t *testing.T) {
	lib := testLibrary()
	o := testOptions()
	o.Mode = "guided"
	o.GuidanceStrength = 0
	r := process(t, solid(RGB{240, 35, 25}), o, &lib)
	c := r.Palette[0].RGB
	if math.Sqrt(distance(ToLab(c), ToLab(RGB{240, 35, 25}))) > .1 {
		t.Fatal(c)
	}
}
func TestStackTerminalRerank(t *testing.T) {
	lib := testLibrary()
	lib.Filaments = lib.Filaments[:2]
	lib.Filaments = append(lib.Filaments, Filament{Name: "Gray 119", RGB: RGB{119, 119, 119}, TD: 1, Owned: true})
	o := testOptions()
	o.PreserveDetails = false // The exact #777777 optimum here is specific to CIELAB.
	o.Mode = "stack"
	o.Colors = 2
	o.HueForge.MaxPerceivedColors = 1
	p, plan, e := planStack(context.Background(), []PaletteEntry{entry(RGB{}, .5, 8), entry(RGB{255, 255, 255}, .5, 8)}, lib, o, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(p) != 1 || p[0].RGB != (RGB{119, 119, 119}) || len(plan.Runs) != 1 || plan.Runs[0].Filament.Name != "Gray 119" {
		t.Fatal(p, plan)
	}
}
func TestStackBudgetCannotForceWorsePlan(t *testing.T) {
	lib := testLibrary()
	lib.Filaments = lib.Filaments[:3]
	o := testOptions()
	o.Mode = "stack"
	o.Colors = 3
	o.HueForge.LayerHeight = 1
	o.HueForge.BaseDepth = 1
	o.HueForge.MaxDepth = 3
	o.HueForge.TDTransmission = .25
	o.HueForge.TDScale = 1
	o.HueForge.BaseTransmissionLimit = 1
	p, plan, e := planStack(context.Background(), []PaletteEntry{entry(RGB{}, .499, 8), entry(RGB{248, 248, 248}, .499, 8), entry(RGB{225, 225, 225}, .002, 8)}, lib, o, nil)
	if e != nil {
		t.Fatal(e)
	}
	if plan.RMS > 1e-6 || len(plan.Runs) != 2 || len(p) != 3 || plan.Runs[1].Layers != 2 {
		t.Fatal(p, plan)
	}
}
func TestStackPrunesUnusedFilaments(t *testing.T) {
	o := testOptions()
	o.Mode = "stack"
	lib := testLibrary()
	r := process(t, solid(RGB{}), o, &lib)
	if len(r.Stack.Runs) != 1 || r.Stack.PlannedDepth != o.HueForge.BaseDepth || len(r.Stack.LayerColors) != o.HueForge.BaseLayers() {
		t.Fatal(r.Stack)
	}
}
func TestStackOutputIsReachable(t *testing.T) {
	o := testOptions()
	o.Mode = "stack"
	o.Colors = 2
	lib := testLibrary()
	r := process(t, gradient(32, 24), o, &lib)
	lookup := map[int]RGB{}
	for _, l := range r.Stack.LayerColors {
		lookup[l.Layer] = l.RGB
	}
	for _, p := range r.Palette {
		if lookup[p.StackLayer] != p.RGB {
			t.Fatal("unreachable color")
		}
	}
	for i, layer := range r.LayerMap {
		if r.Image.Pix[i*4+3] == 0 {
			if layer != 0 {
				t.Fatal("transparent layer")
			}
		} else if lookup[int(layer)] != (RGB{r.Image.Pix[i*4], r.Image.Pix[i*4+1], r.Image.Pix[i*4+2]}) {
			t.Fatal("layer reconstruction differs")
		}
	}
}
func TestOpaqueBaseRequired(t *testing.T) {
	o := testOptions()
	o.Mode = "stack"
	o.HueForge.BaseTransmissionLimit = 1e-100
	lib := testLibrary()
	if _, e := Process(context.Background(), solid(RGB{}), o, &lib, nil); e == nil {
		t.Fatal("accepted translucent base")
	}
}
func TestAtomicNoClobberAndCancellation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "output")
	write := func(w io.Writer) error { _, e := w.Write([]byte("first")); return e }
	if e := AtomicWrite(p, false, write); e != nil {
		t.Fatal(e)
	}
	if e := AtomicWrite(p, false, func(w io.Writer) error { _, e := w.Write([]byte("second")); return e }); e == nil {
		t.Fatal("clobbered")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "first" {
		t.Fatal(string(b))
	}
	q := filepath.Join(filepath.Dir(p), "cancel")
	if e := AtomicWrite(q, false, func(w io.Writer) error { w.Write([]byte("partial")); return context.Canceled }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Stat(q); !os.IsNotExist(e) {
		t.Fatal("published partial")
	}
}
func TestAtomicRacingWriters(t *testing.T) {
	p := filepath.Join(t.TempDir(), "race")
	var wg sync.WaitGroup
	success := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			success <- AtomicWrite(p, false, func(w io.Writer) error { _, e := w.Write([]byte("complete")); return e }) == nil
		}()
	}
	wg.Wait()
	close(success)
	count := 0
	for ok := range success {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
}
func TestPNGAndLayerMapRoundTrips(t *testing.T) {
	o := testOptions()
	o.Mode = "stack"
	o.Colors = 2
	lib := testLibrary()
	r := process(t, gradient(16, 12), o, &lib)
	dir := t.TempDir()
	p := filepath.Join(dir, "image.png")
	meta := ImageMetadata{DPI: [2]float64{300, 300}}
	if e := SavePNG(context.Background(), p, r, meta, false); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadImage(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(loaded.Image.Pix, r.Image.Pix) || math.Abs(loaded.Metadata.DPI[0]-300) > .1 {
		t.Fatal("PNG pixels or DPI changed")
	}
	p = filepath.Join(dir, "layers.png")
	if e := SaveLayerMap(context.Background(), p, r, false); e != nil {
		t.Fatal(e)
	}
	f, _ := os.Open(p)
	defer f.Close()
	decoded, e := png.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	gray, ok := decoded.(*image.Gray16)
	if !ok {
		t.Fatalf("%T", decoded)
	}
	for i, v := range r.LayerMap {
		if binary.BigEndian.Uint16(gray.Pix[i*2:]) != v {
			t.Fatal("16-bit layer changed")
		}
	}
}
func TestPathAliasesProtected(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte("original"), 0600)
	if e := os.Link(a, b); e != nil {
		t.Skip(e)
	}
	if DistinctPaths(a, b) == nil || DistinctPaths(a, a) == nil {
		t.Fatal("alias accepted")
	}
}
func TestExifOrientation(t *testing.T) {
	data := make([]byte, 26)
	copy(data, "II")
	binary.LittleEndian.PutUint16(data[2:], 42)
	binary.LittleEndian.PutUint32(data[4:], 8)
	binary.LittleEndian.PutUint16(data[8:], 1)
	binary.LittleEndian.PutUint16(data[10:], 274)
	binary.LittleEndian.PutUint16(data[12:], 3)
	binary.LittleEndian.PutUint32(data[14:], 1)
	binary.LittleEndian.PutUint16(data[18:], 6)
	var buf bytes.Buffer
	png.Encode(&buf, gradient(5, 3))
	raw := buf.Bytes()
	var tagged bytes.Buffer
	tagged.Write(raw[:33])
	writeChunk(&tagged, "eXIf", data)
	tagged.Write(raw[33:])
	path := filepath.Join(t.TempDir(), "rotated.png")
	os.WriteFile(path, tagged.Bytes(), 0600)
	loaded, e := LoadImage(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	if loaded.Image.Bounds().Dx() != 3 || loaded.Image.Bounds().Dy() != 5 {
		t.Fatal("EXIF ignored")
	}
}
func TestPythonReferenceBehavior(t *testing.T) {
	raw, e := os.ReadFile("testdata/reference.json")
	if e != nil {
		t.Fatal(e)
	}
	var refs []struct {
		Name, Mode string
		Sigma      float64
		Limit      int
		Palette    []RGB
		Mean, RMS  float64
		Analysis   [2]int
	}
	if e = json.Unmarshal(raw, &refs); e != nil {
		t.Fatal(e)
	}
	src, e := LoadImage(context.Background(), "testdata/gradient.png")
	if e != nil {
		t.Fatal(e)
	}
	lib, e := LoadLibrary("testdata/library.json", LibraryFilter{})
	if e != nil {
		t.Fatal(e)
	}
	for _, ref := range refs {
		t.Run(ref.Name, func(t *testing.T) {
			o := testOptions()
			o.TrueBlack = false       // Legacy Python fixtures predate the opt-out black override.
			o.PreserveDetails = false // Keep testing the original CIELAB pipeline exactly.
			o.Mode = ref.Mode
			o.PreblurSigma = ref.Sigma
			o.AnalysisMaxPixels = ref.Limit
			o.HueForge.MaxPerceivedColors = 12
			r := process(t, src.Image, o, &lib)
			if r.AnalysisSize != ref.Analysis {
				t.Fatal("analysis dimensions changed")
			}
			reference, e := LoadImage(context.Background(), "testdata/"+ref.Name+".png")
			if e != nil {
				t.Fatal(e)
			}
			sum, weight := 0., 0.
			for i := 0; i < len(r.Image.Pix); i += 4 {
				a, b := r.Image.Pix[i:i+4], reference.Image.Pix[i:i+4]
				if a[3] != b[3] {
					t.Fatal("alpha differs from Python")
				}
				mass := float64(a[3]) / 255
				sum += math.Sqrt(distance(ToLab(RGB{a[0], a[1], a[2]}), ToLab(RGB{b[0], b[1], b[2]}))) * mass
				weight += mass
			}
			difference := sum / weight
			t.Logf("Go/Python pixel mean ΔE %.5f; RMS %.5f / %.5f; palette %d / %d", difference, r.Quality.RMS, ref.RMS, len(r.Palette), len(ref.Palette))
			if !bytes.Equal(r.Image.Pix, reference.Image.Pix) || math.Abs(r.Quality.RMS-ref.RMS) > .0001 {
				t.Fatalf("port exceeds visual regression bounds: %.4f", difference)
			}
		})
	}
}
func BenchmarkStandard1MP(b *testing.B) {
	img := gradient(1200, 900)
	o := DefaultOptions()
	o.Colors = 8
	for i := 0; i < b.N; i++ {
		if _, e := Process(context.Background(), img, o, nil, nil); e != nil {
			b.Fatal(e)
		}
	}
}
