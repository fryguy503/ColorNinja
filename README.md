# ColorNinja Studio

<img src="build/appicon.png" alt="ColorNinja Studio icon" width="96" align="right" />

**Perceptual color reduction and filament-aware image preparation, on your desktop.**

ColorNinja Studio turns images into smaller, deliberate palettes and helps prepare
artwork for HueForge. It combines a Go image engine, a Wails desktop interface,
and a command-line tool. Image processing happens locally; no account or cloud
service is required.

**Current beta: `v1.0.0-beta.6` — Windows x64.** This release
build adds stronger detail preservation, physical-scale stack diagnostics,
search alternatives, required spools, protected colors, saved comparisons,
reusable processing stages, and tuned Mesh Core TDs by default. See the
[Beta 6 notes](docs/releases/v1.0.0-beta.6.md) and [acceptance checklist](docs/beta6-plan.md).

[Download the Windows beta](https://github.com/fryguy503/ColorNinja/releases/tag/v1.0.0-beta.6)
· [User guide](docs/user-guide.md)
· [Report a bug](https://github.com/fryguy503/ColorNinja/issues)
· [MIT license](LICENSE)

## Download and run

If you already have a Windows ZIP, start at step 2. Local builds are packaged in
`build/releases`.

1. Open the [beta release](https://github.com/fryguy503/ColorNinja/releases/tag/v1.0.0-beta.6)
   and download **`ColorNinja-1.0.0-beta.6-windows-x64.zip`** from **Assets**.
2. Extract the entire ZIP to a folder you can write to.
3. Open **`ColorNinja.exe`**. Try the built-in artwork, or open your own image.
4. Choose a mode, adjust the color budget, compare the preview, and choose **Export → Full-resolution PNG**.

The desktop app requires Windows x64 and the
[Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/).
The build is unsigned, so Windows may display a publisher or reputation warning.
Python, Node.js, and Go are **not required to run the release**. The CLI does not
require WebView2. macOS and Linux binaries are not included in this beta.

The portable ZIP includes the desktop app, `colorninja-cli.exe`, a quick-start
guide, documentation, MIT and third-party license notices, build information,
and checksums. GitHub's **Source code** archives are for building the project;
choose the Windows ZIP to run the compiled app.

To verify the downloaded archive in PowerShell, compare its SHA-256 with the
release's `SHA256SUMS.txt`:

```powershell
Get-FileHash .\ColorNinja-1.0.0-beta.6-windows-x64.zip -Algorithm SHA256
```

## What it does

- Reduce colors without dithering, with independent smoothing and detail preservation.
- Prioritize distinctive or vivid colors while simplifying similar shades.
- Optionally exclude silk, metallic, and related finishes from filament selection.
- Use owned filament colors and transmission distance (TD) from a HueForge library.
- Plan a Front Lit global stack, optionally reuse filaments in later runs,
  choose depth below a hard maximum, and inspect an interactive stack map.
- Optionally reduce layer show-through by considering neighboring image colors
  when choosing stack order and layer allocation.
- Export a stack layer-index map or a HueForge `.hfp` project with both cores and
  configurable mesh settings, including Color Match.
- Compare original and result with a sliding divider or linked side-by-side views.
- Zoom, pan, inspect unique color counts, and review palettes, coverage, and quality metrics.
- Save portable projects with embedded images, filaments, settings, and exact results.
- Create and manage named presets, load/save settings profiles, and optionally write
  a portable project and profile beside every export. Undo settings and export PNGs and palette reports.
- Preserve source dimensions and transparency; apply EXIF orientation and supported ICC profiles.
- Read PNG, JPEG, WebP, GIF, TIFF, and BMP. Animated inputs use the first frame.

## Choose a processing mode

| Mode | Best for | How the budget works |
| --- | --- | --- |
| **Simple reducer** | Simplifying an image without a filament library | New desktop settings use a total limit: 8 means at most 8 colors, including neutrals. Advanced can restore separate budgets. |
| **Filament guided** | Preparing a PNG for final planning in HueForge | Maximum eligible filament anchors; nominal colors and approximate TD-aware blends guide the palette. |
| **Global stack** | Exploring a single ordered filament/layer plan | Maximum filaments in one stack, using TD-aware Front Lit color predictions. |

**Start with Filament guided for the usual HueForge workflow.** Open the exported
PNG in HueForge to make the final print plan. The Filaments tab detects the usual
`%APPDATA%\HueForge\Filaments\personal_library.json` location and lets you choose
another library. Your library is read-only. Material, brand, TD, and color are
shown so similarly named filaments remain distinguishable.

Detail preservation is enabled by default. Guided and stack modes also default
to **Use true black**, which models eligible black filaments as `#000000` without
changing the library. Both options can be disabled for comparison.

For ordinary image reduction, start with **Simple reducer**, choose a maximum
color count, then use **Off / Gentle / Balanced / Strong** smoothing. Balanced
keeps the existing edge-aware filter; Strong flattens more texture and can soften
low-contrast detail. **Advanced** reveals technical tuning without resetting
settings; the output dock's **Image info** view shows image metrics. Older
projects retain their original split budgets. The right-hand inspector separates
tuning, filament libraries, and layers/optics; presets and exports have dedicated
dialogs. See the [complete feature map](docs/ui-redesign.md).

Click **Preferences** in the bottom bar for **Preferences & updates**. Startup checks
can be disabled; **Include beta / alpha builds** opts into prereleases. The
checker opens GitHub release notes and downloads for manual installation.
Image processing remains local and works offline.

## Beta limitations

- Front Lit predictions and HFP compatibility have been checked with HueForge
  0.9.4.3. Physical print colors and a globally optimal stack are not guaranteed.
  Matte, silk, and metallic surface shine is not simulated.
  ColorNinja is independent and is not affiliated with or endorsed by HueForge.
- The optional 16-bit layer map contains literal one-based layer counts, with
  zero for transparency. It is not a normalized height map, STL, or G-code.
- Processing uses 8-bit RGBA. HDR and 16-bit photo precision are not preserved.
  Inputs are limited to 100 megapixels and 512 MB encoded size.
- New `.colorninja` projects embed source pixels, the library snapshot, and any
  current rendered result. Older JSON projects still depend on their original files.
- Builds remain portable and unsigned. The app checks for updates;
  installation is manual. There is no installer or automatic replacement of the app.
- Automated coverage is described in [validation](docs/validation.md). Complete
  native-dialog, display-scaling, and clean-account acceptance testing remains open.

## Command line

Run these commands from the extracted release folder:

```powershell
# Reduce an image's palette.
.\colorninja-cli.exe input.jpg -o output.png --colors 8

# Prepare an image with your HueForge filament library.
.\colorninja-cli.exe input.png -o guided.png `
  --hueforge-library "$env:APPDATA\HueForge\Filaments\personal_library.json" `
  --colors 8 --palette-json guided.json

# Explore a global stack and export its layer-index image.
.\colorninja-cli.exe input.png -o stack.png `
  --hueforge-library "$env:APPDATA\HueForge\Filaments\personal_library.json" `
  --colors 4 --hueforge-stack --palette-json stack.json `
  --hueforge-height-map layers.png

.\colorninja-cli.exe --help
```

Existing outputs require `--force`. Input, library, and output paths must be
distinct. See the [user guide](docs/user-guide.md) for settings and export details.
See [HueForge project export](docs/hueforge-project.md) for core settings, filament
returns, import validation, and mesh sampling limits.

## Build from source

On Windows x64, install [Git](https://git-scm.com/downloads/win) and
[Node.js 24 LTS](https://nodejs.org/), then run in PowerShell:

```powershell
git clone https://github.com/fryguy503/ColorNinja.git
cd ColorNinja
.\scripts\bootstrap.ps1
.\scripts\build.ps1
.\scripts\package.ps1
```

Bootstrap downloads verified, project-local Go 1.27.1, npm 12.0.2, and Wails
2.15.0 tools and restores locked dependencies. Build checks frontend formatting
and regression tests, TypeScript, Go tests, and `go vet`, then compiles both
executables. Outputs are written to `build/bin`; packaging writes a versioned
portable archive and its checksum under `build/releases`.

If an older desktop executable is open, use `-GuiName ColorNinja-Beta.exe` with
both build and package scripts. The scripts do not stop running applications.

For frontend development:

```powershell
. .\scripts\env.ps1
.\.tools\gopath\bin\wails.exe dev -skipbindings
```

## Documentation and feedback

- [User guide](docs/user-guide.md): controls, shortcuts, libraries, projects, and CLI options.
- [Architecture](docs/architecture.md): image engine, optical model, and desktop integration.
- [Validation](docs/validation.md): automated coverage and manual testing boundaries.
- [Beta release notes](docs/releases/v1.0.0-beta.2.md).
- [Python reference](docs/python-reference.md): the original implementation retained for regression comparison.

Please [open an issue](https://github.com/fryguy503/ColorNinja/issues) with your
ColorNinja version, Windows version, mode/settings, expected result, actual
result, and reproduction steps. Attach a small image you have permission to
share if it helps reproduce the issue. Avoid posting private library or project paths.

## License

ColorNinja is released under the [MIT License](LICENSE), copyright 2026 Fryguy.
Third-party components retain their own licenses; portable releases include
`THIRD-PARTY-NOTICES.txt`. Prism image fixtures retain their
[original MIT notice](https://github.com/fryguy503/ColorNinja/blob/main/internal/engine/testdata/PRISM-LICENSE.txt).
The generated Wails frontend runtime retains its
[MIT notice](docs/licenses/WAILS-LICENSE.txt).
