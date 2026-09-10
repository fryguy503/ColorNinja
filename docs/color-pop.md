# Color Pop

Color Pop keeps selected hues in color and converts the remaining pixels to
grayscale. It is available in the Workflow menu. The starting image can be fully
colored: the effect preserves the selected hue and its shading rather than
recoloring every object with one flat color.

## Try the full-color demo

1. Choose **Workflow → Color Pop → Try demo**, or **File → Recent images →
   Load Color Pop demo**.
2. The red poppy is selected automatically. **Side by side** shows the original
   blue sky, green leaves, and landscape beside the red-and-grayscale result.
3. Use **Pick from original** to add a hue, or remove a sample with its × button.
   Up to eight samples can be kept. A color control is also available in the picker.
4. Adjust **Similar hues** to include neighboring hues. **Grayscale tolerance**
   controls whether faint color casts count as gray.
5. **Selection** displays a mask: white stays colored, dark becomes grayscale.
   The mask is a thumbnail; the actual processing uses every source pixel.
6. Choose a color budget and smoothing as usual, then export the PNG.

Selection applies everywhere in the image. A red object and a similarly colored
background can both match; this is hue selection, not object recognition or a
brush mask. The source image is preserved. Transparency and image dimensions
survive preparation and PNG export.

**Use existing image colors** retains every pixel outside grayscale tolerance.
Use this for artwork that already has the desired selective-color treatment.

## Output choices

**Prepare image** needs no filament library. It divides the available palette
between color and grayscale, with at least one entry per nonempty region.
An eight-color total budget allows up to eight colors across both regions.
Smoothing and reduction operate separately in each region to protect small
accents and keep the grayscale region neutral. The quality metric compares
against the deliberately desaturated image, so intentional desaturation is
not counted as an error.

**Plan filament stack** is experimental. Load a filament library, including
neutral filaments, then adjust the physical settings in **Layers**. It assigns
each region a separate brightness-to-height range and fits one physical stack
to those targets. **Height for color** divides the available layers;
**Region order** chooses which region sits higher; **Boundary gap** reserves
transition layers that are not surface targets. Both regions need at least two
layers, and the surface-color and filament budgets must be at least two.

The planner uses fixed thickness, supports filament returns, and defaults to
eight runs when no explicit run limit is set. It reserves room for the upper
region and uses neutral filaments in the grayscale band. Filament RGB/TD,
lighting, order, thickness, and constraints affect the physical result;
particularly with grayscale above color, translucent neutral layers may still
show the colored material beneath. The preview shows those modeled colors.
Automatic depth, ordinary stack refinement, and show-through optimization do
not apply to this planner. Comparison plans instead vary height allocation,
run budget, and region order. Saved comparisons and spool exclusion still work.

PNG, reports, layer maps, portable projects, presets, undo/redo, and normal
preview/export controls use the existing workflows. Switching away from Color
Pop disables its treatment while retaining the selected samples for later.
Older projects and profiles open with Color Pop disabled.

## HueForge export

Exporting a prepared PNG lets you make the final plan in HueForge, including
its native Color Pop mode. Exporting a planned stack as `.hfp` preserves
ColorNinja's calculated heights using **Color Match** and a separate virtual
Mesh Core. Keep that exported mesh mode to retain the planned geometry.

Identical visible RGB values can occur at different heights in the two bands.
The exported project gives these heights distinct nearby RGB keys in its
embedded mesh image. The physical filaments, Color Core, result PNG, and
layer-index map retain the computed values. These virtual image materials are
not spools to print. Nonzero alpha becomes solid mesh, as in other HFP exports.

The full 1254 × 1254 demo export was checked with the installed HueForge 0.9.4.3
native material/blending routines and unmodified Color Match shader: zero layer
mismatches across 1,572,516 pixels and zero byte-level physical RGB differences.
This verifies the exported calculation, not a physical print or an optimal
filament plan. The planner is a bounded search.

## CLI

```powershell
.\colorninja-cli.exe input.png --color-pop `
  --color-pop-selection selected --color-pop-colors '#E52A15' `
  --color-pop-hue-range 8 --total-colors --colors 8 -o pop.png

.\colorninja-cli.exe input.png --color-pop `
  --color-pop-selection selected --color-pop-colors '#E52A15' `
  --color-pop-hue-range 8 --hueforge-stack --colors 3 `
  --hueforge-library library.json --color-pop-color-height 50 `
  --color-pop-gap-layers 1 -o stack.png --hueforge-project stack.hfp
```

Use `--color-pop-gray-on-top` to reverse the bands. `--color-pop-selection existing`
keeps all non-gray colors. See `--help` for grayscale tolerance and other controls.

## Demo asset

The embedded [full-color poppy](../internal/studio/assets/color-pop-poppy.png)
is an original AI-generated illustration, made with built-in image generation.
Its [generation record](../internal/studio/assets/README.md) includes both prompts.
It requires no download or network connection at runtime.
