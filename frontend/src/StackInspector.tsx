import { useEffect, useRef, useState, type CSSProperties } from "react";
import { Layers, X } from "lucide-react";
import type { Result } from "./types";
import { SurfaceCanvas } from "./SurfaceCanvas";

const hex = (rgb: number[]) =>
  "#" +
  rgb
    .map((v) => v.toString(16).padStart(2, "0"))
    .join("")
    .toUpperCase();

export function StackInspector({
  result,
  stale,
}: {
  result: Result;
  stale: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState(1);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (open) dialog.current?.showModal();
    else dialog.current?.close();
  }, [open]);
  const plan = result.stack;
  const view = result.stackView;
  if (!plan || !view || !view.layers.length) return null;
  const row = view.layers[selected - 1] ?? view.layers[0];
  const run = plan.runs.find((r) => r.position === row.runPosition)!;
  const enabledCount = view.layers.filter((r) => r.meshEnabled).length;
  const grid = {
    gridTemplateColumns: `repeat(${view.layers.length}, minmax(18px, 1fr))`,
  } satisfies CSSProperties;
  const selectLayer = (layer: number) =>
    setSelected(Math.max(1, Math.min(view.layers.length, layer)));
  const navigate = (key: string) => {
    if (key === "ArrowLeft") selectLayer(selected - 1);
    if (key === "ArrowRight") selectLayer(selected + 1);
  };
  return (
    <>
      <button className="button secondary full" onClick={() => setOpen(true)}>
        <Layers size={15} /> Visualize stack
      </button>
      <dialog
        ref={dialog}
        className="stack-dialog"
        aria-labelledby="stack-map-title"
        onClose={() => setOpen(false)}
        onClick={(e) => {
          if (e.target === e.currentTarget) setOpen(false);
        }}
      >
        <div className="stack-map-heading">
          <div>
            <span className="eyebrow">FRONT LIT · STACK MAP</span>
            <h2 id="stack-map-title">From filament to image</h2>
            <p>
              {plan.plannedDepth.toFixed(2)} mm · {view.layers.length} layers ·{" "}
              {plan.uniqueFilaments} spools · {plan.runs.length} runs
            </p>
          </div>
          <button
            className="icon-button"
            aria-label="Close stack map"
            onClick={() => setOpen(false)}
          >
            <X size={20} />
          </button>
        </div>
        {stale && (
          <p className="stack-map-notice" role="status">
            Previous preview. Refresh the image to see your latest settings.
          </p>
        )}
        {plan.depthSelection && (
          <p className="stack-map-notice">
            Automatic depth selected {plan.plannedDepth.toFixed(2)} mm under
            your {plan.depthSelection.hardMaximum.toFixed(2)} mm ceiling.{" "}
            {plan.depthSelection.comparedDepths} printable depths compared;
            within {plan.depthSelection.tolerancePercent}% of the best color
            score found.
          </p>
        )}
        <p className="stack-map-hint">
          Build direction: base → top. Select a color cell or use the layer
          control to inspect a height.
        </p>
        <div
          className="stack-map-scroll"
          role="region"
          aria-label="Stack color and mesh tracks"
          tabIndex={0}
        >
          <div
            className="stack-map-tracks"
            style={{ minWidth: Math.max(640, view.layers.length * 18) }}
          >
            <div className="stack-map-axis" style={grid} aria-hidden="true">
              {view.layers.map((r) => (
                <span key={r.layer}>
                  {r.layer === 1 ||
                  r.layer % 5 === 0 ||
                  r.layer === view.layers.length
                    ? r.layer
                    : ""}
                </span>
              ))}
            </div>
            <div className="stack-track-label">
              <strong>Filament runs</strong>
              <span>Physical spools · color core</span>
            </div>
            <div className="stack-run-track" style={grid}>
              {plan.runs.map((r) => (
                <button
                  key={r.position}
                  className={r.position === row.runPosition ? "selected" : ""}
                  style={{
                    gridColumn: `${r.startLayer} / ${r.endLayer + 1}`,
                    background: r.filament.hex,
                  }}
                  onClick={() => selectLayer(r.startLayer)}
                  title={`${r.filament.brand} ${r.filament.name} · ${r.filament.material} · layers ${r.startLayer}–${r.endLayer}`}
                  aria-label={`Run ${r.position}: ${r.filament.name}, layers ${r.startLayer} through ${r.endLayer}`}
                >
                  <span>{r.position}</span>
                </button>
              ))}
            </div>
            <div className="stack-track-label">
              <strong>Layer colors</strong>
              <span>Predicted blends through the stack</span>
            </div>
            <div
              className="stack-color-track"
              style={grid}
              onKeyDown={(e) => {
                if (e.key.startsWith("Arrow")) {
                  e.preventDefault();
                  navigate(e.key);
                }
              }}
            >
              {view.layers.map((r) => (
                <button
                  key={r.layer}
                  tabIndex={r.layer === selected ? 0 : -1}
                  className={r.layer === selected ? "selected" : ""}
                  style={{ background: hex(r.predictedRGB) }}
                  onClick={() => selectLayer(r.layer)}
                  aria-label={`Layer ${r.layer}: ${r.height.toFixed(2)} mm, predicted ${hex(r.predictedRGB)}`}
                  title={`Layer ${r.layer} · ${r.height.toFixed(2)} mm · ${hex(r.predictedRGB)}`}
                />
              ))}
            </div>
            <div className="stack-track-label">
              <strong>Mesh targets</strong>
              <span>
                {view.hasMeshCore
                  ? `${enabledCount} active heights · ${view.meshCore === "legacy-flat" ? "legacy flat image colors" : view.meshCore === "compact-blends" ? "tuned image colors" : "filament blends"}`
                  : "No separate mesh core"}
              </span>
            </div>
            {view.optimization && (
              <p className="field-help">
                {view.optimization.strategy}:{" "}
                {view.optimization.originalEntries} →{" "}
                {view.optimization.entries} mesh entries;{" "}
                {view.optimization.originalDisabledLayers} →{" "}
                {view.optimization.disabledLayers} disabled layers. Virtual TD{" "}
                {view.optimization.minTD.toFixed(2)}–
                {view.optimization.maxTD.toFixed(2)}. Physical filament TDs are
                unchanged.
              </p>
            )}
            {view.hasMeshCore ? (
              <div className="stack-color-track" style={grid}>
                {view.layers.map((r) => (
                  <button
                    key={r.layer}
                    tabIndex={-1}
                    className={`${r.meshEnabled ? "" : "inactive"} ${r.layer === selected ? "selected" : ""}`}
                    style={{
                      backgroundColor: r.meshRGB ? hex(r.meshRGB) : "#26322c",
                    }}
                    onClick={() => selectLayer(r.layer)}
                    aria-label={`Mesh layer ${r.layer}: ${r.meshEnabled ? "active" : "excluded from matching"}`}
                    title={`Layer ${r.layer} · ${r.meshEnabled ? `${(r.pixelFraction * 100).toFixed(2)}% of image` : "Not used as an image surface"}`}
                  />
                ))}
              </div>
            ) : (
              <p className="stack-map-notice">
                HueForge recalculates heights in{" "}
                {view.meshMode.replaceAll("-", " ")} mode. Choose Color Match to
                export a separate mesh core.
              </p>
            )}
          </div>
        </div>
        <div className="stack-layer-control">
          <label htmlFor="stack-layer">
            Inspect layer <strong>{row.layer}</strong>
          </label>
          <input
            id="stack-layer"
            type="range"
            min={1}
            max={view.layers.length}
            value={selected}
            onChange={(e) => selectLayer(Number(e.target.value))}
          />
          <span>{row.height.toFixed(2)} mm</span>
        </div>
        <div className="stack-layer-details">
          <div>
            <small>PRINTING WITH</small>
            <strong>
              {run.filament.brand} · {run.filament.name}
            </strong>
            <span>
              {run.filament.material || "Unspecified material"} · TD{" "}
              {run.filament.td} mm
            </span>
            <span>
              Run {run.position} · layers {run.startLayer}–{run.endLayer}
            </span>
          </div>
          <div>
            <small>PREDICTED COLOR</small>
            <strong>
              <i style={{ background: hex(row.predictedRGB) }} />
              {hex(row.predictedRGB)}
            </strong>
            <span>At {row.height.toFixed(2)} mm total thickness</span>
          </div>
          <div>
            <small>IMAGE SURFACE</small>
            <strong>
              {row.pixelFraction > 0
                ? `${(row.pixelFraction * 100).toFixed(2)}% of visible pixels`
                : "No pixels end here"}
            </strong>
            <span>
              {view.hasMeshCore
                ? row.meshEnabled
                  ? `Mesh target ${row.meshRGB ? hex(row.meshRGB) : ""}`
                  : "Excluded from mesh matching"
                : "Heights will be rebuilt in HueForge"}
            </span>
          </div>
        </div>
        <p className="stack-map-hint">
          Striped heights still print beneath higher surfaces; they are excluded
          from Color Match targets. Each numbered run is one continuous use of a
          spool.
        </p>
        <SurfaceCanvas
          result={result}
          selectedLayer={row.layer}
          onSelect={selectLayer}
        />
      </dialog>
    </>
  );
}
