# ColorNinja Studio

<img src="build/appicon.png" alt="ColorNinja Studio icon" width="96" align="right" />

**Perceptual color reduction and filament-aware image preparation, on your desktop.**

ColorNinja Studio turns images into smaller, deliberate palettes and helps prepare
artwork for HueForge. It combines a Go image engine, a Wails desktop interface,
and a command-line tool. Image processing happens locally; no account or cloud
service is required.

**Current release: `v1.0.0-alpha.1` — Windows x64.** This is an early testing
release. Expect rough edges, and review your output before using it in a print.

[Download the Windows alpha](https://github.com/fryguy503/ColorNinja/releases/tag/v1.0.0-alpha.1)
· [User guide](docs/user-guide.md)
· [Report a bug](https://github.com/fryguy503/ColorNinja/issues)
· [MIT license](LICENSE)

## Download and run

1. Open the [alpha release](https://github.com/fryguy503/ColorNinja/releases/tag/v1.0.0-alpha.1)
   and download **`ColorNinja-1.0.0-alpha.1-windows-x64.zip`** from **Assets**.
2. Extract the entire ZIP to a folder you can write to.
3. Open **`ColorNinja.exe`**. Try the built-in artwork, or open your own image.
4. Choose a mode, adjust the color budget, compare the preview, and **Export PNG**.

The desktop app requires Windows x64 and the
[Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/).
The build is unsigned, so Windows may display a publisher or reputation warning.
Python, Node.js, and Go are **not required to run the release**. The CLI does not
require WebView2. macOS and Linux binaries are not included in this alpha.

The portable ZIP includes the desktop app, `colorninja-cli.exe`, a quick-start
guide, documentation, MIT and third-party license notices, build information,
and checksums. GitHub's **Source code** archives are for building the project;
choose the Windows ZIP to run the compiled app.

To verify the downloaded archive in PowerShell, compare its SHA-256 with the
release's `SHA256SUMS.txt`:

```powershell
Get-FileHash .\ColorNinja-1.0.0-alpha.1-windows-x64.zip -Algorithm SHA256
```

## What it does

- Reduce colors without dithering, with detail preservation and adjustable smoothing.
- Use owned filament colors and transmission distance (TD) from a HueForge library.
- Explore an approximate global filament stack and export its layer-index map.
- Compare original and result with a sliding divider or linked side-by-side views.
- Zoom, pan, inspect unique color counts, and review palettes, coverage, and quality metrics.
- Save projects and presets, undo settings, and export PNGs and JSON palette reports.
- Preserve source dimensions and transparency; apply EXIF orientation and supported ICC profiles.
- Read PNG, JPEG, WebP, GIF, TIFF, and BMP. Animated inputs use the first frame.

## Choose a processing mode

| Mode | Best for | How the budget works |
| --- | --- | --- |
| **Perceptual reduction** | Simplifying an image without a filament library | Separate chromatic and achromatic budgets; 8 can produce up to 16 colors. |
| **Filament guided** | Preparing a PNG for final planning in HueForge | Maximum eligible filament anchors; nominal colors and approximate TD-aware blends guide the palette. |
| **Global stack** | Exploring a single ordered filament/layer plan | Maximum filaments in one stack, using ColorNinja's independent optical approximation. |

**Start with Filament guided for the usual HueForge workflow.** Open the exported
PNG in HueForge to make the final print plan. The Filaments tab detects the usual
`%APPDATA%\HueForge\Filaments\personal_library.json` location and lets you choose
another library. Your library is read-only. Material, brand, TD, and color are
shown so similarly named filaments remain distinguishable.

Detail preservation is enabled by default. Guided and stack modes also default
to **Use true black**, which models eligible black filaments as `#000000` without
changing the library. Both options can be disabled for comparison.

## Alpha limitations

- Global stack is an approximate optical model, not HueForge's engine or a
  guaranteed physical color match. ColorNinja is an independent project and is
  not affiliated with or endorsed by HueForge.
- The optional 16-bit layer map contains literal one-based layer counts, with
  zero for transparency. It is not a normalized height map, STL, or G-code.
- Processing uses 8-bit RGBA. HDR and 16-bit photo precision are not preserved.
  Inputs are limited to 100 megapixels and 512 MB encoded size.
- Projects reference source files and library paths; they do not embed those files.
- This release is portable and unsigned. There is no installer or automatic updater.
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

If an older desktop executable is open, use `-GuiName ColorNinja-Alpha.exe` with
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
- [Alpha release notes](docs/releases/v1.0.0-alpha.1.md).
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
