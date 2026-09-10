import assert from "node:assert/strict";
import test from "node:test";
import { defaults } from "../src/types.ts";
import { applyPrintDimension } from "../src/settings.ts";

test("depths snap to the first-layer offset, in either direction",()=>{
  assert.equal(applyPrintDimension(defaults,"baseDepth",.86).hueforge.baseDepth,.88);
  assert.equal(applyPrintDimension(defaults,"baseDepth",.81).hueforge.baseDepth,.8);
  assert.equal(applyPrintDimension(defaults,"baseDepth",.84).hueforge.baseDepth,.88);
  const unusual=structuredClone(defaults);unusual.hueforge.firstLayerHeight=.19;unusual.hueforge.layerHeight=.07;
  assert.equal(applyPrintDimension(unusual,"baseDepth",.86).hueforge.baseDepth,.89);
  assert.equal(defaults.hueforge.baseDepth,.48);
});
test("base, maximum and changed layer thickness retain a printable top layer",()=>{
  let value=applyPrintDimension(defaults,"baseDepth",3.19);
  assert.equal(value.hueforge.baseDepth,3.2);assert.equal(value.hueforge.maxDepth,3.28);
  value=applyPrintDimension(value,"maxDepth",.5);assert.equal(value.hueforge.maxDepth,3.28);
  for(const field of ["firstLayerHeight","layerHeight"] as const){const result=applyPrintDimension(defaults,field,.11);const h=result.hueforge;
    for(const depth of [h.baseDepth,h.maxDepth])assert.ok(Math.abs((depth-h.firstLayerHeight)/h.layerHeight-Math.round((depth-h.firstLayerHeight)/h.layerHeight))<1e-7);
    assert.ok(h.maxDepth>=h.baseDepth+h.layerHeight-1e-8);
  }
});
test("automatic depth preserves an unaligned hard cap, while manual depth snaps",()=>{
  const o=structuredClone(defaults);o.hueforge.autoDepth=true;
  assert.equal(applyPrintDimension(o,"maxDepth",3.19).hueforge.maxDepth,3.19);
  assert.equal(applyPrintDimension(defaults,"maxDepth",3.19).hueforge.maxDepth,3.2);
  assert.equal(applyPrintDimension(defaults,"baseDepth",NaN),defaults);
});
