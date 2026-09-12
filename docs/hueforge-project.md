# HueForge projects and filament returns

Version 1.0 supports Color Match and Color Pop with Front Lit or Backlit
physical colors. Channel workflows are temporarily disabled. The HFP mesh uses
Color Match to preserve the planned heights, including heights chosen with the
[weighted color-order preference](color-order.md). Backlit and Color Pop retain
distinct virtual RGB keys for repeated colors at different heights. Backlit also
sets the visualizer and light intensity to match the selected optical model.

Color Match exports self-contained `.hfp` projects for HueForge 0.9.4.3 using
Front Lit and Filament Painting. The project embeds the reduced image, real
filament identities and TD, swap positions, dimensions, layer geometry, lighting,
and a separate Color Match Mesh Core.

## Workflow

Development builds also offer [optional internal/external mesh borders](borders.md)
in the HFP export dialog, with configurable width/depth and a frame preview.

1. Choose **Color Match**, select eligible filaments, and set the filament budget.
2. Under **Layers**, enable **Allow filament returns** to explore
   schedules such as A → B → A. **Maximum filament runs** limits swaps. The
   filament budget counts unique spools; six runs can use four spools.
3. Open **Export** and select **HueForge project (.hfp)**. Start with
   **Color Match** and **Tuned image colors (recommended)**. Set width and mesh detail.
4. Use **Refresh preview** in the dialog if settings have changed, then **Export HFP**.
5. Open it in HueForge and review its preview, dimensions, and swap instructions
   before exporting a mesh and slicing.

Consecutive layers of one filament already produce different reachable colors
as their thickness increases. Returns add a different possibility: an earlier
filament can be used again over an intervening material. Its new substrate changes
the result. Search and refinement evaluate returns using cumulative Front Lit
blending. Returns start off to preserve existing plans. Enabling them widens the
search and can take longer; the bounded search does not prove a global optimum.

Both Normal preview and Deeper refinement merge redundant filament runs. When a
run has no visible surface pixels, the planner also checks simpler schedules
with adjusted layer allocations. It accepts these only when the optimization
score is no worse and the existing color and filament constraints still hold.
Buried layers can affect the colors above them, so zero surface coverage alone
does not mean a run can be discarded. Automatic depth then compares the simpler
schedule using the saved depth allowance.

## Reduce layer show-through

Under **Layers**, enable **Reduce layer show-through** for detailed images whose
Color Core preview shows unwanted bands around outlines and small shapes. Refresh
the preview and export again with **Color Match / Tuned image colors**.

Color matching alone scores the top of each region. The mesh between two regions
can cross unrelated colors in the stack. This option scans neighboring source
color groups and adds a penalty for large height differences and intermediate
layer colors outside the color transition between those groups. Complete-stack
search and refinement use this combined objective to choose filament order and
layer allocation. Filament returns, when enabled separately, remain available.

The option starts off and is retained in projects, settings profiles, and presets.
It may trade color accuracy for cleaner boundaries and adds planning time. It
does not blur the image or force all colors into a dark-to-light order. With
automatic depth, the configured depth allowance (1% by default) applies to the
combined score. A separate color-fidelity allowance limits the increase over a
color-only search using the same constraints.

The heuristic scans every source pixel with bounded row storage and a color
lookup. Export width and mesh detail scale the height penalty. **Visualize stack**
adds height-boundary and intermediate-color overlays, an approximate surface,
and coverage; its display grid is capped and can miss fine features. Full-pixel
step statistics are separate from the sampled display. Check the actual HueForge
mesh and material lighting. `stack.surface`
records palette-boundary metrics; `surfaceView` records actual mapped-image
statistics. Neither is a measured show-through percentage.

CLI: add `--hueforge-reduce-show-through` to the stack export command below.

## The two cores

For measured color-order, relief, and material comparisons, see
[Layer-order optimization](print-optimization.md). Layers offers automatic
**Optimize layer order** and, with a required highlight selected,
**Use the highlight filament only in the final run**. Evaluate appearance and
assigned heights alongside volume; an earlier run can otherwise capture the
image colors even when the same spool also appears at the top.

The **Color Core** contains the actual optimized print schedule, including repeated
filament identities. Names, brands, materials, RGB, and TD travel with the project.
A true-black override receives a separate project identity so it does not alias
the unchanged library definition.

