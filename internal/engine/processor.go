package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"image"
)

// Processor owns bounded, immutable stage snapshots for one document. Callers
// serialize Process and never mutate a source image after passing it here.
type Processor struct {
	source      *image.NRGBA
	smoothed    *image.NRGBA
	smoothKey   Options
	analysisKey Options
	analysis    []PaletteEntry
	size        [2]int
	resultKey   Options
	libraryKey  [32]byte
	result      *Result
	optics      candidateMemo
	opticsKey   [32]byte
	Reused      []string
}

func processingKey(o Options) Options {
	o.CalibrationNote = ""
	o.HueForge.MeshMode = ""
	o.HueForge.MeshCore = ""
	if !o.HueForge.ReduceShowThrough || o.Mode != "stack" {
		o.HueForge.ExportWidthMM = 0
		o.HueForge.MeshDetailMM = 0
	}
	return o
}
func smoothingKey(o Options) Options {
	return Options{LegacyColorPipeline: o.LegacyColorPipeline, PreblurSigma: o.PreblurSigma, SmoothingColorSigma: o.SmoothingColorSigma}
}
func discoveryKey(o Options) Options {
	o.CalibrationNote = ""
	if o.Mode != "standard" {
		o.Mode = "guided"
	}
	o.GuidanceStrength = 0
	o.TrueBlack = false
	o.HueForge = HueForgeOptions{}
	return o
}

func (p *Processor) prepare(ctx context.Context, src *image.NRGBA, o, analysisOptions Options, progress Reporter) (*image.NRGBA, []PaletteEntry, [2]int, error) {
	k := smoothingKey(o)
	if p.smoothed != nil && p.smoothKey == k {
		p.Reused = append(p.Reused, "smoothing")
	} else {
		var err error
		p.smoothed = nil
		p.analysis = nil
		working := src
		if !o.LegacyColorPipeline {
			working, err = detailSmoothWithTolerance(ctx, src, o.PreblurSigma, o.SmoothingColorSigma, progress)
		}
		if err != nil {
			return nil, nil, [2]int{}, err
		}
		p.smoothed, p.smoothKey = working, k
	}
	key := discoveryKey(analysisOptions)
	if p.analysis != nil && p.analysisKey == key {
		p.Reused = append(p.Reused, "palette analysis")
	} else {
		palette, size, err := discoverPrepared(ctx, p.smoothed, analysisOptions, progress)
		if err != nil {
			return nil, nil, size, err
		}
		p.analysis, p.size, p.analysisKey = append([]PaletteEntry(nil), palette...), size, key
	}
	return p.smoothed, append([]PaletteEntry(nil), p.analysis...), p.size, nil
}

func (p *Processor) Process(ctx context.Context, src *image.NRGBA, o Options, lib *Library, progress Reporter) (*Result, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.source != src {
		*p = Processor{source: src}
	}
	p.Reused = nil
	raw, _ := json.Marshal(lib)
	libraryKey := sha256.Sum256(raw)
	key := processingKey(o)
	if p.result != nil && p.resultKey == key && p.libraryKey == libraryKey {
		p.Reused = []string{"rendered result"}
		r := ReframeResult(p.result, o)
		var err error
		r.SurfaceView, err = BuildSurfaceView(ctx, r, o)
		if err != nil {
			return nil, err
		}
		if err := report(ctx, progress, "Updating export geometry", .98); err != nil {
			return nil, err
		}
		return r, ctx.Err()
	}
	opticsKey := sha256.Sum256(append(raw, byte(0)))
	if o.TrueBlack {
		opticsKey = sha256.Sum256(append(raw, byte(1)))
	}
	if p.opticsKey != opticsKey {
		p.optics = candidateMemo{}
		p.opticsKey = opticsKey
	}
	ctx = context.WithValue(ctx, candidateMemoKey{}, &p.optics)
	r, err := p.process(ctx, src, o, lib, progress)
	if err == nil {
		p.result, p.resultKey, p.libraryKey = r, key, libraryKey
	}
	return r, err
}

func ReframeResult(original *Result, o Options) *Result {
	r := *original
	if r.Calibration != nil {
		c := *r.Calibration
		c.Note = o.CalibrationNote
		r.Calibration = &c
	}
	if r.Stack != nil {
		s := *r.Stack
		s.Options = o.HueForge
		r.Stack = &s
	}
	if r.Guidance != nil {
		g := *r.Guidance
		g.Options = o.HueForge
		r.Guidance = &g
	}
	r.StackView = buildStackCoreView(&r)
	return &r
}

type candidateMemoKey struct{}
type candidateMemo struct {
	entries map[string][]guidanceCandidate
	count   int
}

func cachedGuidanceCandidates(ctx context.Context, ids []int, lib Library, h HueForgeOptions) ([]guidanceCandidate, error) {
	memo, _ := ctx.Value(candidateMemoKey{}).(*candidateMemo)
	if memo == nil {
		return guidanceCandidatesUncached(ctx, ids, lib, h)
	}
	h.RequiredFilaments = ""
	h.BaseFilament = ""
	h.HighlightFilament = ""
	h.SearchEffort = ""
	h.BeamWidth = 0
	h.ReduceShowThrough = false
	h.SurfaceColorTolerance = 0
	h.DepthTolerance = 0
	h.TDSensitivityPercent = 0
	h.MeshMode = ""
	h.MeshCore = ""
	h.ExportWidthMM = 0
	h.MeshDetailMM = 0
	keyRaw, _ := json.Marshal(struct {
		IDs []int
		H   HueForgeOptions
	}{sorted(ids), h})
	key := string(keyRaw)
	if out, ok := memo.entries[key]; ok {
		return out, ctx.Err()
	}
	out, err := guidanceCandidatesUncached(ctx, ids, lib, h)
	if err == nil && len(out) <= 8192 {
		if memo.entries == nil || memo.count+len(out) > 262144 {
			memo.entries = map[string][]guidanceCandidate{}
			memo.count = 0
		}
		memo.entries[key] = out
		memo.count += len(out)
	}
	return out, err
}
