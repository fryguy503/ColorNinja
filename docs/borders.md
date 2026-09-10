# Optional HueForge borders

Available in ColorNinja 1.1.5 and later.

In **Export → HueForge project (.hfp)**, enable **Add a border**. Set the
placement and width, then choose **Refresh preview**. The dialog shows the frame
around your image, its dimensions, predicted top color, frame material estimate,
and overall print height. Borders start off and are saved in projects, profiles,
presets, and preferences. Changing the frame preserves the image, layer map,
filament selection, and Region Edit groups.

| Setting | Behavior |
| --- | --- |
| External | Keeps the image size and adds the border outside it. A 200 × 100 mm image with a 4 mm border becomes approximately 208 × 108 mm. |
| Internal | Shrinks the entire image proportionally from its shorter side, then adds the frame. A 200 × 100 mm image with a 4 mm border becomes approximately 184 × 92 mm inside a 192 × 100 mm frame. |
| Border width | Frame width on each side, in mm. |
| Match image depth | Follows the final layer of the image's planned stack, including automatic depth changes. |
| Border color | Selects a predicted color available in the print stack and sets the border depth to its layer. Repeated colors choose the layer closest to the current depth. |
| Border depth | Absolute height from the build plate. May be below or above the image top. New entries snap to the nearest printable layer; ties round up. |

The filament stack determines the border color at its height. Above the image,
the last filament continues and its accumulated thickness can change the color.
The preview reports these border-only layers. The image's maximum-depth setting
does **not** cap an explicitly taller border. The depth field stays visible while
matching image depth; entering a different value switches to custom depth.
For example, with a 0.16 mm first layer and 0.08 mm regular layers, entering
0.86 mm selects 0.88 mm. Color and depth stay synchronized. Refresh after stack
changes to choose from its current colors. The selector uses printable stack
colors: an arbitrary screen color would require a different filament plan.
Previously saved depths and CLI inputs retain their existing continuous-depth
behavior; HueForge uses its nearest-layer lookup for their top color.

HueForge samples the image onto its mesh grid, so actual dimensions can differ
from nominal values. Internal borders reduce the image area without cropping
the source image. Very wide internal borders are rejected if they would leave
too little image area for the requested mesh detail.

The frame is rectangular. It does not trace transparent silhouettes, fill holes,
or automatically connect isolated artwork to the frame. Review the connection
between artwork and frame before printing.

**HFP carries the frame as native mesh settings.** PNG and layer-map exports
contain the image only. Surface diagnostics describe the image at its adjusted
physical size, with a separate frame summary; image and frame material estimates
exclude slicer infill, extrusion, and purge. Portable project companions retain
the border settings for reopening and exporting again.

## CLI

Add these options to an existing stack/HFP command:

```text
--hueforge-border --hueforge-border-placement external
--hueforge-border-width-mm 4 --hueforge-border-depth-mm 0
```

Depth `0` follows the image top. For a taller 4 mm frame, use
`--hueforge-border-depth-mm 4`. Width is limited to 100 mm; depth is limited to
40 mm and 998 layers. These are ColorNinja input bounds. Omit `--hueforge-border`
to keep the export borderless.

## Verification

Four HueForge 0.9.4.3 frame compatibility fixtures matched bounds and volume.
A taller-border HFP retained the planned layer of all 6,080 visible fixture pixels,
with zero tested physical-color channel disagreement. Go checks cover optical
modes, persistence, caching, opt-out, transparent input, and Region Edit retention;
the compiled-app browser check covers controls, preview invalidation, export,
reopening, and compact layout.

This establishes the tested software behavior, not physical-print appearance or
every HueForge mesh-export backend.
