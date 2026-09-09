package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"math"
	"runtime"
	"sort"
	"sync"
)

func AnalysisSize(w, h, limit int) (int, int) {
	if limit == 0 || int64(w)*int64(h) <= int64(limit) {
		return w, h
	}
	scale := math.Sqrt(float64(limit) / (float64(w) * float64(h)))
	aw, ah := max(1, int(math.Floor(float64(w)*scale+.5))), max(1, int(math.Floor(float64(h)*scale+.5)))
	if aw*ah > limit {
		if aw >= ah {
			aw = max(1, limit/ah)
		} else {
			ah = max(1, limit/aw)
		}
	}
	return aw, ah
}

type point struct {
	lab    Vec
	mass   float64
	detail bool
}
type histBin struct {
	sum        Vec
	mass       float64
	detail     bool
	fullDetail bool
}

func Discover(ctx context.Context, src *image.NRGBA, o Options, progress Reporter) ([]PaletteEntry, [2]int, error) {
	if err := o.Validate(); err != nil {
		return nil, [2]int{}, err
	}
	if src == nil {
		return nil, [2]int{}, fmt.Errorf("no image loaded")
	}
	if !o.LegacyColorPipeline {
		var err error
		src, err = detailSmoothWithTolerance(ctx, src, o.PreblurSigma, o.SmoothingColorSigma, progress)
		if err != nil {
			return nil, [2]int{}, err
		}
	}
	return discoverPrepared(ctx, src, o, progress)
}

