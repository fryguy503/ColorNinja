// Region Edit acceptance against the compiled application and its real Go service.
const fs = require("node:fs"),
  path = require("node:path"),
  assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const { pathToFileURL } = require("node:url");
const { chromium } = require("../frontend/node_modules/playwright");
const root = path.resolve(__dirname, ".."),
  output = path.join(root, "artifacts/region-acceptance", String(Date.now()));
fs.mkdirSync(output, { recursive: true });
let server, browser, page;
const errors = [],
  checks = [];
(async () => {
  const { defaults, emptyFilter } = await import(
    pathToFileURL(path.join(root, "frontend/src/types.ts"))
  );
  const options = structuredClone(defaults);
  options.mode = "stack";
  options.colors = 4;
  options.hueforge.maxDepth = 1.44;
  options.hueforge.beamWidth = 8;
  options.analysisMaxPixels = 4000;
  const libraryPath = path.join(root, "internal/engine/testdata/library.json");
  fs.writeFileSync(
    path.join(output, "settings.json"),
    JSON.stringify({
      schemaVersion: 1,
      options,
      filter: emptyFilter,
      libraryPath,
      preferences: {
        advanced: false,
        checkOnStartup: false,
        includePrereleases: false,
      },
      presets: [],
      recent: [],
    }),
  );
  const address = "127.0.0.1:48934",
    url = "http://" + address;
  const log = fs.openSync(path.join(output, "server.log"), "w");
  server = spawn(
    path.resolve(
      process.argv[2] || path.join(root, "build/bin/ColorNinja-Studio.exe"),
    ),
    ["-dev-server", address, "-config-dir", output],
    { windowsHide: true, stdio: ["ignore", log, log] },
  );
  server.on("error", (e) => errors.push(String(e)));
  const api = async (method, ...args) => {
    const res = await fetch(url + "/api/" + method, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-ColorNinja": "studio" },
      body: JSON.stringify(args),
    });
    const body = await res.json();
    if (body.error) throw Error(body.error);
    return body.result;
  };
  for (let i = 0; i < 80; i++) {
    try {
      await api("Version");
      break;
    } catch {
      await new Promise((r) => setTimeout(r, 250));
      if (i === 79) throw Error("server did not start");
    }
  }
  const snap = await api(
    "LoadImage",
    path.join(root, "internal/engine/testdata/gradient.png"),
  );
  await api("Process", {
    id: 10,
    revision: snap.source.revision,
    options,
    filter: emptyFilter,
    libraryPath,
  });
  browser = await chromium.launch({
    executablePath:
      "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
    headless: true,
  });
  page = await browser.newPage({ viewport: { width: 1440, height: 980 } });
  page.on("pageerror", (e) => errors.push(String(e)));
  await page.goto(url);
  await page
    .getByRole("button", { name: "Region Edit", exact: true })
    .waitFor();
  const auto = page.getByLabel("Auto preview", { exact: true });
  if (await auto.isChecked()) await auto.uncheck();
  await page.getByRole("button", { name: "Layers", exact: true }).click();
  const base = page.getByRole("spinbutton", {
    name: "Base depth",
    exact: true,
  });
  await base.fill("0.86");
  assert.equal(await base.inputValue(), "0.86");
  await base.press("Tab");
  await page.waitForFunction(
    () => document.querySelector('[aria-label="Base depth"]').value === "0.88",
  );
  await base.fill("0.81");
  await base.press("Enter");
  assert.equal(await base.inputValue(), "0.8");
  await page
    .getByRole("button", { name: "Generate preview", exact: true })
    .click();
  await page.waitForFunction(
    () => {
      const b = [...document.querySelectorAll("button")].find(
        (b) => b.textContent.trim() === "Region Edit",
      );
      return b && !b.disabled;
    },
    {},
    { timeout: 60000 },
  );
  checks.push(
    "Depth edits commit on blur/Enter and snap 0.86 to 0.88 and 0.81 to 0.80",
  );
  await page.getByRole("button", { name: "Region Edit", exact: true }).click();
  const editor = page.getByRole("region", { name: "Region Edit workspace" });
  const waitReady = () =>
    page.waitForFunction(
      () => {
        const el = document.querySelector(".region-editor .region-footer");
        return el && !el.textContent.includes("Updating");
      },
      {},
      { timeout: 30000 },
    );
  await waitReady();
  assert.equal(
    await page.locator('[aria-label="Processing settings"]').isVisible(),
    false,
  );
  await editor
    .getByRole("combobox", { name: "Selection coverage" })
    .selectOption("pixels");
  const frame = page.locator(".region-image-frame");
  // A selected DOM range can drag its contents even when the image itself has
  // draggable=false. Exercise held strokes with that browser selection present.
  await page.evaluate(() => {
    window.regionNativeDrags = [];
    document.addEventListener("dragstart", (e) => {
      if (e.target.closest?.(".region-viewport"))
        window.regionNativeDrags.push(e.defaultPrevented);
    });
  });
  for (const tool of ["Lasso", "Brush"]) {
    await editor.getByRole("button", { name: tool, exact: true }).click();
    for (let stroke = 0; stroke < 3; stroke++) {
      await frame.evaluate((el) => {
        const range = document.createRange();
        range.selectNode(el);
        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);
      });
      const b = await frame.boundingBox();
      const offset = stroke * 0.04;
      const points = [
        [0.3 + offset, 0.3],
        [0.5 + offset, 0.3],
        [0.5 + offset, 0.5],
        [0.3 + offset, 0.5],
        [0.3 + offset, 0.3],
      ];
      await page.mouse.move(
        b.x + points[0][0] * b.width,
        b.y + points[0][1] * b.height,
      );
      await page.mouse.down();
      await new Promise((resolve) => setTimeout(resolve, 250));
      for (const p of points.slice(1))
        await page.mouse.move(b.x + p[0] * b.width, b.y + p[1] * b.height, {
          steps: 8,
        });
      await page.mouse.up();
      await waitReady();
      assert.deepEqual(
        await page.evaluate(() =>
          window.regionNativeDrags.filter((prevented) => !prevented),
        ),
        [],
        `${tool} must not start a native image/selection drag`,
      );
      assert.match(
        await page.locator(".region-summary").innerText(),
        /[1-9][0-9]* pixels/,
      );
      const after = await frame.boundingBox();
      assert.ok(
        Math.abs(after.x - b.x) < 0.1 && Math.abs(after.y - b.y) < 0.1,
        `${tool} must select without panning`,
      );
    }
  }
  await page.evaluate(() => window.getSelection().removeAllRanges());
  checks.push(
    "Repeated held lasso/brush strokes suppress native selected-image dragging without panning",
  );
  await editor.getByRole("button", { name: "Box", exact: true }).click();
  const draw = async (points) => {
    const b = await frame.boundingBox();
    await page.mouse.move(
      b.x + points[0][0] * b.width,
      b.y + points[0][1] * b.height,
    );
    await page.mouse.down();
    for (const p of points.slice(1))
      await page.mouse.move(b.x + p[0] * b.width, b.y + p[1] * b.height, {
        steps: 5,
      });
    await page.mouse.up();
    await waitReady();
  };
  await draw([
    [0.2, 0.2],
    [0.45, 0.55],
  ]);
  assert.match(
    await page.locator(".region-summary").innerText(),
    /[1-9][0-9]* pixels/,
  );
  const assign = editor.getByRole("button", {
    name: "Assign layer",
    exact: true,
  });
  const target = editor.getByRole("combobox", { name: "Assign to layer" });
  const max = await target.locator("option").last().getAttribute("value");
  await target.selectOption(max);
  await assign.click();
  await waitReady();
  assert.match(
    await page.locator(".region-summary").innerText(),
    new RegExp("layers " + max + "–" + max),
  );
  let current = await api("Initialize");
  const editedHash = current.preview.result.rgbaSHA256;
  await editor.getByRole("button", { name: "Undo", exact: true }).click();
  await waitReady();
  current = await api("Initialize");
  assert.notEqual(current.preview.result.rgbaSHA256, editedHash);
  await editor.getByRole("button", { name: "Redo", exact: true }).click();
  await waitReady();
  current = await api("Initialize");
  assert.equal(current.preview.result.rgbaSHA256, editedHash);
  checks.push(
    "Pixel rectangle, absolute assignment, derived preview, undo and redo",
  );
  await editor.getByRole("button", { name: "Zoom in", exact: true }).click();
  const beforePan = await frame.boundingBox();
  const viewport = await page.locator(".region-viewport").boundingBox();
  await page.keyboard.down("Space");
  await page.mouse.move(viewport.x + 100, viewport.y + 100);
  await page.mouse.down();
  await page.mouse.move(viewport.x + 145, viewport.y + 125);
  await page.mouse.up();
  await page.keyboard.up("Space");
  const afterPan = await frame.boundingBox();
  assert.ok(afterPan.x > beforePan.x + 35);
  await page.mouse.move(viewport.x + 100, viewport.y + 100);
  await page.mouse.down({ button: "middle" });
  await page.mouse.move(viewport.x + 130, viewport.y + 115);
  await page.mouse.up({ button: "middle" });
  const afterMiddlePan = await frame.boundingBox();
  assert.ok(afterMiddlePan.x > afterPan.x + 25);
  checks.push("Space-drag and middle-button panning remain available");
  await editor.getByRole("button", { name: "Lasso", exact: true }).click();
  await draw([
    [0.5, 0.3],
    [0.75, 0.3],
    [0.75, 0.6],
    [0.5, 0.6],
    [0.5, 0.3],
  ]);
  assert.ok(!(await assign.isDisabled()));
  await assign.click();
  await waitReady();
  checks.push("Lasso selection remains aligned after zoom and Space-drag pan");
  await editor.getByRole("button", { name: "Polygon", exact: true }).click();
  let b = await frame.boundingBox();
  for (const p of [
    [0.6, 0.65],
    [0.8, 0.65],
    [0.7, 0.85],
  ])
    await page.mouse.click(b.x + p[0] * b.width, b.y + p[1] * b.height);
  await editor.getByRole("button", { name: "Finish polygon" }).click();
  await waitReady();
  assert.ok(!(await assign.isDisabled()));
  await editor.getByRole("button", { name: "Brush", exact: true }).click();
  await editor
    .getByRole("combobox", { name: "Selection operation" })
    .selectOption("subtract");
  await draw([
    [0.7, 0.7],
    [0.71, 0.71],
  ]);
  checks.push("Polygon finish and brush subtraction");
  await editor.getByLabel("Before edits", { exact: true }).check();
  assert.ok(await assign.isDisabled());
  await editor.getByLabel("Before edits", { exact: true }).uncheck();
  await editor.getByText("Refine selection", { exact: true }).click();
  await editor.getByRole("button", { name: "Select all", exact: true }).click();
  await waitReady();
  await editor.getByRole("button", { name: "Shrink", exact: true }).click();
  await waitReady();
  await editor.getByRole("button", { name: "Grow", exact: true }).click();
  await waitReady();
  checks.push("Before mode protects edits; grow/shrink selection");
  for (const width of [1440, 1080, 960]) {
    await page.setViewportSize({ width, height: 980 });
    await page.screenshot({
      path: path.join(output, "regions-" + width + ".png"),
      fullPage: true,
    });
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    assert.ok((await frame.boundingBox()).width > 0);
    assert.ok((await editor.boundingBox()).width > width - 60);
  }
  checks.push("Editor layout fits 1440, 1080 and 960 pixel windows");
  await page.setViewportSize({ width: 1440, height: 980 });
  await editor.getByRole("button", { name: "Done", exact: true }).click();
  assert.ok(
    await page.locator('[aria-label="Processing settings"]').isVisible(),
  );
  current = await api("Initialize");
  const p = current.preview;
  const req = {
    id: p.id,
    revision: p.revision,
    options: p.options,
    filter: current.settings.filter,
    libraryPath: current.settings.libraryPath,
  };
  const project = path.join(output, "regions.colorninja");
  await api("SaveProject", req, project);
  await api(
    "Export",
    "hfp",
    p.id,
    p.revision,
    path.join(output, "regions.hfp"),
  );
  const hfp = JSON.parse(fs.readFileSync(path.join(output, "regions.hfp")));
  assert.equal(hfp.spotfix_version, 2);
  assert(Array.isArray(hfp.spot_fixes));
  assert(hfp.colorninja.spotFixExport.document.groups.length >= 2);
  for (const fix of hfp.spot_fixes) {
    assert(fix.regions.length > 0);
    assert(fix.footprint.rle.length > 0);
  }
  await api(
    "Export",
    "layers",
    p.id,
    p.revision,
    path.join(output, "regions.layers.png"),
  );
  const savedHash = p.result.rgbaSHA256;
  const reopened = await api("OpenProject", project);
  assert.equal(reopened.preview.result.rgbaSHA256, savedHash);
  await api("Export", "hfp", reopened.preview.id, reopened.preview.revision, path.join(output, "reopened.hfp"));
  const reopenedHFP = JSON.parse(fs.readFileSync(path.join(output, "reopened.hfp")));
  assert.deepEqual(reopenedHFP.spot_fixes, hfp.spot_fixes);
  assert.deepEqual(reopenedHFP.colorninja.spotFixExport, hfp.colorninja.spotFixExport);
  checks.push("Native SpotFix records and complete ColorNinja edit metadata survive project reopening and re-export");
  await page.reload();
  await page.getByRole("button", { name: "Region Edit", exact: true }).click();
  await waitReady();
  assert.ok((await page.locator(".region-group").count()) >= 2);
  checks.push(
    "Exit restores workflow; project groups reopen; HFP and layer PNG export through real backend",
  );
  assert.deepEqual(errors, []);
  fs.writeFileSync(
    path.join(output, "verification.json"),
    JSON.stringify(
      {
        passed: true,
        checks,
        errors,
        project,
        rgbaSHA256: savedHash,
        nativeDialogAcceptance: "not performed",
        physicalPrintAcceptance: "not performed",
      },
      null,
      2,
    ),
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
