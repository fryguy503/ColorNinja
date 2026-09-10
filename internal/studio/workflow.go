package studio

import "colorninja/internal/engine"

// Desktop workflows own the color-to-height assignment. Other HueForge mesh
// modes rebuild it and can undo the preview's layer order. Keep legacy modes
// available in the explicit low-level CLI, but normalize desktop inputs.
func workflowOptions(o engine.Options) engine.Options {
	o.HeightMap.Mode = "" // Channel workflows are paused; retain their tuning for a future release.
	if o.Mode == "stack" && o.HueForge.MeshMode != "" {
		o.HueForge.MeshMode = "color-match"
	}
	return o
}
