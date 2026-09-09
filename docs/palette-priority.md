# Palette priority

Color priority offers a choice between area fidelity and retaining distinctive
color relationships. It adapts to each image; no gray, green, or specific subject
is hard-coded. The default, Overall balance, uses area-weighted selection.

## Selection

Distinctive and Vivid share the existing palette ceiling across chromatic and
neutral colors. An old split budget of 8 plus 8 therefore still allows 16 colors.
The image is smoothed normally, and its histogram retains actual alpha-weighted
pixel area. Selection uses separate, temporary importance weights:

- Twelve neighboring hue sectors share their area with linear interpolation,
  so many shades of a common hue do not each receive a rarity bonus.
- Smaller hue families receive a bounded bonus. Colorful histogram bins need
  coherent spatial support or sufficient area to receive that bonus; isolated
  rare noise is not promoted solely for being saturated.
- Light and dark anchors retain weight. Color differences receive more weight
  relative to lightness, helping distinguish a yellow accent from pale green.
- A cluster containing at least 75% neutral source area is recentered on its
  neutral pixels, preventing a small boosted color from tinting a gray subject.
- Near-duplicate fitted colors are consolidated. The requested count is a
  maximum; a simple image need not use every slot.

Distinctive scales Oklab's color axes by 2 for selection and matching; Vivid uses
3. The inverse conversion removes that scale, so this is a distance preference,
not a saturation adjustment. Nearby shades within 4 or 5.5 working-space units
respectively are merged. These constants are explicit heuristics, not a learned
model of beauty or semantic object importance.

Filament Guide and Global Stack use importance weights in their selection
objectives. Reported area fractions are recomputed from actual source area;
whole-image quality remains original-to-output, alpha-weighted CIELAB Delta E76.
Stack pixels still correspond to exact modeled layer colors. A palette color
cannot override the physical limitations of available filaments and layer depth.

## Validation

The reported 3750 x 4688 illustration was processed with Strong smoothing,
preservation off, and the same 16-color ceiling. Both priorities retained the
yellow shirt and separated skin tones from yellow while consolidating similar
neutrals. Vivid additionally allocated a brighter teal. All three results used
16 colors. Whole-image RMS Delta E76 was 10.7228 for Overall balance, 8.7641 for
Distinctive, and 8.7521 for Vivid. This is evidence for this image, not a universal
fidelity or aesthetic guarantee. Comparisons are kept in the local ignored
`artifacts/palette-priority` folder; the user's artwork is not a source fixture.

Regression fixtures cover small red/yellow/blue accents against gray shading,
different dominant red/green/blue families, isolated neon noise, hidden RGB,
grayscale-only artwork, tiny color budgets, determinism, alpha, settings
round-trips, and original-pixel quality metrics. All modes are checked with both
detail-preservation settings; stack output must match its exported layer colors.
Legacy Python fixtures and Front Lit reference cases remain unchanged.

The running interface was checked on the full-size illustration: selecting
Distinctive displayed the original 8-plus-8 ceiling as 16 shared colors; editing
the limit to 8 produced 8 colors. Priority, budget, preservation off, and Strong
smoothing survived reload. Desktop preview, exported PNG, and CLI comparison
matched pixel for pixel. Export retained source dimensions and DPI.

Use `colorPriority: "distinctive"` or `"vivid"` in options JSON, or the CLI
`--color-priority` flag. Missing/empty values and `"balanced"` retain previous
selection. Explicit legacy color matching cannot be combined with priority modes.
