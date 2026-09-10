# Release validation and remaining acceptance

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
