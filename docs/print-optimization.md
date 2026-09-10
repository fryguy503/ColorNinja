# Color Match optimization: appearance and print structure

Overall printed appearance is the priority. Evaluate color relationships, the
girl's recognizable details, coherent foliage, boundaries, and intended relief
together. Material volume is a secondary measurement. A smaller print, a lower
global color error, or a lower average boundary step does not by itself establish
a better-looking result.

## Why the HueForge feedback matters

The Mesh Core maps image colors to heights; the Color Core determines physical
filament blends at those heights. Consequently, ordering affects both color and
relief. See [HueForge's Color Match documentation](https://shop.thehueforge.com/pages/mesh-color-match).

- Low red bands extend underneath regions that continue above them. A red skirt
  occupying little visible area can require red across much of the footprint.
- Reds below Totoro's grays put the skirt in a recess. The shirt, face, hair, and
  shared outlines also matter; raising one color is not equivalent to raising
  the whole girl.
- A late green separated from neighboring foliage colors can create raised
  patches and expose unrelated colors on their slopes. Keep related, touching
  colors reasonably close in height where the optical stack permits it.
- This scene has useful green, neutral, and warm subject groups, but some colors
  occur in more than one object. Rare colors are not automatically foreground.
  Shared yellows, greens, and blacks prevent a universal hue-sorting rule.
- Rebuild the optical blend after every order or thickness change. Filament TD,
  substrate, lighting, and available layer depth constrain the reachable colors.

## Interior layers and preserved exports (dev.3)

The reported Navy Blue run contained six layers at TD 0.3 mm. All six predicted
the same RGB, `#142538`; only the first had any image pixels assigned to it.
Low TD is a reason to investigate a run, not a universal layer cap: substrate,
later translucent blends, minimum base thickness, and relief still matter.

Two search issues allowed this padding. Refinement redistributed a fixed layer
budget, while automatic depth primarily compared shortened endings of the
earlier beam. It did not thin interior runs of the newly refined complete order.
Also, averaging intermediate color detours across layers could reward extra
endpoint-colored layers by diluting an unwanted band's contribution.

Automatic depth now tests one-layer reductions throughout the complete order,
rebuilds the full blend path, and retains a bounded set of candidates at shorter
depths. It repeats this after final refinement. Base and required-spool constraints
remain in force; all final color and appearance guards still apply. The boundary
objective now integrates unwanted color over height, so padding cannot dilute it.
Fixed-depth settings continue to use the requested thickness.

The original image and saved settings were rerun with no manually pinned Navy or
red spool. Both revised searches compared five eligible depths instead of one.

| Result | Print depth | Navy run lengths | Skirt red median | Totoro gray median | Whole-image color RMS (CIELAB) | Mean boundary jump |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| Dev.2 deeper example | 3.20 mm | 6 | 3.20 mm | 1.68 mm | 13.83 | 0.45 mm |
| Dev.3 Normal preview | 2.88 mm | 2, at the base | 2.40 mm | 1.28 mm | 13.61 | 0.31 mm |
| Dev.3 Deeper refinement | 2.80 mm | 2 at the base, then 1 later | 2.80 mm | 1.68 mm | 13.54 | 0.39 mm |

The two base layers satisfy the required 0.24 mm foundation. The later single
Navy layer in deeper refinement resets the substrate before the white blends.
The rendered comparisons retain the face, hair, red skirt, and yellow shirt;
foliage and gray shading differ between candidates. Whole-image metrics do not
replace visual review. Compared with dev.2 Normal preview's RMS of 13.39, dev.3
Normal trades a small amount of color fit for its revised boundary/height score.

Estimated solid volumes were 48.24, 36.34, and 62.62 cm³ respectively. The deeper
candidate uses more volume despite its lower top height because more image area
ends higher. Volume does not rank the automatic layer-preference candidates.
Independent full-resolution masks verified region heights; integrating every
height-map pixel agreed with the reported volume within 1e-6 mm³. Both new HFPs
matched the native HueForge reference for all 17.58 million visible pixel heights
and quantized physical layer colors. Actual printed appearance remains untested.

Desktop exports now use HueForge Color Match for both the Color Match and
ColorNinja Color Pop workflows. The separate mesh-mode selector could carry a
saved HueForge Color Pop value into Color Match and discard its planned heights.
Old desktop inputs and cached exports are corrected while preserving pixels,
runs, and heights. Legacy mesh modes remain explicit CLI choices.

## Automatic layer ordering (dev.2)

**Optimize layer order** replaces the material-reduction checkbox. It is one
automatic control: the user does not need to select red, identify the foreground,
or arrange color families. The engine evaluates:

- Broad hue families, so a pale foliage shade is considered with its greens.
- Region compactness, connectedness, and neighboring families. Sparse colors
  scattered across the background are not treated like a compact accent.
- Directional height preference: a confident compact accent should sit above
  surrounding dominant colors. This uses source colors' mapped heights, including
  repeated filament runs, rather than simply requiring a spool at the end.
- Height spread within dominant chromatic families, adjacent-color height jumps,
  and unrelated blends along slopes. Once an accent clears its surroundings,
  additional height receives no reward.
- Overall color fit and a separate color-error limit for each family, preventing
  a small feature's loss from being hidden by a large background improvement.

Search automatically tries up to three matching accent filaments in later runs,
then refines complete orders and thicknesses. A candidate may temporarily exceed
the color limits while its blends are explored; every final candidate must pass
the strict limits. Explicit base/highlight, required-spool, run, and depth
constraints remain authoritative. Volume is reported but is not part of this
automatic objective.

Enabling the control starts with at least a 5% allowance in the working color
score. Dev.1 settings with material reduction enabled migrate to this automatic
preset. The allowance is adjustable under Advanced; an explicitly saved dev.2
value of 0% remains strict. It is not a percentage guarantee about whole-image
CIELAB error or perceived print quality. Each family's working RMS may increase
by the selected percentage plus at most one working color unit.

The result states which family was inferred, its assigned mean height compared
with surrounding dominant colors, and whether placement was achieved. A partial
result is identified as such. The automatic heuristic is deliberately conservative:
it uses a bounded spatial grid and broad color families, not semantic object or
depth recognition. Shared colors still have one global matching height. It cannot
independently lift the girl's yellow shirt without affecting the same yellows in
the foliage, and color/geometry tradeoffs still require a HueForge review.

CLI: `--hueforge-layer-preference auto`. Disable with an empty preference. Reports
include `result.stack.layerPreference` alongside color, boundary, and volume
measurements. Compare Results offers **Layer preference** as an automatic variant.

### Automatic-order validation on the original Totoro image

The original 3750 × 4688 source, eight-spool budget, ten-run limit, 3.20 mm ceiling,
and saved optical/geometry tuning were retained. No red filament was manually
pinned. Normal preview automatically explored fully refined accent candidates;
the separate show-through option was off to verify that order optimization
includes its own boundary checks. The automatic color allowance was 5%.

| Result | Skirt red median | Totoro gray median | Foliage green median | Whole-image color RMS (CIELAB) | Mean boundary jump |
| --- | ---: | ---: | ---: | ---: | ---: |
| Previous color plan | 1.04 mm | 2.40 mm | 2.80 mm | 13.63 | 0.94 mm |
| Automatic Normal preview | 3.20 mm | 2.00 mm | 0.88 mm | 13.39 | 0.35 mm |
| Automatic Deeper refinement | 3.20 mm | 1.68 mm | 0.24 mm | 13.83 | 0.45 mm |

The primary outcome is actual front-to-back ordering with comparable front-view
colors. Solid-volume estimates were 98.87, 62.79, and 48.24 cm³ respectively, but
volume did not rank these automatic candidates. Normal preview was preferable on
the reported color and boundary metrics; deeper search is not a guarantee that
every metric improves. The skirt still stands above its surroundings, and shared
yellow/black details are not a single object mask.

Independent full-resolution source/position masks measured the heights; integration
of every height-map pixel agreed with reported volume within 1e-6 mm³. The Normal
preview HFP matched the installed HueForge 0.9.4.3 reference for all 17,580,000
visible pixels: zero layer mismatches, zero physical color-channel error, matching
run boundaries. This verifies the export, not physical print appearance.

Local review artifacts (not bundled in releases):
[comparison image](../artifacts/layer-preference-20260910/appearance-review.png),
[region measurements](../artifacts/layer-preference-20260910/region-assessment.json),
[Normal preview HFP](../artifacts/layer-preference-20260910/automatic-preview.hfp).
Earlier rejected candidates are retained alongside these artifacts. They caught
the false cyan accent from area-only inference, pale skin dominating the seed
choice, and a Normal-preview search that failed to explore the useful late reds.

## Earlier material experiment (dev.1)

The following describes the earlier experiment and preserves its measured results.
The dev.2 desktop uses automatic layer ordering above; the legacy CLI material
objective remains available for comparison.

**Reduce printed material** was the dev.1 Color Match control under Layers. It
adds an area-based thickness penalty to the existing color objective and can be
combined with **Reduce layer show-through**. It starts off, including when older
settings are loaded. Existing output pixels, palettes, and run schedules were
verified unchanged with the new controls disabled.

Source area is kept separate from color-priority weights. A small distinctive
accent should retain importance for fitting without being treated as a large
physical region. The search uses analysis-palette area as an estimate; final
volume is integrated independently over every mapped image pixel.

The material penalty is `16 * meanAboveBaseMm²`, added to the squared color score
and optional boundary penalty. The fixed base is excluded from this ranking
term. The color-only search under the same filament and depth constraints supplies
a fidelity guard: **Allowed color-score increase** caps the increase in the
priority-weighted working color metric. It does not promise the same percentage
limit on reported whole-image CIELAB error, individual features, or appearance.
Zero keeps the strict color score, and can leave no useful alternatives. Both
geometry objectives are heuristics; enabling them does not guarantee that every
reported metric improves.

**Use the highlight filament only in the final run** makes an existing highlight
constraint exclusive. Other filaments may still return. This matters because
requiring red as the final spool alone permits an earlier red run: the image can
match that earlier red and leave the skirt recessed. Even an exclusive final
spool is not a semantic foreground mask; inspect the actual assigned heights.

Deeper refinement can now relocate a whole run together with its thickness when
material optimization is enabled. All candidates still rebuild cumulative optical
blending and honor spool, run, base, and highlight constraints.

The result and comparison cards show **estimated solid volume**. Exported JSON
includes `surfaceView.volumeMm3`, `meanThicknessMm`, and per-run `materialRuns`:

- `visibleAreaFraction`: surface pixels that stop within that run.
- `startCoverageFraction`: footprint still printing when that run starts.
- `volumeMm3`: the run's integrated occupied volume.
- `buriedVolumeMm3`: volume below pixels terminating in later runs. This material
  can contribute necessary blending; it is not automatically waste.

Volume includes the distinct first-layer height. Fully transparent pixels are
empty; partial alpha becomes solid coverage as in HFP. Estimates exclude mesh
resampling, infill settings, purge, extrusion behavior, and printing time.

## September 10 optimization runs

Twenty-one configurations were evaluated in two source/tuning groups. The first
seven used the prior reduced Totoro image and unrestricted owned inventory. The
remaining fourteen used the original 3750 × 4688 image and the user's saved
tuning, excluding silk and metallic finishes. HFP mode was explicitly normalized
to Color Match; the saved settings had another export mode selected.

The original-image group retained an eight-spool budget, a 3.20 mm ceiling,
0.16 mm first layer, 0.08 mm regular layers, 0.24 mm base, Neutral White lighting,
Distinctive priority, and 188 mm output width. Returns allowed up to ten runs
except the explicitly labeled no-return experiment. Source, library, options,
and executable hashes are recorded with each run.

### Selected original-image results

| Candidate | RMS color error, ΔE76 ↓ | Mean boundary step, mm ↓ | P95 step, mm ↓ | Solid volume, cm³ | Skirt / Totoro median height, mm |
|---|---:|---:|---:|---:|---:|
| Saved-tuning color baseline | 13.63 | 0.94 | 2.96 | 98.87 | 1.04 / 2.40 |
| Deeper color refinement | 12.99 | 0.64 | 1.92 | 59.49 | 1.36 / 1.84 |
| Material + boundaries, refined | 12.88 | 0.49 | 1.20 | 52.07 | 1.12 / 1.76 |
| Red required last, returns allowed, refined | 12.66 | 0.51 | 1.60 | 50.18 | 1.12 / 1.68 |
| Red last, all returns disabled, refined | 14.36 | 0.34 | 0.56 | 33.88 | 2.48 / 0.96 |
| Red exclusively final, other returns allowed, refined | 13.08 | 0.39 | 1.36 | 37.45 | 3.20 / 1.04 |

The baseline had no geometry objective. Refined geometry runs used a 5% working
color-score allowance. Changing required filaments or return budgets changes
the baseline against which that allowance is enforced. Do not attribute all
differences between rows to material optimization: search effort and constraints
also change. The direct paired comparison at the same refinement effort is
59.49 → 52.07 cm³, with color error 12.99 → 12.88 and mean boundary step
0.64 → 0.49 mm.

The height measurements use fixed source-color and position masks, not object
recognition. The skirt sample contains 168,154 pixels and the neutral Totoro
sample 3,206,403. They reveal ordering but do not measure every part of either
subject or the local step at their shared outline.

### Appearance assessment

The **exclusive final red** candidate is the strongest candidate for reviewing
the requested foreground relationship. Its front view preserves brown hair,
yellow clothing, and red skirt, unlike the no-return experiment. The skirt is
measurably above Totoro, global color error improves relative to the saved
baseline, and the mean boundary step decreases. However, the representative
skirt/gray height difference is 2.16 mm: check that this much relief looks right
in the actual mesh. Green highlights still span several heights.

The **material + boundaries, refined** candidate preserves a similar front-view
appearance and has a lower P95 boundary step. It is a useful gentler alternative,
but the skirt remains recessed. The minimum-volume no-return result is rejected
as an appearance recommendation because it makes the hair gray-blue and shifts
the neutrals. The red-last-with-returns result also shows why a good numerical
score and final spool identity alone are insufficient.

The zero-allowance runs were informative: material-only kept the baseline;
boundary cleanup with material increased volume to 102.26 cm³ while reducing the
mean step. With a 5% allowance, material-only used 69.75 cm³ but increased the mean
step to 1.10 mm. Neither is an automatic appearance improvement.

The pre-reduced-image series similarly showed that boundary cleanup alone can
increase material (43.84 → 51.52 cm³), while combined refinement improved color
and boundaries. Its absolute volumes should not be compared to the original-image
group, which used a different width, source, inventory filter, and lighting.

## Review artifacts

Local artwork and inventory are kept in ignored `artifacts/print-optimization-20260910`.
The tracked repository does not include the user's artwork or filament library.

- [Appearance comparison](../artifacts/print-optimization-20260910/appearance-review.png)
- [All six selected previews](../artifacts/print-optimization-20260910/comparison.png)
- [Exclusive-final-red HFP](../artifacts/print-optimization-20260910/final-only/accent-top-refined.hfp)
- [Gentler alternative HFP](../artifacts/print-optimization-20260910/allowance5/combined-refined.hfp)
- [Baseline HFP](../artifacts/print-optimization-20260910/saved-tuning/baseline.hfp)
- [Region measurements and independent volume checks](../artifacts/print-optimization-20260910/region-assessment.json)

Each experiment folder contains PNGs, 16-bit layer maps, HFPs, options, full
reports, and `summary.json`/`summary.csv`. The assessment script records its exact
source masks. `baseline-compatibility.json` checks the unmodified executable
against disabled new controls: identical pixels, palette, and schedule.

## Validation and further appearance work

Go tests and vet, frontend tests, TypeScript/Vite, and the Windows desktop build
passed. Tests cover analytical volume, first-layer geometry, transparency,
salience-independent area, an opaque sparse-accent optimum, determinism, color
guards, exclusive-highlight constraints, cancellation, HFP export, CLI options,
and portable projects/settings.

Selected exports were checked against the installed HueForge 0.9.4.3 native
filament routines and Color Match GPU shader: 17,580,000 visible pixels per case,
zero layer mismatches, zero physical color-channel error, and matching run
boundaries. This validates mapping and blending compatibility, not triangulated
sidewall appearance, printed colors, or a global optimization optimum. Allowed
HFP height includes one unused layer of headroom; that is not actual print depth.

Further work should prioritize explicit region depth relationships and limits on
foreground rise, feature-specific color checks for hair/skin/clothing, and
connected raised-patch diagnostics. Global RMS and average boundary steps can
hide a damaged face or a large local cliff. Shared colors need region-aware
authoring or carefully chosen alternative color assignments; simply sorting
hues or forcing every rare color upward cannot solve that. Evaluate native mesh
views and physical prints before treating one candidate as the universal default.

## Reproduce

Build the current CLI, then use `scripts/compare-print-plans.ps1` with explicit
source, library, options, executable, and an empty output directory. It records
provenance and never overwrites prior comparison outputs. `-AvoidSilkMetallic`
preserves the finish restriction; `-AccentFilament` accepts a stable library key.
The input options can enable `highlightOnlyAtTop` for that experiment.

CLI controls: `--hueforge-optimize-material`,
`--hueforge-highlight-only-at-top`, `--hueforge-highlight-filament`,
`--hueforge-reduce-show-through`, `--hueforge-surface-color-tolerance`, and
`--hueforge-search-effort refine`.
