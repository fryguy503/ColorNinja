# ColorNinja Studio user guide

Beta 7 adds [Color Pop, a full-color demo, and the Color Match workflow name](releases/v1.0.0-beta.7.md).
See the [calibration protocol](calibration.md) for RGB/TD provenance and print checks.

For the public Windows beta download and installation steps, see the
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

1. Choose **File → Open image**, drop an image into the window, or try the built-in artwork under **File → Recent images**.
2. Start with **Simple reducer**, choose the maximum colors, and select
   **Off**, **Gentle**, **Balanced**, or **Strong** smoothing.
3. Auto preview renders after a short pause. Turn it off to make several changes
   before generating a preview. Cancel stops the current processing job.
4. Use **Compare** for a sliding divider or **Side by side** for separate
   Original and Result panes. Scroll to zoom, drag to pan, and use Fit or 1:1
   to inspect details. Both side-by-side panes share zoom and pan.
5. **Export** opens a dialog for PNG, palette reports, portable projects, and,
   in stack mode, layer maps and HueForge projects. PNG keeps original dimensions.

The output dock beneath the image has **Colors**, **Filaments**, and **Image info**
views. It retains hex-color copying, proportional coverage, selected filaments,
and quality metrics. Collapse it with the chevron; the image toolbar's output
button restores it. **Advanced** reveals technical tuning. Its saved toggle
only changes visibility; it never resets your processing options. Presets,
undo/redo, recent images, projects, and library filters remain available.

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

Select a workflow from the menu. Click **(i)** beside it for a short explanation
of the current mode and its best use. Click again or press Escape to close it.

| Mode | Color budget | Result |
| --- | --- | --- |
| Simple reducer | Maximum colors **in total** for new desktop settings | A reduced palette without dithering. 8 means at most 8 colors, including neutrals. Older projects retain separate budgets (up to twice the count); Advanced lets you change this. |
| Filament guided | Maximum eligible filament anchors | Colors pulled toward nominal filament colors and approximate TD-aware pairwise hues. Open the PNG in HueForge for its final print plan. |
| Color Match | Maximum filaments in one ordered stack | A contiguous layer schedule, reachable palette, and optional layer-index image using the validated HueForge Front Lit calculation. |
| Color Pop | Total prepared colors, or maximum stack filaments | Keep sampled hues and turn the rest grayscale. Prepare a PNG without a library, or plan separate color/grayscale height bands. |

For Color Pop, start with **Try demo** to compare the full-color poppy with its
red-and-grayscale result. Pick colors directly from the original, adjust the
hue range, and inspect **Selection** before exporting. **Prepare image** uses
the familiar reducer controls; **Plan filament stack** adds height allocation,
band order, and a boundary gap. See the [Color Pop guide](color-pop.md).

**Color Match** was previously named **Global stack**. The name now follows the
HueForge workflow of assigning image colors to layer heights. Existing saved
projects and presets keep their settings and behavior; the CLI still uses
`--hueforge-stack` for compatibility.

Switching modes preserves your color budget and other tuning. Built-in presets
change only the color budget; custom presets restore saved tuning while keeping
the active mode. **Reset** deliberately restores default settings for that mode.
New desktop settings and Reset use 8 colors/filaments. The slider covers 1–32;
larger saved budgets expand it automatically. **Advanced → Color & detail
controls → Exact budget** retains the full supported range.

The inspector separates **Tune**, **Filaments**, and **Layers** (or **Optics**
in Filament Guide). Simple Reducer shows Tune directly; **File → Filament library**
still opens the collection without changing workflows. Explanations are available
from the workflow information button and the collapsed help sections.

**Preserve details** is enabled by default in every mode, including older
projects and presets that did not explicitly turn it off. It protects small,
coherent marks during palette selection and maps analyzed color groups
consistently to constrained output palettes. Stack mode also refines filament order and layer allocation
after its initial search. These changes apply to every image and color family.
The existing color, filament, and layer limits still apply; very tight budgets
can still merge features. Turning it off disables that additional detail
protection; smoothing and Oklab color matching continue to work.

