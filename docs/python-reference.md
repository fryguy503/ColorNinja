# Perceptual Color Reduce

`perceptual_color_reduce.py` reduces a local image to a small, controllable
palette and writes the actual processed pixels as a lossless, full-resolution
PNG.

The implementation independently follows the observable ColorSmith-style
pipeline:

- filtered resizing to at most 6,291,456 pixels for palette analysis;
- Gaussian pre-blur for palette discovery only;
- CIE Lab perceptual color clustering;
- separate chromatic and achromatic palette budgets;
- removal of clusters smaller than 0.5% of their population;
- nearest-palette mapping over every original-resolution pixel;
- no dithering, because dithering would reintroduce spatial color noise.

It does not copy, download, or depend on ColorSmith's JavaScript or WebAssembly
engine, so its output will not be pixel-identical to ColorSmith.

## Install

Python 3.10 or newer is required.

```powershell
python -m pip install -r requirements.txt
```

## Run

```powershell
python perceptual_color_reduce.py input.jpg -o output.png
```

The default `--colors 32` is a ceiling for each population. An image containing
both chromatic and achromatic material can therefore use up to 64 colors, though
small-cluster culling commonly produces fewer. This definition applies to the
normal palette-reduction mode. With a HueForge library, `--colors N` instead
sets the maximum number of owned filament anchors, whether you use guided or
strict-stack mode.

Useful examples:

```powershell
# A smaller palette: up to 16 chromatic plus 16 achromatic colors.
python perceptual_color_reduce.py input.jpg -o output.png --colors 16

# Learn from every input pixel rather than the normal 6 MP analysis image.
python perceptual_color_reduce.py input.jpg -o output.png --full-analysis

# Also save the palette, quality metrics, dimensions, and raw-pixel hash.
python perceptual_color_reduce.py input.jpg -o output.png `
  --palette-json output-palette.json
```

Run with `--help` to see all tuning controls.

## HueForge library-guided preprocessing

This is the recommended HueForge workflow. Pass your exported filament library
with `--hueforge-library PATH` (or `--filament-library PATH`). ColorNinja selects
up to `--colors N` relevant owned filaments, then simplifies the image toward
their nominal colors and approximate TD-aware pairwise hues. By default it pulls
each analyzed hue 80% toward its closest owned-derived target, retaining some
image nuance while making the palette much more attainable. The resulting PNG
is intended to be opened in HueForge for the real stack and height conversion.

It does **not** prescribe a global filament order or swap schedule. HueForge
remains responsible for that final step.

On Windows, HueForge's personal library is normally under `%APPDATA%`:

```powershell
$filamentLibrary = Join-Path $env:APPDATA `
  'HueForge\Filaments\personal_library.json'

python .\perceptual_color_reduce.py .\input.png `
  -o .\input-for-hueforge.png `
  --hueforge-library $filamentLibrary `
  --colors 8 `
  --palette-json .\input-for-hueforge.json
```

Open `input-for-hueforge.png` in HueForge. The console and optional JSON report
list the selected filament anchors so you know which owned colors the reduction
favored. The output palette may contain more than eight colors because it also
retains image colors pulled toward perceived hues approximated from ordered
pairs of those eight filaments.

The default TD and layer settings are sufficient for normal preprocessing; you
only need to tune them when calibrating the hue guidance against test prints.

Only entries with `Owned: true` are candidates by default. Useful controls are:

| Option | Meaning |
| --- | --- |
| `--hueforge-library PATH`, `--filament-library PATH` | Load the exported library and enable guided preprocessing. |
| `--colors N` | Maximum number of owned physical filament anchors; default `4` in library mode. |
| `--hueforge-include-unowned` | Also consider entries whose `Owned` value is false. |
| `--hueforge-type TYPE` | Restrict candidates to a material type; repeat to allow several compatible types. |
| `--hueforge-allow-secondary` | Approximate dual/secondary-color entries using their primary `Color`. |
| `--hueforge-analysis-colors N` | Intermediate image-color budget per chromatic/achromatic population. |
| `--hueforge-max-perceived-colors N` | Maximum inventory-guided output colors; default `64`. |
| `--hueforge-guidance-strength F` | Pull toward owned-derived hues: `0` keeps analyzed colors, `1` fully snaps; default `0.8`. |
| `--palette-json PATH` | Record selected filaments, guided-color provenance, metrics, and the library hash. |

The pairwise hues are guidance, not promises that HueForge will reproduce each
RGB exactly. Stored HueForge TD is converted with an explicit approximate
front-lit scale. Advanced calibration controls are `--hueforge-layer-height`,
`--hueforge-base-depth`, `--hueforge-max-depth`,
`--hueforge-td-transmission`, and `--hueforge-td-scale`.

### Optional strict global-stack planning

Add `--hueforge-stack` only when you want ColorNinja to choose one explicit
bottom-to-top stack instead of merely preparing an image for HueForge:

```powershell
python .\perceptual_color_reduce.py .\input.png -o .\stack-preview.png `
  --hueforge-library $filamentLibrary `
  --hueforge-stack `
  --colors 8 `
  --hueforge-height-map .\stack-layers.png `
  --palette-json .\stack-plan.json
```

Strict mode evaluates stack sizes from one through the requested budget and
uses one contiguous run per selected filament. The JSON contains swap heights,
layer ranges, predicted colors, and the selected bottom-to-top order.
`--hueforge-beam-width` controls its deterministic heuristic search, while
`--hueforge-base-transmission-limit` prevents an overly translucent base.

The 16-bit height map stores literal 1-based stack layer indices; transparent
pixels are `0`. It is reconstruction data, not a directly sliceable HueForge
project or STL. Strict planning assumes every layer, including the first, uses
`--hueforge-layer-height`. `--hueforge-max-depth` is a total-height ceiling; if
HueForge labels its control **Blend Depth**, use the JSON value above the base.

Both modes use an independent optical approximation. Actual appearance varies
with filament batch, TD measurement, printer calibration, surface finish,
backing, and viewing light. HueForge's exact front-lit conversion is not public;
ColorNinja defaults to a `0.1` TD scale and 5% transmission at one scaled TD.
Tune those values from calibration prints when color accuracy matters.

Mixing fundamentally different materials such as PLA and PETG can cause adhesion
and temperature problems. Prefer compatible types and use repeatable
`--hueforge-type` filters deliberately.

## Output behavior

- Output dimensions always match the EXIF-oriented input dimensions.
- PNG output is lossless. Do not convert it to JPEG if exact color count matters;
  JPEG compression creates additional RGB values.
- The original alpha channel is preserved byte-for-byte. Fully transparent
  pixels use the first palette RGB so hidden colors do not inflate the color count.
- Animated GIF and WebP inputs use their first frame.
- Embedded ICC profiles are converted to sRGB when Pillow can read them.
- Existing output files are not replaced unless `--force` is supplied.

For very large files, the default analysis ceiling keeps palette discovery
practical while the final remapping still processes every original pixel.
`--full-analysis` can use substantially more memory and usually changes the
palette much more than it changes visible quality.

The decoded source and full-resolution output must still coexist in memory,
along with bounded mapping scratch space and an analysis buffer of up to 6 MP.
As a rough planning figure, very large sources can need more than eight bytes
per pixel plus analysis scratch; a 100 MP image may approach 1 GB of peak
memory even with the default analysis ceiling.
