# Height workflows and Backlit

> Archived RC.1 reference: these channel workflows are temporarily disabled in
> RC.2. Use Color Match or Color Pop. Saved channel projects reopen in Color Match
> and require a new preview. The CLI rejects explicit channel selections.
> The implementation and fixtures remain in the source for future work.


ColorNinja now offers Simple reducer, Filament Guide, Color Match, Standard,
Combo, Max Channel, Scaled Max Channel, Color Aware, and Color Pop. The brightness
and region workflows first assign pixels to heights, then fit one printable
filament stack to that assignment. The filament budget limits unique spools;
the surface-color budget limits occupied height levels. Every stack workflow
supports Front Lit and Backlit, required/base/highlight filaments, explicit run
limits, deeper search, automatic depth, and portable project export.

## Choose an interpretation

| Workflow | How image heights are chosen |
| --- | --- |
| Color Match | Search for reachable colors, then map image colors to suitable layers. Layer-order and material optimization belong to this workflow. |
| Standard | Weighted RGB brightness, or HueForge's perceptual brightness variant. |
| Combo | Blend Standard and Max Channel. Mixing 100 means Standard; 0 means Max Channel. |
| Max Channel | Use the largest RGB channel. Saturated colors can be high even when their weighted brightness is low. |
| Scaled Max Channel | HueForge 0.9.4.3's channel average, with black output when every channel is at most 32. This is a specific native formula. |
| Color Aware | Group dominant red, green, and blue channels into ordered height bands, with brightness variation inside each band. |
| Color Pop | Keep existing or selected colored regions against grayscale, with separate color and grayscale bands. It also works as image preparation without a library. |

In the desktop, choose a workflow under **Tune**, load a filament library under
**Filaments**, and set printable geometry under **Layers**. **Maximum filaments**
controls the physical spools. The surface-color limit and technical search
controls are available in Advanced. Generate a preview, inspect the layer map,
and compare alternatives before exporting.

Standard and Combo offer **RGB weights** and **Perceptual brightness**. **Fill
each band's brightness range** stretches the visible pixels to that band's
available height; disabling it retains the absolute brightness range. Brightness
is an offset before gamma. Gamma uses an exponent of 1/gamma, so values above 1
raise midtones. Inversion reverses height within a band. Transparent pixels do
not determine ranges or consume the filament palette. Constant-brightness bands
use their midpoint. The surface-color limit can intentionally merge nearby
brightness values.

Color Aware supports all six RGB orders, independent relative band heights,
per-band inversions, channel exclusions, shifts from -255 to 255, and 0–8 unused
layers between bands. Empty bands consume no thickness. Every occupied band
needs two available layers and at least one surface-color slot. Channel shifts
follow HueForge's B/G/R evaluation and overflow behavior; an overflow changes
the other channels rather than clipping the selected one. Near-neutral pixels
(channel spread at most 8) and excluded-channel winners go into the first enabled
band in your chosen order. ColorNinja's band allocation and neutral placement
are explicit planning choices; matching the channel formulas does not establish
pixel-for-pixel equivalence to HueForge's entire Color Aware workflow.

The brightness and region modes share an eight-run default, counting the
foundation. A return to a previous spool consumes a run but no additional unique
filament. Increase **Maximum filament runs** if a constrained plan cannot fit.
**Deeper search** evaluates more candidates and refines complete filament runs
and their boundaries. **Reduce layer show-through** fits less distracting
intermediate colors within the color-score allowance while preserving the
assigned pixel heights. It cannot remove a step already dictated by the image's
height bands. Automatic depth rebuilds the bands at up to 24 sampled depths
(48 with deeper search), choosing the thinnest result within the requested
percentage of the best full-image color error found. This is a bounded search,
not a proof of the globally optimal print.

## Front Lit and Backlit

Select **Optical model** under Layers. Front Lit predicts reflected color.
Backlit predicts transmitted color using HueForge 0.9.4.3's modern absorption
model, including its single-material foundation behavior. Choose the same
lighting preset as HueForge. Backlit's default TD scale is **1.20**, corresponding
to HueForge light intensity **20**; intensity is `(TD scale - 1) × 100`.
Changing filament TD, light source, material, thickness, or viewing conditions
can change a real print. Record measurements in the calibration notes and follow
the [physical calibration protocol](calibration.md).

Backlit remains selected across automatic-depth and Color Pop workflow changes.
The old exponential model remains available for older projects. It does not
support these new height workflows or HFP export. Older projects without height
settings retain their existing behavior.

## Export and reopen

The PNG is the predicted physical result. The 16-bit layer map stores integer
layer numbers. The `.colorninja` project embeds source pixels, settings,
filaments, height assignments, and the exact result; it can reopen and rerender
without the original input files. Profiles retain the mode and all its controls.

HFP exports preserve ColorNinja's planned heights using HueForge's Color Match
mesh encoding, including when the originating workflow is Standard or Color
Aware. Fixed-height and Backlit exports use distinct virtual RGB keys if the
same visible color occurs at different heights. Those virtual IMAGE filaments
belong only to the Mesh Core. Print with the physical filament stack. Backlit
exports set the Backlit visualizer and matching light intensity; TD scale must
be in increments of 0.01. Changing HueForge's mesh interpretation after import
rebuilds the image heights and can change the plan.

## CLI examples

```powershell
.\colorninja-cli.exe image.png --hueforge-library filaments.json --height-mode combo --height-mixing 45 --colors 4 --hueforge-project combo.hfp -o combo.png
.\colorninja-cli.exe image.png --hueforge-library filaments.json --height-mode color-aware --height-channel-order bgr --height-red-weight 2 --height-invert-blue --colors 4 --hueforge-project aware.hfp -o aware.png
.\colorninja-cli.exe image.png --hueforge-library filaments.json --height-mode standard --hueforge-optical-model hueforge-0.9.4.3-backlit-v1 --hueforge-auto-depth --hueforge-search-effort refine --colors 4 --hueforge-project backlit.hfp -o backlit.png
```

`--height-mode` selects stack planning and requires a library. Full option names
are available through `--help`. `--options-json` and `--settings-profile` override
processing flags as before. Existing outputs require explicit `--force`.

References: HueForge's [Standard mode documentation](https://shop.thehueforge.com/pages/mesh-standard),
[Color Aware documentation](https://shop.thehueforge.com/pages/mesh-color-aware),
and [0.9.4.2 optical update notes](https://devlog.thehueforge.com/p/hueforge-0942-release-notes-and-whats).