**Color priority** controls how the palette spends its limited colors:

- **Overall balance** retains the existing area-weighted reduction.
- **Distinctive colors** gives different hue families and smaller accents more
  weight, consolidates nearly duplicate shades, and protects essential neutrals.
  Start here when a background's many shades crowd out the colors that define
  the image.
- **Vivid colors** puts a stronger emphasis on colorful accents and hue accuracy,
  with more simplification of subtle shading. It does not boost image saturation.

Priority modes share the existing color ceiling across all families. For example,
an older 8-per-population setting becomes a displayed maximum of 16 shared
colors; choosing a priority does not raise that ceiling. A simpler image may use
fewer colors. Smoothing and Preserve details stay unchanged. The choice also
influences filament selection and stack search, while respecting the available
filaments, output-color limit, and reachable layer colors.

These are automatic color heuristics, not object recognition: they cannot know
which person or object matters most to you. Compare the alternatives at your
intended output size. See [palette priority](palette-priority.md) for the method
and validation.

Smoothing presets keep the **Preserve details** checkbox unchanged.
**Off** disables smoothing, **Gentle** uses a 1 px radius,
**Balanced** uses the existing 1.5 px radius and 5 Delta E76 color tolerance,
and **Strong** uses 2.5 px and a tolerance of 10. Strong removes stronger
texture while protecting contrasting outlines; inspect low-contrast features
with Compare or 1:1. Advanced exposes both values. **Custom** indicates that
your settings differ from a preset, or an imported legacy color pipeline is active.
Smoothing reduces texture; a small palette still produces stepped gradients.

Palette analysis can use a smaller image, but mapping and export use every
original-resolution pixel. Detail smoothing runs before both analysis and
mapping with either preservation setting; zero disables smoothing. In dev.3,
turning preservation off switched back to palette-only blur and CIELAB matching,
which could produce mottled patches in otherwise smooth areas. Dev.4 separates
these controls. Alpha is preserved, and
fully transparent pixels do not influence the palette.

PNG, JPEG, WebP, GIF, TIFF, and BMP are supported. Animated WebP and GIF use the
first frame on the full canvas. EXIF orientation is applied; supported ICC
profiles are converted to sRGB, and PNG export preserves available DPI metadata.
The engine processes 8-bit RGBA, not HDR/16-bit photo data. Input limits are
100 megapixels and 512 MB encoded size. Large images and wide stack searches
use more memory and processing time.

## Filament libraries and layer maps

In **Color Match → Layers**, enable **Choose depth automatically**
and enter a **Hard maximum depth**, for example **4.0 mm**. The ceiling includes
the base and first layer. A value between printable layer heights rounds down.
The base thickness, spool budget, filament filters, color priority, and run limit
still constrain the search. Automatic depth starts off and is saved with projects,
presets, and processing settings. Turning it off snaps the depth down to a valid
manual layer height.

The planner compares complete stacks ending at different heights and selects
the thinnest candidate within 1% of the best weighted color score found. This
is a bounded beam search, not a proof of a globally optimal print. Color priority
and detail settings affect that score. Deeper ceilings increase planning time;
no extra thickness is added merely to fill the ceiling. The result reports the
chosen depth, printable ceiling, and number of depths compared.

Choose **Visualize stack** in the output dock's heading to open the
interactive **Stack map**. Its horizontal tracks show physical filament runs,
predicted Front Lit colors at each layer, and the Color Match mesh targets.
Click a cell or use **Inspect layer** to see its height, filament/material/TD,
predicted color, and actual visible-image coverage. A striped mesh cell is
excluded as an image surface target; the physical layer can still print beneath
higher surfaces. Repeated uses of the same spool have separate run numbers.
For Combo, Color Aware, and Color Pop, the view explains that HueForge rebuilds
heights and there is no separate exported mesh core. Stale previews are labeled.