func discoverPrepared(ctx context.Context, src *image.NRGBA, o Options, progress Reporter) ([]PaletteEntry, [2]int, error) {
	if err := o.Validate(); err != nil {
		return nil, [2]int{}, err
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w < 1 || h < 1 {
		return nil, [2]int{}, fmt.Errorf("image dimensions must be positive")
	}
	aw, ah := AnalysisSize(w, h, o.AnalysisMaxPixels)
	size := [2]int{aw, ah}
	if err := report(ctx, progress, "Preparing analysis", .03); err != nil {
		return nil, size, err
	}
	a := src
	if aw != w || ah != h {
		var err error
		a, err = resizeAnalysis(ctx, src, aw, ah)
		if err != nil {
			return nil, size, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, size, err
	}
	blurred := a
	if o.LegacyColorPipeline && o.PreblurSigma > 0 {
		sigma := math.Max(.5, math.Min(o.PreblurSigma, o.PreblurSigma*float64(aw)/float64(w)))
		var err error
		blurred, err = analysisBlur(ctx, a, sigma)
		if err != nil {
			return nil, size, err
		}
	}
	if err := report(ctx, progress, "Building color histogram", .10); err != nil {
		return nil, size, err
	}
	bins := make(map[int]*histBin)
	shift := 8 - o.HistogramBits
	trackSupport := o.PreserveDetails || o.prioritizeColors()
	// Vertical support cannot reach four pixels on shorter images. Avoid
	// allocating scanline state for that case and for legacy reduction.
	var previousCodes []uint32
	var vertical []uint8
	if trackSupport && ah >= 4 {
		previousCodes, vertical = make([]uint32, aw), make([]uint8, aw)
	}
	// Two rows of directed edge chains retain diagonal and curved marks without
	// treating disconnected pixels as one feature. Length is capped at four.
	var priorChain, chain []uint8
	var chainCodes []uint32
	if trackSupport {
		priorChain, chain, chainCodes = make([]uint8, aw), make([]uint8, aw), make([]uint32, aw)
	}
	for y := 0; y < ah; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, size, err
			}
		}
		horizontal, lastCode := 0, -1
		if trackSupport {
			clear(chain)
		}
		for x := 0; x < aw; x++ {
			i := y*a.Stride + x*4
			j := y*blurred.Stride + x*4
			mass := float64(a.Pix[i+3]) / 255
			if mass == 0 {
				if len(vertical) > 0 {
					previousCodes[x], vertical[x] = 0, 0
				}
				horizontal, lastCode = 0, -1
				continue
			}
			r, g, b := blurred.Pix[j], blurred.Pix[j+1], blurred.Pix[j+2]
			code := int(r>>shift)<<(2*o.HistogramBits) | int(g>>shift)<<o.HistogramBits | int(b>>shift)
			bin := bins[code]
			if bin == nil {
				bin = &histBin{}
				bins[code] = bin
			}
			bin.mass += mass
			if trackSupport {
				length := uint8(1)
				for px := max(0, x-1); px <= min(aw-1, x+1); px++ {
					if chainCodes[px] == uint32(code+1) {
						length = max(length, min(4, priorChain[px]+1))
					}
				}
				if x > 0 && lastCode == code {
					length = max(length, min(4, chain[x-1]+1))
				}
				chain[x] = length
				if lastCode == code {
					horizontal++
				} else {
					horizontal = 1
				}
				verticalSupport := false
				if len(vertical) > 0 {
					if previousCodes[x] == uint32(code+1) {
						vertical[x] = min(4, vertical[x]+1)
					} else {
						vertical[x] = 1
					}
					previousCodes[x] = uint32(code + 1)
					verticalSupport = vertical[x] >= 4
				}
				lastCode = code
				bin.detail = bin.detail || horizontal >= 4 || verticalSupport || length >= 4
			}
			bin.sum[0] += float64(r) * mass
			bin.sum[1] += float64(g) * mass
			bin.sum[2] += float64(b) * mass
		}
		if trackSupport {
			for x := 0; x < aw; x++ {
				i := y*blurred.Stride + x*4
				chainCodes[x] = 0
				if a.Pix[y*a.Stride+x*4+3] > 0 {
					chainCodes[x] = uint32(int(blurred.Pix[i]>>shift)<<(2*o.HistogramBits)|int(blurred.Pix[i+1]>>shift)<<o.HistogramBits|int(blurred.Pix[i+2]>>shift)) + 1
				}
			}
			priorChain, chain = chain, priorChain
		}
	}
	if trackSupport && (aw != w || ah != h) {
		if err := supplementDetails(ctx, src, bins, o, float64(aw*ah)/float64(w*h)); err != nil {
			return nil, size, err
		}
	}
	codes := make([]int, 0, len(bins))
	for k := range bins {
		codes = append(codes, k)
	}
	sort.Ints(codes)
	var groups [2][]point
	combined := (o.TotalColors && o.Mode == "standard") || o.prioritizeColors()
	total := 0.0
	for _, k := range codes {
		b := bins[k]
		v := b.sum
		for c := range v {
			v[c] /= b.mass
		}
		lab := labFloats(v)
		group := 0
		if !combined && math.Hypot(lab[1], lab[2]) < o.NeutralChroma {
			group = 1
		}
		if !o.LegacyColorPipeline {
			lab = okLabLinear(Vec{linear(v[0]), linear(v[1]), linear(v[2])})
			lab[1] *= o.chromaPriority()
			lab[2] *= o.chromaPriority()
		}
		groups[group] = append(groups[group], point{lab: lab, mass: b.mass, detail: b.detail && (b.mass >= 4 || b.fullDetail)})
		total += b.mass
	}
	if total == 0 {
		return []PaletteEntry{entry(RGB{}, 1, o.NeutralChroma)}, size, nil
	}
	result := []PaletteEntry{}
	seen := map[RGB]int{}
	for group, pts := range groups {
		if err := report(ctx, progress, []string{"Clustering colors", "Clustering neutrals"}[group], .15+float64(group)*.15); err != nil {
			return nil, size, err
		}
		var centers []Vec
		var masses []float64
		var err error
		if o.prioritizeColors() {
			budget := o.Colors
			if !o.TotalColors || o.Mode != "standard" {
				budget *= 2
			}
			centers, masses, err = priorityCluster(ctx, pts, o, budget)
		} else {
			centers, masses, err = cluster(ctx, pts, o)
		}
		if err != nil {
			return nil, size, err
		}
		indices := make([]int, len(centers))
		for i := range indices {
			indices[i] = i
		}
		sort.SliceStable(indices, func(i, j int) bool { return masses[indices[i]] > masses[indices[j]] })
		for _, i := range indices {
			rgb := o.colorRGB(centers[i])
			f := masses[i] / total
			if old, ok := seen[rgb]; ok {
				result[old].Fraction += f
			} else {
				e := entry(rgb, f, o.NeutralChroma)
				if !combined {
					e.Population = []string{"chromatic", "achromatic"}[group]
				}
				seen[rgb] = len(result)
				result = append(result, e)
			}
		}
	}
	return protectPalette(result, o), size, nil
}
func assign(pts []point, centers []Vec) ([]int, []float64, []float64) {
	labels := make([]int, len(pts))
	dist := make([]float64, len(pts))
	masses := make([]float64, len(centers))
	for i, p := range pts {
		best := math.Inf(1)
		for j, c := range centers {
			d := distance(p.lab, c)
			if d < best {
				best = d
				labels[i] = j
			}
		}
		dist[i] = best
		masses[labels[i]] += p.mass
	}
	return labels, dist, masses
}
func refine(ctx context.Context, pts []point, centers []Vec, iterations int) ([]Vec, []float64, error) {
	var previous []int
	for iter := 0; iter < iterations; iter++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		labels, dist, masses := assign(pts, centers)
		updated := make([]Vec, len(centers))
		for i, p := range pts {
			for c := 0; c < 3; c++ {
				updated[labels[i]][c] += p.mass * p.lab[c]
			}
		}
		used := map[int]bool{}
		for j := range centers {
			if masses[j] > 0 {
				for c := range updated[j] {
					updated[j][c] /= masses[j]
				}
			} else {
				best := -1.
				index := 0
				for i, p := range pts {
					if !used[i] && p.mass*dist[i] > best {
						best = p.mass * dist[i]
						index = i
					}
				}
				updated[j] = pts[index].lab
				used[index] = true
			}
		}
		stable := len(previous) == len(labels)
		for i := range labels {
			if !stable || previous[i] != labels[i] {
				stable = false
				break
			}
		}
		movement := 0.
		for i := range centers {
			movement = math.Max(movement, distance(updated[i], centers[i]))
		}
		centers = updated
		previous = labels
		if stable || movement < 1e-10 {
			break
		}
	}
	_, _, masses := assign(pts, centers)
	return centers, masses, nil
}
func cluster(ctx context.Context, pts []point, o Options) ([]Vec, []float64, error) {
	if len(pts) == 0 {
		return nil, nil, nil
	}
	mean := Vec{}
	total := 0.
	for _, p := range pts {
		total += p.mass
		for c := range mean {
			mean[c] += p.lab[c] * p.mass
		}
	}
	for c := range mean {
		mean[c] /= total
	}
	first := 0
	best := math.Inf(1)
	for i, p := range pts {
		if d := distance(p.lab, mean); d < best {
			best = d
			first = i
		}
	}
	centers := []Vec{pts[first].lab}
	chosen := map[int]bool{first: true}
	nearest := make([]float64, len(pts))
	for i, p := range pts {
		nearest[i] = distance(p.lab, centers[0])
	}
	for len(centers) < min(o.Colors, len(pts)) {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		candidate := -1
		score := -1.
		for i, p := range pts {
			if !chosen[i] && p.mass*nearest[i] > score {
				candidate = i
				score = p.mass * nearest[i]
			}
		}
		if candidate < 0 {
			break
		}
		chosen[candidate] = true
		centers = append(centers, pts[candidate].lab)
		for i, p := range pts {
			nearest[i] = math.Min(nearest[i], distance(p.lab, pts[candidate].lab))
		}
	}
	centers, masses, err := refine(ctx, pts, centers, o.Iterations)
	if err != nil {
		return nil, nil, err
	}
	for len(centers) > 1 {
		protected := make([]bool, len(centers))
		if o.PreserveDetails {
			labels, _, _ := assign(pts, centers)
			for i, p := range pts {
				if p.detail {
					protected[labels[i]] = true
				}
			}
			for i, c := range centers {
				separation := math.Inf(1)
				for j, other := range centers {
					if i != j {
						separation = math.Min(separation, distance(c, other))
					}
				}
				// Keep coherent, visibly distinct marks under the area cutoff;
				// still cull isolated speckles and near-duplicate shading.
				protected[i] = protected[i] && separation >= 8*8
			}
		}
		keep := []Vec{}
		maxMass := 0
		for i, m := range masses {
			if m > masses[maxMass] {
				maxMass = i
			}
			if m >= total*o.MinClusterFraction || protected[i] {
				keep = append(keep, centers[i])
			}
		}
		if len(keep) == len(centers) {
			break
		}
		if len(keep) == 0 {
			keep = append(keep, centers[maxMass])
		}
		centers, masses, err = refine(ctx, pts, keep, min(8, o.Iterations))
		if err != nil {
			return nil, nil, err
		}
	}
	return centers, masses, nil
}

