import { borderColorChoices } from "./border";
import type { HueForgeOptions } from "./types";
import "./border.css";

export function BorderColorPicker({
  colors,
  depth,
  options,
  onChange,
  stale,
}: {
  colors: { rgb: number[]; layer: number }[];
  depth: number;
  options: HueForgeOptions;
  onChange: (depth: number) => void;
  stale: boolean;
}) {
  const choices = borderColorChoices(colors, depth, options);
  const layer =
    Math.round(
      (depth - (options.firstLayerHeight || options.layerHeight)) /
        options.layerHeight +
        1e-9,
    ) + 1;
  const selectedRGB = colors.find((c) => c.layer === layer)?.rgb;
  const selected =
    selectedRGB &&
    "#" +
      selectedRGB
        .map((v) => v.toString(16).padStart(2, "0"))
        .join("")
        .toUpperCase();
  return (
    <>
      <label className="select-field">
        Border color
        <span className="border-color-control">
          <i
            className="border-swatch"
            style={{ background: selected || "transparent" }}
            aria-hidden="true"
          />
          <select
            aria-label="Border color"
            value={selected || ""}
            disabled={stale || choices.length === 0}
            onChange={(e) => {
              const choice = choices.find((c) => c.hex === e.target.value);
              if (choice) onChange(choice.depth);
            }}
          >
            {!selected && (
              <option value="">
                {choices.length
                  ? "Custom depth above image"
                  : "Generate a preview"}
              </option>
            )}
            {choices.map((choice) => (
              <option key={choice.hex} value={choice.hex}>
                {choice.hex} · layer {choice.layer} · {choice.depth.toFixed(2)}{" "}
                mm
              </option>
            ))}
          </select>
        </span>
      </label>
      <p className="field-help">
        {stale
          ? "Refresh the preview to choose a border color from the current stack."
          : "Choose a printable color from the stack. Selecting a color sets the border depth to its layer; changing depth changes the color."}
      </p>
    </>
  );
}
