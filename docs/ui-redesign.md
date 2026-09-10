# Studio redesign: feature map

The image workspace occupies the left side; a single inspector on the right
contains workflow-specific tuning. Existing engine options, saved formats,
processing requests, and export handlers remain the source of truth.

| Existing capability | New location |
| --- | --- |
| Open image, open/save portable project, recent images, sample | File dialog; Save shortcut in header; existing keyboard shortcuts and file drop |
| Workflow and mode explanations | Inspector heading and information button |
| Built-in budgets; create/apply/rename/delete presets; keep workflow | Presets & profiles dialog |
| Load/save settings profiles | Presets & profiles dialog |
| Maximum colors / filaments | Tune; defaults to 8, slider covers 1–32 and expands for larger saved values |
| Exact budget, total vs separate color/neutral budgets | Tune → Advanced → Color & detail controls |
| Color priority; smoothing presets; preserve details | Tune |
| Filament guidance strength, true black, perceived-color limit | Tune → Advanced |
| Smoothing radius and tolerance, neutral chroma, minimum cluster | Tune → Advanced → Color & detail controls |
| Histogram precision, iterations, analysis resolution and custom pixel limit | Tune → Advanced → Color & detail controls |
| Choose/reload library, library path, embedded-project identity | Filaments; also accessible through File in Simple Reducer |
| Material filter, avoid silk/metallic, unowned, dual-color primary | Filaments → Library filters |
| Filament search; brand, material and TD; eligible/skipped counts | Filaments |
| Front Lit/legacy model; lighting; first/regular layer heights | Layers (Color Match) or Optics (Filament Guide) |
| Base depth, manual/automatic maximum depth, filament returns/run limit | Layers / Optics, as applicable to the workflow |
| Beam width; analysis colors; legacy TD scale/transmission/base limit | Layers / Optics → Advanced |
| Auto preview, refresh, cancel, progress, stale results | Inspector footer and status bar |
| Undo/redo and shortcuts | Status bar; Ctrl+Z / Ctrl+Shift+Z |
| Original/result/compare/side-by-side; full RGB counts | Image toolbar and comparison panels |
| Wheel zoom, linked pan, fit, 1:1, transparency and source dimensions | Image workspace |
| Palette colors, hex copy, image coverage and proportional strip | Output dock → Colors; dock can collapse |
| Selected filaments, repeated runs, brand/material/TD/layer counts | Output dock → Filaments |
| Analysis size, dimensions, quality distance, time, output format | Output dock → Image info |
| Stack depth, runs/layers/spools and auto-depth search summary | Output dock → Filaments |
| Stack map: runs, predictions, mesh targets, layer inspection, coverage, keyboard navigation | Stack map button in output dock |
| PNG, palette JSON, layer map, HFP, exact portable project export | Export dialog |
| HFP mesh mode/core, physical width and mesh detail | Export → HueForge project settings |
| Project/profile companion preference; stale/busy export guards | Export dialog |
| Version, startup checks, stable/preview channel, release page, update errors | Preferences & updates |

Saved processing settings are not reset by changing tabs or hiding Advanced.
Mode changes continue to preserve the current budget. Explicit Reset uses 8 in
every mode; pre-existing projects and presets retain their saved values, including
budgets above 32. HFP setting changes still invalidate the rendered result and
must be refreshed before export. Layer maps and stack inspection continue to use
the engine's real results; the illustrative mockup data is not included.

## Validation

Verified on 2026-09-09:

- All 48 original change-handler/value bindings remain present; the export
  format selector adds one new binding. No processing option schema changed.
- 107 Go tests, eight frontend regression tests, formatting, TypeScript/Vite,
  Go vet, and the Windows production build passed.
- The real application service passed browser checks for comparison layouts,
  zoom/fit, undo/redo, manual/stale previews, hidden advanced values, budgets
  above 32, presets, profiles, filament filters, legacy optics, automatic depth,
  returns, and the full stack map.
- All five export formats and their companions were written through the new
  dialog. Reopening the exported portable project and exporting HFP again
  produced byte-identical HFP output. Profile loading restored the saved tuning.
- Layout checks passed at 1440×980, 1080×720, and 960×720 with no horizontal
  overflow. Browser checks used isolated settings and generated test files.

The browser checks exercise the real Go service and frontend. Native Windows
file-picker interaction was not part of this run; the existing native dialog
and overwrite handlers were retained.
