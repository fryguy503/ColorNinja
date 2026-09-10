# Color Match color order

1. Choose **Color Match**, select your filament library, and generate a preview.
2. In **Tune → Color order**, drag the suggested source color groups into your
   preferred order. The first row is the bottom; the last row is the top.
   The arrow buttons work with keyboard and touch as well.
3. Adjust **Order strength**. The default is 50%; 0% turns off this preference,
   and 100% gives it its strongest influence. Every drag, arrow, or strength edit
   requests a fresh preview, including when Auto preview is off.
4. Inspect the rendered image, layer map, and stack. **Reset order** returns to
   automatic color matching. Undo/redo and saved projects/profiles retain edits.

Groups summarize source hues, with dark, middle, and light neutrals separated.
Their swatches and percentages summarize the analysis palette. The initial
suggestion follows their mean assigned heights in the current Color Match plan.
These groups are not individual spools: several filaments can contribute to a
color, and the same filament can return at different heights. Groups newly
appearing after a settings change are appended; groups absent from the image
are ignored. Reordering uses the currently displayed groups.

The preference adds a weighted cost when source colors appear below colors
that were requested beneath them. It evaluates actual matched image heights,
including repeated RGB colors, while preserving the ordinary color-score
allowance and per-family color safeguards. One layer of separation satisfies a
pair; taller relief earns no extra reward. Filament requirements, base/highlight
pins, available blends, color count, swap budget, and depth limits still apply.
The result reports when colors overlap. This is a bounded search and a preference,
so it cannot guarantee arbitrary color orders or physical-print quality.

The separate **Layers → Optimize layer order** option chooses a compact accent
automatically. Enabling it clears a manually requested order. Dragging an order
selects manual preference instead. **Advanced → Surface color allowance** controls
the permitted change in color score; the order strength does not relax that cap.
Color Pop uses its own band controls and ignores Color Match ordering.

CLI example (source checkout; use `colorninja-cli.exe` in a portable package):

```powershell
./build/bin/colorninja-cli.exe image.png -o ordered.png --hueforge-library personal_library.json --hueforge-stack --colors 4 --color-order black,green,red,white --color-order-weight 75 --palette-json ordered.json --hueforge-project ordered.hfp
```

Valid keys are `black`, `gray`, `white`, `red`, `yellow`, `green`, `cyan`, `blue`,
and `purple`. Supply at least two unique keys, without spaces. The report stores
the requested order, strength, source groups, mean heights, and the fraction of
requested source-color pairs separated in the intended direction. Projects and
profiles store the same options; HFP and layer exports use the resulting heights.

Standard, Combo, Max Channel, Scaled Max Channel, and Color Aware are temporarily
disabled. Older settings and profiles normalize to Color Match. Opening an older
channel project retains its source/library and clears the cached channel preview;
generate a new preview before exporting. Opening does not modify the saved file.
