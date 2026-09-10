# HueForge Spot Fix: executable-backed research

This records the static Ghidra investigation used to inform Region Edit in 1.1.0.
For shipping behavior and limits, see [Region Edit](region-editing.md).
The raw decompiler excerpts remain local investigation evidence and are not bundled.

## Reference and method

- Installed reference: `C:\Program Files\HueForge\HueForge.exe`, file version **0.9.4.3**, 10,401,320 bytes.
- SHA-256: `CAFF612E57571238EB23909B21C92BE7DA74EAD54348BAFBE22B38352EA0EB69`.
- Ghidra **12.1.2**, existing analyzed `HueForge0943` project, headless `-process HueForge.exe -noanalysis -readOnly` sessions with explicit JDK and workspace-local settings/cache.
- Followed Qt method metadata, UI strings/hotkeys, callers, virtual tables, decompiled branches, saved-project fields, region workers, masks, and the final luminance application. Addresses below are virtual addresses in this exact executable, whose image base is `0x140000000`.
- The accompanying `spot-*.c`, per-function excerpts, logs, `qt-methods.txt`, `hotkeys.txt`, and extracted overlay shader preserve evidence. Function names below are descriptive identifications unless a recovered C++/Qt name is stated. Decompiled argument types are not reliable ABI declarations; one apply routine incorrectly types its object pointer as a double.

The vendor's [0.9.4.2 release notes](https://devlog.thehueforge.com/p/hueforge-0942-release-notes-and-whats) explain the move to persistent footprints. The [0.9.4.3 release notes](https://devlog.thehueforge.com/p/hueforge-v0943-bugfix-release) describe the revised clamping, capture fixes, and intentional below-floor cutout behavior. The analysis here is specific to the installed version, not a claim about every release or tiled-export path.

## What Spot Fix actually does

### Regions are connected height areas

Spot Fix is an editor for the height representation used by the mesh. Its worker copies the **pre-fix 16-bit luminance/height data**, quantizes it into layer indices, and labels equal-layer components using **four-neighbor connectivity**. Equal heights separated by other pixels remain separate regions. Diagonally touching pixels do not automatically merge.

The region map also supports adjacency and capture operations. A seed starts at a normalized image coordinate. Above/below tolerances broaden the reachable region around the anchor height; this is height tolerance, not RGB similarity or geometric brush width. Existing owned regions are excluded during resolution. Group members may be spatially disconnected.

The audited entry guard permits width/height up to 4096 and up to 16,777,216 pixels. A background worker rebuilds region data. Readiness and dimension checks prevent use of incompatible masks.

### Interaction and editing

The embedded control table and Qt handlers expose the following:

| Input/action | Observed meaning |
|---|---|
| Click | Create from hovered region, or select an existing group covering the clicked pixel |
| Ctrl+click | Add hovered region to the selected group |
| `+` / `-` | Adjust the group's layer offset |
| `F` | Cycle flatten: off, most common, highest, lowest |
| `w` / `s` | Increase/decrease above-layer tolerance |
| `W` / `S` | Increase/decrease below-layer tolerance |
| `0` | Reset both tolerances |
| `I` / Shift+`I` | Capture enclosed valleys / exposed peaks with the group |
| Ctrl+`I` / Ctrl+Shift+`I` | Capture those regions into a new group |
| Middle button held | Temporarily hide overlays |
| Delete / Backspace | Delete selected group |
| Escape | Deselect |
| Pick Layer | Derive a group offset from sampled height and its seed/anchor |

Tolerance controls span 0–20 layers in the embedded help. No lasso action appears in the audited Spot Fix guide, hotkeys, or Qt method surface.

**Flatten and move are distinct operations.** Flatten, when enabled, is applied first; delta is then applied. A resolved target luminance can be stored. If a target is unavailable, the apply path derives it from a histogram of masked heights: most frequent, highest, or lowest. The histogram mode uses ascending traversal and strictly greater counts, making the lower layer win a count tie.

Consequently, the sampled-layer command is not equivalent to an unconditional absolute assignment of every selected pixel. ColorNinja should offer a plainly named absolute **Assign layer** command as well as **Move ± layers**, which preserves relief until printable bounds intervene.

### Boundaries, capture, and cuts

The group resolver unions seed results, adds requested enclosed/exposed components, and subtracts already owned areas. An edge flood fill over the complement identifies enclosed areas; separate connected-component work handles captured subregions. Seeds are stabilized inside their components, including checking a small interior neighborhood rather than relying on a boundary pixel.

