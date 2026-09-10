import {
  defaultHeightMap,
  type HeightMapInfo,
  type HeightMapOptions,
  type Options,
} from "./types";

export function HeightMapPanel({
  options,
  update,
  info,
  stale,
}: {
  options: Options;
  update: (value: Options) => void;
  info?: HeightMapInfo;
  stale: boolean;
}) {
  const h = { ...defaultHeightMap, ...options.heightMap };
  const change = <K extends keyof HeightMapOptions>(
    key: K,
    value: HeightMapOptions[K],
  ) => update({ ...options, heightMap: { ...h, [key]: value } });
  const number = (
    label: string,
    value: number,
    min: number,
    max: number,
    step: number,
    commit: (v: number) => void,
  ) => (
    <label className="select-field">
      {label}
      <input
        type="number"
        aria-label={label}
        value={value}
        min={min}
        max={max}
        step={step}
        onChange={(e) => {
          const v = e.currentTarget.valueAsNumber;
          if (
            Number.isFinite(v) &&
            v >= min &&
            v <= max &&
            (step !== 1 || Number.isInteger(v))
          )
            commit(v);
        }}
      />
    </label>
  );
  const channelValue = (
    key: "channelShift" | "bandWeights",
    c: number,
    v: number,
  ) => {
    const a = [...h[key]] as [number, number, number];
    a[c] = v;
    change(key, a);
  };
  const channelToggle = (
    key: "ignore" | "invertBands",
    c: number,
    v: boolean,
  ) => {
    const a = [...h[key]] as [boolean, boolean, boolean];
    a[c] = v;
    change(key, a);
  };
  return (
    <section className="height-workflow" aria-label="Height planning">
      <p className="field-help">
        Assign image heights, then fit your filaments to those heights. Export
        preserves this plan in HueForge.
      </p>
      {h.mode === "combo" &&
        number("Standard / Max Channel mix (%)", h.mixing, 0, 100, 1, (v) =>
          change("mixing", v),
        )}
      {(h.mode === "standard" ||
        h.mode === "combo" ||
        h.mode === "color-aware") && (
        <label className="select-field">
          Brightness model
          <select
            aria-label="Brightness model"
            value={h.standardModel || "rgb-weights"}
            onChange={(e) =>
              change(
                "standardModel",
                e.target.value as HeightMapOptions["standardModel"],
              )
            }
          >
            <option value="rgb-weights">RGB weights</option>
            <option value="perceptual">Perceptual brightness</option>
          </select>
        </label>
      )}
      <label className="check-field">
        <input
          type="checkbox"
          checked={h.fullRange}
          onChange={(e) => change("fullRange", e.target.checked)}
        />
        Fill each band’s brightness range
      </label>
      <label className="check-field">
        <input
          type="checkbox"
          checked={h.invert}
          onChange={(e) => change("invert", e.target.checked)}
        />
        Invert brightness heights
      </label>
      {number("Height brightness", h.brightness, -100, 100, 1, (v) =>
        change("brightness", v),
      )}
      {number("Height gamma", h.gamma, 0.1, 5, 0.1, (v) => change("gamma", v))}
      {h.mode === "scaled-max-channel" && (
        <p className="field-help">
          Matches HueForge’s channel-average calculation; pixels with no channel
          above 32 map to the bottom of the brightness range.
        </p>
      )}
      {h.mode === "color-aware" && (
        <>
          <label className="select-field">
            Band order · bottom to top
            <select
              aria-label="Color Aware band order"
              value={h.channelOrder}
              onChange={(e) => change("channelOrder", e.target.value)}
            >
              {["rgb", "rbg", "grb", "gbr", "brg", "bgr"].map((order) => (
                <option key={order} value={order}>
                  {order
                    .split("")
                    .map((c) => ({ r: "Red", g: "Green", b: "Blue" })[c])
                    .join(" → ")}
                </option>
              ))}
            </select>
          </label>
          {number("Boundary gap layers", h.gapLayers, 0, 8, 1, (v) =>
            change("gapLayers", v),
          )}
          <details>
            <summary>Channel controls</summary>
            {["Red", "Green", "Blue"].map((name, c) => (
              <fieldset key={name}>
                <legend>{name} band</legend>
                <label className="check-field">
                  <input
                    type="checkbox"
                    checked={!h.ignore[c]}
                    disabled={
                      !h.ignore[c] && h.ignore.filter((v) => !v).length === 1
                    }
                    onChange={(e) =>
                      channelToggle("ignore", c, !e.target.checked)
                    }
                  />
                  Include {name.toLowerCase()}
                </label>
                {number(
                  `${name} channel shift`,
                  h.channelShift[c],
                  -255,
                  255,
                  1,
                  (v) => channelValue("channelShift", c, v),
                )}
                {number(
                  `${name} relative band height`,
                  h.bandWeights[c],
                  0.1,
                  100,
                  0.1,
                  (v) => channelValue("bandWeights", c, v),
                )}
                <label className="check-field">
                  <input
                    type="checkbox"
                    checked={h.invertBands[c]}
                    onChange={(e) =>
                      channelToggle("invertBands", c, e.target.checked)
                    }
                  />
                  Invert this band
                </label>
              </fieldset>
            ))}
          </details>
        </>
      )}
      {info && !stale && (
        <div className="field-help" aria-live="polite">
          {info.bands.map((b) => (
            <p key={b.channel}>
              {b.name}: layers {b.startLayer}–{b.endLayer} ·{" "}
              {(b.pixelFraction * 100).toFixed(1)}% of image
            </p>
          ))}
          {info.warning && <p>{info.warning}</p>}
        </div>
      )}
    </section>
  );
}
