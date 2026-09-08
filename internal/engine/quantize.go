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
	lab  Vec
	mass float64
}
type histBin struct {
	sum  Vec
	mass float64
}

func Discover(ctx context.Context, src *image.NRGBA, o Options, progress Reporter) ([]PaletteEntry, [2]int, error) {
	if err := o.Validate(); err != nil {
		return nil, [2]int{}, err
	}
	if src == nil {
		return nil, [2]int{}, fmt.Errorf("no image loaded")
	}
	if o.PreserveDetails {
		var err error
		src, err = detailSmooth(ctx, src, o.PreblurSigma, progress)
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
	if o.PreblurSigma > 0 && !o.PreserveDetails {
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
	for y := 0; y < ah; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, size, err
			}
		}
		for x := 0; x < aw; x++ {
			i := y*a.Stride + x*4
			j := y*blurred.Stride + x*4
			mass := float64(a.Pix[i+3]) / 255
			if mass == 0 {
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
			bin.sum[0] += float64(r) * mass
			bin.sum[1] += float64(g) * mass
			bin.sum[2] += float64(b) * mass
		}
	}
	codes := make([]int, 0, len(bins))
	for k := range bins {
		codes = append(codes, k)
	}
	sort.Ints(codes)
	var groups [2][]point
	total := 0.0
	for _, k := range codes {
		b := bins[k]
		v := b.sum
		for c := range v {
			v[c] /= b.mass
		}
		lab := labFloats(v)
		group := 0
		if math.Hypot(lab[1], lab[2]) < o.NeutralChroma {
			group = 1
		}
		if o.PreserveDetails {
			lab = okLabLinear(Vec{linear(v[0]), linear(v[1]), linear(v[2])})
		}
		groups[group] = append(groups[group], point{lab, b.mass})
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
		centers, masses, err := cluster(ctx, pts, o)
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
				e.Population = []string{"chromatic", "achromatic"}[group]
				seen[rgb] = len(result)
				result = append(result, e)
			}
		}
	}
	return result, size, nil
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
		keep := []Vec{}
		maxMass := 0
		for i, m := range masses {
			if m > masses[maxMass] {
				maxMass = i
			}
			if m >= total*o.MinClusterFraction {
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
	mappingSource := src
	if o.PreserveDetails {
		var err error
		mappingSource, err = detailSmooth(ctx, src, o.PreblurSigma, progress)
		if err != nil {
			return nil, err
		}
	}
	palette, size, err := discoverPrepared(ctx, mappingSource, analysisOptions, progress)
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
		palette, result.Stack, err = planStack(ctx, palette, *lib, o, progress)
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
	result.UniqueColors = usedPaletteColorCount(result.Palette)
	result.SHA256 = fmt.Sprintf("%x", digest)
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
						if o.PreserveDetails {
							d = distance(ToLab(RGB{src.Pix[i], src.Pix[i+1], src.Pix[i+2]}), metricLabs[bestIdx])
						}
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
