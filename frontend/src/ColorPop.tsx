import { useRef, useState } from "react";
import { Pipette, X, Flower2 } from "lucide-react";
import { StudioDialog } from "./StudioDialog";
import {
  defaultColorPop,
  type Options,
  type Source,
  type ColorPopInfo,
  type ColorPopOptions,
} from "./types";

function PopRange({
  label,
  value,
  min,
  max,
  suffix = "",
  change,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  suffix?: string;
  change: (n: number) => void;
}) {
  return (
    <label className="pop-range">
      <span>
        {label}
        <strong>
          {value}
          {suffix}
        </strong>
      </span>
      <input
        type="range"
        aria-label={label}
        min={min}
        max={max}
        value={value}
        onChange={(e) => change(Number(e.target.value))}
      />
    </label>
  );
}

export function ColorPopPanel({
  options,
  update,
  source,
  info,
  stale,
  demo,
  disabled,
}: {
  options: Options;
  update: (o: Options) => void;
  source: Source | null;
  info?: ColorPopInfo;
  stale: boolean;
  demo: () => void;
  disabled: boolean;
}) {
  const [picking, setPicking] = useState(false);
  const [error, setError] = useState("");
  const [manual, setManual] = useState("#E52A15");
  const picture = useRef<HTMLImageElement>(null);
  const c = { ...defaultColorPop, ...options.colorPop };
  const samples = c.colors.split(",").filter(Boolean);
  const change = <K extends keyof ColorPopOptions>(
    key: K,
    value: ColorPopOptions[K],
  ) => update({ ...options, colorPop: { ...c, [key]: value } });
  const pick = (color: string) => {
    const values = [...new Set([...samples, color.toUpperCase()])];
    if (values.length > 8) {
      setError("Keep up to eight samples. Remove one to add another.");
      return;
    }
    update({
      ...options,
      colorPop: { ...c, selection: "selected", colors: values.join(",") },
    });
    setPicking(false);
    setError("");
  };
  return (
    <div className="color-pop-panel">
      <div className="pop-heading">
        <span>
          <Flower2 size={16} /> Color Pop
        </span>
        <button className="text-button" onClick={demo} disabled={disabled}>
          Try demo
        </button>
      </div>
      <p className="field-help">
        Keep an accent in color and turn the rest grayscale.
      </p>
      <label className="select-field">
        Color selection
        <select
          aria-label="Color Pop selection"
          value={c.selection}
          onChange={(e) =>
            change("selection", e.target.value as ColorPopOptions["selection"])
          }
        >
          <option value="existing">Use existing image colors</option>
          <option value="selected">Choose colors to keep</option>
        </select>
      </label>
      {c.selection === "selected" ? (
        <>
          <div className="pop-swatches">
            {samples.map((color) => (
              <button
                key={color}
                title={`Remove ${color}`}
                aria-label={`Remove Color Pop sample ${color}`}
                onClick={() =>
                  change("colors", samples.filter((v) => v !== color).join(","))
                }
              >
                <i style={{ background: color }} />
                <span>{color}</span>
                <X size={12} />
              </button>
            ))}
          </div>
          <button
            className="button secondary pop-pick"
            disabled={!source || disabled}
            onClick={() => {
              setError("");
              setPicking(true);
            }}
          >
            <Pipette size={15} /> Pick from original
          </button>
          {!samples.length && (
            <p className="field-help">
              Pick a color to begin. Until then, the result is grayscale.
            </p>
          )}
          <PopRange
            label="Similar hues"
            value={c.hueTolerance}
            min={0}
            max={90}
            suffix="°"
            change={(v) => change("hueTolerance", v)}
          />
          <p className="field-help">
            Includes lighter and darker shades. Wider ranges keep more
            neighboring hues, everywhere in the image.
          </p>
        </>
      ) : (
        <p className="field-help">
          Keeps all non-gray colors. Ideal for artwork already prepared with a
          colored subject against grayscale.
        </p>
      )}
      <PopRange
        label="Grayscale tolerance"
        value={c.grayTolerance}
        min={0}
        max={100}
        change={(v) => change("grayTolerance", v)}
      />
      <p className="field-help">
        Raise this to treat faint color casts as gray.
      </p>
      {info && (
        <div className={`pop-selection-preview ${stale ? "stale" : ""}`}>
          <img
            src={`data:image/png;base64,${info.selectionPng}`}
            alt="Color Pop selection: white pixels stay colored, dark pixels become grayscale"
          />
          <div>
            <strong>
              {stale
                ? "Previous selection"
                : `${Math.round(info.colorFraction * 100)}% in color`}
            </strong>
            <span>White = kept in color</span>
            <span>View Selection on the canvas to inspect.</span>
          </div>
        </div>
      )}
      {info?.warning && !stale && (
        <p className="pop-notice" role="status">
          {info.warning}
        </p>
      )}
      <label className="select-field">
        Output
        <select
          aria-label="Color Pop output"
          value={options.mode}
          onChange={(e) =>
            update({
              ...options,
              mode: e.target.value as Options["mode"],
              hueforge: {
                ...options.hueforge,
                opticalModel: "hueforge-0.9.4.3-frontlit-v1",
              },
            })
          }
        >
          <option value="standard">Prepare image · no library needed</option>
          <option value="stack">Plan filament stack · experimental</option>
        </select>
      </label>
      {options.mode === "stack" && (
        <>
          <div
            className={`pop-bands ${c.grayOnTop ? "gray-on-top" : ""}`}
            aria-label="Color Pop height allocation"
          >
            <span className="pop-color-band" style={{ flex: c.colorPercent }}>
              Color {c.colorPercent}%
            </span>
            <span
              className="pop-gray-band"
              style={{ flex: 100 - c.colorPercent }}
            >
              Gray {100 - c.colorPercent}%
            </span>
          </div>
          <PopRange
            label="Height for color"
            value={c.colorPercent}
            min={10}
            max={90}
            suffix="%"
            change={(v) => change("colorPercent", v)}
          />
          <label className="select-field">
            Region order
            <select
              aria-label="Color Pop region order"
              value={c.grayOnTop ? "gray" : "color"}
              onChange={(e) => change("grayOnTop", e.target.value === "gray")}
            >
              <option value="color">Color above grayscale</option>
              <option value="gray">Grayscale above color</option>
            </select>
          </label>
          <PopRange
            label="Boundary gap"
            value={c.gapLayers}
            min={0}
            max={8}
            suffix=" layers"
            change={(v) => change("gapLayers", v)}
          />
          <p className="field-help">
            Each region gets its own brightness range. The gap reserves layers
            for the transition between regions. Set print thickness and filament
            runs in Layers.
          </p>
          {info && !stale && info.colorLayers[1] > 0 && (
            <p className="field-help">
              Color: layers {info.colorLayers.join("–")}. Grayscale: layers{" "}
              {info.grayLayers.join("–")}.
            </p>
          )}
        </>
      )}
      {picking && source && (
        <StudioDialog
          title="Pick a Color Pop color"
          onClose={() => setPicking(false)}
          className="pop-picker-dialog"
        >
          <p>
            Click the original image to keep that hue and its lighter and darker
            shades. Add another sample for a second hue.
          </p>
          <img
            ref={picture}
            src={source.url}
            alt="Original image for Color Pop sampling"
            draggable={false}
            onClick={(e) => {
              try {
                const img = picture.current!;
                const rect = img.getBoundingClientRect();
                const x = Math.min(
                  img.naturalWidth - 1,
                  Math.max(
                    0,
                    Math.floor(
                      ((e.clientX - rect.left) / rect.width) * img.naturalWidth,
                    ),
                  ),
                );
                const y = Math.min(
                  img.naturalHeight - 1,
                  Math.max(
                    0,
                    Math.floor(
                      ((e.clientY - rect.top) / rect.height) *
                        img.naturalHeight,
                    ),
                  ),
                );
                const canvas = document.createElement("canvas");
                canvas.width = 1;
                canvas.height = 1;
                const ctx = canvas.getContext("2d")!;
                ctx.drawImage(img, x, y, 1, 1, 0, 0, 1, 1);
                const pixel = ctx.getImageData(0, 0, 1, 1).data;
                if (pixel[3] === 0) {
                  setError("Choose a visible part of the image.");
                  return;
                }
                if (
                  Math.max(...pixel.slice(0, 3)) -
                    Math.min(...pixel.slice(0, 3)) <=
                  c.grayTolerance
                ) {
                  setError(
                    "That pixel is within grayscale tolerance. Choose a more colorful pixel.",
                  );
                  return;
                }
                pick(
                  "#" +
                    Array.from(pixel.slice(0, 3), (v) =>
                      v.toString(16).padStart(2, "0"),
                    ).join(""),
                );
              } catch {
                setError(
                  "Could not sample this pixel. Use the color control below.",
                );
              }
            }}
          />
          <div className="pop-manual">
            <label>
              Or choose a color{" "}
              <input
                type="color"
                aria-label="Color Pop sample color"
                value={manual}
                onChange={(e) => setManual(e.target.value)}
              />
            </label>
            <button className="button secondary" onClick={() => pick(manual)}>
              Keep this color
            </button>
          </div>
          {error && <p role="alert">{error}</p>}
        </StudioDialog>
      )}
    </div>
  );
}
