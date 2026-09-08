package engine

import (
	"context"
	"math"
	"testing"
)

func TestWindowsICCConversionPreservesAlphaAndColor(t *testing.T) {
	actual, e := LoadImage(context.Background(), "testdata/adobe-transparent.png")
	if e != nil {
		t.Fatal(e)
	}
	expected, e := LoadImage(context.Background(), "testdata/adobe-srgb-reference.png")
	if e != nil {
		t.Fatal(e)
	}
	if len(actual.Metadata.Warnings) > 0 || actual.Metadata.ColorProfile != "Converted to sRGB" {
		t.Fatal(actual.Metadata)
	}
	total, count, maximum := 0., 0., 0.
	for i := 0; i < len(actual.Image.Pix); i += 4 {
		a, b := actual.Image.Pix[i:i+4], expected.Image.Pix[i:i+4]
		if a[3] != b[3] {
			t.Fatal("ICC conversion changed alpha")
		}
		if a[3] == 0 {
			continue
		}
		d := math.Sqrt(distance(ToLab(RGB{a[0], a[1], a[2]}), ToLab(RGB{b[0], b[1], b[2]})))
		total += d
		count++
		maximum = math.Max(maximum, d)
	}
	t.Logf("Go ICC vs LittleCMS mean ΔE %.4f, maximum %.4f", total/count, maximum)
	if total/count > 1 || maximum > 4 {
		t.Fatal("ICC output differs materially from reference")
	}
}
