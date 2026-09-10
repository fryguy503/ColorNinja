import type { HueForgeOptions } from "./types.ts";

export function borderLayerDepth(layer: number, h: HueForgeOptions): number {
  return Number(
    (
      (h.firstLayerHeight || h.layerHeight) +
      (layer - 1) * h.layerHeight
    ).toFixed(8),
  );
}

export function snapBorderDepth(depth: number, h: HueForgeOptions): number {
  const first = h.firstLayerHeight || h.layerHeight;
  const last = Math.min(
    998,
    Math.floor((40 - first) / h.layerHeight + 1e-9) + 1,
  );
  const layer = Math.max(
    1,
    Math.min(last, Math.round((depth - first) / h.layerHeight + 1e-9) + 1),
  );
  return borderLayerDepth(layer, h);
}

export function borderColorChoices(
  colors: { rgb: number[]; layer: number }[],
  depth: number,
  h: HueForgeOptions,
) {
  const byColor = new Map<
    string,
    { hex: string; layer: number; depth: number }
  >();
  for (const color of colors) {
    const height = borderLayerDepth(color.layer, h);
    if (color.layer < 1 || color.layer > 998 || height > 40) continue;
    const hex =
      "#" +
      color.rgb
        .map((v) => v.toString(16).padStart(2, "0"))
        .join("")
        .toUpperCase();
    const prior = byColor.get(hex);
    if (!prior || Math.abs(height - depth) < Math.abs(prior.depth - depth)) {
      byColor.set(hex, { hex, layer: color.layer, depth: height });
    }
  }
  return [...byColor.values()].sort((a, b) => a.layer - b.layer);
}
