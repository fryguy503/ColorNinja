import type { Options } from "./types.ts";

export function applyAutoDepth(options: Options, enabled: boolean): Options {
  const h = options.hueforge;
  const first = h.firstLayerHeight || h.layerHeight;
  const printable =
    first +
    Math.floor((h.maxDepth - first) / h.layerHeight + 1e-9) * h.layerHeight;
  return {
    ...options,
    hueforge: {
      ...h,
      autoDepth: enabled,
      opticalModel: enabled ? "hueforge-0.9.4.3-frontlit-v1" : h.opticalModel,
      maxDepth: enabled ? h.maxDepth : Number(printable.toFixed(8)),
    },
  };
}

export function prioritizesColors(options: Options): boolean {
  return (
    options.colorPriority === "distinctive" || options.colorPriority === "vivid"
  );
}

export function applyColorPriority(
  options: Options,
  priority: Options["colorPriority"],
): Options {
  return {
    ...options,
    colorPriority: priority,
    legacyColorPipeline:
      priority === "distinctive" || priority === "vivid"
        ? false
        : options.legacyColorPipeline,
  };
}

// Priority modes share the existing ceiling across all families. Display that
// total while retaining the saved split-budget representation and mode budget.
export function displayedColorBudget(options: Options): number {
  return options.mode === "standard" &&
    prioritizesColors(options) &&
    !options.totalColors
    ? options.colors * 2
    : options.colors;
}
export function applyDisplayedColorBudget(
  options: Options,
  colors: number,
): Options {
  if (options.mode === "standard" && prioritizesColors(options)) {
    if (colors > 256)
      return { ...options, colors: Math.ceil(colors / 2), totalColors: false };
    return { ...options, colors, totalColors: true };
  }
  return { ...options, colors };
}

export const smoothingPresets = {
  Off: { preblurSigma: 0, smoothingColorSigma: 0 },
  Gentle: { preblurSigma: 1, smoothingColorSigma: 5 },
  Balanced: { preblurSigma: 1.5, smoothingColorSigma: 5 },
  Strong: { preblurSigma: 2.5, smoothingColorSigma: 10 },
} as const;
export type SmoothingPreset = keyof typeof smoothingPresets;
export function applySmoothing(
  options: Options,
  preset: SmoothingPreset,
): Options {
  return {
    ...options,
    legacyColorPipeline: false,
    ...smoothingPresets[preset],
  };
}
export function smoothingPreset(options: Options): SmoothingPreset | "Custom" {
  if (options.legacyColorPipeline) return "Custom";
  if (options.preblurSigma === 0) return "Off";
  return (
    (Object.keys(smoothingPresets) as SmoothingPreset[]).find((name) => {
      const p = smoothingPresets[name];
      return (
        p.preblurSigma === options.preblurSigma &&
        p.smoothingColorSigma === (options.smoothingColorSigma || 5)
      );
    }) ?? "Custom"
  );
}

// A mode change selects an algorithm; it must not reset the user's tuning.
export function changeProcessingMode(
  options: Options,
  mode: Options["mode"],
): Options {
  return { ...options, mode };
}

// Built-in palette presets describe only a color budget.
export function applyColorBudget(options: Options, colors: number): Options {
  return applyDisplayedColorBudget(options, colors);
}

// Saved presets restore tuning, while the explicitly selected mode stays active.
export function applySavedPreset(options: Options, preset: Options): Options {
  return { ...structuredClone(preset), mode: options.mode };
}
