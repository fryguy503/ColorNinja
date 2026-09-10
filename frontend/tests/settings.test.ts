import assert from "node:assert/strict";
import test from "node:test";
import { defaults, type Options } from "../src/types.ts";
import {
  applyColorBudget,
  colorPopWorkflow,
  applyAutoDepth,
  applySavedPreset,
  normalizeFilter,
  changeProcessingMode,
  applySmoothing,
  smoothingPreset,
  applyColorPriority,
  displayedColorBudget,
  applyDisplayedColorBudget,
} from "../src/settings.ts";

test("Color Pop keeps existing tuning, preserves selection across ordinary presets, and stays opt-in", () => {
  const original = structuredClone(defaults); original.mode = "guided"; original.colors = 12;
  const pop = colorPopWorkflow(original, true); pop.colorPop.selection = "selected"; pop.colorPop.colors = "#EE2211";
  assert.equal(original.colorPop.enabled, false); assert.equal(pop.mode, "standard"); assert.equal(pop.colors, 12);
  const preset = structuredClone(defaults); preset.colors = 4;
  const applied = applySavedPreset(pop, preset);
  assert.equal(applied.colors, 4); assert.deepEqual(applied.colorPop, pop.colorPop);
  preset.hueforge.opticalModel = "legacy-exponential";
  const stack = { ...pop, mode: "stack" as const };
  assert.equal(applySavedPreset(stack, preset).hueforge.opticalModel, "hueforge-0.9.4.3-frontlit-v1");
  const normal = changeProcessingMode(pop, "stack"); assert.equal(normal.colorPop.enabled, false); assert.equal(normal.colorPop.colors, "#EE2211");
  const restored = applySavedPreset(normal, pop, false); assert.equal(restored.colorPop.enabled, true); assert.equal(restored.mode, "standard");
});

test("presets can restore their saved workflow without mutating either settings object", () => {
  const current = structuredClone(defaults);
  const preset = structuredClone(defaults);
  preset.mode = "stack";
  preset.hueforge.autoDepth = true;
  const restored = applySavedPreset(current, preset, false);
  assert.equal(restored.mode, "stack");
  assert.equal(restored.hueforge.autoDepth, true);
  restored.hueforge.maxDepth = 8;
  assert.equal(preset.hueforge.maxDepth, defaults.hueforge.maxDepth);
  assert.equal(current.mode, "standard");
  assert.equal(applySavedPreset(current, preset).mode, "standard");
});

test("profiles and portable projects normalize empty library filters", () => {
  const filter = { includeUnowned: false, materialTypes: null, excludedIds: null, allowSecondary: false, avoidSilkMetallic: true };
  const normalized = normalizeFilter(filter as any);
  assert.deepEqual(normalized.materialTypes, []);
  assert.deepEqual(normalized.excludedIds, []);
  assert.equal(normalized.avoidSilkMetallic, true);
  assert.equal(filter.materialTypes, null);
});

test("automatic depth retains the ceiling and snaps down when returning to manual depth", () => {
  const options = structuredClone(defaults);
  options.hueforge.maxDepth = 4.03;
  const next = applyAutoDepth(options, true);
  assert.equal(next.hueforge.maxDepth, 4.03);
  assert.equal(next.hueforge.autoDepth, true);
  const manual = applyAutoDepth(next, false);
  assert.equal(manual.hueforge.maxDepth, 4);
  assert.equal(manual.hueforge.autoDepth, false);
  assert.equal(options.hueforge.autoDepth, false);
  assert.deepEqual(next.hueforge, { ...options.hueforge, autoDepth: true });
});

test("color priority retains the color ceiling, smoothing, detail choice, calibration and history", () => {
  for (const mode of ["standard", "guided", "stack"] as const) {
    for (const totalColors of [true, false]) {
      const original: Options = {
        ...structuredClone(defaults),
        mode,
        colors: 8,
        totalColors,
        preserveDetails: false,
        preblurSigma: 2.5,
        smoothingColorSigma: 10,
        legacyColorPipeline: true,
      };
      const saved = structuredClone(original);
      for (const priority of ["distinctive", "vivid"] as const) {
        const next = applyColorPriority(original, priority);
        assert.deepEqual(next, {
          ...saved,
          colorPriority: priority,
          legacyColorPipeline: false,
        });
        assert.equal(
          displayedColorBudget(next),
          mode === "standard" && !totalColors ? 16 : 8,
        );
        assert.deepEqual(original, saved);
        const restored = applyColorPriority(next, "balanced");
        assert.equal(restored.colors, original.colors);
        assert.equal(restored.totalColors, totalColors);
      }
    }
  }
});

