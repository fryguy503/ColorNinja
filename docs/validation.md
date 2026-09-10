# Validation scope

## Version 1.0.2 library discovery

Synthetic libraries exercise the Windows roaming directory and the macOS
HueForge container path on every test host. Startup tests use each native
platform's user-directory environment and verify first-launch loading, relaunch,
and preservation of manually selected libraries. Missing saved files retain
their load warning; discovery does not silently switch filament inventories.
Missing or relative user directories and directories named as library files
are rejected. The fixtures are checked for unintended writes.

These regressions run in the native release matrix. They do not replace manual
first-launch testing with a real HueForge installation on a clean Mac account.

## Version 1.0.1 layer planning and native packages

Six synthetic two-spool palettes now reach zero modeled color error in both
Preview and Refine. Regression coverage checks block-transfer bounds, retained
allocation alternatives, strict color and height constraints, cancellation,
determinism, and search/export palette agreement. Local native HueForge 0.9.4.3
checks found zero RGB-byte differences for all twelve returned plans. Private
decompilation and native-reference artifacts are stored outside the repository;
committed tests contain synthetic data only.

The CI workflow builds Windows x64, Linux x64/ARM64, and macOS Intel/Apple Silicon
on native runners. Linux uses Ubuntu 24.04 and WebKitGTK 4.1; macOS uses version
15. `scripts/build-native.sh` runs tests, vet, frontend checks, dependency audits,
and native packaging. `scripts/verify-native.py` verifies every extracted file,
executes four CLI workflows, and starts the packaged desktop HTTP service.
Windows retains its compiled-app browser and extracted-package checks. Native
Linux/macOS window, dialog, and clean-machine acceptance remains separate.

The search remains bounded and heuristic. Exact synthetic palettes and agreement
with tested native blend calculations do not establish global optimality or
physical-print accuracy.

## Release candidate 1

The new native-reference corpus contains 656 Standard/Combo/Max/Scaled Max
brightness cases, 512 Color Aware shifted-channel brightness cases, and 300
Backlit stacks across three lighting presets. The Backlit tests compare floating
channels and require exact rounded RGB bytes through the planner's stack rebuild.
The corpus records the installed executable hash and contains numeric inputs
and outputs only.

The extracted candidate ran all seven stack workflows under Front Lit and
Backlit on the 1,254 × 1,254 poppy demo. All 14 HFPs passed native filament JSON
parsing, inclusive run expansion, physical blending, and HueForge's original
Color Match shader: **zero RGB-byte differences and zero height mismatches over
22,015,224 visible pixels**. Local evidence is under
`artifacts/release-verification/c26d43c7b4f34d169cf361d06232c4ba`.
This counts repeated evaluations of one artwork, not 22 million independent
images or physical measurements.

Go regressions cover five brightness/region workflows preserving a full-height
one-pixel stripe despite a 64-pixel analysis setting, six band orders, ignored
and empty channels, transparency, nonzero image origins, cancellation during
planning, constraints during refinement, depth selection, cache equivalence,
and portable project/profile/export/rerender round trips in both light models.
The browser suite exercises the compiled app's real backend, all new workflow
controls, Backlit, persistence, undo, stale-export guards, Escape, and three
viewport widths. Full native Windows file-picker, clean-user, and physical-print
acceptance remains open.

On the local Ryzen 9 7950X, one-shot 512 × 384 synthetic gradient benchmarks
took 97–104 ms for the five new height workflows and 311 ms for Color Pop, using
4.5–4.6 MB and 22.4 MB of cumulative Go allocation respectively, excluding input
creation. These are measurements of this small fixture, not performance limits
or peak working-set measurements. Reproduce with:

```powershell
. ./scripts/env.ps1
go test ./internal/engine -run '^$' -bench BenchmarkHeightWorkflow -benchmem
```

The maintained release checks are `scripts/build.ps1`, `scripts/audit.ps1`,
`scripts/verify-ui.cjs`, and `scripts/verify-release.ps1`. Their Windows CI
definition is `.github/workflows/verify.yml`; hosted CI must still run after
these changes are pushed. Local npm audit and govulncheck found no known
vulnerabilities. Signing support is implemented but requires an actual publisher
certificate to exercise. See [production readiness](production-readiness.md).

The build script records test results and tool versions in `BUILD-INFO.json`;
the structured Go log is saved to `artifacts/go-tests.jsonl`. It requires Go
tests, `go vet`, frontend state regression tests, TypeScript checking,
production compilation, and source formatting checks to pass.

## Automated coverage

