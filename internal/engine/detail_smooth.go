package engine

import (
	"context"
	"image"
	"math"
	"runtime"
	"sync"
)

// detailSmooth is an alpha-weighted separable joint bilateral filter. Both
// passes use the unchanged source as their color guide: smoothing a texture
// must not progressively weaken an outline. Spatial radius follows the user's
// smoothing control; the range kernel is fixed at 5 CIELAB Delta E76 units.
// Only two RGBA buffers and bounded worker-local scanline tiles are allocated, never a full
// float Lab image or a pixel-by-palette distance matrix.
func detailSmooth(ctx context.Context, src *image.NRGBA, sigma float64, progress Reporter) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sigma <= 0 {
		return src, nil
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	radius := int(math.Ceil(2 * sigma))
	spatial := make([]float64, radius+1)
	for i := range spatial {
		spatial[i] = math.Exp(-float64(i*i) / (2 * sigma * sigma))
	}
	// A LUT avoids an exponential for every neighbor; interpolation keeps
	// color-distance weights smooth rather than introducing another threshold.
	var rangeWeight [1602]float64
	for i := range rangeWeight {
		rangeWeight[i] = math.Exp(-float64(i) / 100)
	}
	a, b := src, image.NewNRGBA(image.Rect(0, 0, w, h))
	for axis := 0; axis < 2; axis++ {
		length, lines := w, h
		if axis == 1 {
			length, lines = h, w
		}
		jobs := make(chan int)
		done := make(chan struct{}, max(1, min(lines, 8, runtime.GOMAXPROCS(0))))
		var workers sync.WaitGroup
		for worker := 0; worker < cap(done); worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				const tile = 8192
				bufferSize := min(length, tile+2*radius)
				guide := make([]Vec, bufferSize)
				values := make([]RGB, bufferSize)
				alpha := make([]uint8, bufferSize)
				for line := range jobs {
					if ctx.Err() != nil {
						continue
					}
					for start := 0; start < length; start += tile {
						if ctx.Err() != nil {
							break
						}
						lo, hi := max(0, start-radius), min(length, start+tile+radius)
						for pos := lo; pos < hi; pos++ {
							x, y := pos, line
							if axis == 1 {
								x, y = line, pos
							}
							i, j := y*src.Stride+x*4, y*a.Stride+x*4
							guide[pos-lo] = ToLab(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]})
							alpha[pos-lo] = src.Pix[i+3]
							values[pos-lo] = RGB{a.Pix[j], a.Pix[j+1], a.Pix[j+2]}
						}
						for pos := start; pos < min(length, start+tile); pos++ {
							if pos%128 == 0 && ctx.Err() != nil {
								break
							}
							x, y := pos, line
							if axis == 1 {
								x, y = line, pos
							}
							j := y*b.Stride + x*4
							b.Pix[j+3] = alpha[pos-lo]
							if alpha[pos-lo] == 0 {
								continue
							}
							sum, mass := Vec{}, 0.
							for q := max(0, pos-radius); q <= min(length-1, pos+radius); q++ {
								if alpha[q-lo] == 0 {
									continue
								}
								d := distance(guide[pos-lo], guide[q-lo]) * 2
								if d >= 1600 {
									continue
								}
								k := int(d)
								weight := (rangeWeight[k] + (d-float64(k))*(rangeWeight[k+1]-rangeWeight[k])) * spatial[max(pos-q, q-pos)] * float64(alpha[q-lo])
								mass += weight
								for c := range sum {
									sum[c] += weight * float64(values[q-lo][c])
								}
							}
							for c := range sum {
								b.Pix[j+c] = byteRound(sum[c] / mass)
							}
						}
					}
					select {
					case done <- struct{}{}:
					case <-ctx.Done():
					}
				}
			}()
		}
		go func() {
			defer close(jobs)
			for line := 0; line < lines; line++ {
				select {
				case jobs <- line:
				case <-ctx.Done():
					return
				}
			}
		}()
		go func() { workers.Wait(); close(done) }()
		completed := 0
		for range done {
			completed++
			if completed%max(1, lines/20) == 0 {
				_ = report(ctx, progress, "Smoothing colors while preserving edges", .005+.02*(float64(axis)+float64(completed)/float64(lines))/2)
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a = b
		if axis == 0 {
			b = image.NewNRGBA(a.Bounds())
		}
	}
	return a, nil
}
