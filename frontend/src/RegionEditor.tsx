import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  ArrowLeft,
  Brush,
  Lasso,
  MousePointer2,
  Pentagon,
  RectangleHorizontal,
  Pipette,
  Undo2,
  Redo2,
  Plus,
  Minus,
  Maximize,
  Eye,
  EyeOff,
  Lock,
  Unlock,
  Trash2,
} from "lucide-react";
import { invoke } from "./bridge";
import type { Preview, Source } from "./types";
import "./regions.css";

type Point = { x: number; y: number };
type Tool = "click" | "lasso" | "polygon" | "rectangle" | "brush" | "pick";
type Group = {
  id: number;
  name: string;
  operation: string;
  value: number;
  enabled: boolean;
  locked: boolean;
  pixels: number;
};
export type RegionState = {
  id: number;
  revision: number;
  version: number;
  pixels: number;
  regions: number;
  minimum: number;
  maximum: number;
  overlay: string;
  baseUrl: string;
  canUndo: boolean;
  canRedo: boolean;
  groups: Group[];
  pickedLayer?: number;
  preview?: Preview;
};
type Command = {
  action: string;
  value?: number;
  groupId?: number;
  name?: string;
  withinSelection?: boolean;
  referenceLayer?: number;
  selection?: {
    tool: string;
    points: Point[];
    combine?: string;
    scope?: string;
    radius?: number;
    above?: number;
    below?: number;
  };
};

