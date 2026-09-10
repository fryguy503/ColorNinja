package engine

import "math"

const FrontlitModel = "hueforge-0.9.4.3-frontlit-v1"
const LegacyModel = "legacy-exponential"

// frontlitState is a value so beam branches never share mutable optical history.
// HueForge blends the whole consecutive run from its saved underlying color.
// Feeding the preceding layer into a one-layer blend is a different model.
type frontlitState struct {
	color, substrate [3]float32 // complement RGB (HueForge's CMY, with K=0)
	depth            float64
	previousL        float64
}

func newFrontlit(light ...[3]float32) frontlitState {
	s := frontlitState{color: [3]float32{.5, .5, .5}}
	if len(light) > 0 {
		for i, v := range light[0] {
			s.color[i] = .5 - (1 - v)
		}
	}
	s.previousL = frontlitL(s.rgb())
	return s
}
func (h HueForgeOptions) light() [3]float32 {
	switch h.LightPreset {
	case "hueforge-default":
		return [3]float32{1, .93, .82}
	case "warm-white":
		return [3]float32{1, .85, .72}
	default:
		return [3]float32{1, 1, 1}
	}
}
func clamp32(v float32) float32 { return max(float32(0), min(float32(1), v)) }
func sqrt32(v float32) float32  { return float32(math.Sqrt(float64(v))) }
func (s frontlitState) rgb() (out [3]float32) {
	for i := range out {
		out[i] = clamp32(1 - s.color[i])
	}
	return
}
func (s frontlitState) RGB() (out RGB) {
	for i, v := range s.rgb() {
		out[i] = byteRound(float64(v) * 255)
	}
	return
}

// The Front Lit compatibility conversion truncates RGB to an 8-bit lookup index.
// Its older XYZ constants and low-light branch differ from our output metric.
func frontlitL(rgb [3]float32) float64 {
	v := [3]float64{}
	for i, c := range rgb {
		v[i] = frontlitLinear[max(0, min(255, int(float64(c)*255)))]
	}
	y := ((v[1]*.7152 + v[0]*.2126 + v[2]*.0722) * 100) / 100
	if y <= .008856 {
		y = y*7.787 + .137931
	} else {
		y = math.Pow(y, 1.0/3)
	}
	return (y*116 - 16) * .01
}

var frontlitLinear = func() (v [256]float64) {
	for i := range v {
		v[i] = linear(float64(i))
	}
	return
}()

func (s *frontlitState) step(f Filament, height float64, newRun bool) {
	if newRun {
		s.substrate, s.depth = s.color, height
	} else {
		s.depth = float64(float32(s.depth)) + height
	}
	td := float32(f.TD * float64(float32(.2)) * .5)
	if td == 0 {
		td = .001
	}
	depth := float32(s.depth)
	a := sqrt32(clamp32(depth / td))
	rgb := [3]float32{}
	for i, c := range f.RGB {
		rgb[i] = float32(c) / 255
	}
	l := float32(frontlitL(rgb))
	if s.previousL-float64(l) > 0 && l < .5 {
		factor := math.Max(s.previousL/(1-(s.previousL-float64(l))), 1)
		adjusted := sqrt32(clamp32(depth / sqrt32(float32(factor)*td)))
		weight := clamp32((1 - float32(f.TD)) / .4)
		a = float32((1-weight)*adjusted) + float32(weight*a)
	}
	remaining := float32(1 - float64(a))
	for i := range s.color {
		s.color[i] = float32((1-rgb[i])*a) + float32(s.substrate[i]*remaining)
	}
	s.previousL = frontlitL(s.rgb())
}

// Optical state shared by guidance, beam expansion, rebuilding, and exports.
type opticalState struct {
	front   frontlitState
	back    backlitState
	current Vec
}

func newOptics(f Filament, h HueForgeOptions) opticalState {
	return opticalState{front: newFrontlit(h.light()), back: newBacklit(h), current: LinearRGB(f.RGB)}
}
func (s *opticalState) layer(f Filament, height float64, h HueForgeOptions, newRun bool) RGB {
	if h.backlit() {
		s.back.step(f, height, h)
		return s.back.RGB()
	}
	if h.frontlit() {
		s.front.step(f, height, newRun)
		return s.front.RGB()
	}
	s.current = blend(s.current, f, h)
	return FromLinear(s.current)
}

func baseOptics(f Filament, h HueForgeOptions) opticalState {
	s := newOptics(f, h)
	if h.compatibleOptics() {
		for i := 1; i <= h.BaseLayers(); i++ {
			height := h.LayerHeight
			if i == 1 {
				height = h.FirstHeight()
			}
			s.layer(f, height, h, i == 1)
		}
	}
	return s
}
func (s *opticalState) step(f Filament, h HueForgeOptions, newRun bool) RGB {
	return s.layer(f, h.LayerHeight, h, newRun)
}
func (s opticalState) RGB(h HueForgeOptions) RGB {
	if h.backlit() {
		return s.back.RGB()
	}
	if h.frontlit() {
		return s.front.RGB()
	}
	return FromLinear(s.current)
}
