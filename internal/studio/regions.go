package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
)

type RegionCommand struct {
	ReferenceLayer  int                    `json:"referenceLayer"`
	ID              uint64                 `json:"id"`
	Revision        uint64                 `json:"revision"`
	Version         uint64                 `json:"version"`
	NewID           uint64                 `json:"newId"`
	Action          string                 `json:"action"`
	Selection       engine.RegionSelection `json:"selection"`
	Value           int                    `json:"value"`
	GroupID         uint64                 `json:"groupId"`
	Name            string                 `json:"name"`
	WithinSelection bool                   `json:"withinSelection"`
}
type RegionGroupView struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Operation string `json:"operation"`
	Value     int    `json:"value"`
	Enabled   bool   `json:"enabled"`
	Locked    bool   `json:"locked"`
	Pixels    int    `json:"pixels"`
}
type RegionState struct {
	ID          uint64            `json:"id"`
	Revision    uint64            `json:"revision"`
	Version     uint64            `json:"version"`
	Pixels      int               `json:"pixels"`
	Regions     int               `json:"regions"`
	Minimum     int               `json:"minimum"`
	Maximum     int               `json:"maximum"`
	Overlay     string            `json:"overlay"`
	BaseURL     string            `json:"baseUrl"`
	CanUndo     bool              `json:"canUndo"`
	CanRedo     bool              `json:"canRedo"`
	Groups      []RegionGroupView `json:"groups"`
	PickedLayer int               `json:"pickedLayer,omitempty"`
	Preview     *Preview          `json:"preview,omitempty"`
}
type regionSnapshot struct {
	Document  engine.RegionDocument
	Selection []engine.PixelSpan
}
type regionSession struct {
	Base            *engine.Result
	Document        engine.RegionDocument
	Mask            []bool
	Index           *engine.RegionIndex
	Version         uint64
	History, Future []regionSnapshot
}

func sameRegionFilter(a, b engine.LibraryFilter) bool {
	return sameSettings(Request{Filter: a}, Request{Filter: b})
}
func newRegionSession(ctx context.Context, r *engine.Result) (*regionSession, error) {
	x, err := engine.BuildRegionIndex(ctx, r, r)
	if err != nil {
		return nil, err
	}
	return &regionSession{Base: r, Document: engine.RegionDocument{Version: 1, Width: x.Width, Height: x.Height, BaseSHA256: r.SHA256, NextID: 1, Groups: []engine.RegionGroup{}}, Mask: make([]bool, len(x.Labels)), Index: x, Version: 1}, nil
}
func regionSnapshotOf(s *regionSession) regionSnapshot {
	return regionSnapshot{s.Document, engine.MaskSpans(s.Mask)}
}
func trimRegionHistory(h []regionSnapshot) []regionSnapshot {
	if len(h) > 40 {
		h = h[len(h)-40:]
	}
	size := 0
	for i := len(h) - 1; i >= 0; i-- {
		size += len(h[i].Selection)
		for _, g := range h[i].Document.Groups {
			size += len(g.Mask)
		}
		if size > 4*engine.RegionMaxSpans {
			h = h[i+1:]
			break
		}
	}
	return h
}
func (s *Studio) RegionEditor(id, revision uint64) (*RegionState, error) {
	return s.EditRegions(RegionCommand{ID: id, Revision: revision, Action: "inspect"})
}

