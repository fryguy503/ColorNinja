// Acceptance against the real Go backend and embedded production frontend.
// Requires the pinned frontend Playwright dependency and installed Microsoft Edge.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const { pathToFileURL } = require('node:url');
const { chromium } = require('../frontend/node_modules/playwright');
const root = path.resolve(__dirname, '..');
const executable = path.resolve(process.argv[2] || path.join(root, 'build/bin/ColorNinja-Studio.exe'));
const output = path.join(root, 'artifacts/ui-acceptance', String(Date.now()));
fs.mkdirSync(output, { recursive: true });
let server, browser, page;
const errors = [], checks = [], renders = [];
(async () => {
  const { defaults, emptyFilter } = await import(pathToFileURL(path.join(root, 'frontend/src/types.ts')));
  const options = structuredClone(defaults);
  options.hueforge.maxDepth = 1.44;
  options.hueforge.maxRuns = 8;
  options.colors = 4;
  fs.writeFileSync(path.join(output, 'settings.json'), JSON.stringify({ schemaVersion: 1, options, filter: emptyFilter, libraryPath: path.join(root, 'internal/engine/testdata/library.json'), preferences: { advanced: false, checkOnStartup: false, includePrereleases: false }, presets: [], recent: [] }));
  const log = fs.openSync(path.join(output, 'server.log'), 'w');
  const address = '127.0.0.1:48933';
  server = spawn(executable, ['-dev-server', address, '-config-dir', output], { windowsHide: true, stdio: ['ignore', log, log] });
  server.on('error', error => errors.push(error.message));
  let started = false;
  for (let i = 0; i < 100; i++) {
    try { if ((await fetch('http://' + address)).ok) { started = true; break; } } catch {}
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert(started, 'Application test server did not start');
  browser = await chromium.launch({ headless: true, channel: 'msedge' });
  page = await browser.newPage({ viewport: { width: 1440, height: 980 } });
  page.on('pageerror', error => errors.push(error.message));
  page.on('response', async response => { if (response.url().endsWith('/api/Process')) { const body = await response.json().catch(() => null); if (body?.result) renders.push(body.result); else if (body?.error) errors.push(body.error); } });
  await page.goto('http://' + address);
  const ready = async () => { await page.getByText('All changes rendered', { exact: true }).waitFor({ timeout: 120000 }); };
  const renderAfter = async action => {
    const response = page.waitForResponse(r=>r.url().endsWith('/api/Process'), {timeout:120000});
    await action();
    const body = await (await response).json();
    assert(!body.error, body.error);
    await ready();
  };
  await ready();
  await page.getByRole('checkbox', { name: 'Auto preview', exact: true }).uncheck();
  const workflow = page.getByRole('combobox', {name:'Processing mode'});
  assert.deepEqual(await workflow.locator('option').evaluateAll(nodes=>nodes.map(n=>n.value)), ['standard','color-pop','guided','stack']);
  checks.push('Only established workflows are available; new channel modes are paused');
  await workflow.selectOption('stack');
  await page.getByRole('button', { name: 'Generate preview', exact: true }).click();
  await ready();
  const groupRows = page.locator('[data-color-group]');
  const original = await groupRows.evaluateAll(nodes=>nodes.map(n=>n.dataset.colorGroup));
  assert(original.length >= 2, 'Suggested source color groups missing');
  await renderAfter(()=>groupRows.first().dragTo(groupRows.last()));
  const expected = [...original.slice(1), original[0]].join(',');
  assert.equal(renders.at(-1).options.hueforge.colorOrder, expected);
  assert.equal(renders.at(-1).result.stack.colorOrder.requested, expected);
  checks.push('Drag/drop triggers real backend recalculation with Auto preview off');
  const strength = page.getByRole('slider', {name:'Color order strength'});
  await renderAfter(()=>strength.fill('75'));
  assert.equal(renders.at(-1).result.stack.colorOrder.weight,75);
  checks.push('Order weight reaches the planner and the result report');
  await groupRows.last().getByRole('button', {name:/toward bottom/}).focus();
  await renderAfter(()=>page.keyboard.press('Enter'));
  const keyboardOrder = await groupRows.evaluateAll(nodes=>nodes.map(n=>n.dataset.colorGroup).join(','));
  assert.notEqual(keyboardOrder, expected);
  assert.equal(renders.at(-1).options.hueforge.colorOrder, keyboardOrder);
  checks.push('Keyboard accessible move buttons recalculate');
  await page.getByRole('button', { name: 'Layers', exact: true }).click();
  await page.getByRole('combobox', { name: 'Optical model', exact: true }).selectOption('hueforge-0.9.4.3-backlit-v1');
  assert.equal(await page.getByRole('spinbutton', { name: 'Backlit TD scale', exact: true }).inputValue(), '1.2');
  await page.getByRole('button', { name: 'Tune', exact: true }).click();
  await page.getByRole('button', { name: 'Generate preview', exact: true }).click();
  await ready();
  assert.equal(renders.at(-1).result.stack.model, 'hueforge-0.9.4.3-backlit-v1');
  checks.push('Backlit controls and optical result');
  for (const [width, height] of [[1440, 980], [1080, 720], [960, 720]]) {
    await page.setViewportSize({ width, height });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'Horizontal page overflow at ' + width);
    await page.screenshot({ path: path.join(output, `layout-${width}.png`), fullPage: true });
  }
  checks.push('Responsive layout at 1440, 1080 and 960 pixels');
  await page.setViewportSize({ width: 1440, height: 980 });
  // A separate settings change still blocks stale exports when Auto preview is off.
  await page.getByRole('button', {name:'Layers',exact:true}).click();
  await page.getByRole('combobox', {name:'Optical model',exact:true}).selectOption('hueforge-0.9.4.3-frontlit-v1');
  await page.getByRole('button', { name: 'Export', exact: true }).click();
  assert(await page.locator('dialog[open]').getByRole('button', { name: /Export (PNG|HFP|file)/ }).isDisabled());
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Undo settings (Ctrl+Z)', exact: true }).click();
  checks.push('Stale export guard, Escape and undo');
  await page.reload();
  await page.getByRole('combobox', { name: 'Processing mode' }).waitFor();
  await page.getByRole('button', {name:'Tune',exact:true}).click();
  assert.equal(await page.getByRole('combobox', { name: 'Processing mode' }).inputValue(), 'stack');
  assert.equal(await strength.inputValue(),'75');
  await ready(); // Startup automatically renders the restored settings.
  await page.getByRole('checkbox', {name:'Auto preview',exact:true}).uncheck();
  assert.equal(renders.at(-1).options.hueforge.colorOrder, keyboardOrder);
  checks.push('Order and strength persist after restart');
  await renderAfter(()=>page.getByRole('button', {name:'Reset order',exact:true}).click());
  assert.equal(renders.at(-1).result.stack.colorOrder.requested,'');
  checks.push('Reset restores automatic color matching');
  await workflow.selectOption('color-pop');
  assert.equal(await page.getByRole('region',{name:'Preferred color order',exact:true}).count(),0);
  await page.getByRole('button', {name:'Try demo',exact:true}).click();
  await page.getByText('Color Pop · Red poppy', {exact:true}).waitFor();
  await page.getByRole('combobox', {name:'Color Pop output',exact:true}).selectOption('stack');
  await renderAfter(()=>page.getByRole('button', {name:'Generate preview',exact:true}).click());
  assert(renders.at(-1).result.colorPop);
  assert(!renders.at(-1).result.stack.colorOrder);
  checks.push('Color Pop remains separate and renders successfully');
  assert.deepEqual(errors, []);
})().then(() => { fs.writeFileSync(path.join(output, 'verification.json'), JSON.stringify({ executable, checks, errors, renders: renders.length }, null, 2)); console.log(JSON.stringify({ output, checks })); }).catch(async error => { console.error(error); fs.writeFileSync(path.join(output, 'failure.txt'), String(error.stack)); fs.writeFileSync(path.join(output,'failure-details.json'),JSON.stringify({checks,errors,renders:renders.length},null,2)); if(page) { await page.screenshot({path:path.join(output,'failure.png'),fullPage:true}).catch(()=>{});fs.writeFileSync(path.join(output,'page.txt'),await page.locator('body').innerText()); } process.exitCode = 1; }).finally(async () => { if (browser) await browser.close(); if (server) server.kill(); });
