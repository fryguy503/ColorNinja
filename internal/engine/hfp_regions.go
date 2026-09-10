package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"math"
	"slices"
)

// HueForge 0.9.4.3 restores footprints onto whole connected height regions.
// Its active groups have exclusive ownership, unlike our ordered pixel edits.
// Export the resolved, disjoint net changes against an unedited input where
// this is lossless; bake incompatible groups without widening their masks.
type hfpSpotSeed struct {
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Above        int     `json:"above"`
	Below        int     `json:"below"`
	CaptureAbove bool    `json:"captureAbove"`
	AnchorLum    float64 `json:"anchorLum"`
}
type hfpSpotFootprint struct {
	Width  int    `json:"w"`
	Height int    `json:"h"`
	RLE    []byte `json:"rle"`
}
type hfpSpotFix struct {
	ID          int              `json:"id"`
	Delta       int              `json:"delta"`
	FlattenMode int              `json:"flattenMode"`
	Enabled     bool             `json:"enabled"`
	TargetLum   float64          `json:"targetLum"`
	Footprint   hfpSpotFootprint `json:"footprint"`
	Regions     []hfpSpotSeed    `json:"regions"`
}
type hfpSpotMapping struct {
	ID        int    `json:"spotFixId"`
	GroupID   uint64 `json:"groupId"`
	GroupName string `json:"groupName"`
}
type hfpSpotExport struct {
	Groups      []hfpSpotMapping `json:"groups"`
	BakedGroups []uint64         `json:"bakedGroupIds"`
	Document    *RegionDocument  `json:"document"`
	Explanation string           `json:"explanation"`
}
type hfpSpotKey struct{ owner, delta, target int }

func spotFixRLE(spans []PixelSpan, size int) []byte {
	data := []byte{}
	end := 0
	for _, span := range spans {
		data = binary.AppendUvarint(data, uint64(span.Start-end))
		data = binary.AppendUvarint(data, uint64(span.Length))
		end = span.Start + span.Length
	}
	if end < size {
		data = binary.AppendUvarint(data, uint64(size-end))
	}
	return data
}

