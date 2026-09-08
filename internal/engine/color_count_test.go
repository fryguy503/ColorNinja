package engine

import (
	"context"
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestUniqueColorsRespectRGBAlphaAndImageBounds(t *testing.T) {
	parent := image.NewNRGBA(image.Rect(7, 11, 13, 13))
	for y := 11; y < 13; y++ {
		for x := 7; x < 13; x++ {
			parent.SetNRGBA(x, y, color.NRGBA{128, 1, 244, 255})
		}
	}
	img := parent.SubImage(image.Rect(8, 11, 12, 13)).(*image.NRGBA)
	pixels := []color.NRGBA{
		{0, 0, 0, 255}, {0, 0, 0, 128}, {0, 0, 63, 1}, {0, 0, 64, 255},
		{0, 1, 0, 255}, {1, 0, 0, 255}, {255, 255, 255, 255}, {42, 4, 1, 0},
	}
	for i, pixel := range pixels {
		img.SetNRGBA(8+i%4, 11+i/4, pixel)
	}
	count, err := CountUniqueColors(context.Background(), img)
	if err != nil || count != 6 {
		t.Fatalf("count = %d, error = %v; want 6 visible RGB colors", count, err)
	}
	for y := 11; y < 13; y++ {
		for x := 8; x < 12; x++ {
			c := img.NRGBAAt(x, y)
			c.A = 0
			img.SetNRGBA(x, y, c)
		}
	}
	count, err = CountUniqueColors(context.Background(), img)
	if err != nil || count != 0 {
		t.Fatal("hidden RGB counted as visible colors", count, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = CountUniqueColors(ctx, img); !errors.Is(err, context.Canceled) {
		t.Fatal("count ignored cancellation", err)
	}
}

func TestResultUniqueColorsCountPixelsRatherThanPaletteSlots(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(0)
			if (x/8)%2 == 0 {
				v = 255
			}
			src.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	o := testOptions()
	o.PreblurSigma, o.Colors = 1.5, 16
	r := process(t, src, o, nil)
	if r.UniqueColors != 2 || len(r.Palette) <= r.UniqueColors {
		t.Fatalf("want 2 used colors and unused analysis colors; got %d of %d", r.UniqueColors, len(r.Palette))
	}
	actual, err := CountUniqueColors(context.Background(), r.Image)
	if err != nil || actual != r.UniqueColors {
		t.Fatal("result count differs from actual pixels", actual, err)
	}
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 0
	}
	r = process(t, src, o, nil)
	if r.UniqueColors != 0 {
		t.Fatal("transparent result must have zero visible colors")
	}
}
