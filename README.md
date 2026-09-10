# ColorNinja Studio

<img src="build/appicon.png" alt="ColorNinja Studio icon" width="96" align="right" />

**Perceptual color reduction and filament-aware image preparation, on your desktop.**

ColorNinja Studio turns images into smaller, deliberate palettes and helps prepare
artwork for HueForge. It combines a Go image engine, a Wails desktop interface,
and a command-line tool. Image processing happens locally; no account or cloud
service is required.

**Version `v1.0.3` — Windows, Linux, and macOS.** Color Match and Color Pop are the supported
stack workflows. Color Match includes a draggable source-color order, adjustable
preference strength, and automatic recalculation. Backlit, Simple reducer, Filament
Guide, portable projects, and self-contained HueForge exports remain available.
The new channel workflows are temporarily disabled.

[Download ColorNinja 1.0.3](https://github.com/fryguy503/ColorNinja/releases/tag/v1.0.3)
· [Release notes](docs/releases/v1.0.3.md)
· [User guide](docs/user-guide.md)
· [Report a bug](https://github.com/fryguy503/ColorNinja/issues)
· [MIT license](LICENSE)

See the [color-order guide](docs/color-order.md) and
[validation and remaining acceptance work](docs/production-readiness.md).
The packages have no publisher signature; macOS builds are not notarized. Software checks do not replace physical
print validation with your own filaments and lighting.

## Download and run

Open the [1.0.3 release](https://github.com/fryguy503/ColorNinja/releases/tag/v1.0.3)
and choose the archive for your operating system and processor from **Assets**.

| System | Archive | Launch and requirements |
| --- | --- | --- |
| Windows x64 | `ColorNinja-1.0.3-windows-x64.zip` | Extract, then open `ColorNinja.exe`; requires [WebView2](https://developer.microsoft.com/en-us/microsoft-edge/webview2/). |
| Linux x64 / ARM64 | `ColorNinja-1.0.3-linux-x64.tar.gz` / `linux-arm64.tar.gz` | Extract, then run `./ColorNinja`; built on Ubuntu 24.04 with GTK 3 and WebKitGTK 4.1. |
| macOS Intel / Apple Silicon | `ColorNinja-1.0.3-macos-x64.zip` / `macos-arm64.zip` | Extract, then open `ColorNinja.app`; native builds tested on macOS 15. |

On Ubuntu 24.04, install the desktop runtime with
`sudo apt install libgtk-3-0t64 libwebkit2gtk-4.1-0`. Other Linux distributions
need compatible GTK, WebKitGTK and glibc libraries; older distributions are not
covered by these builds. Windows may show a publisher warning. On macOS, after
trying to open the app, use **System Settings → Privacy & Security → Open Anyway**
if Gatekeeper blocks this unnotarized download. Only approve your verified download.

Try the built-in artwork, or open an image. Choose a mode, adjust the color budget,
compare the preview, then choose **Export → Full-resolution PNG**.
Python, Node.js, and Go are **not required to run the release**. Linux and macOS
support common matrix ICC profiles; convert images using other ICC profiles to
sRGB before importing. Native window and file-dialog acceptance on these new
platforms remains open.

Each archive includes the desktop app, CLI (`colorninja-cli` on Linux/macOS), a quick-start
guide, documentation, MIT and third-party license notices, build information,
and checksums. GitHub's **Source code** archives are for building the project;
choose a platform archive to run the compiled app.

To verify the downloaded archive in PowerShell, compare its SHA-256 with the
release's `SHA256SUMS.txt`:

```powershell
Get-FileHash .\ColorNinja-1.0.3-windows-x64.zip -Algorithm SHA256
```

## What it does

Choose **Workflow → Color Pop → Try demo** to compare a full-color original
with a selective-color result. See the [Color Pop guide](docs/color-pop.md).

- Reduce colors without dithering, with independent smoothing and detail preservation.
- Prioritize distinctive or vivid colors while simplifying similar shades.
- Optionally exclude silk, metallic, and related finishes from filament selection.
- Use owned filament colors and transmission distance (TD) from a HueForge library.
- Plan a Front Lit filament stack with Color Match, optionally reuse filaments in later runs,
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
| **Color Match** | Exploring a single ordered filament/layer plan | Maximum filaments in one stack, using TD-aware Front Lit color predictions. |
| **Color Pop** | Keeping selected hues against grayscale | Prepared palette colors, or filaments for separate color/grayscale height bands. Includes a full-color poppy demo. |

**Start with Filament guided for the usual HueForge workflow.** Open the exported
PNG in HueForge to make the final print plan. The Filaments tab automatically
detects these HueForge library locations when no library has been selected:

- Windows: `%APPDATA%\HueForge\Filaments\personal_library.json`
- macOS (Intel and Apple Silicon): `~/Library/Containers/com.thehueforge.hueforge/Data/Documents/HueForge/Filaments/personal_library.json`

You can choose another library; ColorNinja remembers your selection across
relaunches. Your library is read-only. Material, brand, TD, and color are shown
so similarly named filaments remain distinguishable.

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

# Create a Color Match plan and export its layer-index image.
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
- [1.0 release notes](docs/releases/v1.0.0.md).
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
