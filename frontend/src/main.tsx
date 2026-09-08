import React, { useEffect, useRef, useState, useLayoutEffect } from "react";
import { createRoot } from "react-dom/client";
import {
  Aperture,
  ArrowDownToLine,
  ArrowUpRight,
  Check,
  ChevronDown,
  ChevronRight,
  FileImage,
  FolderOpen,
  Layers,
  LoaderCircle,
  Maximize,
  Minus,
  Plus,
  Redo2,
  RotateCcw,
  Save,
  SlidersHorizontal,
  Sparkles,
  SwatchBook,
  Undo2,
  X,
  Info,
  Search,
  PanelRightClose,
  PanelRightOpen,
  Trash2,
  Image as ImageIcon,
} from "lucide-react";
import { invoke, on, desktop } from "./bridge";
import {
  applyColorBudget,
  applySavedPreset,
  changeProcessingMode,
} from "./settings";
import {
  defaults,
  emptyFilter,
  type Options,
  type Source,
  type Snapshot,
  type Preview,
  type Library,
  type Filter,
  type Preset,
  type Request,
  type HueForgeOptions,
  type Filament,
} from "./types";
import "./style.css";

const copy = <T,>(v: T): T => JSON.parse(JSON.stringify(v));
const errorMessage = (e: unknown) =>
  e instanceof Error ? e.message : String(e);
const pct = (v: number) => `${Math.round(v * 100)}%`;
function filamentDescription(f: Filament) {
  const details = `${f.brand} · ${f.name}\n${f.material || "Unspecified material"} · TD ${f.td} mm\nColor ${f.hex}`;
  if (!f.libraryRGB) return details;
  const original = f.libraryRGB
    .map((v) => v.toString(16).padStart(2, "0"))
    .join("")
    .toUpperCase();
  return `${details}\nTrue black enabled · library color #${original}`;
}
const compactColorCount = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});
function ColorCount({ count }: { count: number }) {
  const exact = `${count.toLocaleString("en-US")} unique RGB ${count === 1 ? "color" : "colors"}. Counted in sRGB; fully transparent pixels excluded.`;
  return (
    <span className="color-count" title={exact} aria-label={exact}>
      {compactColorCount.format(count)} {count === 1 ? "color" : "colors"}
    </span>
  );
}
function IconButton({
  title,
  onClick,
  disabled = false,
  children,
}: {
  title: string;
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      className="icon-button"
      title={title}
      aria-label={title}
      onClick={onClick}
      disabled={disabled}
    >
      {children}
    </button>
  );
}
function Numeric({
  label,
  value,
  onChange,
  min,
  max,
  step = 1,
  suffix,
  help,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  min: number;
  max: number;
  step?: number;
  suffix?: string;
  help?: string;
}) {
  const [draft, setDraft] = useState(String(value));
  useEffect(() => setDraft(String(value)), [value]);
  return (
    <label className="numeric-field">
      <span title={help}>
        {label}
        {help && <Info size={12} />}
      </span>
      <div>
        <input
          aria-label={label}
          type="number"
          min={min}
          max={max}
          step={step}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            const n = e.target.valueAsNumber;
            if (Number.isFinite(n) && n >= min && n <= max) onChange(n);
          }}
          onBlur={() => setDraft(String(value))}
        />
        {suffix && <small>{suffix}</small>}
      </div>
    </label>
  );
}
function Range({
  label,
  value,
  onChange,
  min,
  max,
  step = 1,
  format,
  help,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  min: number;
  max: number;
  step?: number;
  format?: (v: number) => string;
  help?: string;
}) {
  return (
    <div className="range-field">
      <div>
        <label>
          {label}
          {help && (
            <span className="help-dot" title={help}>
              <Info size={12} />
            </span>
          )}
        </label>
        <span className="range-value">{format ? format(value) : value}</span>
      </div>
      <input
        aria-label={label}
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        style={
          {
            "--fill": `${Math.max(0, Math.min(100, ((value - min) / (max - min)) * 100))}%`,
          } as React.CSSProperties
        }
        onChange={(e) => onChange(Number(e.target.value))}
      />
    </div>
  );
}
function Section({
  title,
  children,
  initial = false,
}: {
  title: string;
  children: React.ReactNode;
  initial?: boolean;
}) {
  return (
    <details className="section" open={initial || undefined}>
      <summary>
        {title}
        <ChevronDown size={14} />
      </summary>
      <div className="section-body">{children}</div>
    </details>
  );
}

