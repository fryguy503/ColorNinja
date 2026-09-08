package engine

import (
	"context"
	"image"
	"image/color"
	"image/gif"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestAnimatedWebPFirstFrame(t *testing.T) {
	for _, name := range []string{"animated-lossless", "animated-lossy"} {
		t.Run(name, func(t *testing.T) {
			got, e := LoadImage(context.Background(), filepath.Join("testdata", name+".webp"))
			if e != nil {
				t.Fatal(e)
			}
			want, e := LoadImage(context.Background(), filepath.Join("testdata", name+"-first.png"))
			if e != nil {
				t.Fatal(e)
			}
			if got.Image.Bounds() != want.Image.Bounds() {
				t.Fatal("frame lost its canvas dimensions")
			}
			if len(got.Metadata.Warnings) == 0 {
				t.Fatal("first-frame behavior was not disclosed")
			}
			var sum, peak float64
			channels := 0
			for i := 0; i < len(got.Image.Pix); i += 4 {
				if got.Image.Pix[i+3] != want.Image.Pix[i+3] {
					t.Fatalf("alpha differs at pixel %d", i/4)
				}
				if want.Image.Pix[i+3] == 0 {
					continue
				}
				for c := 0; c < 3; c++ {
					difference := math.Abs(float64(got.Image.Pix[i+c]) - float64(want.Image.Pix[i+c]))
					sum += difference
					peak = math.Max(peak, difference)
					channels++
				}
			}
			t.Logf("RGB channel difference vs libwebp: mean %.3f; max %.0f", sum/float64(channels), peak)
			if peak > 3 {
				t.Fatal("WebP color conversion differs from libwebp reference")
			}
			if name == "animated-lossless" && peak != 0 {
				t.Fatal("lossless frame color changed")
			}
		})
	}
}

func TestGIFFrameRetainsCanvasAndOffset(t *testing.T) {
	palette := color.Palette{color.NRGBA{}, color.NRGBA{32, 128, 192, 255}}
	frame := image.NewPaletted(image.Rect(2, 2, 6, 5), palette)
	for i := range frame.Pix {
		frame.Pix[i] = 1
	}
	path := filepath.Join(t.TempDir(), "offset.gif")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	e = gif.EncodeAll(f, &gif.GIF{Image: []*image.Paletted{frame}, Delay: []int{0}, Config: image.Config{ColorModel: palette, Width: 12, Height: 8}})
	if ce := f.Close(); e != nil {
		t.Fatal(e)
	} else if ce != nil {
		t.Fatal(ce)
	}
	got, e := LoadImage(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	if got.Image.Bounds() != image.Rect(0, 0, 12, 8) {
		t.Fatalf("wrong canvas: %v", got.Image.Bounds())
	}
	if got.Image.NRGBAAt(2, 2) != (color.NRGBA{32, 128, 192, 255}) || got.Image.NRGBAAt(0, 0).A != 0 {
		t.Fatal("wrong frame placement or transparent background")
	}
}
