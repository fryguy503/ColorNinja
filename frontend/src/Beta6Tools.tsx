import { useEffect, useState } from "react";
import type { Options, Library, Result, Comparison } from "./types";
import { StudioDialog } from "./StudioDialog";

const hex = (rgb: number[]) =>
  "#" +
  rgb
    .map((v) => v.toString(16).padStart(2, "0"))
    .join("")
    .toUpperCase();
export function ProtectedColors({
  options,
  onChange,
}: {
  options: Options;
  onChange: (o: Options) => void;
}) {
  const [value, setValue] = useState(options.protectedColors ?? "");
  useEffect(
    () => setValue(options.protectedColors ?? ""),
    [options.protectedColors],
  );
  const [error, setError] = useState("");
  const apply = () => {
    const entries = value
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (entries.some((s) => !/^#?[0-9a-f]{6}$/i.test(s))) {
      setError("Use six-digit RGB hex colors, separated by commas.");
      return;
    }
    setError("");
    onChange({
      ...options,
      protectedColors: [
        ...new Set(entries.map((s) => "#" + s.replace("#", "").toUpperCase())),
      ].join(","),
    });
  };
  return (
    <div className="beta6-controls">
      <label>
        Protected colors
        <input
          aria-label="Protected colors"
          value={value}
          placeholder="#000000, #FF8844"
          onChange={(e) => setValue(e.target.value)}
          onBlur={apply}
          onKeyDown={(e) => {
            if (e.key === "Enter") apply();
          }}
        />
      </label>
      {error && <p role="alert">{error}</p>}
      <p className="field-help">
        Reserve palette slots in Simple reducer. In filament modes, prioritize
        these source colors within the available blends. Shift-click a result
        swatch to add it here.
      </p>
    </div>
  );
}
export function FilamentConstraints({
  options,
  library,
  onChange,
}: {
  options: Options;
  library: Library;
  onChange: (o: Options) => void;
}) {
  const h = options.hueforge,
    required = (h.requiredFilaments ?? "").split(",").filter(Boolean);
  const change = (key: "baseFilament" | "highlightFilament", value: string) =>
    onChange({ ...options, hueforge: { ...h, [key]: value } });
  return (
    <details className="beta6-controls">
      <summary>Required spools &amp; stack ends</summary>
      <p className="field-help">
        Required spools count toward the filament budget. Stack ends constrain
        All filament stack workflows; Guided includes these spools as
        references.
      </p>
      {(["baseFilament", "highlightFilament"] as const).map((key, i) => (
        <label key={key}>
          {i === 0 ? "Base filament" : "Highlight / final filament"}
          <select
            aria-label={i === 0 ? "Base filament" : "Highlight filament"}
            value={h[key] ?? ""}
            onChange={(e) => change(key, e.target.value)}
          >
            <option value="">Choose automatically</option>
            {library.filaments.map((f) => (
              <option key={f.key ?? f.uuid} value={f.key ?? f.uuid}>
                {f.brand} · {f.name} · {f.material} · TD {f.td}
              </option>
            ))}
          </select>
        </label>
      ))}
      <div className="required-spools">
        {library.filaments.map((f) => (
          <label key={f.key ?? f.uuid}>
            <input
              type="checkbox"
              checked={required.includes(f.key ?? f.uuid)}
              onChange={(e) => {
                const key = f.key ?? f.uuid;
                onChange({
                  ...options,
                  hueforge: {
                    ...h,
                    requiredFilaments: (e.target.checked
                      ? [...required, key]
                      : required.filter((v) => v !== key)
                    ).join(","),
                  },
                });
              }}
            />
            {f.name} · {f.material} · TD {f.td}
          </label>
        ))}
      </div>
      <button
        className="text-button"
        onClick={() =>
          onChange({
            ...options,
            hueforge: {
              ...h,
              requiredFilaments: "",
              baseFilament: "",
              highlightFilament: "",
            },
          })
        }
      >
        Clear filament constraints
      </button>
    </details>
  );
}
export function GuidedExplanation({
  result,
  onStack,
}: {
  result: Result;
  onStack: () => void;
}) {
  const g = result.guidance;
  if (!g) return null;
  return (
    <details className="guided-explanation">
      <summary>Source colors and filament references</summary>
      <p className="field-help">
        Guided output interpolates toward a reference. Different pairs can
        require incompatible stacks. Try Color Match to test one common
        printable order.
      </p>
      <button className="button secondary" onClick={onStack}>
        Try Color Match
      </button>
      <table>
        <thead>
          <tr>
            <th>Source</th>
            <th>Guided</th>
            <th>Reference</th>
            <th>Spools</th>
          </tr>
        </thead>
        <tbody>
          {(g.colors ?? []).map((c, i) => (
            <tr key={i}>
              {[c.sourceRGB ?? c.rgb, c.rgb, c.referenceRGB].map((rgb, j) => (
                <td key={j}>
                  <i style={{ background: hex(rgb) }} />
                  {hex(rgb)}
                </td>
              ))}
              <td>
                {c.filamentPositions
                  .map((p) => g.selectedFilaments[p - 1]?.name ?? p)
                  .join(" + ")}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </details>
  );
}
export function ComparisonsDialog({
  open,
  onClose,
  items,
  saved,
  busy,
  canGenerate,
  canCapture,
  onGenerate,
  onCapture,
  onClear,
  onApply,
}: {
  open: boolean;
  onClose: () => void;
  items: Comparison[];
  saved: Comparison[];
  busy: boolean;
  canGenerate: boolean;
  canCapture: boolean;
  onGenerate: () => void;
  onCapture: () => void;
  onClear: () => void;
  onApply: (c: Comparison) => void;
}) {
  if (!open) return null;
  return (
    <StudioDialog
      title="Compare results"
      onClose={onClose}
      className="comparisons-dialog"
    >
      <p className="field-help">
        Compare plans found by the search and saved previews; these are not
        guaranteed optima. Applying settings uses the current image and library;
        inspect the refreshed result before exporting. Saved thumbnails persist
        across sessions. Alternatives that cannot satisfy the required spool
        budget are omitted.
      </p>
      <div className="comparison-actions">
        <button
          className="button secondary"
          disabled={busy || !canGenerate}
          onClick={onGenerate}
        >
          Find stack alternatives
        </button>
        <button
          className="button secondary"
          disabled={busy || !canCapture}
          onClick={onCapture}
        >
          Save current comparison
        </button>
        <button
          className="text-button"
          disabled={busy || !saved.length}
          onClick={onClear}
        >
          Clear saved
        </button>
      </div>
      {busy && <p role="status">Comparing plans…</p>}
      <div className="comparison-grid">
        {[...items, ...saved].map((c, i) => (
          <article key={i}>
            <strong>{c.name}</strong>
            <small>{c.source}</small>
            <img src={c.image} alt={`${c.name}: ${c.colors} colors`} />
            <dl>
              <dt>RMS color error</dt>
              <dd>{c.quality.rmsDeltaE76.toFixed(2)} ΔE76</dd>
              <dt>Filaments / swaps</dt>
              <dd>
                {c.options.mode === "standard"
                  ? "—"
                  : `${c.spools} / ${c.options.mode === "stack" ? c.swaps : "—"}`}
              </dd>
              <dt>Depth</dt>
              <dd>
                {c.options.mode === "stack" ? `${c.depth.toFixed(2)} mm` : "—"}
              </dd>
              <dt>Mean boundary step</dt>
              <dd>
                {c.options.mode === "stack"
                  ? `${c.boundaryStep.toFixed(3)} mm`
                  : "—"}
              </dd>
              <dt>Estimated solid volume</dt>
              <dd>
                {c.options.mode === "stack" && c.volumeMm3 != null
                  ? `${(c.volumeMm3 / 1000).toFixed(2)} cm³`
                  : "—"}
              </dd>
            </dl>
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => onApply(c)}
            >
              Use these settings
            </button>
          </article>
        ))}
      </div>
      {!items.length && !saved.length && !busy && (
        <p>
          No comparisons yet. Save the current result, or find alternatives in a
          filament stack workflow.
        </p>
      )}
    </StudioDialog>
  );
}