func Process(ctx context.Context, src *image.NRGBA, o Options, lib *Library, progress Reporter) (*Result, error) {
	return (&Processor{}).Process(ctx, src, o, lib, progress)
}
func (p *Processor) process(ctx context.Context, src *image.NRGBA, o Options, lib *Library, progress Reporter) (*Result, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if src == nil {
		return nil, fmt.Errorf("no image loaded")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.Mode != "standard" && (lib == nil || len(lib.Filaments) == 0) {
		return nil, fmt.Errorf("load a filament library with eligible filaments first")
	}
	analysisOptions := o
	if o.Mode != "standard" {
		analysisOptions.Colors = o.HueForge.AnalysisColors
	}
	mappingSource, palette, size, err := p.prepare(ctx, src, o, analysisOptions, progress)
	if err != nil {
		return nil, err
	}
	result := &Result{SourceSize: [2]int{src.Bounds().Dx(), src.Bounds().Dy()}, AnalysisSize: size}
	analysisPalette := palette
	if o.Mode != "standard" && o.TrueBlack {
		normalized := trueBlackLibrary(*lib)
		lib = &normalized
	}
	if o.Mode == "guided" {
		palette, result.Guidance, err = guide(ctx, palette, *lib, o, progress)
	} else if o.Mode == "stack" {
		var boundaries []stackBoundary
		if o.HueForge.ReduceShowThrough {
			if err = report(ctx, progress, "Analyzing neighboring colors", .34); err != nil {
				return nil, err
			}
			boundaries, err = stackBoundaries(ctx, mappingSource, palette, o)
			if err != nil {
				return nil, err
			}
		}
		palette, result.Stack, err = planStack(ctx, palette, *lib, o, progress, boundaries...)
	}
	if err != nil {
		return nil, err
	}
	result.Palette = palette
	if err = report(ctx, progress, "Mapping original pixels", .65); err != nil {
		return nil, err
	}
	result.Image, result.Quality, result.LayerMap, err = mapImage(ctx, src, mappingSource, result.Palette, analysisPalette, o, progress)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(result.Image.Pix)
	if result.Stack != nil && (o.HueForge.RequiredFilaments != "" || o.HueForge.BaseFilament != "" || o.HueForge.HighlightFilament != "") {
		top := 0
		for _, p := range result.Palette {
			if p.PixelFraction > 0 {
				top = max(top, p.StackLayer)
			}
		}
		printed := []int{}
		for _, run := range result.Stack.Runs {
			if run.StartLayer > top {
				break
			}
			for i, f := range lib.Filaments {
				if FilamentKey(f) == FilamentKey(run.Filament) {
					printed = append(printed, i)
					break
				}
			}
		}
		if !completeConstraints(printed, *lib, o) {
			return nil, fmt.Errorf("the required spools are not reached by the mapped image; adjust the palette, depth or constraints")
		}
	}
	result.UniqueColors = usedPaletteColorCount(result.Palette)
	result.SHA256 = fmt.Sprintf("%x", digest)
	result.StackView = buildStackCoreView(result)
	result.SurfaceView, err = BuildSurfaceView(ctx, result, o)
	if err != nil {
		return nil, err
	}
	result.Calibration, err = calibrationInfo(ctx, result, o)
	if err != nil {
		return nil, err
	}
	if err = report(ctx, progress, "Preview ready", 1); err != nil {
		return nil, err
	}
	return result, nil
}

type rowStats struct {
	sum, square, weight, max float64
	mass                     []float64
}

func mapImage(ctx context.Context, src, mappingSource *image.NRGBA, palette, analysisPalette []PaletteEntry, o Options, progress Reporter) (*image.NRGBA, Quality, []uint16, error) {
	stack := o.Mode == "stack"
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	labs := make([]Vec, len(palette))
	metricLabs := make([]Vec, len(palette))
	for i, p := range palette {
		labs[i] = o.colorVector(p.RGB)
		metricLabs[i] = ToLab(p.RGB)
	}
	assignment := make([]int, len(labs))
	for i := range assignment {
		assignment[i] = i
	}
	if o.PreserveDetails && o.Mode != "standard" {
		// Map coherent source color groups to their selected output color. A
		// distant constrained palette must not introduce new thresholds inside
		// a source group that the analysis already decided to flatten.
		sourceLabs := make([]Vec, len(analysisPalette))
		assignment = make([]int, len(sourceLabs))
		for i, p := range analysisPalette {
			sourceLabs[i] = o.colorVector(p.RGB)
			best := math.Inf(1)
			for k, c := range labs {
				if d := distance(sourceLabs[i], c); d < best {
					best, assignment[i] = d, k
				}
			}
		}
		labs = sourceLabs
	}
	rows := make([]rowStats, h)
	var layerMap []uint16
	if stack {
		layerMap = make([]uint16, w*h)
	}
	jobs := make(chan int)
	done := make(chan int, min(8, runtime.GOMAXPROCS(0)))
	var wg sync.WaitGroup
	for worker := 0; worker < cap(done); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range jobs {
				if ctx.Err() != nil {
					continue
				}
				s := rowStats{mass: make([]float64, len(palette))}
				for x := 0; x < w; x++ {
					if x%4096 == 0 && ctx.Err() != nil {
						break
					}
					i := y*src.Stride + x*4
					j := y*out.Stride + x*4
					a := src.Pix[i+3]
					bestIdx := 0
					d := 0.
					if a != 0 {
						m := y*mappingSource.Stride + x*4
						lab := o.colorVector(RGB{mappingSource.Pix[m], mappingSource.Pix[m+1], mappingSource.Pix[m+2]})
						d = math.Inf(1)
						for k, c := range labs {
							if v := distance(lab, c); v < d {
								bestIdx = assignment[k]
								d = v
							}
						}
						// Measure the original image, even when smoothing changed the
						// working pixels or matching used a different color space.
						d = distance(ToLab(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]}), metricLabs[bestIdx])
						weight := float64(a) / 255
						delta := math.Sqrt(d)
						s.sum += delta * weight
						s.square += d * weight
						s.weight += weight
						s.max = math.Max(s.max, delta)
						s.mass[bestIdx] += weight
						if stack {
							layerMap[y*w+x] = uint16(palette[bestIdx].StackLayer)
						}
					}
					copy(out.Pix[j:j+3], palette[bestIdx].RGB[:])
					out.Pix[j+3] = a
				}
				rows[y] = s
				select {
				case done <- y:
				case <-ctx.Done():
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for y := 0; y < h; y++ {
			select {
			case jobs <- y:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(done) }()
	completed := 0
	for range done {
		completed++
		if completed%max(1, h/30) == 0 {
			_ = report(ctx, progress, "Mapping original pixels", .65+.33*float64(completed)/float64(h))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, Quality{}, nil, err
	}
	s := rowStats{mass: make([]float64, len(palette))}
	for _, r := range rows {
		s.sum += r.sum
		s.square += r.square
		s.weight += r.weight
		s.max = math.Max(s.max, r.max)
		for i, m := range r.mass {
			s.mass[i] += m
		}
	}
	q := Quality{Max: s.max}
	if s.weight > 0 {
		q.Mean = s.sum / s.weight
		q.RMS = math.Sqrt(s.square / s.weight)
		for i := range palette {
			palette[i].PixelFraction = s.mass[i] / s.weight
		}
	}
	return out, q, layerMap, nil
}
