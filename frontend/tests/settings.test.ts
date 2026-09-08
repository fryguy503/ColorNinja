import assert from "node:assert/strict";
import test from "node:test";
import { defaults, type Options } from "../src/types.ts";
import {
  applyColorBudget,
  applySavedPreset,
  changeProcessingMode,
} from "../src/settings.ts";

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