func (s *Studio) EditRegions(c RegionCommand) (*RegionState, error) {
	s.worker.Lock()
	defer s.worker.Unlock()
	s.mu.Lock()
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	r, req, src, job, session := s.result, s.resultRequest, s.image, s.job, s.regions
	s.mu.Unlock()
	if r == nil || req.ID != c.ID || req.Revision != c.Revision {
		return nil, fmt.Errorf("preview changed; reopen Region Edit")
	}
	var err error
	if session == nil {
		session, err = newRegionSession(ctx, r)
		if err != nil {
			return nil, err
		}
	}
	if c.Action != "inspect" && c.Version != session.Version {
		return nil, fmt.Errorf("selection changed; retry the command")
	}
	next := *session
	next.Document.Groups = append([]engine.RegionGroup(nil), session.Document.Groups...)
	next.History = append([]regionSnapshot(nil), session.History...)
	next.Future = append([]regionSnapshot(nil), session.Future...)
	changed, record := false, c.Action != "inspect" && c.Action != "pick"
	if c.Action == "undo" || c.Action == "redo" {
		from, to := &next.History, &next.Future
		if c.Action == "redo" {
			from, to = &next.Future, &next.History
		}
		if len(*from) == 0 {
			return nil, fmt.Errorf("nothing to %s", c.Action)
		}
		*to = append(*to, regionSnapshotOf(session))
		snap := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
		next.Document = snap.Document
		next.Mask, err = engine.SpanMask(snap.Selection, len(session.Mask))
		if err != nil {
			return nil, err
		}
		changed = true
		record = false
	} else {
		switch c.Action {
		case "inspect":
		case "select":
			m, e := next.Index.Select(ctx, c.Selection)
			if e != nil {
				return nil, e
			}
			next.Mask, err = engine.CombineRegionMasks(next.Mask, m, c.Selection.Combine)
		case "clear", "all", "invert", "grow", "shrink", "fill", "same-layer", "similar", "speckles":
			previous := next.Mask
			next.Mask, err = next.Index.Refine(ctx, next.Mask, c.Action, c.Value, r, c.ReferenceLayer)
			if err == nil && c.WithinSelection && (c.Action == "same-layer" || c.Action == "similar") {
				next.Mask, err = engine.CombineRegionMasks(previous, next.Mask, "intersect")
			}
		case "pick":
			if len(c.Selection.Points) != 1 {
				return nil, fmt.Errorf("click a layer to sample it")
			}
			p := c.Selection.Points[0]
			if p.X < 0 || p.Y < 0 || p.X >= float64(next.Index.Width) || p.Y >= float64(next.Index.Height) {
				return nil, fmt.Errorf("click inside the image")
			}
			v := int(r.LayerMap[int(p.Y)*next.Index.Width+int(p.X)])
			state, e := s.regionState(&next, r, req)
			if e == nil {
				state.PickedLayer = v
			}
			return state, e
		case "reset":
			next.Document.Groups = []engine.RegionGroup{}
			changed = true
		case "recall", "rename", "enable", "lock", "delete", "split":
			at := -1
			for i, g := range next.Document.Groups {
				if g.ID == c.GroupID {
					at = i
					break
				}
			}
			if at < 0 {
				return nil, fmt.Errorf("edit group no longer exists")
			}
			g := next.Document.Groups[at]
			if g.Locked && c.Action != "lock" && c.Action != "recall" {
				return nil, fmt.Errorf("unlock the group first")
			}
			switch c.Action {
			case "recall":
				next.Mask, err = engine.SpanMask(g.Mask, len(next.Mask))
			case "rename":
				name := strings.TrimSpace(c.Name)
				if name == "" || len(name) > 120 {
					return nil, fmt.Errorf("group name must be 1–120 bytes")
				}
				next.Document.Groups[at].Name = name
				changed = true
			case "enable":
				next.Document.Groups[at].Enabled = !g.Enabled
				changed = true
			case "lock":
				next.Document.Groups[at].Locked = !g.Locked
				changed = true
			case "delete":
				next.Document.Groups = append(next.Document.Groups[:at], next.Document.Groups[at+1:]...)
				changed = true
			case "split":
				if g.Operation == "smooth" {
					return nil, fmt.Errorf("smoothing groups cannot be split; restore and select the islands separately")
				}
				parts, e := splitRegionSpans(ctx, g.Mask, next.Index.Width, next.Index.Height)
				if e != nil {
					return nil, e
				}
				if len(parts) < 2 {
					return nil, fmt.Errorf("group has only one connected area")
				}
				if len(next.Document.Groups)-1+len(parts) > 128 {
					return nil, fmt.Errorf("splitting would exceed 128 groups")
				}
				groups := append([]engine.RegionGroup(nil), next.Document.Groups[:at]...)
				for i, part := range parts {
					copy := g
					copy.ID = next.Document.NextID
					next.Document.NextID++
					copy.Name = fmt.Sprintf("%s %d", g.Name, i+1)
					copy.Mask = part
					groups = append(groups, copy)
				}
				next.Document.Groups = append(groups, next.Document.Groups[at+1:]...)
				changed = true
			}
		case "assign", "shift", "flatten-common", "flatten-high", "flatten-low", "restore", "cut", "smooth", "match-surround":
			mask := append([]bool(nil), next.Mask...)
			for _, g := range next.Document.Groups {
				if g.Enabled && g.Locked {
					for _, sp := range g.Mask {
						for i := sp.Start; i < sp.Start+sp.Length; i++ {
							mask[i] = false
						}
					}
				}
			}
			spans := engine.MaskSpans(mask)
			if len(spans) == 0 {
				return nil, fmt.Errorf("select editable pixels first; locked groups are protected")
			}
			op, value := c.Action, c.Value
			if strings.HasPrefix(op, "flatten-") || op == "match-surround" {
				value, err = regionTarget(next.Index, r.LayerMap, mask, op)
				op = "assign"
			}
			name := strings.TrimSpace(c.Name)
			if name == "" {
				name = fmt.Sprintf("%s %d", strings.ReplaceAll(c.Action, "-", " "), next.Document.NextID)
			}
			next.Document.Groups = append(next.Document.Groups, engine.RegionGroup{ID: next.Document.NextID, Name: name, Enabled: true, Operation: op, Value: value, Mask: spans})
			next.Document.NextID++
			changed = true
		default:
			return nil, fmt.Errorf("unknown region command")
		}
	}
	if err != nil {
		return nil, err
	}
	if err = engine.ValidateSpans(engine.MaskSpans(next.Mask), len(next.Mask)); err != nil {
		return nil, err
	}
	if changed {
		if c.NewID <= c.ID {
			return nil, fmt.Errorf("a new preview identifier is required")
		}
		r, err = engine.ApplyRegionDocument(ctx, next.Base, src, req.Options, next.Document)
		if err != nil {
			return nil, err
		}
		next.Index, err = engine.BuildRegionIndex(ctx, r, next.Base)
		if err != nil {
			return nil, err
		}
		req.ID = c.NewID
	}
	if record {
		next.History = trimRegionHistory(append(next.History, regionSnapshotOf(session)))
		next.Future = nil
	}
	if c.Action != "inspect" {
		next.Version++
	}
	state, err := s.regionState(&next, r, req)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != job || s.resultRequest.ID != c.ID || s.revision != c.Revision {
		return nil, context.Canceled
	}
	s.regions = &next
	if changed {
		s.job++
		s.resultJob = s.job
		s.result = r
		s.resultRequest = req
		state.Preview = &Preview{ID: req.ID, Revision: req.Revision, URL: fmt.Sprintf("/media/result/%d.png", s.resultJob), Result: r, Options: req.Options}
	}
	return state, nil
}

