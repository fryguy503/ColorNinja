# Beta 6 implementation and acceptance

Target: `1.0.0-beta.6`. Baseline: Beta 5, `93486a3`.

This checklist tracks the September 9 application/HueForge review. A checked
item means implemented and verified, not merely scheduled. Native/physical
acceptance remains separate from software fixtures.

- [x] Coherent diagonal/curved details and detail sampling across resolutions.
- [x] Correct CIELAB report units independent of preservation/priority settings.
- [x] Alternative heights for repeated RGB colors, compatible with HFP matching.
- [x] Boundary sampling that retains thin features; physical scale diagnostics.
- [x] Image boundary overlay and approximate surface/coverage preview.
- [x] Diverse stack finalists, structural refinement, independent search effort.
- [x] Comparable color/boundary/swap/depth alternatives.
- [x] Required filament, base/highlight, and protected color controls.
- [x] Guided source/output/reference explanation, global-stack action, exclusion comparisons.
- [x] Image-linked stack inspection and saved result comparisons.
- [x] Reuse processing stages while retaining stale-result/export guarantees.
- [x] Calibration provenance/sensitivity and reproducible swatch protocol.
- [x] HueForge feedback: fitted IMAGE TDs, fewer disables and compact blend paths;
      retain actual Color Core/TDs and validate every used height natively.
- [x] Make tuned Mesh Core TDs the default in desktop and CLI; upgrade saved
      planned-color settings and refresh cached project cores without changing print heights.
- [x] UI/service integration coverage, versioned docs, Windows build/package.
- [x] HueForge native matching regression and inspection of available mesh exports.
- [ ] Controlled new-mesh comparison for triangle interpolation, extent rounding,
      and material lighting; the Beta 6 surface stays explicitly approximate.
- [ ] Human acceptance: physical swatches and prints, native dialogs, clean account,
      display scaling, drag/drop and keyboard operation.

Future integration research: branching ColorDrop cores and the public plugin API
depend on a usable published contract and a different multiple-material-per-layer
planner. Record that dependency rather than claiming single-stack returns support it.
