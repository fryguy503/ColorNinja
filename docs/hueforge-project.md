# HueForge projects and filament returns

Global Stack exports self-contained `.hfp` projects for HueForge 0.9.4.3 using
Front Lit and Filament Painting. The project embeds the reduced image, real
filament identities and TD, swap positions, dimensions, layer geometry, lighting,
and a separate Color Match Mesh Core.

## Workflow

1. Choose **Global stack**, select eligible filaments, and set the filament budget.
2. Under **Layers**, enable **Allow filament returns** to explore
   schedules such as A → B → A. **Maximum filament runs** limits swaps. The
   filament budget counts unique spools; six runs can use four spools.
3. Open **Export** and select **HueForge project (.hfp)**. Start with
   **Color Match** and **Match planned layers**. Set width and mesh detail.
4. Use **Refresh preview** in the dialog if settings have changed, then **Export HFP**.
5. Open it in HueForge and review its preview, dimensions, and swap instructions
   before exporting a mesh and slicing.

Consecutive layers of one filament already produce different reachable colors
as their thickness increases. Returns add a different possibility: an earlier
filament can be used again over an intervening material. Its new substrate changes
the result. Search and refinement evaluate returns using cumulative Front Lit
blending. Returns start off to preserve existing plans. Enabling them widens the
search and can take longer; the bounded search does not prove a global optimum.

## Reduce layer show-through

Under **Layers**, enable **Reduce layer show-through** for detailed images whose
Color Core preview shows unwanted bands around outlines and small shapes. Refresh
the preview and export again with **Color Match / Match planned layers**.

Color matching alone scores the top of each region. The mesh between two regions
can cross unrelated colors in the stack. This option samples neighboring source
color groups and adds a penalty for large height differences and intermediate
layer colors outside the color transition between those groups. Complete-stack
search and refinement use this combined objective to choose filament order and
layer allocation. Filament returns, when enabled separately, remain available.

The option starts off and is retained in projects, settings profiles, and presets.
It may trade color accuracy for cleaner boundaries and adds planning time. It
does not blur the image or force all colors into a dark-to-light order. With
automatic depth, the 1% allowance applies to the combined color and boundary score.

This is a heuristic based on at most 262,144 nearest image samples, not a mesh or
print simulation. Very fine details can be missed by sampling, and with Preserve
details off, individual pixel matches may differ from the sampled color groups.
ColorNinja's preview still shows colors at the chosen heights, not the slopes
between them. Check the actual HueForge preview; mesh detail, physical width, and
filament measurements still affect the result. Other mesh modes rebuild heights.
The report's `stack.surface` records the sampled mean boundary height step, Oklab
color detour, and heuristic penalty; these are not measured show-through percentages.

CLI: add `--hueforge-reduce-show-through` to the stack export command below.

## The two cores

The **Color Core** contains the actual optimized print schedule, including repeated
filament identities. Names, brands, materials, RGB, and TD travel with the project.
A true-black override receives a separate project identity so it does not alias
the unchanged library definition.

The default **Mesh Core** contains a virtual `IMAGE` reference for each used output
color, assigned to its selected print height. Exact RGB matching chooses those
heights, and unused heights are disabled. These references are not extra physical
spools. Very low virtual TD prevents blends between neighboring reference colors.

**Use filament blends** instead uses the physical schedule as the Mesh Core and
HueForge's Oklab matcher. **Combo**, **Color Aware**, and **Color Pop** are also
available export modes. HueForge recomputes their heights, so they do not promise
the same result as ColorNinja's Color Match plan.

Changing export settings marks the preview stale; refresh before exporting.
HFP requires the current Front Lit model, at most 998 planned layers, and first
and regular layer heights in 0.01 mm increments compatible with HueForge's controls.

HueForge's layer builder and UI snapping can otherwise remove the final layer.
The project reserves one unused layer of maximum-depth headroom. Color Match
disables that extra height. The actual print schedule stays unchanged; HueForge
can show a higher allowed thickness than actual thickness.

The image retains full source resolution. Mesh detail controls geometric sampling
in HueForge and can alter fine boundaries. Width is explicit and height follows
image aspect ratio; HueForge quantizes geometry to its detail grid. Partial alpha
becomes solid coverage in HFP because HueForge premultiplies alpha before matching.
Fully transparent pixels remain empty. PNG export preserves fractional alpha.

## CLI

```powershell
.\colorninja-cli.exe input.png -o stack.png `
  --hueforge-stack --colors 4 --hueforge-max-runs 6 `
  --hueforge-library "$env:APPDATA\HueForge\Filaments\personal_library.json" `
  --hueforge-project stack.hfp --palette-json stack.json `
  --hueforge-mesh-mode color-match --hueforge-mesh-core planned-colors `
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
exported report for comparison and begin with **Match planned layers**.
