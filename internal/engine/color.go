package engine

import "math"

func linear(v float64) float64 {
	v /= 255
	if v <= .04045 {
		return v / 12.92
	}
	return math.Pow((v+.055)/1.055, 2.4)
}
func LinearRGB(c RGB) Vec {
	return Vec{linear(float64(c[0])), linear(float64(c[1])), linear(float64(c[2]))}
}
func byteRound(v float64) uint8 { return uint8(math.Floor(math.Max(0, math.Min(255, v)) + .5)) }
func FromLinear(c Vec) RGB {
	var out RGB
	for i, v := range c {
		v = math.Max(0, math.Min(1, v))
		if v <= .0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - .055
		}
		out[i] = byteRound(v * 255)
	}
	return out
}
func ToLab(c RGB) Vec { return labFloats(Vec{float64(c[0]), float64(c[1]), float64(c[2])}) }
func labFloats(c Vec) Vec {
	r, g, b := linear(c[0]), linear(c[1]), linear(c[2])
	xyz := Vec{(.4124564*r + .3575761*g + .1804375*b) / .95047, .2126729*r + .7151522*g + .0721750*b, (.0193339*r + .1191920*g + .9503041*b) / 1.08883}
	d := 6.0 / 29
	for i, v := range xyz {
		if v > d*d*d {
			xyz[i] = math.Cbrt(v)
		} else {
			xyz[i] = v/(3*d*d) + 4.0/29
		}
	}
	return Vec{116*xyz[1] - 16, 500 * (xyz[0] - xyz[1]), 200 * (xyz[1] - xyz[2])}
}
func FromLab(c Vec) RGB {
	fy := (c[0] + 16) / 116
	f := Vec{fy + c[1]/500, fy, fy - c[2]/200}
	d := 6.0 / 29
	for i, v := range f {
		if v > d {
			f[i] = v * v * v
		} else {
			f[i] = 3 * d * d * (v - 4.0/29)
		}
	}
	x, y, z := f[0]*.95047, f[1], f[2]*1.08883
	return FromLinear(Vec{3.2404542*x - 1.5371385*y - .4985314*z, -.9692660*x + 1.8760108*y + .0415560*z, .0556434*x - .2040259*y + 1.0572252*z})
}
func distance(a, b Vec) float64 { x, y, z := a[0]-b[0], a[1]-b[1], a[2]-b[2]; return x*x + y*y + z*z }
func blend(bottom Vec, f Filament, o HueForgeOptions) Vec {
	t := math.Pow(o.TDTransmission, o.LayerHeight/(f.TD*o.TDScale))
	top := LinearRGB(f.RGB)
	for i := range bottom {
		bottom[i] = math.Max(0, math.Min(1, (1-t)*top[i]+t*bottom[i]))
	}
	return bottom
}
