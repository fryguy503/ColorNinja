# Architecture and migration

`internal/engine` implements decoding, metadata/color conversion, deterministic
Lab quantization, filament selection, guided colors, global stack search,
pixel mapping, metrics, PNG/layer exports, and reports. It has no Wails,
JavaScript, or Python dependency.

`internal/studio` owns document state, settings, presets, projects, preview jobs,
progress, and export. Both the desktop host and service tests use it. `main.go`
provides Wails hosting and native dialogs. The embedded React/TypeScript assets
handle layout and interaction. Processing always happens in Go; full-resolution
preview PNGs use a media handler instead of sending pixels through JSON bindings.
`cmd/colorninja-cli` calls the same engine directly.

## Image pipeline

Sources normalize to sRGB and 8-bit non-premultiplied RGBA with orientation
applied. With `preserveDetails` enabled (the default), an alpha-weighted separable
joint bilateral filter smooths the full-resolution working copy. Both passes use
unchanged source CIELAB values as the guide (range sigma 5 Delta E76, spatial
sigma from `preblurSigma`, truncated at two spatial sigmas). Original alpha is
preserved; transparent neighbors contribute no RGB. Eight workers at most use
8192-pixel tiles with halos, bounding float scratch space even for long, thin
images. Zero smoothing returns the original source directly.

Analysis then uses a configurable pixel ceiling and Lanczos3 resizing.
Float64 Oklab clustering separates chromatic and achromatic populations using
the existing CIELAB neutral threshold and repeatedly removes undersized clusters.
Histogram weights include alpha; hidden RGB cannot contaminate the palette.
With preservation disabled, the original CIELAB and analysis-only Gaussian
pipeline remains available, including the Python-compatible fixed-point resize.

Every original-resolution pixel is mapped without dithering. Up to eight row
workers use deterministic metric reduction. Preservation mode matches smoothed
pixels in Oklab; guided/stack modes map through analyzed source color groups so
an ill-fitting constrained palette cannot create new contours inside a group.
Alpha is preserved; RGB under zero alpha may change. Palette percentages and
Delta E76 metrics are alpha-weighted. Metrics always compare original pixels to
actual output in CIELAB, including when selection and mapping use Oklab.

Guidance selects filament anchors and pulls analyzed colors toward their nominal
and approximate TD-aware pairwise colors. It does not promise one global print
stack. Stack mode searches contiguous filament runs with a bounded beam,
selects a capped reachable palette, reranks completed candidates, and trims
unused top layers. With preservation on, up to 16 deterministic improvement
passes try complete-stack filament substitutions, order swaps, and one-layer
transfers. The fixed base depth, eligibility rules, unique-filament constraint,
total layer budget, output cap, and minimum fractions remain enforced. Only
improvements to the capped output objective are accepted. Each output color
corresponds to a layer in that stack; smoothing introduces no off-palette colors.
This is a bounded local search, not a guarantee of the globally optimal stack.

The default-on true-black option copies the working filament list and normalizes
black anchors before selection and optical calculations. It recognizes a “Black”
name token with Lab lightness at most 35 and chroma at most 12, avoiding named
grays and chromatic colors such as Black Cherry. The original RGB is retained
as `libraryRGB` in the selected-filament report; TD and source data do not change.
Guided targets matched directly to black stay exactly black at nonzero guidance.
Other blended hues remain modeled, and no arbitrary output-pixel clamp is used.
The option is ignored in perceptual mode. Old JSON options default it on while
explicit false survives saving and reloading.

Matrix/TRC RGB ICC profiles convert in Go through D50 PCS and adaptation to D65
sRGB. Other profiles, including CMYK, use Windows ICM. Invalid or unsupported
profiles produce an explicit warning. Lossy WebP's YUV planes require explicit
limited-range BT.601 conversion; JPEG YCbCr uses its normal full-range path.
GIF/WebP import the first frame on a full transparent canvas. WebP's optional
animation background hint is ignored.

## State and exports

Each document has a revision; preview requests have IDs and a service generation
with a cancellation context. Only the current worker can publish, and at most
one engine job holds its working buffers at a time. UI changes debounce processing,
cancel obsolete jobs, ignore stale responses, and disable export for unrendered
options. Changing modes and applying presets are independent state operations:
mode changes preserve tuning; built-in presets change only the budget; custom
presets restore tuning while preserving the active mode.

Preview and export share the immutable result image. Export checks the revision
and result ID instead of rerunning processing. Old media URLs cannot retrieve
a newer document. Output writes use temporary siblings; atomic no-clobber
hardlinks prevent racing writers from silently replacing existing files.

Projects and preferences have schema version 1. Projects reference image and
library files; relative paths are resolved against the project on opening.
Presets store processing options. Palette reports include settings, hashes,
metrics, and guidance/stack details; they are not project files. The original
filament library is never written.

The optional `--dev-server 127.0.0.1:PORT` exposes the same service for development
using loopback-only binding, exact-host, POST, origin, custom-header, and request
size checks. Ordinary desktop use opens no listening TCP port and requires no
network connection for processing.

## Compatibility boundaries

The Go CLI accepts legacy processing flags. `--chunk-pixels` is a compatibility
no-op because mapping uses bounded parallel rows. Reports use camelCase fields
and the Go schema rather than reproducing the Python report layout.

Five regression cases with true-black and detail preservation disabled match Python exactly.
This does not guarantee identical
results for every image decoder, ICC profile, or floating-point edge case.
Lossy codec upsampling and Windows/LittleCMS color transforms can differ.
ColorNinja is independent of ColorSmith and HueForge's proprietary engines.
Stack predictions require verification in the printing workflow.

Windows x64 is the release target. Shared engine code is portable, but macOS/Linux
desktop packaging, dialogs, and non-matrix ICC conversion have not been validated
as release targets. The generated icon and sample landscape are original code-drawn
artwork with no remote asset dependency.

## Technical references

- [Wails documentation](https://wails.io/docs/introduction/)
- [Oklab equations and public-domain sRGB matrices](https://bottosson.github.io/posts/oklab/)
- [Tomasi and Manduchi: Bilateral Filtering for Gray and Color Images](https://www.cs.jhu.edu/~misha/ReadingSeminar/Papers/Tomasi98.pdf)
- [Pillow resizing](https://github.com/python-pillow/Pillow/blob/main/src/libImaging/Resample.c)
- [WebP container and color conversion](https://developers.google.com/speed/webp/docs/riff_container)
- [Windows TranslateBitmapBits](https://learn.microsoft.com/en-us/windows/win32/api/icm/nf-icm-translatebitmapbits)
- [Prism metadata and ICC tools](https://github.com/mandykoh/prism)