func regionHFPInput(ctx context.Context, result *Result) (*Result, []hfpSpotFix, hfpSpotExport, error) {
	base, document := result.regionBase, result.regionDocument
	info := hfpSpotExport{Groups: []hfpSpotMapping{}, BakedGroups: []uint64{}, Document: document,
		Explanation: "SpotFixes contain resolved net edits on disjoint whole regions. Some groups split by layer adjustment. Pixel masks, disabled or superseded history, and groups that cannot transfer exactly remain in the ColorNinja document. Baked groups keep their final appearance in the image. Names, locks and ordered undo history are ColorNinja-only."}
	if err := document.Validate(base); err != nil {
		return nil, nil, info, err
	}
	h := result.Stack.Options
	if err := h.Validate(); err != nil {
		return nil, nil, info, err
	}
	w, height, n := document.Width, document.Height, len(base.LayerMap)
	if len(result.LayerMap) != n || result.Image.Bounds() != base.Image.Bounds() {
		return nil, nil, info, fmt.Errorf("edited result and baseline dimensions disagree")
	}
	lo, top := h.BaseLayers(), result.Stack.Runs[len(result.Stack.Runs)-1].EndLayer
	border, err := resolveBorder(result, h)
	if err != nil {
		return nil, nil, info, err
	}
	if border != nil {
		top = max(top, border.TopLayer)
	}
	// Color Match spaces layer values by floor(65535 / (maxLayer - minLayer)).
	// HFP reserves one additional layer of import headroom.
	layerRange := max(top, lo+1) + 1 - lo
	step := 65535 / layerRange
	owners := make([]uint8, n)
	protected, cut := make([]bool, n), make([]bool, n)
	for gi, g := range document.Groups {
		if !g.Enabled {
			continue
		}
		for _, span := range g.Mask {
			for i := span.Start; i < span.Start+span.Length; i++ {
				if i%16384 == 0 && ctx.Err() != nil {
					return nil, nil, info, ctx.Err()
				}
				p := base.Image.PixOffset(base.Image.Rect.Min.X+i%w, base.Image.Rect.Min.Y+i/w)
				if protected[i] || base.Image.Pix[p+3] == 0 || (cut[i] && (g.Operation == "shift" || g.Operation == "smooth")) {
					continue
				}
				owners[i] = uint8(gi + 1)
				cut[i] = g.Operation == "cut"
			}
		}
		if g.Locked {
			for _, span := range g.Mask {
				for i := span.Start; i < span.Start+span.Length; i++ {
					protected[i] = true
				}
			}
		}
	}
	input := *result
	input.regionBase, input.regionDocument = nil, nil
	input.LayerMap = slices.Clone(base.LayerMap)
	input.Image = image.NewNRGBA(base.Image.Bounds())
	draw.Draw(input.Image, input.Image.Bounds(), base.Image, base.Image.Rect.Min, draw.Src)
	baked := make([]bool, len(document.Groups)+1)
	keyAt := func(i int) hfpSpotKey {
		owner := int(owners[i])
		if owner == 0 || baked[owner] {
			return hfpSpotKey{}
		}
		delta := int(result.LayerMap[i]) - int(base.LayerMap[i])
		if document.Groups[owner-1].Operation == "assign" {
			// Native flatten with an explicit anchor assigns a single target to
			// all selected heights, keeping a lasso assignment in one SpotFix.
			return hfpSpotKey{owner: owner, target: int(result.LayerMap[i])}
		}
		if result.LayerMap[i] == 0 {
			// Native removal requires EVERY selected sample to move below zero.
			delta = -layerRange - 1
		}
		return hfpSpotKey{owner: owner, delta: delta}
	}
	// The Color Match shader packs (layer-base)*step into red/blue bytes.
	// Native deltas use that SAME integer step, so a net layer offset gives
	// exactly the bytes a baked target layer would produce, even for long moves.
	// Region indexing then rounds the packed luminance back to physical layers;
	// emulate its float32 depth range so high-layer rounding cannot merge a
	// surviving fix into an adjacent region that we thought was distinct.
	depthRange := float64(float32(hfpMaxDepth(h, max(top, lo+1))) - float32(h.BaseDepth))
	regular := float64(float32(h.LayerHeight))
	nativeBucket := func(layer uint16) uint16 {
		lum := max(0, int(layer)-lo) * step
		return uint16(math.Floor(float64(lum)/65535*depthRange/regular+.5)) + 1
	}
	var index *RegionIndex
	// Baking a pixel selection can merge adjacent input regions. Recheck after
	// each bake so a surviving native fix never captures those adjacent pixels.
	// Bound pathological cascades; the fallback remains the exact edited image.
	for pass := 0; ; pass++ {
		for i, owner := range owners {
			if owner != 0 && baked[owner] {
				input.LayerMap[i] = result.LayerMap[i]
				p := input.Image.PixOffset(input.Image.Rect.Min.X+i%w, input.Image.Rect.Min.Y+i/w)
				rp := result.Image.PixOffset(result.Image.Rect.Min.X+i%w, result.Image.Rect.Min.Y+i/w)
				copy(input.Image.Pix[p:p+4], result.Image.Pix[rp:rp+4])
			}
		}
		// Native indexing sees the luminance under transparent pixels too. Treat
		// zero coverage as the base level to avoid selecting through a hole.
		indexed := input
		indexed.LayerMap = slices.Clone(input.LayerMap)
		indexed.Image = image.NewNRGBA(input.Image.Bounds())
		for i := range indexed.LayerMap {
			indexed.LayerMap[i] = nativeBucket(indexed.LayerMap[i])
			indexed.Image.Pix[i*4+3] = 255
		}
		index, err = BuildRegionIndex(ctx, &indexed, &indexed)
		if err != nil {
			return nil, nil, info, err
		}
		keys := make([]hfpSpotKey, len(index.Areas))
		seen, mixed := make([]bool, len(keys)), make([]bool, len(keys))
		for i, label := range index.Labels {
			key := keyAt(i)
			if seen[label] && keys[label] != key {
				mixed[label] = true
			}
			keys[label], seen[label] = key, true
		}
		invalid := make([]bool, len(baked))
		unique := map[hfpSpotKey]bool{}
		for i, label := range index.Labels {
			key := keyAt(i)
			if key.owner != 0 {
				unique[key] = true
				if mixed[label] || pass >= 8 {
					invalid[key.owner] = true
				}
			}
		}
		changed := false
		for key := range unique {
			if invalid[key.owner] || len(unique) > 512 {
				baked[key.owner], changed = true, true
			}
		}
		if !changed {
			break
		}
	}
	fixes := []hfpSpotFix{}
	positions := map[hfpSpotKey]int{}
	spans := [][]PixelSpan{}
	seeded := make([]bool, len(index.Areas))
	// Native luminance buffers and saved footprints run bottom row first.
	for j := 0; j < n; j++ {
		if j%16384 == 0 && ctx.Err() != nil {
			return nil, nil, info, ctx.Err()
		}
		i := (height-1-j/w)*w + j%w
		key := keyAt(i)
		if key.owner == 0 {
			continue
		}
		pos, ok := positions[key]
		if !ok {
			pos = len(fixes)
			positions[key] = pos
			fixes = append(fixes, hfpSpotFix{ID: pos + 1, Delta: key.delta, Enabled: true, TargetLum: -1,
				Footprint: hfpSpotFootprint{Width: w, Height: height}, Regions: []hfpSpotSeed{}})
			if key.target != 0 {
				fixes[pos].FlattenMode = 1
				fixes[pos].TargetLum = float64((key.target-lo)*step) / 65535
			}
			spans = append(spans, nil)
			g := document.Groups[key.owner-1]
			info.Groups = append(info.Groups, hfpSpotMapping{pos + 1, g.ID, g.Name})
		}
		runs := spans[pos]
		if len(runs) != 0 && runs[len(runs)-1].Start+runs[len(runs)-1].Length == j {
			runs[len(runs)-1].Length++
		} else {
			runs = append(runs, PixelSpan{j, 1})
		}
		spans[pos] = runs
		label := index.Labels[i]
		if !seeded[label] {
			seeded[label] = true
			fixes[pos].Regions = append(fixes[pos].Regions, hfpSpotSeed{
				X: (float64(j%w) + .5) / float64(w), Y: (float64(j/w) + .5) / float64(height),
				AnchorLum: float64((int(input.LayerMap[i])-lo)*step) / 65535})
		}
	}
	for i := range fixes {
		fixes[i].Footprint.RLE = spotFixRLE(spans[i], n)
	}
	for i, g := range document.Groups {
		if baked[i+1] {
			info.BakedGroups = append(info.BakedGroups, g.ID)
		}
	}
	// Rebuild the transport palette from the hybrid baseline. Physical print
	// runs and the displayed/exported PNG continue to describe the final result.
	counts := map[int]int{}
	for _, layer := range input.LayerMap {
		if layer != 0 {
			counts[int(layer)]++
		}
	}
	input.Palette = nil
	for _, c := range input.Stack.LayerColors {
		if count := counts[c.Layer]; count != 0 {
			input.Palette = append(input.Palette, PaletteEntry{RGB: c.RGB, Hex: c.RGB.Hex(),
				PixelFraction: float64(count) / float64(n), StackLayer: c.Layer})
		}
	}
	return &input, fixes, info, nil
}
