# Validation scope

The build script records test results and tool versions in `BUILD-INFO.json`;
the structured Go log is saved to `artifacts/go-tests.jsonl`. It requires Go
tests, `go vet`, frontend state regression tests, TypeScript checking,
production compilation, and source formatting checks to pass.

## Automated coverage

The beta includes [palette-priority validation](palette-priority.md): accent retention
against grayscale and different dominant hue families; neutral protection;
noise/alpha checks; small budgets; all-mode palette and layer consistency; and
frontend controls that retain the existing ceiling and unrelated settings.

Smoothing was also checked on the reported 3750 x 4688 image with 8 colors per
population, Strong smoothing, and preservation off. In a 390 x 110 forehead
patch, neighboring palette-color transitions fell from 10,571 to zero; the
whole image retained 16 colors. The checkbox stayed off when selecting Off and
Strong, and Strong remained selected after reloading the actual image. Desktop
preview, exported PNG, and CLI output matched pixel for pixel; export retained
source dimensions and DPI. This validates the reported texture cleanup, not a
universal fidelity improvement: changing from legacy CIELAB to Oklab matching
also changes the chosen palette (whole-image RMS Delta E76 was 8.9234 before and
10.7228 after).

- Front Lit: 246 compatibility reference cases over three lighting presets;
  float RGB comparison and exact planner rebuild bytes. First-layer geometry,
  saved-model migration, single-filament depth, and exhaustive small guide
  selection checks cover integration.
- Rare coherent 0.05%-area line preservation at the default 0.5% cutoff,
  with an equal-population isolated-speckle rejection control.

- Windows overwrite approval accepts Wails' native Yes response, defaults to No,
  and rejects cancellation or dialog failure. Re-exporting a changed preview
  replaces an existing PNG only when approved, with pixel-exact output checks.

- Total standard-mode palette caps including neutrals, determinism, alpha,
  legacy split-budget compatibility, and round trips for new options.
- Strong smoothing lowers texture error in a synthetic gray checkerboard while
  retaining a one-pixel dark outline; the processing path honors the tolerance.
- With preservation on and off, Strong removes checkerboard palette transitions
  (168 to 0), retains a one-pixel ink line, preserves alpha and palette limits,
  and keeps discovery consistent with processing. Quality metrics compare the
  original pixels to the output in CIELAB, even when preservation is off.
- The detail toggle does not change color matching or smoothing. Smoothed stack
  pixels match their exported layer colors with either preservation setting.
- GitHub version ordering (including beta.2 versus beta.11), draft exclusion,
  stable/prerelease channels, fixed-endpoint pagination, no downgrade, cache
  separation, empty releases, invalid payloads, response caps, and canceled checks.
- UI/update preferences persist without processing an image; old settings retain
  their processing values. Smoothing presets preserve budgets, modes, calibration,
  detail preservation, and undo-history inputs. Presets clear an explicitly
  imported legacy color pipeline without changing the detail checkbox.

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

The beta release gate includes 97 Go tests (with 246 Front Lit reference
subcases and five Python reference cases), six frontend tests, TypeScript and
production builds, formatting, Go vet, and Windows Wails/CLI compilation.
The final packaged executables are checked after extraction: file checksums,
clean source provenance, all three CLI modes, overwrite protection, embedded
frontend loading, and desktop service preview/export identity.

Development UI checks covered palette limits, independent Strong smoothing,
distinctive/vivid priorities, saved settings, update channels, and offline use.
Front Lit checks covered lighting, separate first/regular layer heights, and
HFP imports. With a 4.0 mm automatic-depth ceiling, the full-size test illustration
selected 2.08 mm after comparing 45 depths. The stack map was checked for layer
selection, keyboard navigation, material/TD/height information, and image coverage.
The finish filter reduced a 45-filament test library to 39 eligible entries while
retaining plain alternatives. These results describe the tested cases; physical
print appearance and a global optimum are not guaranteed.

Remaining manual release checks include repeated native open/save dialogs,
drag/drop, comparison and panning at different display scales, keyboard shortcuts,
mode changes with a real library, and a clean Windows account. The portable
release is locally built and unsigned. Public signing, an installer, automatic
updates, and macOS/Linux packages are outside this initial Windows release.

ICC test photographs and their license are retained under engine testdata.
Synthetic fixtures are generated for this project. The release includes runtime
dependency licenses and excludes private libraries and user images.
