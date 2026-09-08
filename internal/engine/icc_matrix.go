package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/disintegration/imaging"
	"image"
	"math"
)

// Matrix/TRC RGB profiles (including sRGB, Adobe RGB, and Display P3) can be
// evaluated directly without a CMM's interpolated color lookup table.
func convertMatrixICC(ctx context.Context, src image.Image, data []byte) (*image.NRGBA, error, bool) {
	if len(data) < 132 || string(data[16:20]) != "RGB " || string(data[20:24]) != "XYZ " {
		return nil, nil, false
	}
	count := int(binary.BigEndian.Uint32(data[128:]))
	if count > (len(data)-132)/12 {
		return nil, fmt.Errorf("invalid ICC tag table"), true
	}
	tags := map[string][]byte{}
	for i := 0; i < count; i++ {
		p := 132 + i*12
		start, n := int(binary.BigEndian.Uint32(data[p+4:])), int(binary.BigEndian.Uint32(data[p+8:]))
		if start > len(data) || n > len(data)-start {
			return nil, fmt.Errorf("ICC tag exceeds profile bounds"), true
		}
		tags[string(data[p:p+4])] = data[start : start+n]
	}
	if tags["A2B0"] != nil {
		return nil, nil, false
	}
	fixed := func(b []byte) float64 { return float64(int32(binary.BigEndian.Uint32(b))) / 65536 }
	var matrix [3]Vec
	var curves [3][256]float64
	for ch, key := range []string{"r", "g", "b"} {
		xyz, trc := tags[key+"XYZ"], tags[key+"TRC"]
		if len(xyz) < 20 || string(xyz[:4]) != "XYZ " || len(trc) < 12 {
			return nil, nil, false
		}
		matrix[ch] = Vec{fixed(xyz[8:]), fixed(xyz[12:]), fixed(xyz[16:])}
		for i := 0; i < 256; i++ {
			x := float64(i) / 255
			v := 0.
			switch string(trc[:4]) {
			case "curv":
				n := int(binary.BigEndian.Uint32(trc[8:]))
				if n > (len(trc)-12)/2 {
					return nil, fmt.Errorf("invalid ICC tone curve"), true
				}
				if n == 0 {
					v = x
				} else if n == 1 {
					v = math.Pow(x, float64(binary.BigEndian.Uint16(trc[12:]))/256)
				} else {
					pos := x * float64(n-1)
					lo := min(n-1, int(pos))
					hi := min(n-1, lo+1)
					a, b := float64(binary.BigEndian.Uint16(trc[12+lo*2:]))/65535, float64(binary.BigEndian.Uint16(trc[12+hi*2:]))/65535
					v = a + (b-a)*(pos-float64(lo))
				}
			case "para":
				kind := int(binary.BigEndian.Uint16(trc[8:]))
				counts := []int{1, 3, 4, 5, 7}
				if kind > 4 || len(trc) < 12+4*counts[kind] {
					return nil, nil, false
				}
				p := [7]float64{}
				for j := 0; j < counts[kind]; j++ {
					p[j] = fixed(trc[12+j*4:])
				}
				g, a, b, c, d, e, f := p[0], p[1], p[2], p[3], p[4], p[5], p[6]
				switch kind {
				case 0:
					v = math.Pow(x, g)
				case 1:
					if a != 0 && x >= -b/a {
						v = math.Pow(a*x+b, g)
					}
				case 2:
					v = c
					if a != 0 && x >= -b/a {
						v = math.Pow(a*x+b, g) + c
					}
				case 3:
					if x >= d {
						v = math.Pow(a*x+b, g)
					} else {
						v = c * x
					}
				case 4:
					if x >= d {
						v = math.Pow(a*x+b, g) + e
					} else {
						v = c*x + f
					}
				}
			default:
				return nil, nil, false
			}
			if !finite(v) {
				return nil, fmt.Errorf("invalid ICC tone curve value"), true
			}
			curves[ch][i] = v
		}
	}
	out := imaging.Clone(src)
	for y := 0; y < out.Bounds().Dy(); y++ {
		if e := ctx.Err(); e != nil {
			return nil, e, true
		}
		for x := 0; x < out.Bounds().Dx(); x++ {
			i := y*out.Stride + x*4
			var xyz Vec
			for ch := 0; ch < 3; ch++ {
				v := curves[ch][out.Pix[i+ch]]
				for k := 0; k < 3; k++ {
					xyz[k] += matrix[ch][k] * v
				}
			}
			// Bradford chromatic adaptation: ICC PCS D50 to the engine's D65 white.
			a, b, c := xyz[0], xyz[1], xyz[2]
			X := .9555766*a - .0230393*b + .0631636*c
			Y := -.0282895*a + 1.0099416*b + .0210077*c
			Z := .0122982*a - .0204830*b + 1.3299098*c
			rgb := FromLinear(Vec{3.2404542*X - 1.5371385*Y - .4985314*Z, -.9692660*X + 1.8760108*Y + .0415560*Z, .0556434*X - .2040259*Y + 1.0572252*Z})
			copy(out.Pix[i:i+3], rgb[:])
		}
	}
	return out, nil, true
}
