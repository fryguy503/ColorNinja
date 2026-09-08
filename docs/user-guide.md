# ColorNinja Studio user guide

For the public Windows alpha download and installation steps, see the
[project README](../README.md). The paths under `build/bin` in this guide refer
to a source checkout; the portable release uses `ColorNinja.exe` and
`colorninja-cli.exe` directly in the extracted folder.

A local desktop studio for perceptual color reduction and filament-guided image
preparation. The image engine is Go; Wails hosts a React/TypeScript interface.
The compiled application and CLI need no Python or Node.js installation.

## Run on Windows

Open `build/bin/ColorNinja-Studio.exe`, or extract a portable archive from
`build/releases` and open `ColorNinja.exe` inside it. The GUI requires Windows
x64 and Microsoft Edge WebView2 Runtime. The CLI does not require WebView2.
These local builds are unsigned.

1. Click **Open image**, drop an image into the window, or try the built-in artwork.
2. Choose a processing mode and adjust the color budget and detail smoothing.
3. Auto preview renders after a short pause. Turn it off to make several changes
   before generating a preview. Cancel stops the current processing job.
4. Use **Compare** for a sliding divider or **Side by side** for separate
   Original and Result panes. Scroll to zoom, drag to pan, and use Fit or 1:1
   to inspect details. Both side-by-side panes share zoom and pan.
5. **Export PNG** saves the rendered result at its original dimensions. The
   adjacent menu exports a palette report or, in stack mode, a 16-bit layer map.

The palette panel shows output colors, coverage, quality metrics, processing
time, and selected filaments. Presets, undo/redo, recent images, projects,
library filters, and advanced processing options are included.

Selected filaments show brand, material type, transmission distance (TD) in mm,
and layer count when planning a stack. This distinguishes same-name entries
such as PLA Blue and PLA SILK Blue. Hover a filament for its full identity and
hex color, including the original library color when true black is enabled.

Unique color counts appear above Original and Result in every preview layout.
Large counts use compact labels such as **32K colors**; hover for the exact
number. Counts use the full-resolution sRGB images, ignore fully transparent
pixels, and count the same RGB at different opacity levels only once. The
result count excludes unused palette entries and reflects the displayed preview.

## Modes and presets

Click the **(i)** beside a mode for a short explanation and its best use.
Reading help does not select the mode. Click again or press Escape to close it.

| Mode | Color budget | Result |
| --- | --- | --- |
| Perceptual reduction | Maximum colors **per population**, chromatic and achromatic | A reduced palette without dithering. A budget of 8 can produce up to 16 colors. |
| Filament guided | Maximum eligible filament anchors | Colors pulled toward nominal filament colors and approximate TD-aware pairwise hues. Open the PNG in HueForge for its final print plan. |
| Global stack | Maximum filaments in one ordered stack | A contiguous layer schedule, reachable palette, and optional layer-index image from ColorNinja's independent optical model. |

Switching modes preserves your color budget and other tuning. Built-in presets
change only the color budget; custom presets restore saved tuning while keeping
the active mode. **Reset** deliberately restores default settings for that mode.

**Preserve details** is enabled by default in every mode, including older
projects and presets. It uses Oklab color matching, smooths similar neighboring
colors while protecting edges, and maps analyzed color groups consistently to
the output palette. Stack mode also refines filament order and layer allocation
after its initial search. These changes apply to every image and color family.
The existing color, filament, and layer limits still apply; very tight budgets
can still merge features. Turn the option off to compare the original reduction.

Palette analysis can use a smaller image, but mapping and export use every
original-resolution pixel. Detail smoothing runs before both analysis and
mapping when preservation is on; zero disables smoothing. With preservation
off, the old pre-blur affects palette discovery only. Alpha is preserved, and
fully transparent pixels do not influence the palette.

PNG, JPEG, WebP, GIF, TIFF, and BMP are supported. Animated WebP and GIF use the
first frame on the full canvas. EXIF orientation is applied; supported ICC
profiles are converted to sRGB, and PNG export preserves available DPI metadata.
The engine processes 8-bit RGBA, not HDR/16-bit photo data. Input limits are
100 megapixels and 512 MB encoded size. Large images and wide stack searches
use more memory and processing time.

