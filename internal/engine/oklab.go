package engine

import "math"

// Oklab uses a more uniform lightness/hue geometry for palette selection and
// mapping. Values are scaled by 100 here. Equations: Bjorn Ottosson's public
// domain sRGB matrices, https://bottosson.github.io/posts/oklab/ (2021 revision).
func toOKLab(c RGB) Vec { return okLabLinear(LinearRGB(c)) }
func okLabLinear(c Vec) Vec {
	l := math.Cbrt(.4122214708*c[0] + .5363325363*c[1] + .0514459929*c[2])
	m := math.Cbrt(.2119034982*c[0] + .6806995451*c[1] + .1073969566*c[2])
	s := math.Cbrt(.0883024619*c[0] + .2817188376*c[1] + .6299787005*c[2])
	return Vec{100 * (.2104542553*l + .7936177850*m - .0040720468*s),
		100 * (1.9779984951*l - 2.4285922050*m + .4505937099*s),
		100 * (.0259040371*l + .7827717662*m - .8086757660*s)}
}
func fromOKLab(c Vec) RGB {
	l := (c[0] + .3963377774*c[1] + .2158037573*c[2]) / 100
	m := (c[0] - .1055613458*c[1] - .0638541728*c[2]) / 100
	s := (c[0] - .0894841775*c[1] - 1.2914855480*c[2]) / 100
	l, m, s = l*l*l, m*m*m, s*s*s
	return FromLinear(Vec{4.0767416621*l - 3.3077115913*m + .2309699292*s,
		-1.2684380046*l + 2.6097574011*m - .3413193965*s,
		-.0041960863*l - .7034186147*m + 1.7076147010*s})
}
func (o Options) colorVector(c RGB) Vec {
	if !o.LegacyColorPipeline {
		v := toOKLab(c)
		v[1] *= o.chromaPriority()
		v[2] *= o.chromaPriority()
		return v
	}
	return ToLab(c)
}
func (o Options) colorRGB(v Vec) RGB {
	if !o.LegacyColorPipeline {
		v[1] /= o.chromaPriority()
		v[2] /= o.chromaPriority()
		return fromOKLab(v)
	}
	return FromLab(v)
}
func (o Options) colorVectors(colors []RGB) []Vec {
	v := make([]Vec, len(colors))
	for i, c := range colors {
		v[i] = o.colorVector(c)
	}
	return v
}

// Planning scores use the working space, but exported Delta E76 metrics always
// measure actual CIELAB error, never an Oklab value under a misleading name.
func paletteRMS76(palette []RGB, target []PaletteEntry, o Options) float64 {
	vectors := o.colorVectors(palette)
	sum, mass := 0., 0.
	for _, t := range target {
		v, nearest, best := o.colorVector(t.RGB), 0, math.Inf(1)
		for i, c := range vectors {
			if d := distance(v, c); d < best {
				nearest, best = i, d
			}
		}
		sum += t.Fraction * distance(ToLab(t.RGB), ToLab(palette[nearest]))
		mass += t.Fraction
	}
	if mass == 0 {
		return 0
	}
	return math.Sqrt(sum / mass)
}
