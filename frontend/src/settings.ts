import {
  defaultColorPop,
  defaultHeightMap,
  type HeightMode,
  type Options,
  type Filter,
} from "./types.ts";

export function heightWorkflow(options: Options, mode: HeightMode): Options {
  return normalizeWorkflowExport({
    ...options,
    mode: "stack",
    colorPop: { ...options.colorPop, enabled: false },
    heightMap: {
      ...structuredClone(defaultHeightMap),
      ...options.heightMap,
      mode,
    },
    hueforge: {
      ...options.hueforge,
      opticalModel:
        options.hueforge.opticalModel === "legacy-exponential"
          ? "hueforge-0.9.4.3-frontlit-v1"
          : options.hueforge.opticalModel,
    },
  });
}

// All desktop stack workflows export their planned heights through HueForge
// Color Match. Older luminance-mode choices must not survive a workflow change.
export function normalizeWorkflowExport(options: Options): Options {
  return {
    ...options,
    heightMap: { ...defaultHeightMap, ...options.heightMap, mode: "" },
    hueforge:
      options.mode === "stack" && options.hueforge.meshMode
        ? { ...options.hueforge, meshMode: "color-match" }
        : options.hueforge,
  };
}

export function colorPopWorkflow(options: Options, enabled: boolean): Options {
  return normalizeWorkflowExport({
    ...options,
    mode: enabled && options.mode === "guided" ? "standard" : options.mode,
    heightMap: {
      ...structuredClone(defaultHeightMap),
      ...options.heightMap,
      mode: "",
    },
    hueforge: enabled
      ? {
          ...options.hueforge,
          opticalModel:
            options.hueforge.opticalModel === "legacy-exponential"
              ? "hueforge-0.9.4.3-frontlit-v1"
              : options.hueforge.opticalModel,
        }
      : options.hueforge,
    colorPop: { ...defaultColorPop, ...options.colorPop, enabled },
  });
}

export function normalizeFilter(filter: Filter): Filter {
  return {
    ...filter,
    materialTypes: filter.materialTypes ?? [],
    excludedIds: filter.excludedIds ?? [],
  };
}

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
      opticalModel:
        enabled && h.opticalModel === "legacy-exponential"
          ? "hueforge-0.9.4.3-frontlit-v1"
          : h.opticalModel,
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
  return normalizeWorkflowExport({
    ...options,
    mode,
    heightMap: {
      ...structuredClone(defaultHeightMap),
      ...options.heightMap,
      mode: "",
    },
    colorPop: { ...options.colorPop, enabled: false },
  });
}

// Built-in palette presets describe only a color budget.
export function applyColorBudget(options: Options, colors: number): Options {
  return applyDisplayedColorBudget(options, colors);
}

// Saved presets restore tuning, while the explicitly selected mode stays active.
export function applySavedPreset(
  options: Options,
  preset: Options,
  keepWorkflow = true,
): Options {
  const applied: Options = {
    ...structuredClone(preset),
    mode: keepWorkflow ? options.mode : preset.mode,
    heightMap: {
      ...structuredClone(defaultHeightMap),
      ...preset.heightMap,
      mode: keepWorkflow
        ? (options.heightMap?.mode ?? "")
        : (preset.heightMap?.mode ?? ""),
    },
    colorPop: {
      ...(keepWorkflow && !preset.colorPop?.enabled
        ? options.colorPop
        : (preset.colorPop ?? defaultColorPop)),
      enabled: keepWorkflow
        ? options.colorPop.enabled
        : !!preset.colorPop?.enabled,
    },
  };
  // A saved legacy optical profile cannot switch an active Color Pop stack
  // away from the model its separate height bands require.
  if (
    (applied.colorPop.enabled ||
      (applied.heightMap?.mode && applied.heightMap.mode !== "color-match")) &&
    applied.hueforge.opticalModel === "legacy-exponential"
  )
    applied.hueforge.opticalModel = "hueforge-0.9.4.3-frontlit-v1";
  return normalizeWorkflowExport(applied);
}