The mesh targets use the same band construction as HFP export. HFP reserves one
unused layer of import headroom, which can make its depth control exceed the
chosen print depth; that headroom is disabled for matching and adds no physical
layer to Color Match output. The hard ceiling constrains the printable plan.
CLI: `--hueforge-stack --hueforge-auto-depth --hueforge-max-depth 4.0`.

The usual HueForge library is detected at:

```text
%APPDATA%\HueForge\Filaments\personal_library.json
```

The **Filaments** tab lets you select a library, search colors, filter materials,
include unowned entries, or allow the primary color of dual-color filaments.
The library is read-only. Invalid entries and entries outside the active filters
are excluded. A filter with no eligible colors produces an explanation.

Enable **Avoid silk & metallic finishes** in the Filaments tab to exclude those
finishes from both Filament Guide and Color Match, including repeated filament
runs. It checks material names, filament names, and tags for silk, metallic,
pearl, Elixir, and Starlight, without assuming that gold or silver colors are
metallic. The eligible count updates immediately. This optional filter starts
off, is saved in preferences and ColorNinja projects, and remains selected when
changing libraries. Simple Reducer does not use the filament library.
The CLI equivalent is `--hueforge-avoid-silk-metallic`.

**Use true black** is enabled by default in filament-guided and Color Match
modes, including when opening older preferences, projects, and presets. It
models dark, near-neutral filaments with “Black” in their name as `#000000`;
other filaments retain their library colors. In guided mode, targets matched
directly to black become exact black when guidance is above zero. Stack blends
use the black anchor throughout the optical calculation, preserving layer-map
consistency. Perceptual reduction and zero-strength guidance are unchanged.
Turn the option off to use the original library colors. Transmission values
and the library file remain unchanged; reports retain overridden `libraryRGB`.

Guidance is the usual preprocessing workflow. Color Match mode uses
TD-aware Front Lit layer-color predictions with HueForge 0.9.4.3 compatibility.
Under **Layers** or **Optics**, match the lighting, first layer height, regular
layer height, base depth, and maximum total depth. New defaults use a 0.16 mm
first layer and 0.08 mm regular layers: a 0.48 mm base is five physical layers.
Old saved calculations keep the legacy approximation; explicitly select
**HueForge Front Lit** to upgrade them. Legacy TD scale/transmission controls
apply only to the legacy model. Turn off **Use true black** to compare with
unchanged library colors in HueForge.

Software color predictions do not guarantee physical print colors and do not
simulate matte or shiny surface appearance. See [validation](validation.md)
for the test scope. The layer map stores literal one-based layer counts, with
zero meaning transparent. It is not a normalized grayscale height map, STL,
or G-code. The JSON report includes both layer heights, the model, lighting,
and stack schedule. For layer N above zero, height is first-layer height plus
`(N - 1) * regular-layer height`.

Color Match also supports **Allow filament returns**, with a separate run limit
while the filament budget counts unique spools. Open **Export** and select
**HueForge project (.hfp)** to configure mesh mode/core, width, and detail.
Start with **Color Match** and **Tuned image colors (recommended)**, then refresh from the
dialog if the preview is stale before exporting. The project
embeds the reduced image, physical Color Core, and virtual Mesh Core. See
[the HFP guide](hueforge-project.md) for the tested import workflow and limits.

## Projects, presets, and profiles

**Save project** (Ctrl+S) creates a portable `.colorninja` file. It contains the
normalized source image and metadata, all processing settings, a filament library
snapshot, and the current rendered result with stack/layer data. **Open project**
(Ctrl+Shift+O), or dropping a project onto the window, restores it without the
original files. Saved previews open immediately; Refresh preview explicitly
recalculates them. When saving unrendered settings, the project contains those
settings and its source; a result is generated on reopening.

The export menu includes **ColorNinja project (.colorninja)** for preserving the
exact current export. Older `.colorninja.json` projects can still be opened when
their referenced files exist. Save again to make them portable.

