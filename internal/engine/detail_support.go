package engine

import (
	"context"
	"image"
	"math"
	"sort"
)

// A bounded full-resolution histogram supplements features lost by Lanczos
// analysis. Only coherent, rare, contrasting bins are added. Their true area
// is transferred from the nearest analysis bin, never invented as extra mass.
func supplementDetails(ctx context.Context, src *image.NRGBA, bins map[int]*histBin, o Options, scale float64) error {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	shift := 8 - o.HistogramBits
	full := map[int]*histBin{}
	codes := make([]int, w)
	previous, current := make([]uint8, w), make([]uint8, w)
	for i := range codes {
		codes[i] = -1
	}
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		clear(current)
		last := -1
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			alpha := float64(src.Pix[i+3]) / 255
			if alpha == 0 {
				last = -1
				continue
			}
			r, g, b := src.Pix[i], src.Pix[i+1], src.Pix[i+2]
			code := int(r>>shift)<<(2*o.HistogramBits) | int(g>>shift)<<o.HistogramBits | int(b>>shift)
			v := full[code]
			if v == nil {
				v = &histBin{}
				full[code] = v
			}
			v.mass += alpha
			v.sum[0] += float64(r) * alpha
			v.sum[1] += float64(g) * alpha
			v.sum[2] += float64(b) * alpha
			n := uint8(1)
			for px := max(0, x-1); px <= min(w-1, x+1); px++ {
				if codes[px] == code {
					n = max(n, min(4, previous[px]+1))
				}
			}
			if last == code && x > 0 {
				n = max(n, min(4, current[x-1]+1))
			}
			current[x] = n
			v.detail = v.detail || n >= 4
			last = code
		}
		for x := 0; x < w; x++ {
			i := y*src.Stride + x*4
			codes[x] = -1
			if src.Pix[i+3] > 0 {
				codes[x] = int(src.Pix[i]>>shift)<<(2*o.HistogramBits) | int(src.Pix[i+1]>>shift)<<o.HistogramBits | int(src.Pix[i+2]>>shift)
			}
		}
		previous, current = current, previous
	}
	ids := []int{}
	for code, b := range full {
		if b.detail && b.mass >= 4 && b.mass <= float64(w*h)*.01 {
			ids = append(ids, code)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if full[ids[i]].mass != full[ids[j]].mass {
			return full[ids[i]].mass > full[ids[j]].mass
		}
		return ids[i] < ids[j]
	})
	analysisIDs := []int{}
	for code := range bins {
		analysisIDs = append(analysisIDs, code)
	}
	sort.Ints(analysisIDs)
	added := 0
	ids = ids[:min(len(ids), 256)]
	for _, code := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		v := full[code]
		if existing := bins[code]; existing != nil {
			existing.detail, existing.fullDetail = true, true
			continue
		}
		rgb := RGB{}
		for c := range rgb {
			rgb[c] = byteRound(v.sum[c] / v.mass)
		}
		lab := o.colorVector(rgb)
		best, nearest := math.Inf(1), -1
		for _, id := range analysisIDs {
			b := bins[id]
			if b.mass <= 0 {
				continue
			}
			c := RGB{}
			for k := range c {
				c[k] = byteRound(b.sum[k] / b.mass)
			}
			if d := distance(lab, o.colorVector(c)); d < best {
				best, nearest = d, id
			}
		}
		if nearest < 0 || best < 64 {
			continue
		}
		donor := bins[nearest]
		mass := math.Min(v.mass*scale, donor.mass*.25)
		if mass <= 0 {
			continue
		}
		fraction := 1 - mass/donor.mass
		donor.mass *= fraction
		for c := range donor.sum {
			donor.sum[c] *= fraction
		}
		b := &histBin{mass: mass, detail: true, fullDetail: true}
		for c := range b.sum {
			b.sum[c] = float64(rgb[c]) * mass
		}
		bins[code] = b
		added++
		if added >= 64 {
			break
		}
	}
	return nil
}