## Filament libraries and layer maps

The usual HueForge library is detected at:

```text
%APPDATA%\HueForge\Filaments\personal_library.json
```

The **Filaments** tab lets you select a library, search colors, filter materials,
include unowned entries, or allow the primary color of dual-color filaments.
The library is read-only. Invalid entries and entries outside the active filters
are excluded. A filter with no eligible colors produces an explanation.

**Use true black** is enabled by default in filament-guided and global-stack
modes, including when opening older preferences, projects, and presets. It
models dark, near-neutral filaments with “Black” in their name as `#000000`;
other filaments retain their library colors. In guided mode, targets matched
directly to black become exact black when guidance is above zero. Stack blends
use the black anchor throughout the optical calculation, preserving layer-map
consistency. Perceptual reduction and zero-strength guidance are unchanged.
Turn the option off to use the original library colors. Transmission values
and the library file remain unchanged; reports retain overridden `libraryRGB`.

Guidance is the usual preprocessing workflow. Global stack mode is an optional
optical approximation, not HueForge's proprietary engine or a guaranteed
physical color match. Its layer map stores literal one-based layer counts, with
zero meaning transparent. It is not a normalized grayscale height map, STL,
or G-code. The JSON report includes the layer height and stack schedule.

## Projects and shortcuts

Projects (`*.colorninja.json`) store the source path, options, library path,
and filters. They reference original files rather than embedding them, so keep
those files available. Preferences and presets live under `%APPDATA%\ColorNinja`.

| Shortcut | Action |
| --- | --- |
| Ctrl+O | Open image |
| Ctrl+Shift+O | Open project |
| Ctrl+S | Save project |
| Ctrl+E | Export PNG |
| Ctrl+Z / Ctrl+Shift+Z | Undo / redo settings |

## Go command line

```powershell
.\build\bin\colorninja-cli.exe input.jpg -o output.png --colors 8

.\build\bin\colorninja-cli.exe input.png -o guided.png `
  --hueforge-library "$env:APPDATA\HueForge\Filaments\personal_library.json" `
  --colors 8 --palette-json guided.json

.\build\bin\colorninja-cli.exe input.png -o stack.png `
  --hueforge-library "$env:APPDATA\HueForge\Filaments\personal_library.json" `
  --colors 4 --hueforge-stack --palette-json stack.json `
  --hueforge-height-map layers.png

.\build\bin\colorninja-cli.exe --help
```

The original processing flags remain available. `--chunk-pixels` is accepted
for compatibility; Go maps bounded parallel rows instead. The CLI defaults to
32 colors per population, or 4 anchors with a library. The GUI starts at 8.
The true-black option also defaults on in the CLI; `--true-black=false` disables it.
`--options-json` accepts a Go Options object and overrides processing flags;
it is not a project or old Python report. Report JSON uses a versioned Go schema
with camelCase fields.

Existing outputs require `--force` in the CLI. Source, library, and output paths
must be distinct, including hardlink aliases. Exports use temporary sibling
files and atomic writes. If an optional report or layer-map export fails after
the PNG succeeds, the CLI explicitly reports which output was saved.

## Build from source

The supplied scripts target Windows x64. Install Node.js 24 LTS, then run:

```powershell
.\scripts\bootstrap.ps1
.\scripts\build.ps1
.\scripts\package.ps1
```

Bootstrap installs pinned Go 1.27.1, npm 12.0.2, and Wails 2.15.0 tools inside
`.tools`, verifies downloaded tool archives, and restores locked dependencies.
Build checks formatting, frontend regression tests, TypeScript, Go tests, and
`go vet`, generates the original application icon, and builds the executables.
Package creates a new portable ZIP with licenses, build information, and hashes.

To build while an older executable is open, use the same alternative name for
both scripts: `-GuiName ColorNinja-Next.exe`. The scripts never stop running apps.

For frontend development:

```powershell
. .\scripts\env.ps1
.\.tools\gopath\bin\wails.exe dev -skipbindings
```

See [architecture](architecture.md) and [validation](validation.md).
The original Python program remains as a reference, documented in
[the Python reference guide](python-reference.md). The GUI and Go CLI do
not invoke it.
