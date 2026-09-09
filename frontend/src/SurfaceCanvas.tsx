import { useEffect, useRef, useState } from "react";
import type { Result } from "./types";

export function SurfaceCanvas({
  result,
  selectedLayer,
  onSelect,
}: {
  result: Result;
  selectedLayer: number;
  onSelect: (layer: number) => void;
}) {
  const ref = useRef<HTMLCanvasElement>(null);
  const [mode, setMode] = useState<
    "surface" | "boundaries" | "detours" | "layer" | "coverage"
  >("surface");
  const v = result.surfaceView,
    plan = result.stack;
  const rows = result.stackView?.layers;
  useEffect(() => {
    const canvas = ref.current;
    if (!canvas || !v || !rows) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const scale = 3,
      w = (v.width - 1) * scale,
      h = (v.height - 1) * scale;
    canvas.width = w;
    canvas.height = h;
    const pixels = ctx.createImageData(w, h),
      colors = new Map(rows.map((r) => [r.layer, r.predictedRGB]));
    const height = (l: number) => rows[Math.max(0, l - 1)]?.height ?? 0;
    const first = rows[0]?.height ?? 0.16,
      regular = (rows[1]?.height ?? first + 0.08) - first;
    for (let y = 0; y < h; y++)
      for (let x = 0; x < w; x++) {
        const gx = Math.floor(x / scale),
          gy = Math.floor(y / scale),
          u = ((x % scale) + 0.5) / scale,
          t = ((y % scale) + 0.5) / scale;
        const a = v.layers[gy * v.width + gx],
          b = v.layers[gy * v.width + gx + 1],
          c = v.layers[(gy + 1) * v.width + gx],
          d = v.layers[(gy + 1) * v.width + gx + 1];
        const nearest = t < 0.5 ? (u < 0.5 ? a : b) : u < 0.5 ? c : d;
        const i = (y * w + x) * 4;
        if (!nearest) continue;
        let layer = nearest,
          shade = 1;
        const jump =
          Math.max(
            Math.abs(a - b),
            Math.abs(a - c),
            Math.abs(d - b),
            Math.abs(d - c),
          ) * regular;
        if ((mode === "surface" || mode === "detours") && a && b && c && d) {
          const z =
            u >= t
              ? height(a) * (1 - u) + height(b) * (u - t) + height(d) * t
              : height(a) * (1 - t) + height(c) * (t - u) + height(d) * u;
          layer = Math.max(
            1,
            Math.min(rows.length, Math.floor((z - first) / regular + 0.5) + 1),
          );
          shade =
            0.72 +
            0.28 / Math.sqrt(1 + (jump / Math.max(0.001, v.spacingMm)) ** 2);
        }
        const rgb = colors.get(layer) ?? [0, 0, 0];
        for (let k = 0; k < 3; k++) pixels.data[i + k] = rgb[k] * shade;
        if (mode === "detours") {
          const endpoints = [a, b, c, d]
            .filter(Boolean)
            .map((l) => colors.get(l) ?? [0, 0, 0]);
          const nearestDistance = Math.sqrt(
            Math.min(
              ...endpoints.map((c) =>
                c.reduce((sum, value, k) => sum + (value - rgb[k]) ** 2, 0),
              ),
            ),
          );
          const amount = Math.min(0.9, nearestDistance / 160);
          for (let k = 0; k < 3; k++)
            pixels.data[i + k] =
              rgb[k] * (1 - amount) + [255, 65, 190][k] * amount;
        }
        if (mode === "boundaries" && jump > 0) {
          const amount = Math.min(
            0.85,
            0.25 + (jump / Math.max(0.001, v.spacingMm)) * 0.25,
          );
          for (let k = 0; k < 3; k++)
            pixels.data[i + k] =
              rgb[k] * (1 - amount) + [255, 90, 40][k] * amount;
        }
        if (mode === "layer" && nearest !== selectedLayer) {
          for (let k = 0; k < 3; k++) pixels.data[i + k] = rgb[k] * 0.18;
        }
        if (mode === "coverage") {
          pixels.data[i] = 220;
          pixels.data[i + 1] = 235;
          pixels.data[i + 2] = 226;
        }
        pixels.data[i + 3] = 255;
      }
    ctx.putImageData(pixels, 0, 0);
  }, [v, rows, selectedLayer, mode]);
  if (!v || !plan)
    return (
      <p className="field-help">
        Refresh this project to build surface diagnostics.
      </p>
    );
  return (
    <section className="surface-inspector" aria-label="Surface diagnostics">
      <div className="surface-modes">
        {(
          ["surface", "boundaries", "detours", "layer", "coverage"] as const
        ).map((m) => (
          <button
            type="button"
            key={m}
            aria-pressed={mode === m}
            onClick={() => setMode(m)}
          >
            {
              {
                surface: "Approximate surface",
                boundaries: "Height boundaries",
                detours: "Intermediate colors",
                layer: "Selected layer",
                coverage: "HFP coverage",
              }[m]
            }
          </button>
        ))}
      </div>
      <canvas
        ref={ref}
        aria-label="Sampled stack surface; click a region to inspect its layer"
        role="img"
        onClick={(e) => {
          const box = e.currentTarget.getBoundingClientRect();
          const x = Math.max(
            0,
            Math.min(
              v.width - 1,
              Math.round(((e.clientX - box.left) / box.width) * (v.width - 1)),
            ),
          );
          const y = Math.max(
            0,
            Math.min(
              v.height - 1,
              Math.round(((e.clientY - box.top) / box.height) * (v.height - 1)),
            ),
          );
          const layer = v.layers[y * v.width + x];
          if (layer) onSelect(layer);
        }}
      />
      <div className="surface-metrics">
        <span>
          Mean step <b>{v.meanJumpMm.toFixed(3)} mm</b>
        </span>
        <span>
          95th percentile <b>{v.p95JumpMm.toFixed(3)} mm</b>
        </span>
        <span>
          Largest step <b>{v.maxJumpMm.toFixed(3)} mm</b>
        </span>
        <span>
          Source pixel <b>{v.pixelMm.toFixed(3)} mm</b>
        </span>
      </div>
      <p className="field-help">
        {v.widthMm.toFixed(1)} × {v.heightMm.toFixed(1)} mm. Click the sampled
        image to select a layer; use the layer slider for keyboard inspection.
        Orange marks height changes, not measured print defects. Pink marks
        interpolated surface colors that differ from the surrounding vertex
        colors.
      </p>
      <p className="field-help">
        Approximate triangulation at {v.spacingMm.toFixed(3)} mm spacing
        {v.sampled
          ? ` (requested ${v.requestedSpacingMm.toFixed(3)} mm; display resolution limited)`
          : ""}
        . Fine features can be missed by this display. Statistics use every
        image pixel. Compare the actual mesh in HueForge before printing.
        Estimated area near boundaries:{" "}
        {(v.boundaryAreaFraction * 100).toFixed(1)}% (one requested grid
        interval around source edges, capped at 100%).
      </p>
      {v.solidifiedFraction > 0 && (
        <p className="field-help">
          HFP makes {(v.solidifiedFraction * 100).toFixed(2)}% of covered pixels
          fully opaque. PNG retains their partial transparency.
        </p>
      )}
    </section>
  );
}
