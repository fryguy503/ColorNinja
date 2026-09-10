import { useState } from "react";
import { ArrowDown, ArrowUp, GripVertical } from "lucide-react";
import type { ColorOrderReport, Options } from "./types";
import { orderedColorGroups, moveColorGroup } from "./colorOrderSettings.ts";
import "./colorOrder.css";

const names: Record<string, string> = {
  black: "Dark neutrals",
  gray: "Mid neutrals",
  white: "Light neutrals",
  red: "Reds",
  yellow: "Yellows",
  green: "Greens",
  cyan: "Cyans",
  blue: "Blues",
  purple: "Purples",
};

export function ColorOrderPanel({
  options,
  report,
  stale,
  disabled,
  recalculate,
}: {
  options: Options;
  report?: ColorOrderReport;
  stale: boolean;
  disabled: boolean;
  recalculate: (o: Options) => void;
}) {
  const [dragged, setDragged] = useState<string | null>(null);
  const [over, setOver] = useState<string | null>(null);
  const groups = orderedColorGroups(
    report?.groups ?? [],
    options.hueforge.colorOrder ?? "",
  );
  const active = !!options.hueforge.colorOrder;
  const change = (
    order: string,
    weight = options.hueforge.colorOrderWeight ?? 50,
  ) =>
    recalculate({
      ...options,
      hueforge: {
        ...options.hueforge,
        colorOrder: order,
        colorOrderWeight: weight,
        layerPreference: "",
      },
    });
  const move = (key: string, target: string) => {
    const next = moveColorGroup(
      groups.map((g) => g.key),
      key,
      target,
    );
    if (next.join(",") !== groups.map((g) => g.key).join(","))
      change(next.join(","));
    setDragged(null);
    setOver(null);
  };
  return (
    <section className="color-order" aria-label="Preferred color order">
      <div className="color-order-heading">
        <strong>Color order</strong>
        <button
          className="text-button"
          disabled={!active || disabled}
          onClick={() => change("")}
        >
          Reset order
        </button>
      </div>
      <p>
        Drag source color groups from bottom to top. Recalculates the stack
        while protecting color accuracy.
      </p>
      {groups.length < 2 ? (
        <p className="muted">
          Generate a Color Match preview with at least two color groups to
          arrange them.
        </p>
      ) : (
        <>
          <div className="color-order-direction">
            First = bottom · Last = top
          </div>
          <ol aria-label="Color groups, bottom to top">
            {groups.map((g, i) => (
              <li
                key={g.key}
                data-color-group={g.key}
                className={over === g.key ? "drag-over" : ""}
                draggable={!disabled}
                onDragStart={(e) => {
                  setDragged(g.key);
                  e.dataTransfer.effectAllowed = "move";
                  e.dataTransfer.setData("text/plain", g.key);
                }}
                onDragEnd={() => {
                  setDragged(null);
                  setOver(null);
                }}
                onDragOver={(e) => {
                  if (!disabled && dragged && dragged !== g.key) {
                    e.preventDefault();
                    e.dataTransfer.dropEffect = "move";
                    setOver(g.key);
                  }
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  if (!disabled && dragged) move(dragged, g.key);
                }}
              >
                <GripVertical size={14} aria-hidden="true" />
                <span
                  className="color-order-swatch"
                  style={{ backgroundColor: `rgb(${g.rgb.join(",")})` }}
                />
                <span className="color-order-name">
                  {names[g.key] ?? g.key}
                  <small>{Math.round(g.sourceFraction * 100)}% of source</small>
                </span>
                <button
                  aria-label={`Move ${names[g.key] ?? g.key} toward bottom`}
                  disabled={disabled || i === 0}
                  onClick={() => move(g.key, groups[i - 1].key)}
                >
                  <ArrowUp size={14} />
                </button>
                <button
                  aria-label={`Move ${names[g.key] ?? g.key} toward top`}
                  disabled={disabled || i === groups.length - 1}
                  onClick={() => move(g.key, groups[i + 1].key)}
                >
                  <ArrowDown size={14} />
                </button>
              </li>
            ))}
          </ol>
          <label className="color-order-strength">
            Order strength{" "}
            <output>{options.hueforge.colorOrderWeight ?? 50}%</output>
            <input
              type="range"
              aria-label="Color order strength"
              min="0"
              max="100"
              step="5"
              value={options.hueforge.colorOrderWeight ?? 50}
              disabled={disabled}
              onChange={(e) =>
                change(
                  groups.map((g) => g.key).join(","),
                  Number(e.target.value),
                )
              }
            />
          </label>
          <p aria-live="polite">
            {stale
              ? "Preview out of date. These groups come from the last render."
              : active && options.hueforge.colorOrderWeight === 0
                ? "Order preference is off at 0%."
                : report?.message}
          </p>
        </>
      )}
    </section>
  );
}
