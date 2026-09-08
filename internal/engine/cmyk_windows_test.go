package engine

import (
	"context"
	"math"
	"testing"
)

func TestCMYKColorProfile(t *testing.T) {
	a, e := LoadImage(context.Background(), "testdata/cmyk.jpg")
	if e != nil {
		t.Fatal(e)
	}
	b, e := LoadImage(context.Background(), "testdata/cmyk-srgb-reference.png")
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Metadata.Warnings) > 0 {
		t.Fatal(a.Metadata)
	}
	sum := 0.
	count := 0
	for i := 0; i < len(a.Image.Pix); i += 16 {
		p, q := a.Image.Pix[i:i+3], b.Image.Pix[i:i+3]
		sum += math.Sqrt(distance(ToLab(RGB{p[0], p[1], p[2]}), ToLab(RGB{q[0], q[1], q[2]})))
		count++
	}
	t.Logf("CMYK mean ΔE vs LittleCMS: %.4f", sum/float64(count))
	if sum/float64(count) > 2 {
		t.Fatal("CMYK conversion differs materially")
	}
}
