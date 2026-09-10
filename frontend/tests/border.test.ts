import test from "node:test";
import assert from "node:assert/strict";
import { borderColorChoices, borderLayerDepth, snapBorderDepth } from "../src/border.ts";
import { defaults } from "../src/types.ts";

test("border depth snaps to printable layers, including halfway and bounds", () => {
  const h = { ...defaults.hueforge, firstLayerHeight: .16, layerHeight: .08 };
  assert.equal(snapBorderDepth(.86, h), .88);
  assert.equal(snapBorderDepth(.83, h), .8);
  assert.equal(snapBorderDepth(.84, h), .88);
  assert.equal(snapBorderDepth(.01, h), .16);
  assert.equal(snapBorderDepth(50, h), 40);
  assert.equal(borderLayerDepth(10, h), .88);
  assert.equal(snapBorderDepth(40, { ...h, layerHeight: .01 }), 10.13);
});

test("border color choices keep the closest depth for optical plateaus", () => {
  const h = { ...defaults.hueforge, firstLayerHeight: .16, layerHeight: .08 };
  const colors = [{ rgb: [0, 0, 0], layer: 1 }, { rgb: [0, 0, 0], layer: 2 }, { rgb: [255, 0, 0], layer: 3 }, { rgb: [255, 255, 255], layer: 999 }];
  assert.deepEqual(borderColorChoices(colors, .25, h), [
    { hex: "#000000", layer: 2, depth: .24 },
    { hex: "#FF0000", layer: 3, depth: .32 },
  ]);
  assert.equal(borderColorChoices(colors, .16, h)[0].layer, 1);
  assert.equal(colors.length, 4);
});