test("priority palette edits and built-in presets mean the displayed total, including old split budgets", () => {
  const original: Options = {
    ...structuredClone(defaults),
    colors: 8,
    totalColors: false,
    colorPriority: "distinctive",
  };
  assert.equal(displayedColorBudget(original), 16);
  for (const count of [1, 8, 17, 256]) {
    const edited = applyDisplayedColorBudget(original, count);
    assert.equal(displayedColorBudget(edited), count);
    assert.equal(edited.totalColors, true);
    assert.deepEqual(applyColorBudget(original, count), edited);
  }
  const large = { ...original, colors: 256 };
  assert.equal(displayedColorBudget(large), 512);
  assert.equal(
    displayedColorBudget(applyDisplayedColorBudget(large, 510)),
    510,
  );
  assert.equal(changeProcessingMode(original, "guided").colors, 8);
});

test("smoothing presets keep detail preservation, budgets, mode, calibration and undo entries intact", () => {
  for (const mode of ["standard", "guided", "stack"] as const) {
    for (const preserveDetails of [true, false]) {
      const original = {
        ...structuredClone(defaults),
        mode,
        colors: 13,
        preserveDetails,
        legacyColorPipeline: true,
      };
      const saved = structuredClone(original);
      for (const preset of ["Off", "Gentle", "Balanced", "Strong"] as const) {
        const next = applySmoothing(original, preset);
        assert.equal(smoothingPreset(next), preset);
        assert.equal(next.colors, 13);
        assert.equal(next.mode, mode);
        assert.deepEqual(next.hueforge, original.hueforge);
        assert.equal(next.totalColors, original.totalColors);
        assert.equal(next.preserveDetails, preserveDetails);
        assert.equal(next.legacyColorPipeline, false);
        assert.deepEqual(original, saved);
      }
    }
  }
  assert.equal(smoothingPreset(defaults), "Balanced");
  assert.equal(smoothingPreset({ ...defaults, preblurSigma: 2.3 }), "Custom");
  assert.equal(
    smoothingPreset({ ...defaults, preserveDetails: false }),
    "Balanced",
  );
  assert.equal(
    smoothingPreset({ ...defaults, legacyColorPipeline: true }),
    "Custom",
  );
});

test("preset followed by mode changes retains the requested budget and calibration", () => {
  for (const startingMode of ["standard", "guided", "stack"] as const) {
    for (const budget of [4, 8, 16]) {
      const original: Options = {
        ...structuredClone(defaults),
        mode: startingMode,
        preblurSigma: 2.7,
        guidanceStrength: 0.65,
        trueBlack: false,
        preserveDetails: false,
        hueforge: { ...defaults.hueforge, tdScale: 0.23 },
      };
      let current = applyColorBudget(original, budget);
      assert.equal(current.mode, startingMode, "preset changed mode");
      for (const nextMode of [
        "guided",
        "stack",
        "standard",
        "guided",
      ] as const) {
        current = changeProcessingMode(current, nextMode);
        assert.deepEqual(current, {
          ...original,
          mode: nextMode,
          colors: budget,
        });
      }
      assert.equal(
        original.colors,
        defaults.colors,
        "preset mutated the undo-history entry",
      );
    }
  }
});

test("saved presets keep the active mode and do not share mutable calibration", () => {
  const current: Options = { ...structuredClone(defaults), mode: "stack" };
  const preset: Options = {
    ...structuredClone(defaults),
    mode: "standard",
    colors: 12,
    trueBlack: false,
    preserveDetails: false,
  };
  const applied = applySavedPreset(current, preset);
  assert.equal(applied.mode, "stack");
  assert.equal(applied.colors, 12);
  assert.equal(applied.trueBlack, false);
  assert.equal(applied.preserveDetails, false);
  applied.hueforge.tdScale = 0.5;
  assert.equal(preset.hueforge.tdScale, defaults.hueforge.tdScale);
});
