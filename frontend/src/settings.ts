import type { Options } from "./types.ts";

// A mode change selects an algorithm; it must not reset the user's tuning.
export function changeProcessingMode(
  options: Options,
  mode: Options["mode"],
): Options {
  return { ...options, mode };
}

// Built-in palette presets describe only a color budget.
export function applyColorBudget(options: Options, colors: number): Options {
  return { ...options, colors };
}

// Saved presets restore tuning, while the explicitly selected mode stays active.
export function applySavedPreset(options: Options, preset: Options): Options {
  return { ...structuredClone(preset), mode: options.mode };
}