The default **Mesh Core**, **Tuned image colors**, uses virtual `IMAGE` references.
Their TDs control blending between image colors; they are not measured filament
TDs or additional physical spools. Beta 6 fits higher virtual TDs so target colors appear at their chosen heights,
with useful blends in between. It also evaluates a compact IMAGE version of the
physical blend path. Selection prefers fewer necessary disables, then fewer
entries. Redundant later equal colors can remain enabled; earlier conflicts may
still require disables. The inspector reports counts and virtual TD range.
Real Color Core filament RGB/TD and the selected image heights do not change.
Retain sufficient output colors when preparing gradients; a Mesh Core cannot
restore source shades already removed by reduction.

Older **Match planned layers** (`planned-colors`) settings, presets, profiles,
and projects automatically use the tuned core. Opening a saved project refreshes
its Mesh Core display while preserving the saved image and print heights.
Already exported HFP files need to be exported again and reopened in HueForge.
**Flat image colors (legacy)** (`legacy-flat`) explicitly reproduces the old
0.01 TD bands and disables unused heights; this is no longer the default.

**Use filament blends** instead uses the physical schedule as the Mesh Core and
HueForge's Oklab matcher. Desktop HFP exports always use HueForge **Color Match**;
the separate mesh-mode selector has been removed. This also applies to
ColorNinja's **Color Pop** workflow, whose planned color/grayscale bands are
preserved using Color Match. Old desktop settings, presets, profiles, and projects
with another mesh mode are corrected automatically. Reopening a saved project
preserves its rendered pixels, filament runs, and assigned heights.

**Combo**, **Color Aware**, and HueForge **Color Pop** remain explicit low-level
CLI options for compatibility. HueForge recomputes their heights, so these do not
promise the same result as ColorNinja's plan.

Changing export settings marks the preview stale; refresh before exporting.
HFP requires the current Front Lit model, at most 998 planned layers, and first
and regular layer heights in 0.01 mm increments compatible with HueForge's controls.

HueForge's layer builder and UI snapping can otherwise remove the final layer.
The project reserves one unused layer of maximum-depth headroom. Color Match
does not match above the final slider; older cores also disable that height.
The actual print schedule stays unchanged; HueForge
can show a higher allowed thickness than actual thickness.

The image retains full source resolution. Mesh detail controls geometric sampling
in HueForge and can alter fine boundaries. Width is explicit and height follows
image aspect ratio; HueForge quantizes geometry to its detail grid. Partial alpha
becomes solid coverage in HFP because HueForge premultiplies alpha before matching.
Fully transparent pixels remain empty. PNG export preserves fractional alpha.

## CLI

The fitted/compact IMAGE core is the default, including with older options files.
`--hueforge-mesh-core legacy-flat` explicitly selects the older exact-band core.

```powershell
.\colorninja-cli.exe input.png -o stack.png `
  --hueforge-stack --colors 4 --hueforge-max-runs 6 `
  --hueforge-library "$env:APPDATA\HueForge\Filaments\personal_library.json" `
  --hueforge-project stack.hfp --palette-json stack.json `
  --hueforge-mesh-mode color-match --hueforge-mesh-core compact-blends `
  --hueforge-width-mm 150 --hueforge-mesh-detail-mm 0.2
```

`--hueforge-max-runs 0` preserves one contiguous run per unique filament.
Use `--force` for existing outputs. Source, library, and output paths must differ.

## Validation

Compatibility was checked with HueForge 0.9.4.3. ColorNinja exports projects
without requiring HueForge to be installed.

Checks completed September 9, 2026:

- A two-filament A → B → A fixture reproduced its target with zero RMS error,
  improving the no-return search with detail preservation both on and off.
- A 3,750 × 4,688 illustration with 45 eligible filaments produced four spools,
  six runs, and fourteen colors. Compatibility checks found matching layer colors
  and assignments across 17,580,000 visible pixels.
- The real HueForge app opened Front Lit, Filament Painting, and Color Match with
  aligned core previews. Saving retained every run identity and endpoint. Its
  generated Front STL measured 2.08 mm actual thickness, matching the plan; the
  project retained 2.16 mm allowed thickness as import headroom.
- Automated checks cover return budgets, cancellation, persisted options,
  serialization, depth boundaries, mode selectors, stale results, and atomic writes.

This validates the tested software version and cases, not measured physical print
colors, a global optimum, or identical detail after mesh resampling. Keep the
exported report for comparison and begin with **Tuned image colors**.
