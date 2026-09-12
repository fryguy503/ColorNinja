# Release validation and remaining acceptance

Version 1.2.2 checks simpler filament schedules in both search modes and favors
fewer runs on equal scores. Regression tests reproduce a buried return whose
direct merge is initially worse, then verify that reallocating the simpler
schedule improves the objective. Coverage includes required spools, optically
necessary buried layers, determinism and cancellation. The saved-project check
verifies every layer assignment and the exported run schedule. See the
[1.2.2 notes](releases/v1.2.2.md). Physical print acceptance remains separate.

Version 1.1.6 adds HueForge SpotFix interchange and synchronized border color/depth
controls. Regression coverage includes whole-region ownership, overlapping edits,
clamping, target-layer assignment, transparency, partial-selection fallback,
project round trips, depth snapping and border color selection. Twelve HueForge
0.9.4.3 compatibility fixtures preserve the expected pixel heights. See the
[1.1.6 notes](releases/v1.1.6.md), [Region Edit](region-editing.md) and
[borders](borders.md). Physical-print and full native-window acceptance remain open.

Version 1.1.5 fixes native drag interference with Region Edit, optimizes repeated
filament refinement scoring, and adds optional HueForge borders. Exact scorer
comparisons, bounded-cache tests, border fixtures, persistence and browser checks
are part of the release gates. See [1.1.5 notes](releases/v1.1.5.md),
[refinement measurements](refinement-performance.md) and [borders](borders.md).

Version 1.1.0 adds Region Edit and printable depth entry. Engine/service tests
cover topology, selection geometry, protected footprints, clamping, restoration,
history, stale requests, malformed projects and exact saved-plan replay. Export
tests include Color Pop and Backlit. Browser checks use the real compiled Go
service for lasso/polygon/box/brush, zoom/pan, undo/redo, depth snapping, resizing
and portable exports. See [1.1.0 notes](releases/v1.1.0.md) and the
[Region Edit guide](region-editing.md). These earlier checks did not cover HueForge SpotFix interchange.
Version 1.1.6 adds the coverage described above; physical-print accuracy remains open.

Version 1.0.3 adds adaptive parallel processing and comparison pane swapping.
Checks cover deterministic results across CPU limits, bounded workers and
working-memory estimates, cancellation, race detection, and browser pane
behavior. See [1.0.3 notes](releases/v1.0.3.md) and [performance](performance.md).
Minimum-spec physical-machine endurance remains open.

Version 1.0.2 adds automatic discovery of the HueForge library in its macOS
container on Intel and Apple Silicon. Regression tests cover both platform
layouts, first launch, relaunch, saved selections, and missing files or user
directories. See [1.0.2 notes](releases/v1.0.2.md). Native-window, file-dialog,
and clean-machine acceptance remains open.

Version 1.0.1 adds native Linux x64/ARM64 and macOS Intel/Apple Silicon packages.
Their release gates include native builds, Go/frontend tests, vet, dependency
audits, extracted-file hashes, four CLI workflows, and desktop HTTP service
startup. Native-window, file-dialog, clean-machine, and signing/notarization
acceptance for these platforms remains open. See [1.0.1 notes](releases/v1.0.1.md)
for platform requirements and the layer-planning changes. The checklist below
records the earlier Windows release scope.

Version 1.0 focuses release scope on Color Match and Color Pop. New channel workflows
are paused; their retained engine fixtures are development evidence only.
This work extends the existing Beta 7 development changes. A checked item means
implemented and verified; an unchecked item is still open. Software compatibility,
desktop acceptance, and physical print acceptance are separate evidence.

## Implementation

- [x] Brightness planning: Standard, Combo, Max Channel, Scaled Max Channel.
- [x] Color Aware region planning with explicit bands and predictable controls.
- [x] Color Pop depth, refinement, constraints, and boundary optimization.
- [x] Backlit optical prediction with independent native reference cases.
- [x] Desktop, CLI, project, profile, preset, comparison, and HFP integration.
- [x] Existing project regression tests and current development fixes retained.
- [x] Full-resolution demo acceptance, thin-feature regressions, synthetic timing benchmark, and machine-readable results.
- [x] Maintained build/browser/package checks and Windows CI definition.
- [x] Windows package checksums, build identity, and source-file manifest.
- [x] Updated user documentation, CLI examples, and compiled Windows release.
- [ ] Exercise Authenticode signing integration with a real publisher identity.