Beta 7 adds Color Pop hue wrapping, grayscale tolerance, transparency, separate
region budgets, accent survival, old-settings defaults, fixed-band geometry,
neutral-band materials, duplicate-height mesh keys, cache invalidation, and
cancellation checks. Portable project tests cover the full-color demo and a
planned stack, including reopened exports and band comparison alternatives.
Frontend state tests cover workflow switching, preset compatibility, and
legacy optical profiles. The final build count is recorded in `BUILD-INFO.json`.

Both band orders of the full 1254 × 1254 demo passed the installed HueForge
0.9.4.3 native material/blending routines and unmodified Color Match shader:
zero layer mismatches across 1,572,516 visible pixels and zero physical RGB-byte
differences. Local evidence is under `artifacts/color-pop-qa`. Browser checks
covered the picker, mask, side-by-side comparison, mode switching, undo, stack
controls, and the renamed Color Match help. Physical print verification remains
separate from these software checks.

Beta 6 adds full-resolution/downsampled detail regressions, unconditional CIELAB
report checks, alternate matching heights, required-spool survival through
mapping, cache/fresh equivalence and cancellation, TD sensitivity, comparison
persistence/exclusion/export identity, and three small stack cases checked
against exhaustive enumeration. `BUILD-INFO.json` records the current test count;
historical counts below describe their original releases.

The blended Mesh Core covers three lighting presets, physical-core immutability,
matching heights, higher virtual TDs, and gap blending. The installed HueForge
0.9.4.3 harness was rerun for a constrained 96×64 fixture, a 480×64 gradient,
3750×4688 artwork, and a Black → Magenta → Black → Green case using black at
layer 16 rather than layer 5. All returned zero matching-layer and physical RGB-channel
errors. Raw reports are under `artifacts/beta6` locally.

The default-TD correction also covers omitted and Beta 5 `planned-colors`
settings, nested presets, profiles, and portable project restoration. The rebuilt
Windows interface reopened an old flat-core project, selected **Tuned image colors**,
and exported it with TD 0.78–2.29 and no disables. The saved image and print heights
were retained. CLI defaults and old options files passed the native harness too:
the gradient used four entries instead of fifteen (TD 0.2–6.3), and the artwork
used TD 0.78–5.59 with twenty disables removed. Evidence is under
`artifacts/beta6/td-default-fix` locally.

The Beta 6 build gate passed 131 Go tests and eight frontend state
tests. A browser pass of the compiled interface covered saving comparisons,
opening a portable constrained project, generating alternatives, and selecting
a layer both from the slider and from the image. It also checked the surface
layout, binary-alpha explanation, compact Mesh Core reporting, export blocking
after settings changes, and keyboard focus restoration after modal dismissal.
The focus restoration defect found during this check was fixed and rechecked.
Native Windows dialogs and display scaling
remain separately pending.

An existing native STL contains 649,412 triangles on a 751×939 XY coordinate
grid at 0.2 mm spacing, with a 150×187.6×2.08 mm bounding box. Its requested
187.52 mm height was rounded up to the grid. The diagnostic display is not mesh
parity: extent rounding, triangle decimation/orientation, and material lighting
still need controlled new-mesh comparisons. The UI labels its surface approximate.

On a synthetic 1200×900 reducer image, three iterations averaged 177.6 ms and
40.0 MB allocated for fresh processing versus 2.4 microseconds and about 2 KB for
an export-only cache lookup on a Ryzen 9 7950X. These are engine allocations per
operation, not retained memory, desktop latency, or stack-search speedup claims.
The cache retains current-document stage snapshots, one completed result, and
a bounded optical-candidate set.

The Beta 3 export companion fix passes 107 Go tests, eight frontend tests,
formatting checks, Go vet, and the TypeScript/Windows production build. Its
regressions cover portable companions for every export kind, exact reopening and
rerendering without original inputs in all three workflows, separate overwrite
approvals, option opt-out, and profile/report JSON rejection without changing the
current document.

Beta 2 adds exact portable project round trips in all three modes, including
alpha, metadata, stack cores and layer maps after original files are removed;
changed-on-disk library snapshots; corrupt archives; legacy JSON projects;
unrendered settings; companion profiles for every export type; collision checks;
profile opt-out; preset rename persistence; and CLI profile reproduction.

A Beta 2 UI pass created and renamed a preset, exported a portable project with
its companion profile, changed the color budget, reopened the saved eight-color
result, and restored options from the exported profile. The profile preference
and named preset were persisted in an isolated settings directory.

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

The beta release gate includes the current Go suite (with 246 Front Lit reference
subcases and five Python reference cases), eight frontend tests, TypeScript and
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
