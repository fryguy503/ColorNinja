# Validation scope

The build script records test results and tool versions in `BUILD-INFO.json`;
the structured Go log is saved to `artifacts/go-tests.jsonl`. It requires Go
tests, `go vet`, frontend state regression tests, TypeScript checking,
production compilation, and source formatting checks to pass.

## Automated coverage

- Lab and Oklab reference values/round trips, finite options, exact layer-depth validation.
- Detail preservation: one-pixel horizontal, vertical, and diagonal lines in
  red, green, blue, and gray across all three modes; retained contrast; noise
  suppression without bleeding between similar-color regions; consistent
  mapping of weak gradients; correct Delta E76 units; tile boundaries and
  subimage stride; transparency; zero smoothing; high-radius cancellation;
  defaults/opt-out persistence; complete-stack recovery of missing shades; and
  exact agreement of constrained output RGB with the layer map.
- Source dimensions, preserved alpha, transparent pixels, deterministic results,
  cluster culling, and cancellation during expensive work.
- Full-resolution source and rendered-result unique color counts, nonzero-alpha
  semantics, image subregions/row stride, cancellation, unused palette entries,
  and replacement of cached counts when opening another image.
- Five pixel-exact Python fixtures: standard, blur, blur plus reduced analysis
  size, guided, and global stack, with true-black and detail preservation disabled.
  `scripts/generate_reference.py` intentionally
  refreshes them; ordinary Go tests need no Python.
- Library filters and empty/invalid inputs, anchor selection, reachable palette
  selection, transmission, opaque bases, stack budgets, terminal candidate
  reranking, and removal of unused filaments.
- PNG and 16-bit layer-map round trips, DPI, orientation, atomic no-clobber
  races, hardlink aliases, input/library protection, CLI exports and errors.
- Adobe RGB with transparency and CMYK fixtures compared with LittleCMS.
  Color transformation uses tolerances; alpha must match exactly.
- Lossless/lossy animated WebP first frames compared with libwebp, including
  alpha/canvas dimensions; cropped GIF frame offsets.
- Preview/export pixel identity, stale workers, cancellation, project/preset
  round trips, malformed settings, and invalid project recovery.
- The reported preset/mode sequence across all three modes and all three
  built-in budgets; custom preset mode retention and immutable calibration.
- True-black on/off rendering for the tinted `#212721` Black filament in both
  guided and stack modes, exact alpha, original-library preservation, correct
  blend/layer-map colors, gray/chromatic exclusions, CLI control, legacy JSON
  defaults, and persistence through projects, presets, and preferences.

The original Python suite passed all 32 tests during migration.

## Measurements

Standard reduction with detail preservation on a synthetic 1200 x 900 image
measured about 0.140 seconds and 20.2 MB allocated per operation (three iterations)
on this AMD Ryzen 9 7950X workstation.
This is an engine benchmark, not an end-to-end performance guarantee for
decoding, encoding, arbitrary images, or stack searches.

The Adobe RGB transparent fixture had mean Delta E76 0.037 and maximum 1.21
against LittleCMS. The CMYK fixture's Windows CMM conversion had mean 0.630.
The lossless WebP fixture matched libwebp exactly; the lossy fixture differed by
at most one RGB channel value after BT.601 normalization. These figures describe
the checked fixtures, not universal error bounds.

## Desktop validation boundary

The Wails application launched successfully, the interface and sample preview
were visually inspected, and accessible controls were observed. Desktop control
was then stopped by the user with Escape. Later import and interface fixes were
verified through code/build checks; the final executable has not had a complete
interactive acceptance pass.

Remaining manual release checks include repeated native open/save dialogs,
drag/drop, comparison and panning at different display scales, keyboard shortcuts,
mode changes with a real library, and a clean Windows account. The portable
release is locally built and unsigned. Public signing, an installer, automatic
updates, and macOS/Linux packages are outside this initial Windows release.

ICC test photographs and their license are retained under engine testdata.
Synthetic fixtures are generated for this project. The release includes runtime
dependency licenses and excludes private libraries and user images.
