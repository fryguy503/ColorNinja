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
  Pencil,
  SlidersHorizontal,
  Sparkles,
  SwatchBook,
  Undo2,
  X,
  Info,
  Search,
  Trash2,
  Image as ImageIcon,
} from "lucide-react";
import { invoke, on, desktop } from "./bridge";
import { Updates } from "./Updates";
import { StudioDialog } from "./StudioDialog";
import { StackInspector } from "./StackInspector";
import {
  applyColorBudget,
  applyAutoDepth,
  applyColorPriority,
  prioritizesColors,
  displayedColorBudget,
  applyDisplayedColorBudget,
  applySavedPreset,
  normalizeFilter,
  changeProcessingMode,
  applySmoothing,
  smoothingPreset,
  smoothingPresets,
  type SmoothingPreset,
} from "./settings";
import {
  defaults,
  emptyFilter,
  defaultPreferences,
  type Preferences,
  type Options,
  type Source,
  type Snapshot,
  type Preview,
  type Library,
  type Filter,
  type Preset,
  type SettingsProfile,
  type Request,
  type HueForgeOptions,
  type Filament,
} from "./types";
import "./style.css";
import "./studio.css";

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
      <div className="range-captions">
        <span>{format ? format(min) : min}</span>
        <span>{format ? format(max) : max}</span>
      </div>
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
  onInspect,
  onTogglePalette,
  showPalette,
}: {
  onInspect: () => void;
  onTogglePalette: () => void;
  showPalette: boolean;
  source: Source | null;
  preview: Preview | null;
  busy: boolean;
  dirty: boolean;
}) {
  const [view, setView] = useState<
    "original" | "reduced" | "split" | "side-by-side"
  >("reduced");
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
          Math.max(1, size.w - (sideBySide ? 32 : 48)) / source.width,
          Math.max(1, size.h - (sideBySide ? 32 : 64)) / source.height,
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
          {(["original", "reduced", "split", "side-by-side"] as const).map(
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
          <IconButton title="Image information" onClick={onInspect}>
            <Info size={15} />
          </IconButton>
          <IconButton
            title={showPalette ? "Hide output panel" : "Show output panel"}
            onClick={onTogglePalette}
          >
            <SwatchBook size={15} />
          </IconButton>
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
        <span>{source?.metadata.colorProfile ?? "sRGB"} · Alpha preserved</span>
      </div>
    </section>
  );
}

function App() {
  const [preferences, setPreferences] = useState<Preferences>(
    copy(defaultPreferences),
  );
  const [preferencesReady, setPreferencesReady] = useState(false);
  const [preferencesSaving, setPreferencesSaving] = useState(false);
  const advanced = preferences.advanced;
  const savePreferences = async (next: Preferences) => {
    setPreferencesSaving(true);
    try {
      setPreferences(await invoke<Preferences>("SavePreferences", next));
    } finally {
      setPreferencesSaving(false);
    }
  };
  const [source, setSource] = useState<Source | null>(null),
    [options, setOptions] = useState<Options>(copy(defaults)),
    [preview, setPreview] = useState<Preview | null>(null),
    [library, setLibrary] = useState<Library | null>(null),
    [libraryPath, setLibraryPath] = useState(""),
    [filter, setFilter] = useState<Filter>(copy(emptyFilter)),
    [presets, setPresets] = useState<Preset[]>([]),
    [recent, setRecent] = useState<string[]>([]);
  const [tab, setTab] = useState<"adjust" | "filaments" | "layers">("adjust"),
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
  const [showFile, setShowFile] = useState(false);
  const [showPresets, setShowPresets] = useState(false);
  const [exportKind, setExportKind] = useState("png");
  const [resultTab, setResultTab] = useState<
    "palette" | "filaments" | "insights"
  >("palette");
  useEffect(() => {
    if (options.mode === "standard" && tab === "layers") setTab("adjust");
    if (
      options.mode !== "stack" &&
      (exportKind === "hfp" || exportKind === "layers")
    )
      setExportKind("png");
  }, [options.mode, tab]);
  const [keepWorkflow, setKeepWorkflow] = useState(true);
  const [modeHelp, setModeHelp] = useState<Options["mode"] | null>(null);
  const [dialogError, setDialogError] = useState("");
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
    setPreferences({ ...defaultPreferences, ...s.settings.preferences });
    setPreferencesReady(true);
    setSource(s.source);
    setOptions(copy(s.settings.options));
    setLibraryPath(s.settings.libraryPath ?? "");
    setFilter(normalizeFilter({ ...copy(emptyFilter), ...s.settings.filter }));
    setLibrary(s.library);
    setPresets(s.settings.presets ?? []);
    setRecent(s.settings.recent ?? []);
    setPreview(s.preview ?? null);
    renderedKey.current = s.preview
      ? s.source.revision +
        ":" +
        JSON.stringify([
          s.settings.options,
          s.settings.libraryPath ?? "",
          normalizeFilter({ ...copy(emptyFilter), ...s.settings.filter }),
        ])
      : "";
    if (s.preview) {
      seq.current = Math.max(seq.current, s.preview.id);
      setProgress({ stage: "Saved project restored", fraction: 1 });
    }
    setHistory([copy(s.settings.options)]);
    setHistoryIndex(0);
    setDirty(!s.preview);
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
    if (signature === renderedKey.current && !manualRequested) return;
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
  ) => {
    setShowFile(false);
    setShowPresets(false);
    setShowExport(false);
    setDialogError("");
    setDialog({ title, label, value, submit });
  };
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
      const f = {
        ...copy(emptyFilter),
        avoidSilkMetallic: filter.avoidSilkMetallic,
      };
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
        if (output)
          notify(
            preferences.exportProfile
              ? `Saved ${output} with ${kind === "project" ? "its settings profile" : "a ColorNinja project and settings profile"}.`
              : `Saved ${output}`,
          );
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
              : kind === "hfp"
                ? "HueForge project"
                : kind === "project"
                  ? "ColorNinja project"
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
        setShowFile(false);
        setShowPresets(false);
        setModeHelp(null);
      }
      if ((e.target as HTMLElement).closest("dialog, [role=dialog], .modal"))
        return;
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
    textDialog(
      "Save a preset",
      "Name (an existing name replaces that preset)",
      "",
      async (name) => {
        setPresets(await invoke<Preset[]>("SavePreset", name, options));
        notify(`Preset saved: ${name}`);
      },
    );
  const renamePreset = (preset: Preset) =>
    textDialog("Rename preset", "New name", preset.name, async (name) => {
      setPresets(await invoke<Preset[]>("RenamePreset", preset.name, name));
      notify(`Preset renamed: ${name}`);
    });
  const saveProfile = async () => {
    const run = async (path?: string) => {
      const output = await invoke<string>(
        "SaveProfile",
        "Custom settings",
        request(),
        ...(path ? [path] : []),
      );
      if (output) notify(`Settings profile saved: ${output}`);
    };
    try {
      if (!desktop) {
        textDialog("Save settings profile", "Absolute profile path", "", run);
        return;
      }
      await run();
    } catch (e) {
      handleError(e);
    }
  };
  const openProfile = async () => {
    const run = async (path?: string) => {
      const profile = await invoke<SettingsProfile | null>(
        "OpenProfile",
        ...(path ? [path] : []),
      );
      if (!profile) return;
      const matches =
        !!profile.librarySHA256 && profile.librarySHA256 === library?.sha256;
      const next = normalizeFilter({
        ...copy(emptyFilter),
        ...profile.filter,
        excludedIds: matches ? (profile.filter.excludedIds ?? []) : [],
      });
      if (libraryPath)
        setLibrary(await invoke<Library>("SetLibrary", libraryPath, next));
      setFilter(next);
      update(copy(profile.options));
      notify(
        matches || !profile.filter.excludedIds?.length
          ? "Settings profile loaded. Save it as a preset to reuse it."
          : "Settings loaded. Individual filament exclusions were cleared because this library differs.",
      );
    };
    try {
      if (!desktop) {
        textDialog("Open settings profile", "Absolute profile path", "", run);
        return;
      }
      await run();
    } catch (e) {
      handleError(e);
    }
  };
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
  useEffect(() => {
    if (!selected.length && resultTab === "filaments") setResultTab("palette");
  }, [selected.length, resultTab]);
  const visibleFilaments =
    library?.filaments.filter((f) =>
      `${f.name} ${f.brand} ${f.material}`
        .toLowerCase()
        .includes(search.toLowerCase()),
    ) ?? [];
  const workflowControl = (
    <>
      <div className="simple-mode">
        <label className="select-field">
          Workflow
          <select
            aria-label="Processing mode"
            value={options.mode}
            onChange={(e) => {
              setModeHelp(null);
              update(
                changeProcessingMode(
                  options,
                  e.target.value as Options["mode"],
                ),
              );
            }}
          >
            <option value="standard">Simple reducer</option>
            <option value="guided">Filament Guide</option>
            <option value="stack">Global stack · experimental</option>
          </select>
        </label>
        <button
          className="mode-help-toggle"
          aria-label={`About ${modeInfo[options.mode][0]}`}
          aria-expanded={modeHelp === options.mode}
          aria-controls="workflow-help"
          onClick={() =>
            setModeHelp(modeHelp === options.mode ? null : options.mode)
          }
        >
          <Info size={15} />
        </button>
      </div>
      <div
        id="workflow-help"
        className="mode-help"
        hidden={modeHelp !== options.mode}
      >
        <p>{modeInfo[options.mode][1]}</p>
        <p>{modeInfo[options.mode][2]}</p>
      </div>
    </>
  );

  const presetsContent = (
    <>
      <p className="field-help">
        Save named settings for other images. Profiles let you share or import
        settings.
      </p>
      <div className="preset-buttons">
        <button onClick={() => update(applyColorBudget(options, 4))}>
          Minimal · 4
        </button>
        <button onClick={() => update(applyColorBudget(options, 8))}>
          Balanced · 8
        </button>
        <button onClick={() => update(applyColorBudget(options, 16))}>
          Detailed · 16
        </button>
      </div>
      <label className="check-field">
        <input
          type="checkbox"
          checked={keepWorkflow}
          onChange={(e) => setKeepWorkflow(e.target.checked)}
        />
        Keep current workflow when applying a preset
      </label>
      {presets.map((p) => (
        <div className="custom-preset" key={p.name}>
          <button
            onClick={() =>
              update(applySavedPreset(options, p.options, keepWorkflow))
            }
          >
            {p.name}
          </button>
          <IconButton
            title={`Rename preset ${p.name}`}
            onClick={() => renamePreset(p)}
          >
            <Pencil size={13} />
          </IconButton>
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
      <button className="text-button save-preset" onClick={savePreset}>
        <Plus size={13} /> Create preset from current settings
      </button>
      <div className="profile-actions">
        <button className="text-button" onClick={saveProfile}>
          <Save size={13} /> Save profile
        </button>
        <button className="text-button" onClick={openProfile}>
          <FolderOpen size={13} /> Load profile
        </button>
      </div>
    </>
  );

  const recentContent = (
    <>
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
    </>
  );

  const tuningContent = (
    <>
      <Range
        label={
          options.mode === "standard"
            ? options.totalColors || prioritizesColors(options)
              ? "Maximum colors"
              : "Colors per population"
            : "Maximum filaments"
        }
        value={displayedColorBudget(options)}
        min={1}
        max={Math.max(32, displayedColorBudget(options))}
        step={displayedColorBudget(options) > 256 ? 2 : 1}
        onChange={(v) => update(applyDisplayedColorBudget(options, v))}
      />

      <label className="select-field">
        Color priority
        <select
          aria-label="Color priority"
          value={options.colorPriority || "balanced"}
          onChange={(e) =>
            update(
              applyColorPriority(
                options,
                e.target.value as Options["colorPriority"],
              ),
            )
          }
        >
          <option value="balanced">Overall balance</option>
          <option value="distinctive">Distinctive colors</option>
          <option value="vivid">Vivid colors</option>
        </select>
      </label>

      <div className="smoothing-control">
        <div className="group-heading">
          Smoothing <small>{smoothingPreset(options)}</small>
        </div>
        <div className="smoothing-presets">
          {(Object.keys(smoothingPresets) as SmoothingPreset[]).map((name) => (
            <button
              key={name}
              aria-pressed={smoothingPreset(options) === name}
              onClick={() => update(applySmoothing(options, name))}
            >
              {name}
            </button>
          ))}
        </div>
      </div>

      <label className="check-field">
        <input
          type="checkbox"
          checked={options.preserveDetails}
          onChange={(e) => change("preserveDetails", e.target.checked)}
        />
        Preserve details
      </label>

      <Section title="Tuning help">
        <p className="field-help">
          {options.mode === "standard"
            ? prioritizesColors(options)
              ? `Up to ${displayedColorBudget(options)} colors shared across color families, including essential light and dark tones.`
              : options.totalColors
                ? `At most ${options.colors} colors total, including neutrals.`
                : `Separate budgets: up to ${options.colors * 2} colors. Change this in Advanced.`
            : "The output can contain more colors than physical filaments."}
        </p>
        <p className="field-help">
          {options.colorPriority === "vivid"
            ? "Favor vivid accents more strongly, with fewer similar shades. Protects essential neutrals without adding saturation."
            : options.colorPriority === "distinctive"
              ? "Give distinctive hues and smaller accents more room while keeping the tones that define shapes."
              : "Balance colors by how much of the image they cover. Choose Distinctive colors to give accents more room."}
        </p>
        <p className="field-help">
          Keep small marks and similar-colored shapes distinct. Smoothing works
          with either setting.
        </p>
        <p className="field-help">
          Balanced protects outlines. Strong smooths more texture. Off keeps
          original pixel detail.
        </p>
      </Section>
    </>
  );

  const advancedTuningContent = (
    <>
      {advanced && (
        <Numeric
          label="Exact budget"
          value={displayedColorBudget(options)}
          min={1}
          max={Math.max(256, displayedColorBudget(options))}
          step={displayedColorBudget(options) > 256 ? 2 : 1}
          onChange={(v) => update(applyDisplayedColorBudget(options, v))}
        />
      )}
      {advanced && options.mode === "guided" && (
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
      {advanced && options.mode !== "standard" && (
        <div className="true-black-option">
          <label className="check-field">
            <input
              type="checkbox"
              checked={options.trueBlack}
              onChange={(e) => change("trueBlack", e.target.checked)}
            />
            Use true black
            <span className="true-black-chip" aria-hidden="true" />
          </label>
        </div>
      )}
      {advanced && (
        <Range
          label="Detail smoothing"
          help={
            options.legacyColorPipeline
              ? "Legacy pre-blur affects palette discovery only. Choose a smoothing preset or change the radius to smooth the output too."
              : "Smooth similar neighboring colors before analysis and mapping, while protecting contrasting edges. Zero disables smoothing."
          }
          value={options.preblurSigma}
          min={0}
          max={Math.max(5, options.preblurSigma)}
          step={0.1}
          format={(v) => `${v.toFixed(1)} px`}
          onChange={(v) =>
            update({
              ...options,
              preblurSigma: v,
              legacyColorPipeline: false,
            })
          }
        />
      )}
      {advanced && options.mode !== "standard" && (
        <Numeric
          label="Output color limit"
          value={options.hueforge.maxPerceivedColors}
          min={1}
          max={256}
          onChange={(v) => changeHF("maxPerceivedColors", v)}
        />
      )}
      {options.mode === "standard" && !prioritizesColors(options) && (
        <>
          <label className="check-field">
            <input
              type="checkbox"
              checked={!options.totalColors}
              onChange={(e) => change("totalColors", !e.target.checked)}
            />
            Separate color and neutral budgets
          </label>
        </>
      )}
      <Numeric
        label="Smoothing color tolerance"
        value={options.smoothingColorSigma || 5}
        min={1}
        max={25}
        step={0.5}
        onChange={(v) =>
          update({
            ...options,
            smoothingColorSigma: v,
            legacyColorPipeline: false,
          })
        }
      />

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
        value={Number((options.minClusterFraction * 100).toFixed(4))}
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
          options.legacyColorPipeline
            ? "Legacy pre-blur radius"
            : "Detail smoothing radius"
        }
        value={options.preblurSigma}
        min={0}
        max={100}
        step={0.1}
        suffix="px"
        onChange={(v) =>
          update({
            ...options,
            preblurSigma: v,
            legacyColorPipeline: false,
          })
        }
      />
      <label className="select-field">
        Analysis resolution
        <select
          aria-label="Analysis resolution"
          value={
            [250000, 1000000, 6291456, 0].includes(options.analysisMaxPixels)
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

      <Section title="About advanced tuning">
        <p className="field-help">
          Use #000000 for black filaments. Keeps your library unchanged.
        </p>
        <p className="field-help">
          Allows up to twice the selected count. Existing projects keep this
          setting.
        </p>
        <p className="field-help">
          Higher values flatten stronger texture and may soften low-contrast
          details. Works with either Preserve details setting.
        </p>
        <p className="field-help">
          0 analyzes every pixel. Export dimensions always match the original.
        </p>
      </Section>
    </>
  );

  const layersContent = (
    <>
      {options.mode === "stack" && (
        <>
          <label className="check-field">
            <input
              type="checkbox"
              checked={(options.hueforge.maxRuns || 0) > 0}
              onChange={(e) =>
                changeHF(
                  "maxRuns",
                  e.target.checked ? Math.min(64, options.colors + 2) : 0,
                )
              }
            />
            Allow filament returns
          </label>

          {(options.hueforge.maxRuns || 0) > 0 && (
            <Numeric
              label="Maximum filament runs"
              value={options.hueforge.maxRuns}
              min={1}
              max={64}
              onChange={(v) => changeHF("maxRuns", v)}
            />
          )}
        </>
      )}
      <label className="select-field">
        Front Lit model
        <select
          value={options.hueforge.opticalModel}
          onChange={(e) =>
            update({
              ...options,
              hueforge: {
                ...applyAutoDepth(
                  options,
                  e.target.value === "legacy-exponential"
                    ? false
                    : (options.hueforge.autoDepth ?? false),
                ).hueforge,
                opticalModel: e.target.value as HueForgeOptions["opticalModel"],
              },
            })
          }
        >
          <option value="hueforge-0.9.4.3-frontlit-v1">
            HueForge Front Lit
          </option>
          <option value="legacy-exponential">Legacy approximation</option>
        </select>
      </label>
      {options.hueforge.opticalModel === "hueforge-0.9.4.3-frontlit-v1" && (
        <label className="select-field">
          Lighting
          <select
            value={options.hueforge.lightPreset || "neutral-white"}
            onChange={(e) =>
              changeHF(
                "lightPreset",
                e.target.value as HueForgeOptions["lightPreset"],
              )
            }
          >
            <option value="hueforge-default">
              HueForge default · setting 1
            </option>
            <option value="neutral-white">Neutral white · setting 2</option>
            <option value="warm-white">Warm white · setting 0</option>
          </select>
        </label>
      )}

      <Numeric
        label="First layer height"
        value={
          options.hueforge.firstLayerHeight || options.hueforge.layerHeight
        }
        min={0.01}
        max={1}
        step={0.01}
        suffix="mm"
        onChange={(v) => changeHF("firstLayerHeight", v)}
      />
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
      {options.mode === "stack" && (
        <>
          <label className="check-field">
            <input
              type="checkbox"
              checked={options.hueforge.autoDepth ?? false}
              onChange={(e) =>
                update(applyAutoDepth(options, e.target.checked))
              }
            />
            Choose depth automatically
          </label>
        </>
      )}
      <Numeric
        label={
          options.mode === "stack" && options.hueforge.autoDepth
            ? "Hard maximum depth"
            : "Maximum total depth"
        }
        value={options.hueforge.maxDepth}
        min={0.02}
        max={40}
        step={options.hueforge.autoDepth ? 0.01 : 0.08}
        suffix="mm"
        onChange={(v) => changeHF("maxDepth", v)}
      />

      <Section title="Layer planning help">
        <p className="field-help">
          Reuse a filament after another color, such as black → yellow → black.
          The filament budget counts unique spools.
        </p>
        <p className="field-help">
          Match the lighting, first layer, regular layers, and filament TD
          values in HueForge. Printed results still depend on the filament
          measurements.
        </p>
        <p className="field-help">
          Compare printable depths and prefer the thinnest plan within 1% of the
          best color score found. The ceiling includes the base.
        </p>
        <p className="field-help">
          Total depth is the first layer plus whole regular layers.
          {options.hueforge.autoDepth
            ? " A ceiling between layers rounds down. Try 4.0 mm to allow a deeper search."
            : " The maximum includes the base."}
        </p>
      </Section>
    </>
  );

  const calibrationContent = (
    <>
      {options.mode === "stack" && (
        <Numeric
          label="Search beam width"
          value={options.hueforge.beamWidth}
          min={1}
          max={512}
          onChange={(v) => changeHF("beamWidth", v)}
        />
      )}
      <Numeric
        label="Analysis colors per population"
        value={options.hueforge.analysisColors}
        min={1}
        max={256}
        onChange={(v) => changeHF("analysisColors", v)}
      />
      {options.hueforge.opticalModel !== "hueforge-0.9.4.3-frontlit-v1" && (
        <>
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
        </>
      )}
    </>
  );

  const hfpContent = (
    <>
      <label className="select-field">
        Mesh mode
        <select
          value={options.hueforge.meshMode || "color-match"}
          onChange={(e) =>
            changeHF("meshMode", e.target.value as HueForgeOptions["meshMode"])
          }
        >
          <option value="color-match">Color Match</option>
          <option value="combo">Combo</option>
          <option value="color-aware">Color Aware</option>
          <option value="color-pop">Color Pop</option>
        </select>
      </label>
      {!options.hueforge.meshMode ||
      options.hueforge.meshMode === "color-match" ? (
        <>
          <label className="select-field">
            Mesh core
            <select
              value={options.hueforge.meshCore || "planned-colors"}
              onChange={(e) =>
                changeHF(
                  "meshCore",
                  e.target.value as HueForgeOptions["meshCore"],
                )
              }
            >
              <option value="planned-colors">Match planned layers</option>
              <option value="filament-blends">Use filament blends</option>
            </select>
          </label>
          <p className="field-help">
            The Color Core uses the optimized print stack. Match planned layers
            builds a separate Mesh Core from the output colors; filament blends
            copies the print stack into both cores.
          </p>
        </>
      ) : (
        <p className="field-help">
          HueForge will rebuild heights in this mode. Its mesh and colors can
          differ from this preview. Use Color Match to retain the planned
          color-to-layer assignments.
        </p>
      )}
      <Numeric
        label="Export width"
        value={options.hueforge.exportWidthMm || 200}
        min={1}
        max={2000}
        step={1}
        suffix="mm"
        onChange={(v) => changeHF("exportWidthMm", v)}
      />
      <Numeric
        label="Mesh detail"
        value={options.hueforge.meshDetailMm || 0.2}
        min={0.01}
        max={10}
        step={0.01}
        suffix="mm"
        onChange={(v) => changeHF("meshDetailMm", v)}
      />
      <p className="field-help">
        HFP embeds the simplified image and keeps its aspect ratio. Mesh detail
        controls HueForge's sampling resolution. Partially transparent pixels
        become solid mesh.
      </p>
    </>
  );

  const libraryContent = (
    <>
      <button className="button secondary full" onClick={chooseLibrary}>
        <FolderOpen size={15} />{" "}
        {libraryPath ? "Change library" : "Load filament library"}
      </button>
      {libraryPath && (
        <div className="library-path" title={libraryPath}>
          {libraryPath === "embedded:project-filaments"
            ? "Project filaments (embedded)"
            : libraryPath.split(/[\\/]/).pop()}
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
        Reads your HueForge library. Your original library is never modified.
      </p>
      <Section title="Library filters">
        <label className="check-field">
          <input
            type="checkbox"
            checked={filter.avoidSilkMetallic}
            disabled={!libraryPath}
            onChange={(e) =>
              refreshLibrary({
                ...filter,
                avoidSilkMetallic: e.target.checked,
              })
            }
          />{" "}
          Avoid silk &amp; metallic finishes
        </label>
        <p className="field-help">
          Excludes silk, metallic, pearl, Elixir, and Starlight from Filament
          Guide and Global Stack. Checks material, name, and tags.
        </p>
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
                JSON.stringify(types) !== JSON.stringify(filter.materialTypes)
              )
                refreshLibrary({ ...filter, materialTypes: types });
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
            }}
          />
        </label>
        <p className="field-help">
          Use compatible materials together. Separate material names with
          commas.
        </p>
      </Section>
      {library ? (
        <>
          <div className="collection-summary">
            <strong>{library.filaments.length}</strong>
            <span>
              eligible filaments{" "}
              <small>of {library.total} library entries</small>
            </span>
          </div>
          {library.skippedFinish > 0 && (
            <p className="field-help">
              {library.skippedFinish} excluded for silk or metallic finish.
            </p>
          )}
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
            Load your HueForge personal library to see available colors and
            guide the image toward your collection.
          </p>
        </div>
      )}
    </>
  );

  const advancedControl = (
    <>
      <div className="advanced-heading">
        <label className="switch-label">
          <input
            type="checkbox"
            checked={advanced}
            disabled={preferencesSaving || !preferencesReady}
            onChange={(e) =>
              savePreferences({
                ...preferences,
                advanced: e.target.checked,
              }).catch(handleError)
            }
          />
          <span className="switch" />
          Advanced
        </label>
      </div>
    </>
  );

  const historyControls = (
    <>
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
    </>
  );

  const autoControl = (
    <>
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
    </>
  );

  const previewAction = (
    <>
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
    </>
  );

  const exportBlocked =
    !preview || dirty || busy || loading || exporting || preferencesSaving;
  const exportNeedsStack = exportKind === "hfp" || exportKind === "layers";
  const matchingPreset = presets.find(
    (p) => JSON.stringify(p.options) === JSON.stringify(options),
  );
  return (
    <div className="app-shell redesigned-studio">
      <header className="app-header">
        <div className="brand">
          <div className="brand-mark">
            <Aperture size={23} strokeWidth={1.6} />
          </div>
          <div>ColorNinja</div>
        </div>
        <button
          className="text-button file-trigger"
          onClick={() => setShowFile(true)}
          disabled={loading}
        >
          File <ChevronDown size={14} />
        </button>
        <div className="document-title">
          <FileImage size={15} />
          <span>{source?.name ?? "Untitled"}</span>
          {source?.demo && <small>Sample</small>}
          {dirty && source && <i title="Settings differ from the preview" />}
        </div>
        <div className="header-actions">
          <IconButton
            title="Save project (Ctrl+S)"
            onClick={saveProject}
            disabled={!source || loading || exporting}
          >
            <Save size={17} />
          </IconButton>
          <button
            className="button primary"
            onClick={() => setShowExport(true)}
            disabled={exporting || loading}
          >
            {exporting ? (
              <LoaderCircle className="spin" size={16} />
            ) : (
              <ArrowDownToLine size={16} />
            )}{" "}
            Export
          </button>
        </div>
      </header>
      <div className="studio-layout">
        <main className="image-workspace">
          <Viewer
            source={source}
            preview={preview}
            busy={busy}
            dirty={dirty}
            showPalette={showPalette}
            onTogglePalette={() => setShowPalette(!showPalette)}
            onInspect={() => {
              setShowPalette(true);
              setResultTab("insights");
            }}
          />
          {showPalette && (
            <section className="results-panel" aria-label="Output panel">
              <div className="results-heading">
                <div
                  className="result-tabs"
                  role="group"
                  aria-label="Output details"
                >
                  <button
                    aria-pressed={resultTab === "palette"}
                    onClick={() => setResultTab("palette")}
                  >
                    Colors <small>{preview?.result.uniqueColors ?? "—"}</small>
                  </button>
                  {selected.length > 0 && (
                    <button
                      aria-pressed={resultTab === "filaments"}
                      onClick={() => setResultTab("filaments")}
                    >
                      Filaments
                    </button>
                  )}
                  <button
                    aria-pressed={resultTab === "insights"}
                    onClick={() => setResultTab("insights")}
                  >
                    Image info
                  </button>
                </div>
                <div className="result-actions">
                  {dirty && preview && (
                    <span className="stale-label">Previous result</span>
                  )}
                  {preview?.result.stack && (
                    <StackInspector
                      key={preview.result.rgbaSHA256 + ":" + preview.id}
                      result={preview.result}
                      stale={dirty}
                    />
                  )}
                  <IconButton
                    title="Hide palette panel"
                    onClick={() => setShowPalette(false)}
                  >
                    <ChevronDown size={15} />
                  </IconButton>
                </div>
              </div>
              <div className="results-scroll">
                {preview ? (
                  <>
                    <div
                      hidden={resultTab !== "palette"}
                      className="palette-content"
                    >
                      <div className="palette-strip">
                        {preview.result.palette
                          .filter((c) => c.pixelFraction > 0)
                          .map((c) => (
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
                        {preview.result.palette
                          .filter((c) => c.pixelFraction > 0)
                          .map((c) => (
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
                    </div>
                    <div
                      hidden={resultTab !== "insights"}
                      className="insights-content"
                    >
                      <div className="result-section">
                        <div className="field-label">IMAGE INSIGHTS</div>
                        <div className="stat-row">
                          <span>Dimensions</span>
                          <strong>
                            {preview.result.sourceSize.join(" × ")}
                          </strong>
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
                    </div>
                    <div
                      hidden={resultTab !== "filaments"}
                      className="filament-results"
                    >
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
                                  {f.brand} ·{" "}
                                  {f.material || "Unspecified material"}
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
                        <>
                          <div className="stack-summary">
                            <Layers size={17} />
                            <div>
                              <strong>
                                {preview.result.stack.plannedDepth.toFixed(2)}{" "}
                                mm total depth
                              </strong>
                              <span>
                                {preview.result.stack.uniqueFilaments} filaments
                                · {preview.result.stack.runs.length} runs ·{" "}
                                {preview.result.stack.layerColors.length} layers
                              </span>
                            </div>
                          </div>
                          {preview.result.stack.depthSelection && (
                            <p className="field-help">
                              Auto depth ·{" "}
                              {
                                preview.result.stack.depthSelection
                                  .comparedDepths
                              }{" "}
                              depths compared ·{" "}
                              {preview.result.stack.depthSelection.hardMaximum.toFixed(
                                2,
                              )}{" "}
                              mm ceiling.
                            </p>
                          )}
                        </>
                      )}
                      {preview.options.mode !== "standard" && (
                        <div className="optics-note">
                          <Info size={14} />
                          <p>
                            {preview.options.mode === "guided"
                              ? "Filament colors and pairwise hues guide this image. Open the PNG in HueForge to create the final layer plan."
                              : preview.options.hueforge.opticalModel ===
                                  "hueforge-0.9.4.3-frontlit-v1"
                                ? "Front Lit layer colors follow the validated HueForge model. Match your lighting, filament measurements, and swap heights before printing."
                                : "This stack uses the legacy optical approximation. Choose HueForge Front Lit for the validated layer-color model."}
                          </p>
                        </div>
                      )}
                    </div>
                  </>
                ) : (
                  <div className="palette-empty">
                    <SwatchBook size={22} />
                    <p>
                      Generate a preview to inspect its colors and filaments.
                    </p>
                  </div>
                )}
              </div>
            </section>
          )}
        </main>
        <aside className="controls-panel" aria-label="Processing settings">
          <div className="inspector-heading">
            {workflowControl}
            <div className="preset-toolbar">
              <button
                className="text-button"
                onClick={() => setShowPresets(true)}
              >
                <Save size={13} />
                {matchingPreset?.name ?? "Presets & profiles"}
                <ChevronDown size={13} />
              </button>
              <IconButton
                title="Reset processing settings"
                onClick={() => {
                  const n = copy(defaults);
                  n.mode = options.mode;
                  n.colors = 8;
                  update(n);
                }}
              >
                <RotateCcw size={14} />
              </IconButton>
            </div>
          </div>
          {(options.mode !== "standard" || tab === "filaments") && (
            <nav className="panel-tabs" aria-label="Settings category">
              <button
                className={tab === "adjust" ? "active" : ""}
                aria-pressed={tab === "adjust"}
                onClick={() => setTab("adjust")}
              >
                Tune
              </button>
              <button
                className={tab === "filaments" ? "active" : ""}
                aria-pressed={tab === "filaments"}
                onClick={() => setTab("filaments")}
              >
                Filaments {library && <small>{library.filaments.length}</small>}
              </button>
              {options.mode !== "standard" && (
                <button
                  className={tab === "layers" ? "active" : ""}
                  aria-pressed={tab === "layers"}
                  onClick={() => setTab("layers")}
                >
                  {options.mode === "stack" ? "Layers" : "Optics"}
                </button>
              )}
            </nav>
          )}
          <div className="controls-scroll">
            <div hidden={tab !== "adjust"}>
              {tuningContent}
              {options.mode !== "standard" && (
                <button
                  className="library-link"
                  onClick={() => setTab("filaments")}
                >
                  <SwatchBook size={16} />
                  <span>
                    {libraryPath
                      ? `${library?.filaments.length ?? 0} eligible filaments`
                      : "Choose your filament library"}
                  </span>
                  <ChevronRight size={15} />
                </button>
              )}
              {advancedControl}
              {advanced && (
                <div id="advanced-controls">
                  <Section title="Color & detail controls">
                    {advancedTuningContent}
                  </Section>
                </div>
              )}
            </div>
            <div hidden={tab !== "filaments"}>{libraryContent}</div>
            {options.mode !== "standard" && (
              <div hidden={tab !== "layers"}>
                {layersContent}
                {advancedControl}
                {advanced && (
                  <Section title="Optical calibration">
                    {calibrationContent}
                  </Section>
                )}
              </div>
            )}
          </div>
          <div className="controls-footer">
            {autoControl}
            {previewAction}
          </div>
        </aside>
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
          {historyControls}
          <Updates
            workspaceControls={
              <>
                {advancedControl}
                {autoControl}
              </>
            }
            preferences={preferences}
            savePreferences={savePreferences}
            ready={preferencesReady}
          />
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
      {showFile && (
        <StudioDialog
          title="File"
          onClose={() => setShowFile(false)}
          className="file-dialog"
        >
          <div className="file-actions">
            <button
              className="menu-action"
              onClick={() => {
                setShowFile(false);
                void open();
              }}
              disabled={loading}
            >
              <span>
                <Plus size={16} />
                Open image
              </span>
              <kbd>Ctrl+O</kbd>
            </button>
            <button
              className="menu-action"
              onClick={() => {
                setShowFile(false);
                void openProject();
              }}
              disabled={loading}
            >
              <span>
                <FolderOpen size={16} />
                Open project
              </span>
              <kbd>Ctrl+Shift+O</kbd>
            </button>
            <button
              className="menu-action"
              onClick={() => {
                setShowFile(false);
                void saveProject();
              }}
              disabled={!source || loading || exporting}
            >
              <span>
                <Save size={16} />
                Save project
              </span>
              <kbd>Ctrl+S</kbd>
            </button>
            <button
              className="menu-action"
              onClick={() => {
                setShowFile(false);
                setTab("filaments");
              }}
            >
              <span>
                <SwatchBook size={16} />
                Filament library
              </span>
              <ChevronRight size={14} />
            </button>
          </div>
          <Section title="Recent images" initial>
            <div onClick={() => setShowFile(false)}>{recentContent}</div>
          </Section>
        </StudioDialog>
      )}
      {showPresets && (
        <StudioDialog
          title="Presets & profiles"
          onClose={() => setShowPresets(false)}
          className="presets-dialog"
        >
          {presetsContent}
        </StudioDialog>
      )}
      {showExport && (
        <StudioDialog
          title="Export"
          onClose={() => setShowExport(false)}
          className="export-dialog"
        >
          <div className="export-summary">
            <FileImage size={21} />
            <div>
              <strong>{source?.name ?? "No image"}</strong>
              <span>
                {preview
                  ? `${preview.result.sourceSize.join(" × ")} px · ${preview.result.uniqueColors} colors`
                  : "Generate a preview first"}
              </span>
            </div>
          </div>
          <label className="select-field">
            Format
            <select
              aria-label="Export format"
              value={exportKind}
              onChange={(e) => setExportKind(e.target.value)}
            >
              <option value="png">Full-resolution PNG</option>
              <option value="palette">Palette report (.json)</option>
              <option value="project">ColorNinja project (.colorninja)</option>
              {options.mode === "stack" && (
                <>
                  <option value="layers">16-bit layer map</option>
                  <option value="hfp">HueForge project (.hfp)</option>
                </>
              )}
            </select>
          </label>
          {exportKind === "hfp" && options.mode === "stack" && (
            <div className="hfp-settings">{hfpContent}</div>
          )}
          <label className="export-profile-option">
            <input
              type="checkbox"
              checked={preferences.exportProfile}
              disabled={preferencesSaving || exporting}
              onChange={(e) =>
                savePreferences({
                  ...preferences,
                  exportProfile: e.target.checked,
                }).catch(handleError)
              }
            />
            <span>
              Also save project and settings profile
              <small>
                Reopen the .colorninja project. Reuse the JSON settings with
                Load profile.
              </small>
            </span>
          </label>
          {(dirty || busy) && (
            <div className="export-preview-warning" role="status">
              <span>
                {busy
                  ? progress.stage
                  : "Settings changed. Refresh before exporting."}
              </span>
              <button
                className="button secondary"
                disabled={busy || loading || !source}
                onClick={() => setRerun((v) => v + 1)}
              >
                {busy ? (
                  <LoaderCircle size={14} className="spin" />
                ) : (
                  <Sparkles size={14} />
                )}
                Refresh preview
              </button>
            </div>
          )}
          <div className="modal-actions">
            <button
              className="button secondary"
              onClick={() => setShowExport(false)}
            >
              Cancel
            </button>
            <button
              className="button primary"
              disabled={
                exportBlocked || (exportNeedsStack && !preview?.result.stack)
              }
              onClick={() => exportFile(exportKind)}
            >
              <ArrowDownToLine size={15} />
              Export{" "}
              {exportKind === "png"
                ? "PNG"
                : exportKind === "hfp"
                  ? "HFP"
                  : "file"}
            </button>
          </div>
        </StudioDialog>
      )}
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
        <StudioDialog title={dialog.title} onClose={() => setDialog(null)}>
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              const d = dialog;
              try {
                await d.submit(d.value);
                setDialog(null);
              } catch (err) {
                setDialogError(errorMessage(err));
                handleError(err);
              }
            }}
          >
            <label className="select-field">
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
            {dialogError && (
              <p className="dialog-error" role="alert">
                {dialogError}
              </p>
            )}
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
        </StudioDialog>
      )}
    </div>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
