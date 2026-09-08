package engine

import (
	"context"
	"image"
	"math"
)

type tap struct {
	index  int
	weight int64
}

// Lanczos-3 resampling with 22-bit fixed-point coefficients and an 8-bit
// premultiplied intermediate preserves the original utility's resize semantics.
func resampleTaps(input, output int) [][]tap {
	scale := float64(input) / float64(output)
	filterScale := math.Max(1, scale)
	support := 3 * filterScale
	out := make([][]tap, output)
	sinc := func(v float64) float64 {
		if v == 0 {
			return 1
		}
		v *= math.Pi
		return math.Sin(v) / v
	}
	for x := 0; x < output; x++ {
		center := (float64(x) + .5) * scale
		start := max(0, int(center-support+.5))
		end := min(input, int(center+support+.5))
		weights := make([]float64, end-start)
		sum := 0.
		for i := range weights {
			d := (float64(start+i) - center + .5) / filterScale
			if d >= -3 && d < 3 {
				weights[i] = sinc(d) * sinc(d/3)
				sum += weights[i]
			}
		}
		for i, w := range weights {
			if sum != 0 {
				w /= sum
			}
			out[x] = append(out[x], tap{start + i, int64(math.Round(w * (1 << 22)))})
		}
	}
	return out
}
func resizeAnalysis(ctx context.Context, src *image.NRGBA, w, h int) (*image.NRGBA, error) {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	premult := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	for y := 0; y < sh; y++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		for x := 0; x < sw; x++ {
			i := y*src.Stride + x*4
			j := y*premult.Stride + x*4
			alpha := int(src.Pix[i+3])
			for c := 0; c < 3; c++ {
				premult.Pix[j+c] = uint8((int(src.Pix[i+c])*alpha + 127) / 255)
			}
			premult.Pix[j+3] = uint8(alpha)
		}
	}
	horizontal := premult
	if w != sw {
		weights := resampleTaps(sw, w)
		horizontal = image.NewNRGBA(image.Rect(0, 0, w, sh))
		for y := 0; y < sh; y++ {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			for x, taps := range weights {
				for c := 0; c < 4; c++ {
					sum := int64(1 << 21)
					for _, t := range taps {
						sum += int64(premult.Pix[y*premult.Stride+t.index*4+c]) * t.weight
					}
					horizontal.Pix[y*horizontal.Stride+x*4+c] = uint8(max(0, min(255, sum>>22)))
				}
			}
		}
	}
	output := horizontal
	if h != sh {
		weights := resampleTaps(sh, h)
		output = image.NewNRGBA(image.Rect(0, 0, w, h))
		for y, taps := range weights {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			for x := 0; x < w; x++ {
				for c := 0; c < 4; c++ {
					sum := int64(1 << 21)
					for _, t := range taps {
						sum += int64(horizontal.Pix[t.index*horizontal.Stride+x*4+c]) * t.weight
					}
					output.Pix[y*output.Stride+x*4+c] = uint8(max(0, min(255, sum>>22)))
				}
			}
		}
	}
	for i := 0; i < len(output.Pix); i += 4 {
		alpha := int(output.Pix[i+3])
		if alpha > 0 && alpha < 255 {
			for c := 0; c < 3; c++ {
				output.Pix[i+c] = uint8(min(255, int(output.Pix[i+c])*255/alpha))
			}
		}
	}
	return output, nil
}
