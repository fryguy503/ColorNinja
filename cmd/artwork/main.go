// Generate the repository's original code-drawn application icon.
package main

import (
	"bytes"
	"encoding/binary"
	"github.com/disintegration/imaging"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	const size = 1024
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			u, v := (float64(x)-512)/512, (float64(y)-512)/512
			radius := .25
			qx, qy := math.Max(math.Abs(u)-.65, 0), math.Max(math.Abs(v)-.65, 0)
			if math.Hypot(qx, qy) > radius {
				continue
			}
			c := color.NRGBA{185, 216, 188, 255}
			d := math.Hypot(u, v)
			a := math.Atan2(v, u) + math.Pi
			sector := math.Mod(a, math.Pi/3)
			edge := .32 / math.Cos(sector-math.Pi/6)
			if d < .64 && d > edge {
				c = color.NRGBA{30, 49, 38, 255}
				if math.Abs(sector-.15) < .055 && d < .60 {
					c = color.NRGBA{185, 216, 188, 255}
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var encoded bytes.Buffer
	if e := png.Encode(&encoded, imaging.Resize(img, 256, 256, imaging.Lanczos)); e != nil {
		panic(e)
	}
	if e := os.MkdirAll("build/windows", 0755); e != nil {
		panic(e)
	}
	if e := os.WriteFile("build/appicon.png", encoded.Bytes(), 0644); e != nil {
		panic(e)
	}
	// Modern Windows ICO supports a PNG-compressed 256px image (zero dimensions).
	icon := make([]byte, 22)
	binary.LittleEndian.PutUint16(icon[2:], 1)
	binary.LittleEndian.PutUint16(icon[4:], 1)
	binary.LittleEndian.PutUint16(icon[10:], 1)
	binary.LittleEndian.PutUint16(icon[12:], 32)
	binary.LittleEndian.PutUint32(icon[14:], uint32(encoded.Len()))
	binary.LittleEndian.PutUint32(icon[18:], 22)
	icon = append(icon, encoded.Bytes()...)
	if e := os.WriteFile("build/windows/icon.ico", icon, 0644); e != nil {
		panic(e)
	}
}
