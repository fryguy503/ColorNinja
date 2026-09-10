// Optional-border acceptance against the compiled desktop app's real service.
const fs = require("node:fs"),
  path = require("node:path"),
  assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const { pathToFileURL } = require("node:url");
const { chromium } = require("../frontend/node_modules/playwright");
const root = path.resolve(__dirname, "..");
const output = path.join(
  root,
  "artifacts/border-acceptance",
  String(Date.now()),
);
fs.mkdirSync(output, { recursive: true });
let server, browser, page;
const checks = [],
  errors = [];
(async () => {
  const { defaults, emptyFilter } = await import(
    pathToFileURL(path.join(root, "frontend/src/types.ts"))
  );
  const options = structuredClone(defaults);
  options.mode = "stack";
  options.colors = 3;
  options.hueforge.maxDepth = 0.96;
  const libraryPath = path.join(root, "internal/engine/testdata/library.json");
  fs.writeFileSync(
    path.join(output, "settings.json"),
    JSON.stringify({
      schemaVersion: 1,
      options,
      filter: emptyFilter,
      libraryPath,
      preferences: { checkOnStartup: false, exportProfile: false },
      presets: [],
      recent: [],
    }),
  );
  const log = fs.openSync(path.join(output, "server.log"), "w"),
    url = "http://127.0.0.1:48935";
  server = spawn(
    path.resolve(
      process.argv[2] || path.join(root, "build/bin/ColorNinja-Studio.exe"),
    ),
    ["-dev-server", "127.0.0.1:48935", "-config-dir", output],
    { windowsHide: true, stdio: ["ignore", log, log] },
  );
  const api = async (method, ...args) => {
    const res = await fetch(url + "/api/" + method, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-ColorNinja": "studio" },
      body: JSON.stringify(args),
    });
    const b = await res.json();
    if (b.error) throw Error(b.error);
    return b.result;
  };
  for (let i = 0; i < 100; i++) {
    try {
      await api("Version");
      break;
    } catch (e) {
      if (i === 99) throw e;
      await new Promise((r) => setTimeout(r, 100));
    }
  }
  const snap = await api(
    "LoadImage",
    path.join(root, "internal/engine/testdata/gradient.png"),
  );
  const base = await api("Process", {
    id: 10,
    revision: snap.source.revision,
    options,
    filter: emptyFilter,
    libraryPath,
  });
  browser = await chromium.launch({ channel: "msedge", headless: true });
  page = await browser.newPage({ viewport: { width: 1280, height: 1000 } });
  page.on("pageerror", (e) => errors.push(String(e)));
  await page.goto(url);
  await page.getByText("All changes rendered", { exact: true }).waitFor();
  await page
    .getByRole("checkbox", { name: "Auto preview", exact: true })
    .uncheck();
  await page.getByRole("button", { name: "Export", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Export", exact: true });
  await dialog
    .getByRole("combobox", { name: "Export format" })
    .selectOption("hfp");
  const toggle = dialog.getByRole("checkbox", {
    name: "Add a border",
    exact: true,
  });
  assert.equal(await toggle.isChecked(), false);
  await toggle.check();
  assert(
    await dialog
      .getByRole("button", { name: "Export HFP", exact: true })
      .isDisabled(),
  );
  const refresh = async () => {
    const response = page.waitForResponse((r) =>
      r.url().endsWith("/api/Process"),
    );
    await dialog
      .getByRole("button", { name: "Refresh preview", exact: true })
      .click();
    const b = await (await response).json();
    assert(!b.error, b.error);
    await dialog.getByLabel("HueForge border preview").waitFor();
    return b.result;
  };
  let p = await refresh();
  assert.equal(p.result.rgbaSHA256, base.result.rgbaSHA256);
  assert.equal(p.result.surfaceView.border.placement, "external");
  await page.screenshot({
    path: path.join(output, "external.png"),
    fullPage: true,
  });
  checks.push(
    "Border defaults off; enabling requires refresh; external frame preserves image pixels",
  );
  await dialog
    .getByRole("combobox", { name: "Border placement" })
    .selectOption("internal");
  await dialog.getByRole("checkbox", { name: "Match image depth" }).uncheck();
  await dialog
    .getByRole("spinbutton", { name: "Border depth", exact: true })
    .fill("4");
  await dialog
    .getByRole("spinbutton", { name: "Border depth", exact: true })
    .press("Tab");
  p = await refresh();
  assert.equal(p.result.rgbaSHA256, base.result.rgbaSHA256);
  const b = p.result.surfaceView.border;
  assert.equal(b.placement, "internal");
  assert.equal(b.printHeightMm, 4);
  assert(b.extraLayers > 0);
  assert(b.imageWidthMm < 200);
  assert.equal(p.result.surfaceView.widthMm, b.imageWidthMm);
  await page.screenshot({
    path: path.join(output, "internal-tall.png"),
    fullPage: true,
  });
  const hfp = path.join(output, "frame.hfp");
  await api("Export", "hfp", p.id, p.revision, hfp);
  const doc = JSON.parse(fs.readFileSync(hfp));
  assert.equal(doc.borderless, false);
  assert.equal(doc.external_border, false);
  assert.equal(doc.border_height, 4);
  assert.equal(doc.slider_values.at(-1), b.topLayer);
  const project = path.join(output, "frame.colorninja");
  await api(
    "SaveProject",
    {
      id: p.id,
      revision: p.revision,
      options: p.options,
      filter: emptyFilter,
      libraryPath,
    },
    project,
  );
  checks.push(
    "Internal placement scales image metrics; custom tall depth exports and survives project reopening",
  );
  await page.setViewportSize({ width: 560, height: 780 });
  await dialog.locator(".border-controls").scrollIntoViewIfNeeded();
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  );
  assert(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth + 1));
  await page.screenshot({
    path: path.join(output, "compact.png"),
    fullPage: true,
  });
  await toggle.uncheck();
  p = await refreshWithoutBorder();
  async function refreshWithoutBorder() {
    const response = page.waitForResponse((r) =>
      r.url().endsWith("/api/Process"),
    );
    await dialog
      .getByRole("button", { name: "Refresh preview", exact: true })
      .click();
    const b = await (await response).json();
    assert(!b.error, b.error);
    return b.result;
  }
  assert(!p.result.surfaceView.border);
  assert.equal(p.result.rgbaSHA256, base.result.rgbaSHA256);
  const restored = await api("OpenProject", project);
  assert.equal(restored.preview.result.surfaceView.border.heightMm, 4);
  checks.push(
    "Compact export dialog has no horizontal overflow; disabling restores borderless output",
  );
  assert.deepEqual(errors, []);
  fs.writeFileSync(
    path.join(output, "summary.json"),
    JSON.stringify({ checks, errors, hfp, project }, null, 2),
  );
  console.log(JSON.stringify({ output, checks }, null, 2));
})()
  .catch(async (e) => {
    console.error(e);
    fs.writeFileSync(path.join(output, "failure.txt"), String(e));
    if (page)
      await page
        .screenshot({ path: path.join(output, "failure.png"), fullPage: true })
        .catch(() => {});
    process.exitCode = 1;
  })
  .finally(async () => {
    if (browser) await browser.close();
    if (server) server.kill();
  });
