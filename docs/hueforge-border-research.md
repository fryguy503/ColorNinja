# HueForge border investigation

## Reference

- HueForge.exe **0.9.4.3**, SHA-256 `caff612e57571238eb23909b21c92be7da74ead54348bafbe22b38352ea0eb69`.
- libMesh.dll, SHA-256 `9709f4cf84e98f1eb3ada8994c8a4549e50e14c9f9a0e2ba9ac1e7d6d24d72ad`.
- Ghidra **12.1.2**; existing analyzed executable plus a fresh import of the mesh
  library. Follow-up decompilation used headless `-process`, `-noanalysis`, and
  `-readOnly`. The installed application and its settings were not modified.
- Addresses below are virtual addresses in those exact binaries. Descriptive
  function names are identifications unless an exported symbol is named.
- Local evidence is retained under `artifacts/border-20260910`. Decompiled
  implementation excerpts are investigation material and are not distributed.

## Controls, state, and persistence

The physical frame has four project fields: `borderless`, `border_width`,
`border_height`, and `external_border`. It is active only when `borderless` is
false and width is nonzero. The `Border` button is therefore the inverse of the
stored `borderless` flag. Width and depth are millimeters, and depth is absolute
from the build plate, not a height added to the image.

The project writer at `140212690` reads width/depth from state offsets `150h` and
`154h`, and booleans from `158h`/`159h`. The loader at `14020cf40` defaults missing
width to 4, depth to 3, borderless to false, and external to true. UI restoration
at `140087760` restores the controls. ColorNinja deliberately defaults to an
inactive frame, 4 mm width, and a depth that follows the image top.

The Qt dispatcher `140264a50` handles enable state (`1eh`), placement (`1fh`),
width (`27h`), and depth (`28h`). These mark geometry or rendering dirty; depth
updates use a 50 ms timer. Enabling also refreshes the layer range. The
`14019c400` layer-height builder uses the larger of border height and image maximum
when a border is active. Its heights accumulate in float32 and stop strictly below
the maximum, explaining the need to retain export headroom.

`DisplayBorderOnCore` is a **global presentation preference**, stored through
QSettings at `1400a4880`. Its embedded tooltip distinguishes showing every tall
border layer from showing a single top layer on the core. It does not change
frame geometry and is not one of the four HFP fields. ColorNinja retains its
image-stack inspector and reports the frame and extra layers separately.

The installed UI explicitly disables borders in **ColorDrop** mode
(`14008ddd0`). ColorDrop is distinct from Color Pop. ColorNinja supports this
frame on its HFP stack workflows, including Color Pop and Backlit; it does not
implement HueForge's ColorDrop or tiling workspace.

## Geometry

`1401ac490` (CreateTriangles) rounds the image dimensions to its effective mesh
spacing. External borders retain the image grid. Internal borders subtract twice
the rounded border width in grid intervals from the shorter grid dimension and
scale the longer dimension using the original grid aspect ratio. This preserves
the whole image rather than simply cutting an equal-width strip from all four
edges. On a nonsquare image, the longer overall dimension also decreases.

The same routine guards a grid smaller than 2 × 2, logging a warning and clamping
it. ColorNinja rejects a nominal internal frame with insufficient remaining image
area instead. Its displayed dimensions and material volumes are nominal estimates;
actual HueForge grid rounding remains the authority.

`1401ab120` builds the viewer's rectangular frame vertices at the image bounds,
their width-expanded outer bounds, Z=0, and Z=border height. The standalone
mesh-export worker `140184f00` also appends a native border mesh after generating
the image mesh, establishing that this is exported geometry rather than a visual
overlay. The frame does not follow alpha contours.

The library exports:

| libMesh address | Export / purpose |
| --- | --- |
| `180013030` | `BorderMeshUtil::generateRect`: complete rectangular frame |
| `180013080` | `generateRectPartial`: selected edges |
| `1800130e0` | `generateRectSlice`: height interval |
| `180013160` | `generateRectSlicePartial`: height interval plus selected edges |
| `180012700` | Full-frame vertex/index construction |
| `180012d50` | Partial-frame construction |

The full-frame builder emits 16 vertices and 32 triangles for the two rings,
top/bottom, and inner/outer walls. Slice exports reject nonpositive height
intervals. The mesh-export worker uses a per-job `border_edges` mask for tiling,
with an absent value selecting all four edges. These are tiling/export-job
controls, not an additional global per-side HFP border setting. Native release
notes describe outer-only tile edges and per-tile checkerboard borders.
[HueForge 0.9.3 release notes](https://devlog.thehueforge.com/p/hueforge-093-release-notes).

## Color and taller borders

The mesh shaders select physical color from height using
`floor((z - first_layer_height) / layer_height + 0.5) + 1`, with float32
arithmetic and bounds checks. Border depth can be below or above the image top.
The ordinary filament expansion continues the final filament after its endpoint;
additional consecutive thickness participates in blending. There is no separate
arbitrary border RGB in the audited project contract.

ColorNinja writes the native four border fields and extends the final physical
slider when needed. It reserves one import layer above the taller print,
preserves virtual Mesh Core entries/endpoints, and disables the added height
range for image matching. This prevents a tall frame from exposing new image
match candidates. Image pixels and layer assignments remain unchanged in
ColorNinja; native spatial resampling can still alter fine boundaries.

## Executed checks

The installed `generateRect` export was called in an isolated process on four
fixtures. Measured vertex/index counts, axis-aligned bounds, and signed-tetrahedron
volume matched the rectangular-ring calculation. The measurements and binary
hash are stored in `internal/engine/testdata/hueforge-border-reference.json` and
checked by Go tests. This directly tests the full-frame library export; it is
not a claim that all partial/tiled/mesh backends were exercised.

A 4 mm border above a 0.88 mm image was exported, then checked using the installed
filament parser, run expansion, blending code, layer builder, and original Color
Match shader on the local GPU. All 6,080 visible fixture pixels matched the
planned layer and the tested physical colors had zero channel disagreement.
The predicted frame-top color also matched the native expanded physical stack.

The [0.9.4.2 release notes](https://devlog.thehueforge.com/p/hueforge-0942-release-notes-and-whats)
corroborate the external-border default for older projects. Behavior here is
specific to the installed binaries. Native GUI interaction, physical prints,
and exhaustive tiled/backend parity remain outside these checks.