function Viewer({
  source,
  preview,
  busy,
  dirty,
}: {
  source: Source | null;
  preview: Preview | null;
  busy: boolean;
  dirty: boolean;
}) {
  const [view, setView] = useState<
    "original" | "reduced" | "split" | "side-by-side"
  >("split");
  const sideBySide = view === "side-by-side";
  const [split, setSplit] = useState(50);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [size, setSize] = useState({ w: 800, h: 600 });
  const viewport = useRef<HTMLDivElement>(null);
  const comparisonViewport = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; y: number; px: number; py: number } | null>(
    null,
  );
  useLayoutEffect(() => {
    const el = sideBySide ? comparisonViewport.current : viewport.current;
    if (!el) return;
    const observer = new ResizeObserver(() =>
      setSize({ w: el.clientWidth, h: el.clientHeight }),
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [sideBySide, source?.revision]);
  useEffect(() => {
    setPan({ x: 0, y: 0 });
    setZoom(1);
  }, [source?.revision]);
  const fit = source
    ? Math.max(
        0.000001,
        Math.min(
          Math.max(1, size.w - (sideBySide ? 32 : 80)) / source.width,
          Math.max(1, size.h - (sideBySide ? 32 : 92)) / source.height,
        ),
      )
    : 1;
  const scale = fit * zoom;
  const w = (source?.width ?? 100) * scale,
    h = (source?.height ?? 100) * scale;
  const frameStyle: React.CSSProperties = {
    width: w,
    height: h,
    transform: `translate(calc(-50% + ${pan.x * scale}px), calc(-50% + ${pan.y * scale}px))`,
  };
  const adjustZoom = (factor: number) => {
    // Keep 1:1 reachable even when a large image fits into a narrow comparison pane.
    setZoom((z) =>
      Math.max(0.25, Math.min(Math.max(12, 12 / fit), z * factor)),
    );
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
  return (
    <section className="workspace">
      <div className="viewer-toolbar">
        <div
          className="segmented view-tabs"
          role="group"
          aria-label="Preview comparison"
        >
          {(["original", "split", "side-by-side", "reduced"] as const).map(
            (v) => (
              <button
                key={v}
                className={view === v ? "active" : ""}
                aria-pressed={view === v}
                onClick={() => setView(v)}
                disabled={v !== "original" && !preview}
              >
                {v === "split"
                  ? "Compare"
                  : v === "side-by-side"
                    ? "Side by side"
                    : v === "original"
                      ? "Original"
                      : "Result"}
              </button>
            ),
          )}
        </div>
        <div className="zoom-controls">
          <IconButton title="Zoom out" onClick={() => adjustZoom(1 / 1.25)}>
            <Minus size={15} />
          </IconButton>
          <span>{Math.round(scale * 100)}%</span>
          <IconButton title="Zoom in" onClick={() => adjustZoom(1.25)}>
            <Plus size={15} />
          </IconButton>
          <i />
          <IconButton
            title="Fit image to window"
            onClick={() => {
              setZoom(1);
              setPan({ x: 0, y: 0 });
            }}
          >
            <Maximize size={15} />
          </IconButton>
          <button
            className="text-button"
            onClick={() => {
              setZoom(1 / fit);
              setPan({ x: 0, y: 0 });
            }}
          >
            1:1
          </button>
        </div>
      </div>
      {!sideBySide && source && (
        <div className="preview-counts-bar" aria-label="Image color counts">
          {(view !== "reduced" || !preview) && (
            <div>
              <strong>Original</strong>
              <ColorCount count={source.uniqueColors} />
            </div>
          )}
          {view !== "original" && (
            <div>
              <strong>{preview && dirty ? "Previous result" : "Result"}</strong>
              {preview ? (
                <ColorCount count={preview.result.uniqueColors} />
              ) : (
                <span>Awaiting preview</span>
              )}
            </div>
          )}
        </div>
      )}
      <div
        className="canvas-area"
        ref={viewport}
        onPointerDown={(e) => {
          if (e.button !== 0) return;
          if ((e.target as HTMLElement).closest("input,button")) return;
          drag.current = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y };
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => {
          if (drag.current)
            setPan({
              x: drag.current.px + (e.clientX - drag.current.x) / scale,
              y: drag.current.py + (e.clientY - drag.current.y) / scale,
            });
        }}
        onPointerUp={() => {
          drag.current = null;
        }}
        onPointerCancel={() => {
          drag.current = null;
        }}
        onLostPointerCapture={() => {
          drag.current = null;
        }}
      >
        <div className="canvas-caption">
          <span>
            <span className="status-dot" />{" "}
            {source?.demo ? "SAMPLE IMAGE" : "IMAGE WORKSPACE"}
          </span>
          <span>
            {source?.metadata.colorProfile ?? "sRGB"}{" "}
            <span className="caption-dot">·</span> Alpha preserved
          </span>
        </div>
        {source && sideBySide ? (
          <div className="side-by-side-view">
            <section
              className="comparison-pane"
              aria-label="Original image panel"
            >
              <div className="comparison-pane-heading">
                <strong>Original</strong>
                <ColorCount count={source.uniqueColors} />
              </div>
              <div className="comparison-pane-body" ref={comparisonViewport}>
                <div className="image-frame checker" style={frameStyle}>
                  <img
                    draggable={false}
                    alt="Original image"
                    src={source.url}
                  />
                </div>
              </div>
            </section>
            <section
              className="comparison-pane"
              aria-label="Processed image panel"
            >
              <div className="comparison-pane-heading">
                <strong title={preview && dirty ? "Previous result" : "Result"}>
                  {preview && dirty ? "Previous result" : "Result"}
                </strong>
                {preview ? (
                  <ColorCount count={preview.result.uniqueColors} />
                ) : (
                  <span>Awaiting preview</span>
                )}
              </div>
              <div className="comparison-pane-body">
                {preview ? (
                  <div className="image-frame checker" style={frameStyle}>
                    <img
                      draggable={false}
                      alt="Processed image"
                      src={preview.url}
                    />
                  </div>
                ) : (
                  <div className="comparison-placeholder">
                    <Aperture size={24} />
                    <span>Generate a preview to compare</span>
                  </div>
                )}
              </div>
            </section>
          </div>
        ) : source ? (
          <div className="image-frame checker" style={frameStyle}>
            <img draggable={false} alt="Original image" src={source.url} />
            {preview && view !== "original" && (
              <img
                draggable={false}
                alt="Processed image"
                src={preview.url}
                style={{
                  clipPath:
                    view === "split" ? `inset(0 0 0 ${split}%)` : undefined,
                }}
              />
            )}
            {view === "split" && preview && (
              <>
                <div className="comparison-line" style={{ left: `${split}%` }}>
                  <div>
                    <ChevronRight
                      size={14}
                      style={{ transform: "rotate(180deg)" }}
                    />
                    <ChevronRight size={14} />
                  </div>
                </div>
                <input
                  className="comparison-input"
                  aria-label="Before and after divider"
                  type="range"
                  min="0"
                  max="100"
                  value={split}
                  onChange={(e) => setSplit(Number(e.target.value))}
                />
              </>
            )}
            <div className="image-badges">
              {view !== "reduced" && <span>ORIGINAL</span>}
              {view !== "original" && preview && (
                <span>
                  {dirty
                    ? "PREVIOUS RESULT"
                    : `${preview.result.palette.length} COLORS`}
                </span>
              )}
            </div>
          </div>
        ) : (
          <div className="loading-document">
            <Aperture size={36} />
            <p>Preparing your workspace…</p>
          </div>
        )}
        {(busy || dirty) && (
          <div className="preview-state">
            {busy ? (
              <LoaderCircle size={13} className="spin" />
            ) : (
              <Info size={13} />
            )}{" "}
            {busy
              ? "Rendering full-resolution preview"
              : dirty
                ? "Settings changed · preview needed"
                : ""}
          </div>
        )}
        <div className="canvas-bottom">
          <span>
            {sideBySide ? "Zoom and pan linked" : "Scroll to zoom"} <b>·</b>{" "}
            {sideBySide ? "Scroll or drag either image" : "Drag to pan"}
          </span>
          <span>
            {source
              ? `${source.width.toLocaleString()} × ${source.height.toLocaleString()} px`
              : ""}
          </span>
        </div>
      </div>
      <div className="workspace-note">
        <span>
          <Check size={13} /> Full-resolution output. No dithering.
        </span>
        <span>Made for deliberate color.</span>
      </div>
    </section>
  );
}

function App() {
  const [source, setSource] = useState<Source | null>(null),
    [options, setOptions] = useState<Options>(copy(defaults)),
    [preview, setPreview] = useState<Preview | null>(null),
    [library, setLibrary] = useState<Library | null>(null),
    [libraryPath, setLibraryPath] = useState(""),
    [filter, setFilter] = useState<Filter>(copy(emptyFilter)),
    [presets, setPresets] = useState<Preset[]>([]),
    [recent, setRecent] = useState<string[]>([]);
  const [tab, setTab] = useState<"adjust" | "filaments">("adjust"),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [exporting, setExporting] = useState(false),
    [dirty, setDirty] = useState(true),
    [auto, setAuto] = useState(true),
    [rerun, setRerun] = useState(0),
    [progress, setProgress] = useState({
      stage: "Preparing workspace",
      fraction: 0,
    }),
    [toast, setToast] = useState<{ message: string; error: boolean } | null>(
      null,
    ),
    [showExport, setShowExport] = useState(false),
    [showPalette, setShowPalette] = useState(true),
    [search, setSearch] = useState("");
  const [modeHelp, setModeHelp] = useState<Options["mode"] | null>(null);
  const [dialog, setDialog] = useState<{
    title: string;
    label: string;
    value: string;
    submit: (v: string) => Promise<void>;
  } | null>(null);
  const [history, setHistory] = useState<Options[]>([]),
    [historyIndex, setHistoryIndex] = useState(-1);
  const seq = useRef(0);
  const live = useRef({ source, options, libraryPath, filter, preview, dirty });
  live.current = { source, options, libraryPath, filter, preview, dirty };
  const initialized = useRef(false);
  const notify = (message: string, error = false) =>
    setToast({ message, error });
  const handleError = (e: unknown) => {
    const m = errorMessage(e);
    if (!/context canceled|export canceled/.test(m)) notify(m, true);
  };
  const applySnapshot = (s: Snapshot) => {
    setSource(s.source);
    setOptions(copy(s.settings.options));
    setLibraryPath(s.settings.libraryPath ?? "");
    setFilter({ ...copy(emptyFilter), ...s.settings.filter });
    setLibrary(s.library);
    setPresets(s.settings.presets ?? []);
    setRecent(s.settings.recent ?? []);
    setPreview(null);
    setHistory([copy(s.settings.options)]);
    setHistoryIndex(0);
    setDirty(true);
    if (s.warning) notify(s.warning, true);
    else if (s.source.metadata.warnings?.length)
      notify(s.source.metadata.warnings.join(" "));
  };
  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;
    invoke<Snapshot>("Initialize")
      .then(applySnapshot)
      .catch(handleError)
      .finally(() => setLoading(false));
  }, []);
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), toast.error ? 14000 : 6500);
    return () => clearTimeout(t);
  }, [toast]);
  const update = (next: Options) => {
    setOptions(next);
    setDirty(true);
    setHistory((prev) =>
      [...prev.slice(0, historyIndex + 1), copy(next)].slice(-60),
    );
    setHistoryIndex(Math.min(59, historyIndex + 1));
  };
  const change = <K extends keyof Options>(key: K, value: Options[K]) =>
    update({ ...options, [key]: value });
  const changeHF = <K extends keyof HueForgeOptions>(
    key: K,
    value: HueForgeOptions[K],
  ) => update({ ...options, hueforge: { ...options.hueforge, [key]: value } });
  const undo = () => {
    if (historyIndex > 0) {
      setHistoryIndex(historyIndex - 1);
      setOptions(copy(history[historyIndex - 1]));
      setDirty(true);
    }
  };
  const redo = () => {
    if (historyIndex < history.length - 1) {
      setHistoryIndex(historyIndex + 1);
      setOptions(copy(history[historyIndex + 1]));
      setDirty(true);
    }
  };
  const request = (): Request => ({
    id: seq.current,
    revision: live.current.source?.revision ?? 0,
    options: live.current.options,
    libraryPath: live.current.libraryPath,
    filter: live.current.filter,
  });
  const stateKey = JSON.stringify([options, libraryPath, filter]);
  const manualVersion = useRef(0);
  const renderedKey = useRef("");
  useEffect(() => {
    if (!source || loading) return;
    const id = ++seq.current;
    const signature = source.revision + ":" + stateKey;
    const manualRequested = rerun !== manualVersion.current;
    manualVersion.current = rerun;
    setDirty(signature !== renderedKey.current);
    setBusy(false);
    const canceled = invoke("Cancel").catch(() => {});
    if (!auto && !manualRequested) return;
    if (options.mode !== "standard" && !libraryPath) {
      setProgress({
        stage: "Choose a filament library to continue",
        fraction: 0,
      });
      return;
    }
    const t = setTimeout(
      async () => {
        await canceled;
        if (seq.current !== id) return;
        setBusy(true);
        setProgress({ stage: "Analyzing image", fraction: 0 });
        try {
          const result = await invoke<Preview>("Process", {
            id,
            revision: source.revision,
            options,
            libraryPath,
            filter,
          });
          if (seq.current === id) {
            renderedKey.current = signature;
            setPreview(result);
            setDirty(false);
            setProgress({ stage: "Ready", fraction: 1 });
            if (result.warning) notify(result.warning, true);
          }
        } catch (e) {
          if (seq.current === id) {
            handleError(e);
            setProgress({
              stage: "Preview could not be generated",
              fraction: 0,
            });
          }
        } finally {
          if (seq.current === id) setBusy(false);
        }
      },
      auto ? 450 : 0,
    );
    return () => {
      clearTimeout(t);
    };
  }, [source?.revision, stateKey, loading, auto, rerun]);
  useEffect(
    () =>
      on("progress", (p: { id: number; stage: string; fraction: number }) => {
        if (p.id === seq.current) setProgress(p);
      }),
    [],
  );
  const loadPath = async (path: string) => {
    if (!path) return;
    setLoading(true);
    seq.current++;
    try {
      applySnapshot(await invoke<Snapshot>("LoadImage", path));
    } catch (e) {
      handleError(e);
    } finally {
      setLoading(false);
    }
  };
  const textDialog = (
    title: string,
    label: string,
    value: string,
    submit: (v: string) => Promise<void>,
  ) => setDialog({ title, label, value, submit });
  const open = async () => {
    if (!desktop) {
      textDialog("Open image", "Absolute image path", "", loadPath);
      return;
    }
    setLoading(true);
    seq.current++;
    try {
      const s = await invoke<Snapshot | null>("OpenImage");
      if (s) applySnapshot(s);
    } catch (e) {
      handleError(e);
    } finally {
      setLoading(false);
    }
  };
  const demo = async () => {
    setLoading(true);
    seq.current++;
    try {
      applySnapshot(await invoke<Snapshot>("UseDemo"));
    } catch (e) {
      handleError(e);
    } finally {
      setLoading(false);
    }
  };
  const chooseLibrary = async () => {
    const apply = async (path: string) => {
      if (!path) return;
      const f = copy(emptyFilter);
      const lib = await invoke<Library>("SetLibrary", path, f);
      setLibraryPath(path);
      setFilter(f);
      setLibrary(lib);
      setDirty(true);
      setRerun((v) => v + 1);
      notify(`Loaded ${lib.filaments.length} eligible filaments`);
    };
    try {
      if (!desktop) {
        textDialog(
          "Load filament library",
          "Absolute path to personal_library.json",
          libraryPath,
          apply,
        );
        return;
      }
      const path = await invoke<string>("ChooseLibrary");
      await apply(path);
    } catch (e) {
      handleError(e);
    }
  };
  const refreshLibrary = async (next = filter) => {
    try {
      const lib = await invoke<Library>("SetLibrary", libraryPath, next);
      setLibrary(lib);
      setFilter(next);
      setDirty(true);
      setRerun((v) => v + 1);
    } catch (e) {
      handleError(e);
    }
  };
  const exportFile = async (kind: string) => {
    setShowExport(false);
    const p = live.current.preview;
    if (!p || live.current.dirty) return;
    const run = async (path?: string) => {
      setExporting(true);
      try {
        const output = await invoke<string>(
          "Export",
          kind,
          p.id,
          p.revision,
          ...(path ? [path] : []),
        );
        if (output) notify(`Saved ${output}`);
      } catch (e) {
        handleError(e);
      } finally {
        setExporting(false);
      }
    };
    if (!desktop) {
      textDialog(
        "Export " +
          (kind === "png"
            ? "PNG"
            : kind === "layers"
              ? "layer map"
              : "palette report"),
        "Absolute output path",
        "",
        run,
      );
      return;
    }
    await run();
  };
  const saveProject = async () => {
    const run = async (path?: string) => {
      const output = await invoke<string>(
        "SaveProject",
        request(),
        ...(path ? [path] : []),
      );
      if (output) notify(`Project saved: ${output}`);
    };
    try {
      if (!desktop) {
        textDialog("Save project", "Absolute project path", "", run);
        return;
      }
      await run();
    } catch (e) {
      handleError(e);
    }
  };
  const openProject = async () => {
    const run = async (path?: string) => {
      setLoading(true);
      seq.current++;
      try {
        const s = await invoke<Snapshot | null>(
          "OpenProject",
          ...(path ? [path] : []),
        );
        if (s) applySnapshot(s);
      } finally {
        setLoading(false);
      }
    };
    try {
      if (!desktop) {
        textDialog("Open project", "Absolute project path", "", run);
        return;
      }
      await run();
    } catch (e) {
      handleError(e);
    }
  };
  const callbacks = useRef({
    open,
    saveProject,
    openProject,
    exportFile,
    loadPath,
    undo,
    redo,
  });
  callbacks.current = {
    open,
    saveProject,
    openProject,
    exportFile,
    loadPath,
    undo,
    redo,
  };
  useEffect(() => {
    const off1 = on("file-dropped", (path: string) =>
      callbacks.current.loadPath(path),
    );
    const off2 = on("command", (c: string) => {
      if (c === "open") callbacks.current.open();
      if (c === "open-project") callbacks.current.openProject();
      if (c === "save-project") callbacks.current.saveProject();
      if (c === "export") callbacks.current.exportFile("png");
    });
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setDialog(null);
        setShowExport(false);
        setModeHelp(null);
      }
      if (!(e.ctrlKey || e.metaKey)) return;
      const target = e.target as HTMLElement;
      if (e.key.toLowerCase() === "z" && !target.closest("input,textarea")) {
        e.preventDefault();
        e.shiftKey ? callbacks.current.redo() : callbacks.current.undo();
      }
      if (e.key.toLowerCase() === "o") {
        e.preventDefault();
        e.shiftKey ? callbacks.current.openProject() : callbacks.current.open();
      }
      if (e.key.toLowerCase() === "s") {
        e.preventDefault();
        callbacks.current.saveProject();
      }
      if (e.key.toLowerCase() === "e") {
        e.preventDefault();
        callbacks.current.exportFile("png");
      }
    };
    window.addEventListener("keydown", key);
    return () => {
      off1();
      off2();
      window.removeEventListener("keydown", key);
    };
  }, []);
  const savePreset = () =>
    textDialog("Save a preset", "Preset name", "", async (name) => {
      setPresets(await invoke<Preset[]>("SavePreset", name, options));
      notify(`Preset saved: ${name}`);
    });
  const cancel = () => {
    seq.current++;
    setBusy(false);
    setAuto(false);
    setDirty(true);
    setProgress({ stage: "Preview canceled", fraction: 0 });
    invoke("Cancel").catch(handleError);
  };
  const modeInfo = {
    standard: [
      "Perceptual reduction",
      "Reduces the image to a smaller palette drawn from its own colors. No filament library needed.",
      "Best for clean, simplified artwork while keeping the original hues.",
    ],
    guided: [
      "Filament guidance",
      "Pulls the palette toward your selected filaments and their predicted blends.",
      "Best for preparing an image for HueForge, where you choose the final layer plan.",
    ],
    stack: [
      "Stack planning",
      "Predicts colors from one ordered filament stack and generates its layer plan.",
      "Best for exploring filament swaps and layer heights. Predictions are approximate; verify the plan in HueForge.",
    ],
  } as const;
  const selected =
    preview?.result.guidance?.selectedFilaments ??
    preview?.result.stack?.runs.map((r) => r.filament) ??
    [];
  const visibleFilaments =
    library?.filaments.filter((f) =>
      `${f.name} ${f.brand} ${f.material}`
        .toLowerCase()
        .includes(search.toLowerCase()),
    ) ?? [];
  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="brand">
          <div className="brand-mark">
            <Aperture size={25} strokeWidth={1.6} />
          </div>
          <div>
            ColorNinja<span>STUDIO</span>
          </div>
        </div>
        <div className="document-title">
          <FileImage size={15} />
          <span>{source?.name ?? "Untitled"}</span>
          {source?.demo && <small>Sample</small>}
          {dirty && source && <i title="Settings differ from the preview" />}
        </div>
        <div className="header-actions">
          <IconButton title="Open project (Ctrl+Shift+O)" onClick={openProject}>
            <FolderOpen size={17} />
          </IconButton>
          <IconButton
            title="Save project (Ctrl+S)"
            onClick={saveProject}
            disabled={!source}
          >
            <Save size={17} />
          </IconButton>
          <span className="header-divider" />
          <button
            className="button secondary"
            onClick={open}
            disabled={loading}
          >
            <Plus size={16} /> Open image
          </button>
          <div className="export-container">
            <button
              className="button primary"
              onClick={() => exportFile("png")}
              disabled={!preview || dirty || busy || exporting}
            >
              {exporting ? (
                <LoaderCircle className="spin" size={16} />
              ) : (
                <ArrowDownToLine size={16} />
              )}{" "}
              Export PNG
            </button>
            <button
              className="export-caret"
              aria-label="More export options"
              onClick={() => setShowExport(!showExport)}
              disabled={!preview || dirty || busy}
            >
              <ChevronDown size={15} />
            </button>
            {showExport && (
              <>
                <button
                  className="menu-backdrop"
                  aria-label="Close export menu"
                  onClick={() => setShowExport(false)}
                />
                <div className="dropdown export-menu">
                  <button onClick={() => exportFile("png")}>
                    <FileImage size={15} /> Full-resolution PNG
                  </button>
                  <button onClick={() => exportFile("palette")}>
                    <SwatchBook size={15} /> Palette & settings JSON
                  </button>
                  <button
                    disabled={!preview?.result.stack}
                    onClick={() => exportFile("layers")}
                  >
                    <Layers size={15} /> 16-bit layer map
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      </header>
      <div
        className={"studio-layout " + (!showPalette ? "palette-hidden" : "")}
      >
        <aside className="controls-panel">
          <div className="panel-tabs">
            <button
              className={tab === "adjust" ? "active" : ""}
              onClick={() => setTab("adjust")}
            >
              <SlidersHorizontal size={15} /> Adjust
            </button>
            <button
              className={tab === "filaments" ? "active" : ""}
              onClick={() => setTab("filaments")}
            >
              <Layers size={15} /> Filaments{" "}
              {library && <small>{library.filaments.length}</small>}
            </button>
          </div>
          <div className="controls-scroll">
            {tab === "adjust" ? (
              <>
                <div className="panel-heading">
                  <span>Make every color count.</span>
                  <p>Less noise. More intention.</p>
                </div>
                <div className="field-label">PROCESSING MODE</div>
                <div className="mode-list">
                  {(["standard", "guided", "stack"] as const).map((m, i) => {
                    const Icon = [Aperture, SwatchBook, Layers][i];
                    return (
                      <div
                        key={m}
                        className={
                          "mode-choice " +
                          (options.mode === m ? "selected" : "")
                        }
                      >
                        <div className="mode-choice-row">
                          <button
                            type="button"
                            className="mode-option"
                            aria-pressed={options.mode === m}
                            onClick={() =>
                              update(changeProcessingMode(options, m))
                            }
                          >
                            <span className="mode-icon">
                              <Icon size={17} />
                            </span>
                            <span>
                              <strong>{modeInfo[m][0]}</strong>
                              <small>
                                {m === "standard"
                                  ? "Discover a perceptual palette"
                                  : m === "guided"
                                    ? "Use your owned filament colors"
                                    : "Plan layers and filament swaps"}
                              </small>
                            </span>
                            <span className="radio-dot" aria-hidden="true" />
                          </button>
                          <button
                            type="button"
                            className="mode-help-toggle"
                            aria-label={`About ${modeInfo[m][0]}`}
                            title={`About ${modeInfo[m][0]}`}
                            aria-expanded={modeHelp === m}
                            aria-controls={`mode-help-${m}`}
                            onClick={() =>
                              setModeHelp(modeHelp === m ? null : m)
                            }
                          >
                            <Info size={15} aria-hidden="true" />
                          </button>
                        </div>
                        <div
                          id={`mode-help-${m}`}
                          className="mode-help"
                          hidden={modeHelp !== m}
                        >
                          <p>{modeInfo[m][1]}</p>
                          <p>{modeInfo[m][2]}</p>
                        </div>
                      </div>
                    );
                  })}
                </div>
                <div className="settings-group">
                  <div className="group-heading">
                    Palette{" "}
                    <button
                      className="text-button"
                      onClick={() => {
                        const n = copy(defaults);
                        n.mode = options.mode;
                        n.colors = options.mode === "standard" ? 8 : 4;
                        update(n);
                      }}
                      title="Reset processing settings"
                    >
                      <RotateCcw size={12} /> Reset
                    </button>
                  </div>
                  <Range
                    label={
                      options.mode === "standard"
                        ? "Colors per population"
                        : "Maximum filaments"
                    }
                    value={options.colors}
                    min={1}
                    max={32}
                    onChange={(v) => change("colors", v)}
                  />
                  <Numeric
                    label="Exact budget"
                    value={options.colors}
                    min={1}
                    max={256}
                    onChange={(v) => change("colors", v)}
                  />
                  <p className="field-help">
                    {options.mode === "standard"
                      ? `Up to ${options.colors} colors + ${options.colors} neutrals. Small clusters are removed.`
                      : "The output can contain more colors than physical filaments."}
                  </p>
                  {options.mode === "guided" && (
                    <>
                      <Range
                        label="Filament guidance"
                        value={options.guidanceStrength}
                        min={0}
                        max={1}
                        step={0.05}
                        format={pct}
                        onChange={(v) => change("guidanceStrength", v)}
                      />
                      <div className="range-captions">
                        <span>Keep image hues</span>
                        <span>Favor filaments</span>
                      </div>
                    </>
                  )}
                  {options.mode !== "standard" && (
                    <div className="true-black-option">
                      <label className="check-field">
                        <input
                          type="checkbox"
                          checked={options.trueBlack}
                          onChange={(e) =>
                            change("trueBlack", e.target.checked)
                          }
                        />
                        Use true black
                        <span className="true-black-chip" aria-hidden="true" />
                      </label>
                      <p className="field-help">
                        Use #000000 for black filaments. Keeps your library
                        unchanged.
                      </p>
                    </div>
                  )}
                  <label className="check-field">
                    <input
                      type="checkbox"
                      checked={options.preserveDetails}
                      onChange={(e) =>
                        change("preserveDetails", e.target.checked)
                      }
                    />
                    Preserve details
                  </label>
                  <p className="field-help">
                    Keep outlines and similar-colored shapes distinct while
                    flattening texture. Turn off to use the original reduction.
                  </p>
                  <Range
                    label="Detail smoothing"
                    help={
                      options.preserveDetails
                        ? "Smooth similar neighboring colors before analysis and mapping, while protecting contrasting edges. Zero disables smoothing."
                        : "Pre-blur affects palette discovery only. Original-resolution pixels are preserved until remapping."
                    }
                    value={options.preblurSigma}
                    min={0}
                    max={5}
                    step={0.1}
                    format={(v) => `${v.toFixed(1)} px`}
                    onChange={(v) => change("preblurSigma", v)}
                  />
                  {options.mode !== "standard" && (
                    <Numeric
                      label="Output color limit"
                      value={options.hueforge.maxPerceivedColors}
                      min={1}
                      max={256}
                      onChange={(v) => changeHF("maxPerceivedColors", v)}
                    />
                  )}
                </div>
                {options.mode !== "standard" && (
                  <div
                    className={
                      "library-callout " + (!libraryPath ? "needs-library" : "")
                    }
                  >
                    <SwatchBook size={17} />
                    <div>
                      <strong>
                        {libraryPath
                          ? `${library?.filaments.length ?? "—"} filaments available`
                          : "Choose your filament library"}
                      </strong>
                      <span>
                        {libraryPath
                          ? "Owned colors guide your result."
                          : "Load personal_library.json to begin."}
                      </span>
                      <button
                        className="text-button"
                        onClick={() => setTab("filaments")}
                      >
                        {libraryPath ? "Manage collection" : "Load library"}{" "}
                        <ArrowUpRight size={12} />
                      </button>
                    </div>
                  </div>
                )}
                {options.mode === "stack" && (
                  <Section title="Layer & stack settings" initial>
                    <Numeric
                      label="Layer height"
                      value={options.hueforge.layerHeight}
                      min={0.01}
                      max={1}
                      step={0.01}
                      suffix="mm"
                      onChange={(v) => changeHF("layerHeight", v)}
                    />
                    <Numeric
                      label="Base depth"
                      value={options.hueforge.baseDepth}
                      min={0.01}
                      max={20}
                      step={0.08}
                      suffix="mm"
                      onChange={(v) => changeHF("baseDepth", v)}
                    />
                    <Numeric
                      label="Maximum total depth"
                      value={options.hueforge.maxDepth}
                      min={0.02}
                      max={40}
                      step={0.08}
                      suffix="mm"
                      onChange={(v) => changeHF("maxDepth", v)}
                    />
                    <Numeric
                      label="Search beam width"
                      value={options.hueforge.beamWidth}
                      min={1}
                      max={512}
                      onChange={(v) => changeHF("beamWidth", v)}
                    />
                    <p className="field-help">
                      Depths must be exact multiples of layer height. The
                      maximum includes the base.
                    </p>
                  </Section>
                )}
                <Section title="Advanced color controls">
                  <Numeric
                    label="Neutral chroma threshold"
                    value={options.neutralChroma}
                    min={0}
                    max={200}
                    step={0.5}
                    onChange={(v) => change("neutralChroma", v)}
                  />
                  <Numeric
                    label="Minimum cluster"
                    value={Number(
                      (options.minClusterFraction * 100).toFixed(4),
                    )}
                    min={0}
                    max={99}
                    step={0.1}
                    suffix="%"
                    onChange={(v) => change("minClusterFraction", v / 100)}
                  />
                  <Numeric
                    label="Histogram precision"
                    value={options.histogramBits}
                    min={3}
                    max={7}
                    suffix="bits"
                    onChange={(v) => change("histogramBits", v)}
                  />
                  <Numeric
                    label="Clustering iterations"
                    value={options.iterations}
                    min={1}
                    max={1000}
                    onChange={(v) => change("iterations", v)}
                  />
                  <Numeric
                    label={
                      options.preserveDetails
                        ? "Detail smoothing radius"
                        : "Pre-blur radius"
                    }
                    value={options.preblurSigma}
                    min={0}
                    max={100}
                    step={0.1}
                    suffix="px"
                    onChange={(v) => change("preblurSigma", v)}
                  />
                  <label className="select-field">
                    Analysis resolution
                    <select
                      aria-label="Analysis resolution"
                      value={
                        [250000, 1000000, 6291456, 0].includes(
                          options.analysisMaxPixels,
                        )
                          ? options.analysisMaxPixels
                          : "custom"
                      }
                      onChange={(e) => {
                        if (e.target.value !== "custom")
                          change("analysisMaxPixels", Number(e.target.value));
                      }}
                    >
                      <option value={250000}>250K pixels · fast</option>
                      <option value={1000000}>1 megapixel · balanced</option>
                      <option value={6291456}>6 megapixels · detailed</option>
                      <option value={0}>Every source pixel</option>
                      <option value="custom">Custom pixel limit</option>
                    </select>
                  </label>
                  <Numeric
                    label="Analysis pixel limit"
                    value={options.analysisMaxPixels}
                    min={0}
                    max={100000000}
                    step={1000}
                    onChange={(v) => change("analysisMaxPixels", v)}
                  />
                  <p className="field-help">
                    0 analyzes every pixel. Export dimensions always match the
                    original.
                  </p>
                </Section>
                {options.mode !== "standard" && (
                  <Section title="Optical calibration">
                    {options.mode === "guided" && (
                      <>
                        <Numeric
                          label="Layer height"
                          value={options.hueforge.layerHeight}
                          min={0.01}
                          max={1}
                          step={0.01}
                          suffix="mm"
                          onChange={(v) => changeHF("layerHeight", v)}
                        />
                        <Numeric
                          label="Base depth"
                          value={options.hueforge.baseDepth}
                          min={0.01}
                          max={20}
                          step={0.08}
                          suffix="mm"
                          onChange={(v) => changeHF("baseDepth", v)}
                        />
                        <Numeric
                          label="Maximum total depth"
                          value={options.hueforge.maxDepth}
                          min={0.02}
                          max={40}
                          step={0.08}
                          suffix="mm"
                          onChange={(v) => changeHF("maxDepth", v)}
                        />
                      </>
                    )}
                    <Numeric
                      label="Analysis colors per population"
                      value={options.hueforge.analysisColors}
                      min={1}
                      max={256}
                      onChange={(v) => changeHF("analysisColors", v)}
                    />
                    <Numeric
                      label="TD scale"
                      value={options.hueforge.tdScale}
                      min={0.001}
                      max={100}
                      step={0.01}
                      onChange={(v) => changeHF("tdScale", v)}
                    />
                    <Numeric
                      label="Transmission at one TD"
                      value={options.hueforge.tdTransmission}
                      min={0.001}
                      max={0.999}
                      step={0.01}
                      onChange={(v) => changeHF("tdTransmission", v)}
                    />
                    <Numeric
                      label="Base transmission limit"
                      value={options.hueforge.baseTransmissionLimit}
                      min={0.001}
                      max={1}
                      step={0.01}
                      onChange={(v) => changeHF("baseTransmissionLimit", v)}
                    />
                  </Section>
                )}
                <Section title="Presets" initial>
                  <div className="preset-buttons">
                    <button
                      onClick={() => update(applyColorBudget(options, 4))}
                    >
                      Minimal · 4
                    </button>
                    <button
                      onClick={() => update(applyColorBudget(options, 8))}
                    >
                      Balanced · 8
                    </button>
                    <button
                      onClick={() => update(applyColorBudget(options, 16))}
                    >
                      Detailed · 16
                    </button>
                  </div>
                  {presets.map((p) => (
                    <div className="custom-preset" key={p.name}>
                      <button
                        onClick={() =>
                          update(applySavedPreset(options, p.options))
                        }
                      >
                        {p.name}
                      </button>
                      <IconButton
                        title={`Delete preset ${p.name}`}
                        onClick={() =>
                          invoke<Preset[]>("DeletePreset", p.name)
                            .then(setPresets)
                            .catch(handleError)
                        }
                      >
                        <Trash2 size={13} />
                      </IconButton>
                    </div>
                  ))}
                  <button
                    className="text-button save-preset"
                    onClick={savePreset}
                  >
                    <Plus size={13} /> Save current settings
                  </button>
                </Section>
                <Section title="Recent images">
                  {recent.length ? (
                    recent.map((path) => (
                      <button
                        className="recent-file"
                        key={path}
                        title={path}
                        onClick={() => loadPath(path)}
                      >
                        <FileImage size={13} />
                        {path.split(/[\\/]/).pop()}
                      </button>
                    ))
                  ) : (
                    <p className="field-help">
                      Your recently opened images will appear here.
                    </p>
                  )}
                  <button className="text-button" onClick={demo}>
                    <ImageIcon size={13} /> Load sample artwork
                  </button>
                </Section>
              </>
            ) : (
              <>
                <div className="panel-heading">
                  <span>Your color collection.</span>
                  <p>Make the most of what you own.</p>
                </div>
                <button
                  className="button secondary full"
                  onClick={chooseLibrary}
                >
                  <FolderOpen size={15} />{" "}
                  {libraryPath ? "Change library" : "Load filament library"}
                </button>
                {libraryPath && (
                  <div className="library-path" title={libraryPath}>
                    {libraryPath.split(/[\\/]/).pop()}
                    <button
                      className="text-button"
                      onClick={() => refreshLibrary()}
                      aria-label="Reload filament library"
                    >
                      <RotateCcw size={12} />
                    </button>
                  </div>
                )}
                <p className="field-help">
                  Reads your HueForge library. Your original library is never
                  modified.
                </p>
                <div className="library-filters">
                  <label className="check-field">
                    <input
                      type="checkbox"
                      checked={filter.includeUnowned}
                      disabled={!libraryPath}
                      onChange={(e) =>
                        refreshLibrary({
                          ...filter,
                          includeUnowned: e.target.checked,
                        })
                      }
                    />{" "}
                    Include unowned filaments
                  </label>
                  <label className="check-field">
                    <input
                      type="checkbox"
                      checked={filter.allowSecondary}
                      disabled={!libraryPath}
                      onChange={(e) =>
                        refreshLibrary({
                          ...filter,
                          allowSecondary: e.target.checked,
                        })
                      }
                    />{" "}
                    Use primary color of dual-color filaments
                  </label>
                  <label className="select-field">
                    Material filter
                    <input
                      aria-label="Material filter"
                      placeholder="All materials, or PLA, PLA+…"
                      defaultValue={filter.materialTypes.join(", ")}
                      key={filter.materialTypes.join(",")}
                      onBlur={(e) => {
                        const types = e.target.value
                          .split(",")
                          .map((v) => v.trim())
                          .filter(Boolean);
                        if (
                          JSON.stringify(types) !==
                          JSON.stringify(filter.materialTypes)
                        )
                          refreshLibrary({ ...filter, materialTypes: types });
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") e.currentTarget.blur();
                      }}
                    />
                  </label>
                  <p className="field-help">
                    Use compatible materials together. Separate material names
                    with commas.
                  </p>
                </div>
                {library ? (
                  <>
                    <div className="collection-summary">
                      <strong>{library.filaments.length}</strong>
                      <span>
                        eligible filaments{" "}
                        <small>of {library.total} library entries</small>
                      </span>
                    </div>
                    <div className="search-field">
                      <Search size={14} />
                      <input
                        aria-label="Search filaments"
                        placeholder="Search your collection"
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                      />
                    </div>
                    <div className="filament-list">
                      {visibleFilaments.map((f) => (
                        <div
                          className="filament-card"
                          key={f.sourceIndex}
                          title={filamentDescription(f)}
                        >
                          <span
                            className="filament-color"
                            style={{ background: f.hex }}
                          />
                          <div>
                            <strong>{f.name}</strong>
                            <span>
                              {f.brand} · {f.material || "Unspecified"}
                            </span>
                          </div>
                          <small title={`Transmission distance: ${f.td} mm`}>
                            {f.td}
                            <br />
                            TD mm
                          </small>
                        </div>
                      ))}
                    </div>
                    {!visibleFilaments.length && (
                      <p className="field-help">No matching filaments.</p>
                    )}
                  </>
                ) : (
                  <div className="empty-library">
                    <SwatchBook size={35} />
                    <strong>Bring your palette along.</strong>
                    <p>
                      Load your HueForge personal library to see available
                      colors and guide the image toward your collection.
                    </p>
                  </div>
                )}
              </>
            )}
          </div>
          <div className="controls-footer">
            <div>
              <label className="switch-label">
                <input
                  type="checkbox"
                  checked={auto}
                  onChange={(e) => {
                    setAuto(e.target.checked);
                  }}
                />
                <span className="switch" /> Auto preview
              </label>
              <div className="history-controls">
                <IconButton
                  title="Undo settings (Ctrl+Z)"
                  disabled={historyIndex <= 0}
                  onClick={undo}
                >
                  <Undo2 size={15} />
                </IconButton>
                <IconButton
                  title="Redo settings (Ctrl+Shift+Z)"
                  disabled={historyIndex >= history.length - 1}
                  onClick={redo}
                >
                  <Redo2 size={15} />
                </IconButton>
              </div>
            </div>
            {busy ? (
              <button className="button secondary full" onClick={cancel}>
                <X size={14} /> Cancel processing
              </button>
            ) : (
              <button
                className="button preview-button full"
                disabled={!source || loading}
                onClick={() => setRerun((v) => v + 1)}
              >
                <Sparkles size={15} />{" "}
                {dirty ? "Generate preview" : "Refresh preview"}
              </button>
            )}
          </div>
        </aside>
        <Viewer source={source} preview={preview} busy={busy} dirty={dirty} />
        {showPalette && (
          <aside className="results-panel">
            <div className="results-heading">
              <span>
                <SwatchBook size={15} /> Output palette
              </span>
              <IconButton
                title="Hide palette panel"
                onClick={() => setShowPalette(false)}
              >
                <PanelRightClose size={15} />
              </IconButton>
            </div>
            <div className="results-scroll">
              <div className="palette-total">
                <strong>{preview?.result.palette.length ?? "—"}</strong>
                <div>
                  perceptual colors
                  <span>
                    {dirty
                      ? "Update preview to refresh"
                      : "Ready for your next creation"}
                  </span>
                </div>
              </div>
              {preview ? (
                <>
                  <div className="palette-strip">
                    {preview.result.palette.map((c) => (
                      <span
                        key={c.hex}
                        title={`${c.hex} · ${pct(c.pixelFraction)}`}
                        style={{
                          background: c.hex,
                          flex: Math.max(0.01, c.pixelFraction),
                        }}
                      />
                    ))}
                  </div>
                  <div className="palette-grid">
                    {preview.result.palette.map((c) => (
                      <button
                        className="palette-swatch"
                        key={c.hex}
                        title={`Copy ${c.hex} · ${pct(c.pixelFraction)} of visible pixels`}
                        onClick={async () => {
                          try {
                            await navigator.clipboard.writeText(c.hex);
                            notify(`Copied ${c.hex}`);
                          } catch {
                            notify(c.hex);
                          }
                        }}
                      >
                        <span style={{ background: c.hex }} />
                        <div>
                          <strong>{c.hex}</strong>
                          <small>
                            {c.pixelFraction < 0.005
                              ? "<1%"
                              : pct(c.pixelFraction)}
                          </small>
                        </div>
                      </button>
                    ))}
                  </div>
                  <div className="result-section">
                    <div className="field-label">IMAGE INSIGHTS</div>
                    <div className="stat-row">
                      <span>Dimensions</span>
                      <strong>{preview.result.sourceSize.join(" × ")}</strong>
                    </div>
                    <div className="stat-row">
                      <span>Analysis</span>
                      <strong>
                        {(
                          (preview.result.analysisSize[0] *
                            preview.result.analysisSize[1]) /
                          1000000
                        ).toFixed(2)}{" "}
                        MP
                      </strong>
                    </div>
                    <div className="stat-row">
                      <span title="Alpha-weighted mean CIE76 distance from the original">
                        Mean color distance <Info size={11} />
                      </span>
                      <strong>
                        {preview.result.quality.meanDeltaE76.toFixed(2)} ΔE
                      </strong>
                    </div>
                    <div className="stat-row">
                      <span>Processing time</span>
                      <strong>{preview.seconds.toFixed(2)} s</strong>
                    </div>
                    <div className="stat-row">
                      <span>Output</span>
                      <strong>Lossless PNG</strong>
                    </div>
                  </div>
                  {selected.length > 0 && (
                    <div className="result-section">
                      <div className="field-label">
                        {preview.result.stack
                          ? "FILAMENT ORDER · BOTTOM TO TOP"
                          : "SELECTED FILAMENTS"}
                      </div>
                      {selected.map((f, i) => (
                        <div
                          className="selected-filament"
                          key={f.sourceIndex}
                          title={filamentDescription(f)}
                        >
                          <span
                            className="filament-color"
                            style={{ background: f.hex }}
                          />
                          <div>
                            <strong>{f.name}</strong>
                            <span>
                              {f.brand} · {f.material || "Unspecified material"}
                            </span>
                            <span>
                              <abbr title="Transmission distance">TD</abbr>{" "}
                              {f.td} mm
                              {preview.result.stack &&
                                ` · ${preview.result.stack.runs[i].layers} ${preview.result.stack.runs[i].layers === 1 ? "layer" : "layers"}`}
                            </span>
                          </div>
                          {preview.result.stack && <small>{i + 1}</small>}
                        </div>
                      ))}
                    </div>
                  )}
                  {preview.result.stack && (
                    <div className="stack-summary">
                      <Layers size={17} />
                      <div>
                        <strong>
                          {preview.result.stack.plannedDepth.toFixed(2)} mm
                          total depth
                        </strong>
                        <span>
                          {preview.result.stack.runs.length} filaments ·{" "}
                          {preview.result.stack.layerColors.length} layers
                        </span>
                      </div>
                    </div>
                  )}
                  {preview.options.mode !== "standard" && (
                    <div className="optics-note">
                      <Info size={14} />
                      <p>
                        {preview.options.mode === "guided"
                          ? "Filament colors and pairwise hues guide this image. Open the PNG in HueForge to create the final layer plan."
                          : "This stack uses an independent optical approximation. Verify the colors and swap heights in your printing workflow."}
                      </p>
                    </div>
                  )}
                </>
              ) : (
                <div className="palette-empty">
                  <Aperture size={30} />
                  <p>Your image’s palette will appear here.</p>
                </div>
              )}
            </div>
            <div className="results-footer">
              <span className="local-dot" /> Processed locally on your computer
            </div>
          </aside>
        )}
        {!showPalette && (
          <button
            className="show-palette icon-button"
            title="Show palette panel"
            aria-label="Show palette panel"
            onClick={() => setShowPalette(true)}
          >
            <PanelRightOpen size={18} />
          </button>
        )}
      </div>
      <footer className="status-bar">
        <div>
          {busy || loading ? (
            <LoaderCircle size={12} className="spin" />
          ) : (
            <span className="status-dot" />
          )}
          <span>
            {loading
              ? "Opening image…"
              : busy
                ? progress.stage
                : dirty
                  ? "Preview needs updating"
                  : "All changes rendered"}
          </span>
          {busy && (
            <div className="progress-track">
              <span
                style={{ width: `${Math.max(3, progress.fraction * 100)}%` }}
              />
            </div>
          )}
        </div>
        <div>
          <span>ColorNinja 1.0</span>
          <i />{" "}
          <span>
            {options.mode === "standard"
              ? "Perceptual reduction"
              : options.mode === "guided"
                ? "Filament guidance"
                : "Stack planning"}
          </span>
        </div>
      </footer>
      {toast && (
        <div
          role={toast.error ? "alert" : "status"}
          className={"toast " + (toast.error ? "error" : "")}
        >
          <span>{toast.error ? <Info size={18} /> : <Check size={18} />}</span>
          <p>{toast.message}</p>
          <IconButton
            title="Dismiss notification"
            onClick={() => setToast(null)}
          >
            <X size={15} />
          </IconButton>
        </div>
      )}
      {dialog && (
        <div
          className="modal-backdrop"
          onPointerDown={(e) => {
            if (e.target === e.currentTarget) setDialog(null);
          }}
        >
          <form
            className="modal"
            onSubmit={async (e) => {
              e.preventDefault();
              const d = dialog;
              try {
                await d.submit(d.value);
                setDialog(null);
              } catch (err) {
                handleError(err);
              }
            }}
          >
            <div>
              <h2>{dialog.title}</h2>
              <IconButton title="Close dialog" onClick={() => setDialog(null)}>
                <X size={18} />
              </IconButton>
            </div>
            <label>
              {dialog.label}
              <input
                autoFocus
                required
                value={dialog.value}
                onChange={(e) =>
                  setDialog({ ...dialog, value: e.target.value })
                }
              />
            </label>
            <div className="modal-actions">
              <button
                type="button"
                className="button secondary"
                onClick={() => setDialog(null)}
              >
                Cancel
              </button>
              <button className="button primary" type="submit">
                Continue <ChevronRight size={15} />
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