## Acceptance

- [x] Existing Go suite passes before implementation.
- [x] Engine and workflow regressions pass after implementation.
- [x] Native HueForge compatibility checks pass for tested calculations and HFP height encoding.
- [x] Frontend state tests, formatting, type checks, Go vet, and production build.
- [x] Browser acceptance: paused mode menu, color-order drag/drop and keyboard controls, strength, persistence, stale-export guard, responsive layout, Escape and undo.
- [x] Cancellation during planning, invalid settings, and existing malformed/oversized input regressions.
- [x] Extracted Windows package checksums, Color Match/Color Pop and weighted-order cases in both light models, plus disabled-channel rejection.
- [x] Dependency audits: no known findings from npm audit or govulncheck.
- [ ] Hosted Windows CI run after pushing the reviewed source.
- [ ] Long-session endurance and large-image memory testing on minimum-spec hardware.
- [ ] Physical swatches and held-out prints under recorded conditions.
- [ ] Clean Windows account and full native-dialog acceptance.
- [ ] External release-candidate acceptance.
- [ ] Signed, clean-source final release, with verified published artifacts.

Physical hardware, external users, and a publisher signing identity are required
to complete their respective acceptance items. A successful software build does
not check those boxes or constitute production publication.

## Reproduce the software checks

From the source checkout, after `scripts/bootstrap.ps1`:

```powershell
./scripts/build.ps1
./scripts/audit.ps1
node ./scripts/verify-ui.cjs
./scripts/package.ps1
./scripts/verify-release.ps1 -Archive build/releases/ColorNinja-1.0.0-windows-x64.zip
```

The browser check uses installed Microsoft Edge and a private settings directory
under `artifacts`. It starts and stops only its own loopback test process.
The package check uses the compiled CLI from a freshly extracted ZIP, verifies
every file, exercises Color Match, weighted order, and Color Pop in both light models, and checks
that an existing output cannot be overwritten without `--force`. Evidence goes
under `artifacts/ui-acceptance` and `artifacts/release-verification`.

`BUILD-INFO.json` records test counts, versions, binary hashes, signing state,
the Git base commit and whether the source tree contained edits. A
`SOURCE-MANIFEST.json` records exact source-file hashes for local edited builds.
Neither a source manifest nor a hash is a publisher signature.

## Remaining acceptance and signed release procedure

1. Run the [calibration protocol](calibration.md): measured TD swatches and held-out
   prints covering neutral gradients, saturated colors, tiny accents, returns,
   region transitions, and Backlit illumination. Record actual failures and
   compare against an agreed reference before accepting each workflow.
2. On a clean supported Windows account, test WebView2 availability, native file
   selection, overwrite confirmations, Unicode paths, scaling, shutdown/restart,
   portable project reopening, and a long processing/cancellation session.
3. Obtain external candidate acceptance. Review and commit the intended source,
   run hosted CI, and build the final version from the clean commit.
4. With a real signing identity installed, use
   `scripts/build.ps1 -SigningCertificateThumbprint <thumbprint> -SignToolPath <signtool.exe>`.
   Signing uses SHA-256, an RFC 3161 timestamp, and signature verification before
   recording hashes. No certificate files or passwords belong in the repository.
5. Package once and run `scripts/verify-release.ps1 -Archive <zip> -RequireSigned`.
   This rejects unsigned executables and edited-source provenance. Publish only
   the verified assets, checksums, and accurate acceptance notes; verify the
   public tag and downloaded assets afterward.

Version 1.0 is published on the stable release channel at the project owner's
request, with unsigned Windows executables. Its source is committed before the
release build, and the packaged source identity, hashes, and software verification
are checked against that commit. Signing, physical-print trials, clean-machine
native dialogs, minimum-spec endurance, and external acceptance remain open;
publication does not mark those items complete. The procedure above describes
the remaining work for a signed, externally accepted distribution.
