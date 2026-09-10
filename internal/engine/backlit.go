package engine

import "math"

const BacklitModel = "hueforge-0.9.4.3-backlit-v1"

// Backlit keeps absorption history for the complete stack. HueForge's modern
// model retains the legacy single-material foundation, then switches to spectral
// attenuation at the first material change. Value semantics isolate beam nodes.
type backlitState struct {
	cmy, density, color [3]float32
	first               Filament
	started, mixed      bool
	prefixDepth         float64
	mixedDepth          float32
}

func newBacklit(h HueForgeOptions) backlitState {
	s := backlitState{}
	for i, light := range h.light() {
		s.cmy[i] = (1 - light) / float32(h.TDScale)
		s.color[i] = clamp32(1 - s.cmy[i])
	}
	return s
}
func log32(v float32) float32 { return float32(math.Log(float64(v))) }
func exp32(v float32) float32 { return float32(math.Exp(float64(v))) }

func (s *backlitState) step(f Filament, height float64, h HueForgeOptions) {
	if !s.started {
		s.first = f
		s.started = true
	}
	// Optical identity is independent of spool identity or its display name.
	if f.RGB != s.first.RGB || math.Abs(f.TD-s.first.TD) >= .0001 {
		s.mixed = true
	}
	light := h.light()
	if !s.mixed {
		td := float64(float32(h.TDScale)) * f.TD
		if td <= 0 {
			for i := range s.color {
				s.color[i] = 0
				s.density[i] = -log32(.000001)
			}
			return
		}
		n := float64(float32(height)) / td
		q := float32(math.Sqrt(n))
		for i, v := range f.RGB {
			c := float32(1 - float64(v)/255)
			s.cmy[i] += 2 * clamp32(q*(c-s.cmy[i]))
		}
		s.prefixDepth += float64(float32(n))
		low := min(s.cmy[0], s.cmy[1], s.cmy[2])
		if float64(low) < s.prefixDepth {
			for i := range s.cmy {
				s.cmy[i] = float32(float64(s.cmy[i]) + s.prefixDepth - float64(low))
			}
		}
		for i := range s.cmy {
			s.cmy[i] = clamp32(s.cmy[i])
			s.color[i] = clamp32(1 - s.cmy[i])
			s.density[i] = -log32(max(float32(.000001), s.color[i]/max(float32(.000001), light[i])))
		}
		return
	}
	previous := [3]float32{exp32(-s.density[0]), exp32(-s.density[1]), exp32(-s.density[2])}
	high, low := max(previous[0], previous[1], previous[2]), min(previous[0], previous[1], previous[2])
	t := clamp32((high - low - .1) / .35)
	adjustment := (3 - 2*t) * t * t * .42
	rgb := [3]float32{float32(f.RGB[0]) / 255, float32(f.RGB[1]) / 255, float32(f.RGB[2]) / 255}
	peak := max(rgb[0], rgb[1], rgb[2])
	factor := sqrt32(peak)*.78 + .12
	n := float32(height) / max(float32(.001), float32(float64(float32(h.TDScale))*f.TD))
	q := sqrt32(n)
	s.mixedDepth += n
	remaining := max(float32(0), 1-s.mixedDepth)
	for i, v := range rgb {
		ratio := float32(1)
		if peak > 0 {
			ratio = v / peak
		}
		absorption := -log32(max(float32(.000001), (ratio*.92+.08)*factor))
		if high > .0001 {
			weight := clamp32(previous[i] / high)
			absorption *= 1 - adjustment*weight*weight
		}
		s.density[i] += absorption * q
		s.color[i] = clamp32(exp32(-s.density[i]) * light[i] * remaining)
	}
}
func (s backlitState) RGB() (out RGB) {
	for i, v := range s.color {
		out[i] = byteRound(float64(v) * 255)
	}
	return
}