Clamping is recalculated against the actual usable range. The installed version also allows a downward step beyond the floor. The final pixel routine checks whether **all masked samples** would fall below zero; that path marks a companion coverage buffer for the group while clamping luminance. This is consistent with the vendor-described deliberate cutout behavior. It must not be confused with ordinary lowering of a region while keeping the base intact.

Split Clamped Regions partitions group state. The code has an explicit refusal for groups with captured regions, so capture/split combinations are a real edge case rather than universally interchangeable commands.

### Persistent footprints and history

Project serialization writes `spotfix_version: 2` and a `spot_fixes` array. A group includes:

| Group fields | Purpose |
|---|---|
| `id`, `enabled` | Identity and bypass state |
| `delta`, `flattenMode`, `targetLum` | Height operation and resolved flatten anchor |
| `footprint: {w, h, rle}` | Dimensions and base64-encoded resolved mask |
| `regions` | Seed and capture metadata |

Each region includes `x`, `y`, `above`, `below`, `insideLoop`, `captureAbove`, and `anchorLum`.

The footprint decoder reads alternating absent/present run lengths beginning with absent, using a 7-bit continuation encoding. It rejects overflow, overrun, incomplete encoding, and a decoded size different from the expected pixel count. This is binary mask data, not a list of lasso vertices. Reconciliation first attempts the footprint path, including resolution transfer; unavailable/invalid footprints fall back to seed resolution. The system retains both a settled selection and information for rebuilding it.

Recalculate deliberately clears footprints and regenerates region membership. It records undo state and is exposed through confirmation UI. Undo/redo snapshot and restore group metadata and masks. Image replacement has a keep-fixes path; full reset clears fixes and selection.

This is the most consequential part of the design to retain: **an edit must remain attached to its selected pixels rather than accidentally spreading when two regenerated regions merge.**

### Actual application and presentation

Changing a fix emits the SpotFixesChanged signal. The connected callback marks the project and Spot Fix data dirty and starts a 50 ms timer when updates are not suppressed. The OpenGL render path checks those flags and invokes the virtual **MeshModel::CalculateLuminance** function.

That function saves pre-fix luminance, iterates enabled groups with a flatten/offset operation, validates mask size, and calls the masked pixel apply routine. The routine flattens first, shifts second, clamps 16-bit output, and handles the all-below-floor coverage case. CalculateLuminance then rebuilds the height image and clears dirty flags. This establishes that fixes affect height data, not only a screen overlay.

The overlay itself is separate: a categorical R8 texture distinguishes unselected space, committed groups, selected groups, hovered regions, and subregions. The shader adds category tint and boundaries. A stale-size helper logs and skips mismatched masks.

The analysis reaches the mesh height-data application and project persistence. It is **not** an exhaustive proof of every downstream triangulation/export backend or native GPU path. Those need controlled native fixtures before a compatibility claim.

## Ghidra evidence map

| Address | Identified responsibility |
|---|---|
| `14004fc80` | Size/availability guard |
| `1400e96a0` → `1400ed6b0` → `1400dcdc0` | Rebuild dispatch, worker, layer quantization and four-connected labeling |
| `1400ebe90` | Seed/anchor/tolerance region resolution |
| `1400ea340`, `1400e3c70`, `1400ed730` | Group masks, enclosed-area flood fill, capture components |
| `1400e7450` | Seed stabilization |
| `1400e7b40`, `1400e7890` | Click/freeze and add-to-group |
| `1400e0dc0`, `1400dff60`, `1400e80d0` | Create/adjust delta and sampled layer offset |
| `1400f2150`, `1400e1ed0` | Flatten cycle and target calculation |
| `1400efba0`, `1400f1b70`, `1400f1760`, `1400ee900` | Delta bounds, clamp state, split clamped regions |
| `1400e9060`, `1400f1120`, `1400f0060`, `1400ed140` | History snapshot, undo, redo, restore |
| `1400ec630`, `1400f06a0`, `1400ef640` | Reconcile footprints, transfer/resolve, decode runs |
| `1400e9bf0`, `140173240`, `1400e2280` | Recalculate, UI confirmation, legacy migration |
| `140214fa0`, `140215450`, `140212690`, `14020cf40` | Group/seed writer and project save/load |
| `140215a40` → `140274780` → `1400a4f90` | Edit notification, signal, dirty/debounce callback |
| `1401b4970` → vtable `1402d5248 + 168h` → `1401a8b40` | Render invalidation to CalculateLuminance |
| `1401b2f60` | Flatten/shift/clamp/cut application to masked pixels |
| `1401736a0`, `140171ae0`, `1401b1ad0` | Overlay shader setup/draw and stale-mask handling |

The local investigation retains per-function outputs, callers and constants.
