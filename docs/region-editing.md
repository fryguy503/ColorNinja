# Region Edit

Generate a **Color Match** or filament-stack **Color Pop** preview, then click
**Region Edit** above the result. The editor temporarily uses the preview area
and its own tools panel. **Done** returns to the normal workflow with its view,
zoom and pan preserved. Simple reducer and Filament guided have no printable
layer map, so Region Edit is unavailable there.

## Select several regions at once

The default **Lasso** lets you draw around several areas in one gesture.
Choose how the drawn shape selects:

| Coverage | Result |
| --- | --- |
| Enclosed regions | Entire connected height regions only when all their pixels are enclosed. |
| Touching regions | Entire regions with at least one pixel inside the shape. Useful for small islands. |
| Pixels | Only covered pixels; can make a new boundary through an existing region. |

A region is a connected area at one layer height. Diagonal contact alone does
not join regions. Selection works at the image's original resolution, including
when the displayed image is scaled. Original transparent pixels are excluded;
pixels cut by this editor remain selectable for restoration.

- **Click** selects a connected region. Its above/below layer tolerances can
  include connected neighboring heights around the clicked layer.
- **Lasso** closes a freehand loop when released. Self-crossing loops use the
  even-odd fill rule.
- **Polygon** adds a corner per click. Use **Finish polygon** or Enter to close it.
- **Box** draws a rectangle; **Brush** paints a continuous stroke with a radius
  measured in source pixels.
- Choose Replace, Add, Subtract or Intersect. Shift adds and Alt subtracts while
  dragging or clicking. For Polygon, choose the operation before finishing it.
- Escape cancels an unfinished gesture. Space-drag or the middle mouse button
  pans; the wheel and zoom controls magnify; Fit and 1:1 restore known scales.

The yellow overlay and selected pixel/region counts show what will be affected.
When a whole-region lasso selects nothing, try Touching regions or Pixels.

## Assign or adjust height

**Assign layer** places every selected editable pixel at the chosen physical
layer. The selector shows height and predicted color, with the corresponding
filament's brand, material and TD beneath it. **Pick layer** samples a layer
from the result for assignment or same-layer/similar-color selection.

**Move layers** raises or lowers the selected pixels relative to their current
heights, retaining relief until the stack's printable limits are reached.
Ordinary edits clamp between the base and the existing stack's top layer.
They do not change filament order, add filament swaps or increase stack depth.

Every applied operation creates an edit group. Use **Before edits** to compare
with the generated result; it temporarily disables editing. Undo/Redo includes
selection changes and group operations, with Ctrl/Cmd+Z and Ctrl/Cmd+Shift+Z.

## Complementary tools

Open **Refine selection** or **Cleanup** as needed:

- Select all, clear, invert, grow, shrink, and fill enclosed selection holes.
  Growth/shrinkage uses four-neighbor pixel steps. Fill holes changes the
  selection, not original image transparency.
- Select the target layer, or similar predicted colors by Delta E76 tolerance;
  optionally limit the search to the current selection.
- Select small connected regions by pixel count, useful before assigning or
  matching isolated speckles. With a selection present, only small
  regions fully inside it are considered.
- Flatten to the most common, highest or lowest selected height. Ties for most
  common favor the lower layer.
- Match surrounding height uses the most common immediately adjacent printable
  layer around the selection. If there is no suitable surrounding area, choose
  a target layer directly.
- Smooth heights averages a small square neighborhood and rounds to printable
  layers; only selected editable pixels change. It can alter detail, so compare
  the result and use Undo when necessary.
- Restore generated heights returns selected pixels to their original plan.
  **Cut hole** explicitly confirms removing selected coverage through the base;
  ordinary lowering keeps the base. Restore also repairs editor-created cuts.

## Keep edits organized

Rename groups, select their footprints again, toggle their visibility, protect
them, delete them, or split a footprint into separate connected islands.
Later groups apply after earlier ones. An enabled protected group excludes its
pixels from later operations and cannot be changed until unlocked. Splitting
retains the operation and its covered pixels; smoothing groups cannot be split
because neighboring pixels affect the result.

Groups retain exact pixel footprints. A later edit that merges two height
regions does not expand an earlier group's footprint. Changing the generated
stack, source image, filaments, lighting or reduction settings requires an
explicit reset of existing edits before regeneration. Export dimensions and
other settings that do not change the generated plan can still be updated.

Save a **.colorninja** project to preserve the generated baseline and named
groups. These projects use schema 3 and require ColorNinja 1.1.0 or later.
Existing schema 2 projects still open; projects without region groups continue
to use schema 2. Export project companions also preserve edits. Reopening starts
a fresh undo history; the saved groups remain editable. Settings profiles store
processing settings, not image-specific region footprints.

## Export and limits

Edited heights feed the result PNG, layer-index PNG, palette/quality summary,
stack/surface views, project and HueForge Color Match export. Color Pop's
selection view retains the original hue classification; its edited result can
intentionally cross the original bands. Manual-edit quality is measured against
visible pixels of the original image. Optimizer diagnostics that no longer
describe the manually edited plan are cleared.

The HueForge project embeds separate height-transport color keys so identical
predicted RGB values at different heights remain distinguishable. The displayed
and exported result PNG retains the predicted physical colors. Region groups
are ColorNinja project data, not native HueForge Spot Fix groups. Check the
imported height plan in HueForge before printing.

The editor supports images up to 4096 pixels on either side and 16,777,216 pixels,
128 groups, up to one million region labels and one million compressed footprint
runs. History retains up to 40 actions within a compressed-mask memory budget;
complex documents may retain fewer. Extremely complex brush strokes are rejected
with an instruction to use shorter strokes. Large edits need time to rebuild
the physical preview and export views.

Unit and service tests cover topology, masks, clamping, protection, restoration,
history, stale requests, malformed projects, exact reopening and height export,
including Color Pop and Backlit. Browser acceptance uses the compiled Go service
for selection, edit/undo/redo, zoom/pan alignment, depth entry, resizing and
save/reopen/export. Ghidra analysis of HueForge 0.9.4.3 informed the region model;
these checks do not establish native Spot Fix group interchange, all native
mesh paths or physical-print accuracy.

## Printable depth entry

Base and manual maximum depth snap to the nearest value of
`first layer height + whole regular layers` when you leave the field or press
Enter. Exact midpoint ties round upward. With first layer 0.16 mm and regular
layers 0.08 mm, entering 0.86 gives 0.88 mm; 0.81 gives 0.80 mm. Changing either
layer height resnaps dependent depths and retains at least one base and one top
layer. When choosing depth automatically, the maximum remains a hard ceiling
and is not rounded upward. CLI/config validation remains strict.
