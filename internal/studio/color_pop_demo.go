package studio

import (
	"colorninja/internal/engine"
	"context"
	_ "embed"
)

//go:embed assets/color-pop-poppy.png
var colorPopDemo []byte

func (s *Studio) UseColorPopDemo() (Snapshot, error) {
	ctx, serial := s.beginLoad()
	img, err := projectPNG(colorPopDemo)
	if err != nil {
		return Snapshot{}, err
	}
	// DecodeConfig/Decode happens through the same bounded PNG project reader.
	if err = ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	err = s.installImage(ctx, &engine.LoadedImage{Image: img, Metadata: engine.ImageMetadata{Format: "sample", ColorProfile: "sRGB", Warnings: []string{}}}, "Color Pop · Red poppy", true, serial)
	if err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	if serial != s.loadSerial {
		s.mu.Unlock()
		return Snapshot{}, context.Canceled
	}
	o := s.settings.Options
	o.Mode = "standard"
	o.Colors = 8
	o.TotalColors = true
	o.ColorPop = engine.DefaultColorPopOptions()
	o.ColorPop.Enabled = true
	o.ColorPop.Selection = "selected"
	o.ColorPop.Colors = "#E52A15"
	o.ColorPop.HueTolerance = 8
	s.settings.Options = o
	s.mu.Unlock()
	return s.Snapshot(), nil
}