**Presets & profiles**, directly below Workflow, opens a dialog with named reusable settings.
Create a preset from the current settings, click its name to apply it, or use its
rename/delete controls. Saving the same name replaces that preset. Presets keep
the current workflow by default; clear **Keep current workflow** to restore the
saved mode as well. Presets are stored in local preferences and do not contain
images or change the current filament library.

**Save profile** writes a `.colorninja-profile.json` with processing options and
filament filters. **Load profile** restores those options, including workflow,
for the current image. It does not embed an image or library. Filament exclusions
refer to library entries, so they are retained only when the library fingerprint
matches; otherwise the desktop clears individual exclusions. Save imported settings
as a named preset if you want to keep them in the preset list.

Enable **Also save project and settings profile** in the export menu to save a
portable project and a reusable settings profile beside every PNG, palette report,
layer map, or HFP export. For example, exporting `Totoro_Test1.hfp` also creates:

- `Totoro_Test1.hfp.colorninja`: open this with **Open project** to restore the
  source image, embedded filaments, settings, and exact rendered result.
- `Totoro_Test1.hfp.colorninja-profile.json`: use **Presets & profiles → Load
  profile** to apply the settings to another image.

Exporting a ColorNinja project adds only the profile, without a duplicate project.
The checkbox is remembered, including a checked Beta 2 **Also save settings profile**
preference. Both companions describe the exported preview's actual settings.
Existing companion files require separate overwrite approval before any files are
written. Each file is saved atomically; a later I/O failure identifies the files
already saved. With the option unchecked, only the selected export is written.

A settings profile or palette report cannot restore a full project. **Open
project** identifies these JSON files and explains how to use them. The default
file filter shows projects; **Older project JSON** also shows legacy JSON names.

Ctrl+O opens an image, Ctrl+E exports PNG, and Ctrl+Z / Ctrl+Shift+Z undo/redo tuning.
Preferences live under `%APPDATA%\ColorNinja`.

For batch exports, add `--colorninja-project design.colorninja` and
`--export-profile`. Reuse a profile with `--settings-profile FILE`; it overrides
processing flags and cannot be combined with `--options-json`. The CLI requires
the matching library when a profile includes individual filament exclusions.
Portable project libraries are limited to 32 MB.

## Version and updates

Click **Preferences** in the bottom bar. Workspace preferences and version/update
settings share this dialog. **Check now** reads this project's public
GitHub releases; **Check for updates on startup** is enabled initially and can
be disabled. **Include beta / alpha builds** is off initially; enable it to
receive preview builds. The latest eligible semantic version is offered, even
if releases were published out of order. Older versions are never offered as
updates. If there are only alpha releases, the stable channel says that no
stable releases are available yet.

An available update remains visible in the bottom bar. **Download & release
notes** opens its GitHub page. Extract the new portable ZIP, close the old app,
and launch the new copy. Checks never install or replace files. Images, library
contents, and project paths are not sent to GitHub. Offline and rate-limit errors
appear in this panel without interrupting image processing. Checks time out
after 15 seconds. Successful startup results are cached for 15 minutes in this
session; repeated manual checks within one minute reuse the last result.

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
32 colors per population, or 4 anchors with a library. New GUI settings start at
8 total colors. Add `--total-colors --colors 8` for the same total limit in the
CLI. Add `--preblur-sigma 2.5 --smoothing-color-tolerance 10` for Strong smoothing.
Use `--color-priority distinctive` or `--color-priority vivid` to favor accents.
`--color-priority balanced` retains the previous selection. Priority modes use
the current color pipeline and cannot be combined with `--legacy-color-pipeline`.
This also works with `--preserve-details=false`. For compatibility with the old
color pipeline, use `--legacy-color-pipeline --preserve-details=false`; this
restores CIELAB matching and palette-only Gaussian blur. The options JSON field
is `legacyColorPipeline`, default false. Choosing a smoothing preset or changing
its radius or color tolerance in the GUI returns to the current color pipeline.
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
