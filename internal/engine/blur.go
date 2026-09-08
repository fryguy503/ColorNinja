package engine

import (
	"context"
	"image"
	"math"
)

// Three extended box passes approximate a Gaussian in O(pixels), including at
// large radii. RGB is premultiplied so invisible colors cannot contaminate edges.
func analysisBlur(ctx context.Context, src *image.NRGBA, sigma float64) (*image.NRGBA, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	a := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := image.NewNRGBA(a.Bounds())
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			j := y*a.Stride + x*4
			alpha := float64(src.Pix[i+3]) / 255
			for c := 0; c < 3; c++ {
				a.Pix[j+c] = byteRound(float64(src.Pix[i+c]) * alpha)
			}
			a.Pix[j+3] = src.Pix[i+3]
		}
	}
	variance := sigma * sigma / 3
	r := int(math.Floor((math.Sqrt(12*variance+1) - 1) / 2))
	fraction := float64(2*r+1) * (variance - float64(r*(r+1))/3) / (2 * (float64((r+1)*(r+1)) - variance))
	norm := float64(2*r+1) + 2*fraction
	for axis := 0; axis < 2; axis++ {
		length, lines, stride := w, h, 4
		if axis == 1 {
			length, lines, stride = h, w, a.Stride
		}
		for pass := 0; pass < 3; pass++ {
			for line := 0; line < lines; line++ {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				base := line * a.Stride
				if axis == 1 {
					base = line * 4
				}
				for c := 0; c < 4; c++ {
					at := func(i int) float64 { return float64(a.Pix[base+max(0, min(length-1, i))*stride+c]) }
					sum := 0.
					for k := -r; k <= r; k++ {
						sum += at(k)
					}
					for pos := 0; pos < length; pos++ {
						v := (sum + fraction*(at(pos-r-1)+at(pos+r+1))) / norm
						b.Pix[base+pos*stride+c] = byteRound(v)
						sum += at(pos+r+1) - at(pos-r)
					}
				}
			}
			a, b = b, a
		}
	}
	for i := 0; i < len(a.Pix); i += 4 {
		alpha := float64(a.Pix[i+3])
		for c := 0; c < 3; c++ {
			if alpha == 0 {
				a.Pix[i+c] = 0
			} else {
				a.Pix[i+c] = byteRound(float64(a.Pix[i+c]) * 255 / alpha)
			}
		}
	}
	return a, nil
}
