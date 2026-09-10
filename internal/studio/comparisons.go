package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"encoding/base64"
	"fmt"
	"image"
)

type Comparison struct {
	Name          string               `json:"name"`
	Source        string               `json:"source"`
	Options       engine.Options       `json:"options"`
	Filter        engine.LibraryFilter `json:"filter"`
	LibrarySHA256 string               `json:"librarySHA256,omitempty"`
	Image         string               `json:"image"`
	Quality       engine.Quality       `json:"quality"`
	Colors        int                  `json:"colors"`
	Spools        int                  `json:"spools"`
	Swaps         int                  `json:"swaps"`
	Depth         float64              `json:"depth"`
	BoundaryStep  float64              `json:"boundaryStep"`
	SHA256        string               `json:"rgbaSHA256"`
}

func comparison(ctx context.Context, name, source string, req Request, r *engine.Result) (Comparison, error) {
	c := Comparison{Name: name, Source: source, Options: req.Options, Filter: req.Filter, Quality: r.Quality, Colors: r.UniqueColors, SHA256: r.SHA256}
	if r.Stack != nil {
		c.Spools = r.Stack.UniqueFilaments
		c.Swaps = max(0, len(r.Stack.Runs)-1)
		c.Depth = r.Stack.PlannedDepth
		c.LibrarySHA256 = r.Stack.LibrarySHA256
	}
	if r.Guidance != nil {
		c.Spools = len(r.Guidance.Selected)
		c.LibrarySHA256 = r.Guidance.LibrarySHA256
	}
	if r.SurfaceView != nil {
		c.BoundaryStep = r.SurfaceView.MeanJumpMM
	}
	w, h := r.Image.Bounds().Dx(), r.Image.Bounds().Dy()
	scale := float64(max(w, h)) / 640
	if scale < 1 {
		scale = 1
	}
	nw, nh := max(1, int(float64(w)/scale)), max(1, int(float64(h)/scale))
	thumb := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		if err := ctx.Err(); err != nil {
			return c, err
		}
		for x := 0; x < nw; x++ {
			thumb.SetNRGBA(x, y, r.Image.NRGBAAt(r.Image.Rect.Min.X+x*w/nw, r.Image.Rect.Min.Y+y*h/nh))
		}
	}
	var png bytes.Buffer
	if err := engine.WritePNG(ctx, &png, thumb, [2]float64{}); err != nil {
		return c, err
	}
	c.Image = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png.Bytes())
	return c, nil
}

func (s *Studio) CaptureComparison(req Request) ([]Comparison, error) {
	s.mu.RLock()
	r, source := s.result, s.source
	if r == nil || !sameSettings(req, s.resultRequest) {
		s.mu.RUnlock()
		return nil, fmt.Errorf("refresh the preview before saving a comparison")
	}
	s.mu.RUnlock()
	c, err := comparison(s.ctx, "Saved result", source.Name, req, r)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.settings.Comparisons = append(s.settings.Comparisons, c)
	if len(s.settings.Comparisons) > 4 {
		s.settings.Comparisons = s.settings.Comparisons[len(s.settings.Comparisons)-4:]
	}
	out := append([]Comparison(nil), s.settings.Comparisons...)
	s.mu.Unlock()
	return out, s.saveSettings()
}
func (s *Studio) ClearComparisons() error {
	s.mu.Lock()
	s.settings.Comparisons = nil
	s.mu.Unlock()
	return s.saveSettings()
}