export function RegionEditor({
  source,
  preview,
  dirty,
  onExit,
  onPreview,
  nextID,
  onBusy,
}: {
  source: Source;
  preview: Preview;
  dirty: boolean;
  onExit: () => void;
  onPreview: (p: Preview) => void;
  nextID: () => number;
  onBusy: (busy: boolean) => void;
}) {
  const [state, setState] = useState<RegionState | null>(null),
    [pending, setPending] = useState(false),
    [error, setError] = useState("");
  const [tool, setTool] = useState<Tool>("lasso"),
    [scope, setScope] = useState("inside"),
    [combine, setCombine] = useState("replace"),
    [radius, setRadius] = useState(4),
    [above, setAbove] = useState(0),
    [below, setBelow] = useState(0);
  const [target, setTarget] = useState(preview.options.hueforge.baseDepth),
    [delta, setDelta] = useState(1),
    [refine, setRefine] = useState(1),
    [tolerance, setTolerance] = useState(10),
    [speckle, setSpeckle] = useState(8),
    [smooth, setSmooth] = useState(1),
    [within, setWithin] = useState(false),
    [before, setBefore] = useState(false),
    [resetConfirm, setResetConfirm] = useState(false),
    [cutConfirm, setCutConfirm] = useState(false);
  const [zoom, setZoom] = useState(1),
    [pan, setPan] = useState({ x: 0, y: 0 }),
    [size, setSize] = useState({ w: 800, h: 500 }),
    [path, setPath] = useState<Point[]>([]);
  const viewport = useRef<HTMLDivElement>(null),
    frame = useRef<HTMLDivElement>(null),
    space = useRef(false),
    gesture = useRef<{ points: Point[]; combine: string } | null>(null),
    panning = useRef<{ x: number; y: number; px: number; py: number } | null>(
      null,
    ),
    inFlight = useRef(false);
  const stateRef = useRef(state);
  stateRef.current = state;
  const layers =
    preview.result.stackView?.layers.filter(
      (l) =>
        l.layer >=
        Math.round(
          1 +
            (preview.options.hueforge.baseDepth -
              (preview.options.hueforge.firstLayerHeight ||
                preview.options.hueforge.layerHeight)) /
              preview.options.hueforge.layerHeight,
        ),
    ) ?? [];
  const targetRow = layers.find((l) => l.layer === target);
  const targetRun = preview.result.stack?.runs.find(
    (r) => r.position === targetRow?.runPosition,
  );
  useEffect(() => {
    if (
      stateRef.current?.id === preview.id &&
      stateRef.current?.revision === preview.revision
    )
      return;
    let canceled = false;
    setError("");
    inFlight.current = true;
    setPending(true);
    onBusy(true);
    invoke<RegionState>("RegionEditor", preview.id, preview.revision)
      .then((s) => {
        if (!canceled) {
          setState(s);
          setTarget((t) =>
            layers.some((l) => l.layer === t) ? t : (layers[0]?.layer ?? 1),
          );
        }
      })
      .catch((e) => {
        if (!canceled) setError(String(e));
      })
      .finally(() => {
        if (!canceled) {
          setPending(false);
          inFlight.current = false;
          onBusy(false);
        }
      });
    return () => {
      canceled = true;
      onBusy(false);
    };
  }, [preview.id, preview.revision]);
  useLayoutEffect(() => {
    const el = viewport.current;
    if (!el) return;
    const ro = new ResizeObserver(() =>
      setSize({ w: el.clientWidth, h: el.clientHeight }),
    );
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const fit = Math.max(
      0.000001,
      Math.min(
        Math.max(1, size.w - 40) / source.width,
        Math.max(1, size.h - 40) / source.height,
      ),
    ),
    scale = fit * zoom;
  const adjustZoom = (factor: number) =>
    setZoom((z) =>
      Math.max(0.25, Math.min(Math.max(12, 12 / fit), z * factor)),
    );
  const run = async (command: Command) => {
    const current = stateRef.current;
    if (!current || inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    onBusy(true);
    setError("");
    try {
      const result = await invoke<RegionState>("EditRegions", {
        ...command,
        id: current.id,
        revision: current.revision,
        version: current.version,
        newId: nextID(),
      });
      setState(result);
      stateRef.current = result;
      if (result.pickedLayer !== undefined) {
        if (result.pickedLayer > 0) setTarget(result.pickedLayer);
        else setError("This is a cutout. Pick a printable layer.");
        setTool("click");
      }
      if (result.preview) onPreview(result.preview);
    } catch (e) {
      setError(String(e));
    } finally {
      inFlight.current = false;
      setPending(false);
      onBusy(false);
    }
  };
  const disabled = pending || before || !state || dirty,
    empty = disabled || !state?.pixels;
  const point = (e: { clientX: number; clientY: number }): Point => {
    const b = frame.current!.getBoundingClientRect();
    return {
      x: ((e.clientX - b.left) * source.width) / b.width,
      y: ((e.clientY - b.top) * source.height) / b.height,
    };
  };
  const finish = (points: Point[], operation = combine) => {
    setPath([]);
    gesture.current = null;
    if (points.length === 0) return;
    void run({
      action: tool === "pick" ? "pick" : "select",
      selection: {
        tool,
        points,
        scope,
        combine: operation,
        radius,
        above,
        below,
      },
    });
  };
  useEffect(() => {
    const el = viewport.current;
    if (!el) return;
    const wheel = (e: WheelEvent) => {
      e.preventDefault();
      adjustZoom(e.deltaY < 0 ? 1.12 : 1 / 1.12);
    };
    el.addEventListener("wheel", wheel, { passive: false });
    return () => el.removeEventListener("wheel", wheel);
  }, [fit]);
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest("input,select,textarea")) return;
      if (e.code === "Space") {
        space.current = true;
        e.preventDefault();
      }
      if (e.key === "Escape") {
        e.stopImmediatePropagation();
        e.preventDefault();
        if (path.length) {
          setPath([]);
          gesture.current = null;
        } else if (!pending) onExit();
      }
      if (e.key === "Enter" && tool === "polygon" && path.length >= 3) {
        e.preventDefault();
        finish(path);
      }
      if (
        (e.ctrlKey || e.metaKey) &&
        (e.key.toLowerCase() === "z" || e.key.toLowerCase() === "y")
      ) {
        e.preventDefault();
        e.stopImmediatePropagation();
        if (!disabled) {
          const redo = e.shiftKey || e.key.toLowerCase() === "y";
          if (redo ? state?.canRedo : state?.canUndo)
            void run({ action: redo ? "redo" : "undo" });
        }
      }
    };
    const up = (e: KeyboardEvent) => {
      if (e.code === "Space") space.current = false;
    };
    const blur = () => {
      space.current = false;
      panning.current = null;
      gesture.current = null;
      setPath([]);
    };
    window.addEventListener("keydown", down, true);
    window.addEventListener("keyup", up);
    window.addEventListener("blur", blur);
    return () => {
      window.removeEventListener("keydown", down, true);
      window.removeEventListener("keyup", up);
      window.removeEventListener("blur", blur);
    };
  }, [path, tool, disabled, pending, state]);
  const number = (
    label: string,
    value: number,
    set: (v: number) => void,
    min: number,
    max: number,
  ) => (
    <label className="region-number">
      <span>{label}</span>
      <input
        aria-label={label}
        type="number"
        min={min}
        max={max}
        step={1}
        value={value}
        onChange={(e) => {
          const n = e.target.valueAsNumber;
          if (Number.isInteger(n) && n >= min && n <= max) set(n);
        }}
      />
    </label>
  );
  const tools: [Tool, string, React.ReactNode][] = [
    ["click", "Click", <MousePointer2 size={15} />],
    ["lasso", "Lasso", <Lasso size={15} />],
    ["polygon", "Polygon", <Pentagon size={15} />],
    ["rectangle", "Box", <RectangleHorizontal size={15} />],
    ["brush", "Brush", <Brush size={15} />],
  ];
  return (
    <section
      className="workspace region-editor"
      aria-label="Region Edit workspace"
    >
      <div className="viewer-toolbar region-heading">
        <button
          className="button secondary"
          onClick={onExit}
          disabled={pending}
        >
          <ArrowLeft size={15} />
          Done
        </button>
        <strong>Region Edit</strong>
        <span className="region-hint">
          Select areas, then adjust their printed height.
        </span>
        <div className="zoom-controls">
          <button aria-label="Zoom out" onClick={() => adjustZoom(1 / 1.25)}>
            <Minus size={15} />
          </button>
          <span>{Math.round(scale * 100)}%</span>
          <button aria-label="Zoom in" onClick={() => adjustZoom(1.25)}>
            <Plus size={15} />
          </button>
          <button
            aria-label="Fit image"
            onClick={() => {
              setZoom(1);
              setPan({ x: 0, y: 0 });
            }}
          >
            <Maximize size={15} />
          </button>
          <button
            onClick={() => {
              setZoom(1 / fit);
              setPan({ x: 0, y: 0 });
            }}
          >
            1:1
          </button>
        </div>
      </div>
      <div className="region-toolbar">
        <div className="segmented">
          {tools.map(([t, label, icon]) => (
            <button
              key={t}
              disabled={disabled}
              aria-pressed={tool === t}
              onClick={() => {
                setTool(t);
                setPath([]);
                gesture.current = null;
              }}
            >
              {icon}
              {label}
            </button>
          ))}
        </div>
        <select
          aria-label="Selection operation"
          value={combine}
          disabled={disabled}
          onChange={(e) => setCombine(e.target.value)}
        >
          <option value="replace">New selection</option>
          <option value="add">Add</option>
          <option value="subtract">Subtract</option>
          <option value="intersect">Intersect</option>
        </select>
        <select
          aria-label="Selection coverage"
          value={scope}
          disabled={disabled || tool === "click"}
          onChange={(e) => setScope(e.target.value)}
        >
          <option value="inside">Enclosed regions</option>
          <option value="touch">Touching regions</option>
          <option value="pixels">Pixels inside</option>
        </select>
        {tool === "brush" &&
          number("Brush radius (px)", radius, setRadius, 1, 128)}
        {tool === "polygon" && (
          <button
            disabled={disabled || path.length < 3}
            onClick={() => finish(path)}
          >
            Finish polygon
          </button>
        )}
        <span className="region-hint">
          Shift adds · Alt subtracts · Space-drag pans
        </span>
      </div>
      {dirty && (
        <div className="region-message">
          Settings differ from this edited result. Reset edits below before
          generating a new stack.
        </div>
      )}
      {error && (
        <div className="region-message error" role="alert">
          {error}
        </div>
      )}
      <div className="region-layout">
        <div
          className="region-viewport checker"
          ref={viewport}
          aria-label="Editable image"
          onContextMenu={(e) => e.preventDefault()}
          onPointerDown={(e) => {
            if (!frame.current) return;
            if (e.button === 1 || (e.button === 0 && space.current)) {
              panning.current = {
                x: e.clientX,
                y: e.clientY,
                px: pan.x,
                py: pan.y,
              };
              e.currentTarget.setPointerCapture(e.pointerId);
              e.preventDefault();
              return;
            }
            if (e.button !== 0 || disabled) return;
            const p = point(e);
            if (tool === "polygon") {
              if (path.length < 8192) setPath([...path, p]);
              return;
            }
            gesture.current = {
              points: [p],
              combine: e.altKey ? "subtract" : e.shiftKey ? "add" : combine,
            };
            setPath([p]);
            e.currentTarget.setPointerCapture(e.pointerId);
          }}
          onPointerMove={(e) => {
            if (panning.current) {
              const p = panning.current;
              setPan({
                x: p.px + (e.clientX - p.x) / scale,
                y: p.py + (e.clientY - p.y) / scale,
              });
              return;
            }
            const g = gesture.current;
            if (!g) return;
            const p = point(e);
            if (tool === "rectangle") g.points = [g.points[0], p];
            else if (
              tool !== "click" &&
              tool !== "pick" &&
              g.points.length < 8192
            ) {
              const last = g.points[g.points.length - 1];
              if (Math.hypot(p.x - last.x, p.y - last.y) * scale >= 2)
                g.points.push(p);
            }
            setPath([...g.points]);
          }}
          onPointerUp={() => {
            if (panning.current) {
              panning.current = null;
              return;
            }
            const g = gesture.current;
            if (g) finish(g.points, g.combine);
          }}
          onPointerCancel={() => {
            gesture.current = null;
            panning.current = null;
            setPath([]);
          }}
          onLostPointerCapture={() => {
            gesture.current = null;
            panning.current = null;
          }}
        >
          <div
            className="region-image-frame"
            ref={frame}
            style={{
              width: source.width * scale,
              height: source.height * scale,
              transform: `translate(calc(-50% + ${pan.x * scale}px),calc(-50% + ${pan.y * scale}px))`,
            }}
          >
            <img
              draggable={false}
              alt={
                before
                  ? "Generated image before region edits"
                  : "Editable result"
              }
              src={before && state ? state.baseUrl : preview.url}
            />
            {!before && state && (
              <img
                draggable={false}
                className="region-overlay"
                alt="Selected pixels"
                src={state.overlay}
              />
            )}
            <svg
              className="region-path"
              viewBox={`0 0 ${source.width} ${source.height}`}
              aria-hidden="true"
            >
              {tool === "rectangle" && path.length === 2 ? (
                <rect
                  x={Math.min(path[0].x, path[1].x)}
                  y={Math.min(path[0].y, path[1].y)}
                  width={Math.abs(path[1].x - path[0].x)}
                  height={Math.abs(path[1].y - path[0].y)}
                />
              ) : (
                <polyline points={path.map((p) => `${p.x},${p.y}`).join(" ")} />
              )}
            </svg>
          </div>
        </div>
        <aside className="region-panel" aria-label="Region edit controls">
          <div className="region-summary" aria-live="polite">
            <strong>{state?.regions ?? 0} regions selected</strong>
            <span>
              {(state?.pixels ?? 0).toLocaleString()} pixels
              {state?.pixels
                ? ` · layers ${state.minimum}–${state.maximum}`
                : ""}
            </span>
          </div>
          <fieldset disabled={disabled}>
            <label className="select-field">
              <span>Assign to layer</span>
              <select
                aria-label="Assign to layer"
                value={target}
                onChange={(e) => setTarget(Number(e.target.value))}
              >
                {layers.map((l) => (
                  <option key={l.layer} value={l.layer}>
                    Layer {l.layer} · {l.height.toFixed(2)} mm
                  </option>
                ))}
              </select>
            </label>
            {targetRow && (
              <div className="region-target">
                <span
                  className="region-swatch"
                  style={{
                    background: `rgb(${targetRow.predictedRGB.join(",")})`,
                  }}
                />
                <span>
                  Predicted layer color
                  {targetRun && (
                    <small>
                      {targetRun.filament.brand} {targetRun.filament.name} ·{" "}
                      {targetRun.filament.material} · TD {targetRun.filament.td}
                    </small>
                  )}
                </span>
              </div>
            )}
            <button
              className="button primary region-wide"
              disabled={empty}
              onClick={() => void run({ action: "assign", value: target })}
            >
              Assign layer
            </button>
            <button
              className="button secondary region-wide"
              aria-pressed={tool === "pick"}
              onClick={() => {
                setTool("pick");
                setPath([]);
              }}
            >
              <Pipette size={15} />
              Pick layer from image
            </button>
            <div className="region-pair">
              <button
                disabled={empty}
                onClick={() => void run({ action: "shift", value: -delta })}
              >
                −{delta} layer{delta === 1 ? "" : "s"}
              </button>
              <button
                disabled={empty}
                onClick={() => void run({ action: "shift", value: delta })}
              >
                +{delta} layer{delta === 1 ? "" : "s"}
              </button>
            </div>
            {number("Move step (layers)", delta, setDelta, 1, 4096)}
            <span className="region-hint">
              Moving keeps relief and stops at printable bounds.
            </span>
          </fieldset>
          <div className="region-pair">
            <button
              disabled={disabled || !state?.canUndo}
              onClick={() => void run({ action: "undo" })}
            >
              <Undo2 size={14} />
              Undo
            </button>
            <button
              disabled={disabled || !state?.canRedo}
              onClick={() => void run({ action: "redo" })}
            >
              <Redo2 size={14} />
              Redo
            </button>
          </div>
          <label className="check-field">
            <input
              type="checkbox"
              checked={before}
              disabled={pending || !state}
              onChange={(e) => setBefore(e.target.checked)}
            />
            Before edits
          </label>
          <details>
            <summary>Refine selection</summary>
            <fieldset disabled={disabled}>
              <div className="region-pair">
                <button onClick={() => void run({ action: "all" })}>
                  Select all
                </button>
                <button onClick={() => void run({ action: "clear" })}>
                  Clear
                </button>
                <button onClick={() => void run({ action: "invert" })}>
                  Invert
                </button>
              </div>
              {number("Refine radius (px)", refine, setRefine, 1, 64)}
              <div className="region-pair">
                <button
                  disabled={empty}
                  onClick={() => void run({ action: "grow", value: refine })}
                >
                  Grow
                </button>
                <button
                  disabled={empty}
                  onClick={() => void run({ action: "shrink", value: refine })}
                >
                  Shrink
                </button>
              </div>
              <button
                disabled={empty}
                onClick={() => void run({ action: "fill" })}
              >
                Fill selection holes
              </button>
              {number("Click above (layers)", above, setAbove, 0, 256)}
              {number("Click below (layers)", below, setBelow, 0, 256)}
              <button
                disabled={empty}
                onClick={() =>
                  void run({ action: "same-layer", referenceLayer: target })
                }
              >
                Select target layer
              </button>
              {number(
                "Similar color tolerance",
                tolerance,
                setTolerance,
                0,
                100,
              )}
              <label className="check-field">
                <input
                  type="checkbox"
                  checked={within}
                  onChange={(e) => setWithin(e.target.checked)}
                />
                Limit similarity to selection
              </label>
              <button
                disabled={empty}
                onClick={() =>
                  void run({
                    action: "similar",
                    value: tolerance,
                    referenceLayer: target,
                    withinSelection: within,
                  })
                }
              >
                Select similar colors
              </button>
              <span className="region-hint">
                Similarity uses the target layer above. Use the eyedropper to
                choose a reference, then optionally limit the search to your
                lasso.
              </span>
            </fieldset>
          </details>
          <details>
            <summary>Height and cleanup</summary>
            <fieldset disabled={disabled}>
              <div className="region-pair">
                {["common", "high", "low"].map((mode) => (
                  <button
                    key={mode}
                    disabled={empty}
                    onClick={() => void run({ action: `flatten-${mode}` })}
                  >
                    Flatten {mode}
                  </button>
                ))}
              </div>
              <button
                disabled={empty}
                onClick={() => void run({ action: "match-surround" })}
              >
                Match surrounding layer
              </button>
              <button
                disabled={empty}
                onClick={() => void run({ action: "restore" })}
              >
                Restore generated heights
              </button>
              {number("Speckle area (pixels)", speckle, setSpeckle, 1, 100000)}
              <button
                onClick={() => void run({ action: "speckles", value: speckle })}
              >
                Select small islands
              </button>
              <span className="region-hint">
                Select islands within your selection, then assign or match their
                surrounding layer.
              </span>
            </fieldset>
          </details>
          <details>
            <summary>Advanced edits</summary>
            <fieldset disabled={disabled}>
              {number("Smooth radius (px)", smooth, setSmooth, 1, 4)}
              <button
                disabled={empty}
                onClick={() => void run({ action: "smooth", value: smooth })}
              >
                Smooth selected heights
              </button>
              <span className="region-hint">
                Averages nearby heights onto printable layers. May introduce
                intermediate colors; use Before and Undo to compare.
              </span>
              {cutConfirm ? (
                <div className="region-confirm">
                  <span>Remove selected pixels through the base?</span>
                  <div className="region-pair">
                    <button
                      onClick={() => {
                        setCutConfirm(false);
                        void run({ action: "cut" });
                      }}
                    >
                      Cut through base
                    </button>
                    <button onClick={() => setCutConfirm(false)}>Cancel</button>
                  </div>
                </div>
              ) : (
                <button disabled={empty} onClick={() => setCutConfirm(true)}>
                  Cut hole…
                </button>
              )}
              <span className="region-hint">
                Restore generated heights also restores cut coverage.
              </span>
            </fieldset>
          </details>
          <details open={!!state?.groups.length}>
            <summary>Edit groups ({state?.groups.length ?? 0})</summary>
            <div className="region-groups">
              {state?.groups.map((g) => (
                <div key={g.id} className="region-group">
                  <input
                    aria-label={`Name for ${g.name}`}
                    defaultValue={g.name}
                    key={`${g.id}:${g.name}`}
                    maxLength={100}
                    disabled={disabled || g.locked}
                    onBlur={(e) => {
                      if (e.target.value.trim() !== g.name)
                        void run({
                          action: "rename",
                          groupId: g.id,
                          name: e.target.value,
                        });
                    }}
                  />
                  <small>
                    {g.operation}{" "}
                    {g.operation === "assign" ||
                    g.operation === "shift" ||
                    g.operation === "smooth"
                      ? g.value
                      : ""}{" "}
                    · {g.pixels.toLocaleString()} px
                  </small>
                  <div className="region-pair">
                    <button
                      disabled={disabled}
                      onClick={() =>
                        void run({ action: "recall", groupId: g.id })
                      }
                    >
                      Select
                    </button>
                    <button
                      aria-label={`${g.enabled ? "Disable" : "Enable"} ${g.name}`}
                      disabled={disabled || g.locked}
                      onClick={() =>
                        void run({ action: "enable", groupId: g.id })
                      }
                    >
                      {g.enabled ? <Eye size={14} /> : <EyeOff size={14} />}
                    </button>
                    <button
                      aria-label={`${g.locked ? "Unlock" : "Protect"} ${g.name}`}
                      disabled={disabled}
                      onClick={() =>
                        void run({ action: "lock", groupId: g.id })
                      }
                    >
                      {g.locked ? <Lock size={14} /> : <Unlock size={14} />}
                    </button>
                    <button
                      aria-label={`Delete ${g.name}`}
                      disabled={disabled || g.locked}
                      onClick={() =>
                        void run({ action: "delete", groupId: g.id })
                      }
                    >
                      <Trash2 size={14} />
                    </button>
                    <button
                      disabled={disabled || g.locked}
                      onClick={() =>
                        void run({ action: "split", groupId: g.id })
                      }
                    >
                      Split
                    </button>
                  </div>
                </div>
              ))}
            </div>
            <span className="region-hint">
              Later groups apply last. Protected groups exclude their pixels
              from later edits.
            </span>
          </details>
          {resetConfirm ? (
            <div className="region-confirm">
              <span>Remove all groups and return to the generated image?</span>
              <div className="region-pair">
                <button
                  disabled={pending}
                  onClick={() => {
                    setResetConfirm(false);
                    void run({ action: "reset" });
                  }}
                >
                  Reset all edits
                </button>
                <button onClick={() => setResetConfirm(false)}>Cancel</button>
              </div>
            </div>
          ) : (
            <button
              disabled={pending || !state?.groups.length}
              onClick={() => setResetConfirm(true)}
            >
              Reset all edits…
            </button>
          )}
        </aside>
      </div>
      <div className="region-footer" role="status">
        {pending
          ? "Updating regions…"
          : tool === "pick"
            ? "Click a printable pixel to sample its layer."
            : tool === "polygon"
              ? "Click vertices, then Finish polygon or Enter. Escape cancels."
              : "Selections and edits use full-resolution pixels. Undo: Ctrl+Z · Redo: Ctrl+Shift+Z"}
      </div>
    </section>
  );
}