func regionTarget(x *engine.RegionIndex, layers []uint16, mask []bool, op string) (int, error) {
	hist := map[int]int{}
	if op == "match-surround" {
		seen := make([]bool, len(mask))
		for i, v := range mask {
			if !v {
				continue
			}
			visit := func(j int) {
				if j >= 0 && j < len(mask) && !mask[j] && !seen[j] && layers[j] > 0 {
					seen[j] = true
					hist[int(layers[j])]++
				}
			}
			if i%x.Width > 0 {
				visit(i - 1)
			}
			if i%x.Width < x.Width-1 {
				visit(i + 1)
			}
			visit(i - x.Width)
			visit(i + x.Width)
		}
	} else {
		for i, v := range mask {
			if v && layers[i] > 0 {
				hist[int(layers[i])]++
			}
		}
	}
	if len(hist) == 0 {
		return 0, fmt.Errorf("selection has no printable reference layer")
	}
	keys := []int{}
	for l := range hist {
		keys = append(keys, l)
	}
	sort.Ints(keys)
	if op == "flatten-high" {
		return keys[len(keys)-1], nil
	}
	if op == "flatten-low" {
		return keys[0], nil
	}
	best := keys[0]
	for _, l := range keys {
		if hist[l] > hist[best] {
			best = l
		}
	}
	return best, nil
}
func splitRegionSpans(ctx context.Context, spans []engine.PixelSpan, w, h int) ([][]engine.PixelSpan, error) {
	mask, err := engine.SpanMask(spans, w*h)
	if err != nil {
		return nil, err
	}
	parts := [][]engine.PixelSpan{}
	for i, v := range mask {
		if !v {
			continue
		}
		if len(parts) >= 128 {
			return nil, fmt.Errorf("too many disconnected parts")
		}
		q := []int{i}
		mask[i] = false
		for head := 0; head < len(q); head++ {
			if head%16384 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			p := q[head]
			visit := func(j int) {
				if j >= 0 && j < len(mask) && mask[j] {
					mask[j] = false
					q = append(q, j)
				}
			}
			if p%w > 0 {
				visit(p - 1)
			}
			if p%w < w-1 {
				visit(p + 1)
			}
			visit(p - w)
			visit(p + w)
		}
		sort.Ints(q)
		runs := []engine.PixelSpan{}
		for _, p := range q {
			if len(runs) > 0 && runs[len(runs)-1].Start+runs[len(runs)-1].Length == p {
				runs[len(runs)-1].Length++
			} else {
				runs = append(runs, engine.PixelSpan{Start: p, Length: 1})
			}
		}
		parts = append(parts, runs)
	}
	return parts, nil
}
func (s *Studio) regionState(session *regionSession, r *engine.Result, req Request) (*RegionState, error) {
	x := session.Index
	out := &RegionState{ID: req.ID, Revision: req.Revision, Version: session.Version, Minimum: 4097, CanUndo: len(session.History) > 0, CanRedo: len(session.Future) > 0, Groups: []RegionGroupView{}, BaseURL: fmt.Sprintf("/media/region-base/%d/%s.png", req.Revision, session.Base.SHA256)}
	ids := map[int32]bool{}
	scale := max(1., float64(max(x.Width, x.Height))/1200)
	w, h := max(1, int(float64(x.Width)/scale)), max(1, int(float64(x.Height)/scale))
	overlay := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i, v := range session.Mask {
		if !v {
			continue
		}
		out.Pixels++
		ids[x.Labels[i]] = true
		l := int(r.LayerMap[i])
		out.Minimum = min(out.Minimum, l)
		out.Maximum = max(out.Maximum, l)
		xx, yy := min(w-1, (i%x.Width)*w/x.Width), min(h-1, (i/x.Width)*h/x.Height)
		overlay.SetNRGBA(xx, yy, color.NRGBA{R: 255, G: 213, B: 70, A: 110})
	}
	delete(ids, 0)
	// Outline the downsampled selection as well as tinting it. Membership stays
	// full-resolution; this overlay is only a bounded presentation buffer.
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			if overlay.NRGBAAt(xx, yy).A == 0 {
				continue
			}
			if xx == 0 || yy == 0 || xx == w-1 || yy == h-1 || overlay.NRGBAAt(xx-1, yy).A == 0 || overlay.NRGBAAt(xx+1, yy).A == 0 || overlay.NRGBAAt(xx, yy-1).A == 0 || overlay.NRGBAAt(xx, yy+1).A == 0 {
				overlay.SetNRGBA(xx, yy, color.NRGBA{R: 255, G: 223, B: 98, A: 240})
			}
		}
	}
	out.Regions = len(ids)
	if out.Pixels == 0 {
		out.Minimum = 0
	}
	for _, g := range session.Document.Groups {
		count := 0
		for _, sp := range g.Mask {
			count += sp.Length
		}
		out.Groups = append(out.Groups, RegionGroupView{g.ID, g.Name, g.Operation, g.Value, g.Enabled, g.Locked, count})
	}
	var png bytes.Buffer
	if err := engine.WritePNG(s.ctx, &png, overlay, [2]float64{}); err != nil {
		return nil, err
	}
	out.Overlay = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png.Bytes())
	return out, nil
}
