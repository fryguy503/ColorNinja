import assert from "node:assert/strict";
import test from "node:test";
import { moveColorGroup, orderedColorGroups } from "../src/colorOrderSettings.ts";
import { applySavedPreset } from "../src/settings.ts";
import { defaults } from "../src/types.ts";

test("dragging in both directions keeps each color once without changing the input", () => {
  const keys = ["black", "red", "green", "white"];
  assert.deepEqual(moveColorGroup(keys,"red","white"),["black","green","white","red"]);
  assert.deepEqual(moveColorGroup(keys,"white","red"),["black","white","red","green"]);
  assert.deepEqual(moveColorGroup(keys,"unknown","red"),keys);
  assert.deepEqual(keys,["black","red","green","white"]);
});

test("rerenders keep the requested order, append new groups, and omit absent groups", () => {
  const groups = ["green","white","black"].map(key=>({key,rgb:[0,0,0],sourceFraction:1/3,meanHeightMm:1}));
  assert.deepEqual(orderedColorGroups(groups,"black,red,green").map(g=>g.key),["black","green","white"]);
  assert.deepEqual(orderedColorGroups(groups,"").map(g=>g.key),["green","white","black"]);
  const preset=structuredClone(defaults);
  preset.mode="stack";preset.hueforge.colorOrder="black,red,white";preset.hueforge.colorOrderWeight=75;
  const restored=applySavedPreset(defaults,preset,false);
  assert.equal(restored.hueforge.colorOrder,preset.hueforge.colorOrder);
  assert.equal(restored.hueforge.colorOrderWeight,75);
});
