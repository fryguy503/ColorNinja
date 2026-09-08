package engine

import (
	"image"
	"math"
)

// SampleImage is original procedural artwork shipped with the app. It exercises
// continuous tones, fine texture, and flat shapes without an external asset.
func SampleImage() *image.NRGBA {
	const w, h = 1440, 1080
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	mix := func(a, b RGB, t float64) RGB {
		var c RGB
		for i := range c {
			c[i] = byteRound(float64(a[i])*(1-t) + float64(b[i])*t)
		}
		return c
	}
	smooth := func(t float64) float64 { return t * t * (3 - 2*t) }
	for y := 0; y < h; y++ {
		v := float64(y) / h
		for x := 0; x < w; x++ {
			u := float64(x) / w
			c := mix(RGB{224, 222, 207}, RGB{247, 214, 175}, smooth(v))
			dx, dy := (u-.70)*w, (v-.265)*h
			radius := 126.
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist < radius+1 {
				sun := mix(RGB{209, 94, 56}, RGB{232, 139, 83}, v/.4)
				c = mix(c, sun, math.Min(1, radius+1-dist))
			}
			ridge := .48 + .05*math.Sin(u*9) + .05*math.Sin(u*21+.8)
			if v > ridge {
				c = mix(RGB{151, 163, 157}, RGB{116, 139, 136}, (v-ridge)*2)
			}
			ridge = .56 + .07*math.Sin(u*7+1.2) + .035*math.Sin(u*14)
			if v > ridge {
				c = mix(RGB{87, 120, 120}, RGB{59, 99, 107}, (v-ridge)*3)
			}
			ridge = .68 + .11*math.Sin(u*4.5-1.4)
			if v > ridge {
				c = mix(RGB{211, 151, 102}, RGB{234, 187, 130}, math.Min(1, (v-ridge)*4))
				line := math.Sin((v-ridge)*100 + u*7)
				if line > .96 {
					c = mix(c, RGB{247, 215, 164}, .16)
				}
			}
			ridge = .91 - .20*u + .025*math.Sin(u*7)
			if v > ridge {
				c = mix(RGB{168, 92, 63}, RGB{196, 119, 76}, (v-ridge)*3)
			}
			ridge = .96 + .075*math.Sin(u*4-2)
			if v > ridge {
				c = mix(RGB{56, 76, 77}, RGB{41, 64, 70}, (v-ridge)*4)
			}
			noise := float64(((uint32(x)*374761393+uint32(y)*668265263)^(uint32(x*y)*1274126177))%31)/31 - .5
			for i := range c {
				c[i] = byteRound(float64(c[i]) + noise*2.5)
			}
			p := y*img.Stride + x*4
			copy(img.Pix[p:p+3], c[:])
			img.Pix[p+3] = 255
		}
	}
	return img
}
