import {test} from "node:test";
import assert from "node:assert/strict";
import {newFilament,measuredEntry,profileEntry,remapFilamentKeys,profileDetailURL} from "../src/filamentTypes.ts";
test("browser import opens only the preferred site's detail URLs",()=>{
 const url="https://3dfilamentprofiles.com/filament/details/23212";
 assert.equal(profileDetailURL("23212"),url);assert.equal(profileDetailURL(url),url);
 for(const invalid of ["javascript:alert(1)","https://3dfilamentprofiles.com.evil/filament/details/23212",url+"?redirect=elsewhere","0"]){assert.equal(profileDetailURL(invalid),"")}
});
test("import profile then replace only its TD with measured data",()=>{const entry=profileEntry({id:"123",brand:"Acme",material:"PLA",type:"Matte",color:"Red",hex:"#AA0000",secondaryHex:"",td:4,url:"https://3dfilamentprofiles.com/filament/details/123"});const result=measuredEntry(entry,{td:5.5,color:"#BB0022",serial:"TD1S",capturedAt:"2026-09-11T00:00:00Z"},false);assert.equal(result.color,"#AA0000");assert.equal(result.td,5.5);assert.equal(result.brand,"Acme");assert.equal(result.material,"PLA Matte");assert.equal(result.sourceURL,entry.sourceURL);assert.equal(result.measurement?.serial,"TD1S");assert.equal(entry.td,4);assert.equal(measuredEntry(entry,result.measurement!,true).color,"#BB0022")});
test("unknown TD stays unknown and new scans keep selected brand/material",()=>{const entry=profileEntry({id:"1",brand:"Acme",material:"PLA",type:"Basic",color:"White",hex:"#FFFFFF",secondaryHex:"",td:0,url:""});assert.equal(entry.td,0);assert.equal(entry.material,"PLA");assert.equal(newFilament().index,-1)});
test("constraints follow changed spool identity and remove deleted spools",()=>{assert.equal(remapFilamentKeys("a, b,c",{a:"new",b:""}),"new,c");assert.equal(remapFilamentKeys(undefined,{}),"")});