// Compare runs on the same serialized worker and never publishes over the
// current rendered result. Applying a candidate uses the normal guarded path.
func (s *Studio) ComparePlans(req Request) ([]Comparison, error) {
	if req.Options.Mode != "stack" {
		return nil, fmt.Errorf("plan comparisons require Color Match or Color Pop stack planning")
	}
	return s.comparePlans(req, "")
}
func (s *Studio) CompareWithout(req Request, key string) ([]Comparison, error) {
	if req.Options.Mode == "standard" || key == "" {
		return nil, fmt.Errorf("choose a selected filament to compare")
	}
	return s.comparePlans(req, key)
}
func (s *Studio) comparePlans(req Request, excluded string) ([]Comparison, error) {
	if err := req.Options.Validate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if req.Revision != s.revision || s.image == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("image changed; try again")
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.cancel = cancel
	s.job++
	job := s.job
	src, source := s.image, s.source
	s.mu.Unlock()
	defer cancel()
	s.worker.Lock()
	defer s.worker.Unlock()
	raw, err := s.libraryBytes(req.LibraryPath)
	if err != nil {
		return nil, err
	}
	lib, err := engine.ParseLibrary(raw, req.Filter)
	if err != nil {
		return nil, err
	}
	names := []string{"Best color", "Cleaner boundaries", "Fewer swaps", "Thinner print"}
	if req.Options.ColorPop.Enabled {
		names = []string{"Current bands", "More color height", "Fewer swaps", "Reversed regions"}
		if req.Options.ColorPop.ColorPercent > 80 {
			names[1] = "More grayscale height"
		}
	}
	if excluded != "" {
		names = []string{"Current settings", "Without selected spool"}
	}
	out := []Comparison{}
	for i, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidate := req
		candidateLib := lib
		o := req.Options
		if excluded != "" {
			if i == 1 {
				found := false
				candidate.Filter.ExcludedIDs = append([]int(nil), req.Filter.ExcludedIDs...)
				for _, f := range lib.Filaments {
					if engine.FilamentKey(f) == excluded {
						candidate.Filter.ExcludedIDs = append(candidate.Filter.ExcludedIDs, f.SourceIndex)
						name = "Without " + f.Name
						found = true
					}
				}
				if !found {
					return nil, fmt.Errorf("this spool is no longer in the eligible library")
				}
				candidateLib, err = engine.ParseLibrary(raw, candidate.Filter)
				if err != nil {
					return nil, err
				}
			}
		} else if o.ColorPop.Enabled {
			switch i {
			case 1:
				if o.ColorPop.ColorPercent > 80 {
					o.ColorPop.ColorPercent -= 10
				} else {
					o.ColorPop.ColorPercent += 10
				}
			case 2:
				runs := o.HueForge.MaxRuns
				if runs == 0 {
					runs = 8
				}
				o.HueForge.MaxRuns = max(1, runs-1)
			case 3:
				o.ColorPop.GrayOnTop = !o.ColorPop.GrayOnTop
			}
		} else {
			o.HueForge.ReduceShowThrough = false
			o.HueForge.AutoDepth = false
			// Snap an automatic ceiling down before evaluating fixed-depth plans.
			if req.Options.HueForge.AutoDepth {
				o.HueForge.MaxDepth = req.Options.HueForge.Height(req.Options.HueForge.MaxLayers())
			}
			switch i {
			case 1:
				o.HueForge.ReduceShowThrough = true
			case 2:
				if o.HueForge.MaxRuns > 0 {
					o.HueForge.MaxRuns = max(1, o.HueForge.MaxRuns-1)
				} else if o.Colors > 1 {
					o.Colors--
				}
			case 3:
				if o.HueForge.OpticalModel != engine.FrontlitModel {
					continue
				}
				o.HueForge.AutoDepth = true
			}
		}
		candidate.Options = o
		s.emit("progress", Progress{req.ID, engine.Progress{Stage: "Comparing " + name, Fraction: float64(i) / 4}})
		r, e := s.processor.Process(ctx, src, o, &candidateLib, nil)
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if i == 2 {
				continue
			}
			return nil, e
		}
		c, e := comparison(ctx, name, source.Name, candidate, r)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	s.mu.RLock()
	current := s.job == job && s.revision == req.Revision
	s.mu.RUnlock()
	if !current {
		return nil, context.Canceled
	}
	return out, ctx.Err()
}
