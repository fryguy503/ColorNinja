import { useEffect, useRef, useState } from "react";
import { Usb, RefreshCw, Download } from "lucide-react";
import { desktop, invoke } from "./bridge";
import { StudioDialog } from "./StudioDialog";
import type {
  DevicePort,
  DeviceState,
  DeviceResponse,
  Reading,
  RecoveryPlan,
} from "./filamentTypes";

const settingLabels: Record<string, string> = {
  display_type: "Display controller",
  display_flip: "Flip display",
  color_rgb: "Show RGB values instead of hex",
  continuous_mode: "Continuous TD measurements",
  continuous_color: "Continuous color measurements",
  continuous_start_time: "Delay before continuous scanning (seconds)",
  sample_rate: "Scan interval (seconds)",
  sample_distance: "Sample distance (mm)",
  RGB_Enabled: "Enable color sensor",
  optical_button: "Use optical button",
  output_raw_rgb: "Include raw RGB readings",
  scale_adjustment: "Enable sensor scale adjustment",
  scale_output: "Scale color output",
  screen_mirror: "Mirror device display",
};
function download(name: string, text: string, type = "text/plain") {
  const url = URL.createObjectURL(new Blob([text], { type })),
    a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function resource(kind: "td1" | "calibration") {
  if (desktop) void invoke("OpenFilamentResource", kind);
  else
    window.open(
      kind === "td1"
        ? "https://ajax-3d.com/resources/"
        : "https://ajax-3d.com/td1-td1s-color-calibration-guide/",
      "_blank",
      "noopener,noreferrer",
    );
}
function savedIgnored(serial: string): string[] {
  try {
    const value = JSON.parse(
      localStorage.getItem(`colorninja.td1.ignored.${serial}`) ?? "[]",
    );
    return Array.isArray(value)
      ? value.filter((s) => typeof s === "string")
      : [];
  } catch {
    return [];
  }
}
export function TD1Panel({
  onReading,
  onUse,
  selected,
}: {
  onReading: (r: Reading) => void;
  onUse: (r: Reading, create: boolean) => void;
  selected: string;
}) {
  const [ports, setPorts] = useState<DevicePort[]>([]),
    [port, setPort] = useState(""),
    [state, setState] = useState<DeviceState>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [settings, setSettings] = useState<Record<string, string>>(),
    [original, setOriginal] = useState<Record<string, string>>(),
    [diagnostic, setDiagnostic] = useState(""),
    [deviceErrors, setDeviceErrors] = useState<string[]>([]),
    [ignoredErrors, setIgnoredErrors] = useState<string[]>([]),
    [autoBackup, setAutoBackup] = useState(
      () => localStorage.getItem("colorninja.td1.autoBackup") !== "false",
    ),
    [firmware, setFirmware] = useState<DeviceResponse>(),
    [recovery, setRecovery] = useState<RecoveryPlan>(),
    [recoveryPath, setRecoveryPath] = useState(""),
    [recoveryBackup, setRecoveryBackup] = useState(""),
    [recoveryVolume, setRecoveryVolume] = useState(""),
    [advanced, setAdvanced] = useState(
      () => localStorage.getItem("colorninja.td1.advanced") === "true",
    ),
    [rgb, setRGB] = useState([0, 0, 0]),
    [confirmation, setConfirmation] = useState<{
      title: string;
      text: string;
      run: () => void;
    } | null>(null);
  const lastReading = useRef(0);
  const accept = (next: DeviceState) => {
    setState(next);
    if (next.latest && next.latest.id !== lastReading.current) {
      lastReading.current = next.latest.id;
      onReading(next.latest);
    }
  };
  const refresh = async () => {
    try {
      const ps = await invoke<DevicePort[]>("TD1Ports");
      setPorts(ps);
      setPort((old) =>
        ps.some((p) => p.name === old) ? old : (ps[0]?.name ?? ""),
      );
    } catch (e) {
      setError(String(e));
    }
  };
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const next = await invoke<DeviceState>("TD1State");
        if (!stopped) accept(next);
      } catch (e) {
        if (!stopped) setError(String(e));
      } finally {
        if (!stopped) timer = setTimeout(poll, 1000);
      }
    };
    void refresh();
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, []);
  const operation = async (
    action: string,
    extra: Record<string, unknown> = {},
  ) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const r = await invoke<DeviceResponse>("TD1Operation", {
        action,
        ...extra,
      });
      accept(r.state);
      setNotice(r.text || "Command sent. Follow the TD1/S display.");
      if (r.settings) {
        setSettings(r.settings);
        setOriginal(r.settings);
      }
      if (r.firmware || r.plan) setFirmware(r);
      if (r.recovery) setRecovery(r.recovery);
      if (action === "backup" && r.path) setRecoveryBackup(r.path);
      if (action === "download-recovery" && r.path) {
        setRecoveryPath(r.path);
        setRecovery(undefined);
      }
      if (action === "apply-recovery") setRecovery(undefined);
      if (r.files?.length) setDiagnostic(r.files.join("\n"));
      if (
        [
          "errors",
          "boot-info",
          "rgb-offsets",
          "empty-lux",
          "license-request",
        ].includes(action)
      )
        setDiagnostic(r.text);
      if (action === "errors")
        setDeviceErrors([
          ...new Set(
            r.text
              .split(/\r?\n/)
              .map((s) => s.trim())
              .filter(Boolean),
          ),
        ]);
      if (
        [
          "save-settings",
          "reboot",
          "apply-firmware",
          "apply-license",
          "restore-backup",
          "bootloader",
        ].includes(action)
      ) {
        setSettings(undefined);
        setOriginal(undefined);
      }
      return r;
    } catch (e) {
      setError(String(e));
      return undefined;
    } finally {
      setBusy(false);
    }
  };
  const confirm = (
    title: string,
    text: string,
    action: string,
    extra: Record<string, unknown> = {},
  ) =>
    setConfirmation({ title, text, run: () => void operation(action, extra) });
  const connect = async () => {
    setBusy(true);
    setError("");
    try {
      const connected = await invoke<DeviceState>("ConnectTD1", port);
      accept(connected);
      setIgnoredErrors(savedIgnored(connected.port.serial));
      setDeviceErrors([]);
      setSettings(undefined);
      setOriginal(undefined);
      const version = await invoke<DeviceResponse>("TD1Operation", {
        action: "version",
      });
      accept(version.state);
      const errors = await invoke<DeviceResponse>("TD1Operation", {
        action: "errors",
      });
      setDeviceErrors([
        ...new Set(
          errors.text
            .split(/\r?\n/)
            .map((s) => s.trim())
            .filter(Boolean),
        ),
      ]);
      if (autoBackup) {
        const backup = await invoke<DeviceResponse>("TD1Operation", {
          action: "backup",
        });
        setDiagnostic(backup.files?.join("\n") ?? "");
        if (backup.path) setRecoveryBackup(backup.path);
      }
      setNotice(
        "Connected. Insert filament to take a measurement; follow the device display.",
      );
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  const choose = async (kind: string) => {
    if (!desktop) {
      setError(
        "File selection for device maintenance is available in the desktop app.",
      );
      return "";
    }
    try {
      return await invoke<string>("ChooseTD1File", kind);
    } catch (e) {
      setError(String(e));
      return "";
    }
  };
  const disabled = busy || !!state?.busy,
    connected = !!state?.connected,
    unavailable = disabled || !connected;
  const changes = Object.fromEntries(
    Object.entries(settings ?? {}).filter(([k, v]) => v !== original?.[k]),
  );
  const dismissError = (line: string) => {
    const next = [...ignoredErrors, line];
    setIgnoredErrors(next);
    localStorage.setItem(
      `colorninja.td1.ignored.${state?.port.serial ?? ""}`,
      JSON.stringify(next),
    );
  };
  return (
    <section className="td1-panel">
      <p className="field-help">
        Connect TD1 or TD1S with a USB data cable. Close HueForge and other TD1
        applications so ColorNinja can use the device.
      </p>
      <div className="filament-toolbar">
        <Usb size={18} />
        <select
          aria-label="TD1 USB device"
          disabled={connected || disabled}
          value={port}
          onChange={(e) => setPort(e.target.value)}
        >
          {!ports.length && <option value="">No TD1/S found</option>}
          {ports.map((p) => (
            <option key={p.name} value={p.name}>
              {p.product || "TD1 / TD1S"} · {p.name} · {p.serial}
            </option>
          ))}
        </select>
        <button
          className="icon-button"
          aria-label="Refresh TD1 USB devices"
          disabled={disabled || connected}
          onClick={() => void refresh()}
        >
          <RefreshCw size={17} />
        </button>
        {connected ? (
          <button
            className="button secondary"
            onClick={async () => {
              await invoke("DisconnectTD1");
              setSettings(undefined);
              setOriginal(undefined);
              accept(await invoke<DeviceState>("TD1State"));
            }}
          >
            Disconnect
          </button>
        ) : (
          <button
            className="button primary"
            disabled={disabled || !port}
            onClick={() => void connect()}
          >
            Connect
          </button>
        )}
      </div>
      <label className="check-field">
        <input
          type="checkbox"
          checked={autoBackup}
          onChange={(e) => {
            setAutoBackup(e.target.checked);
            localStorage.setItem(
              "colorninja.td1.autoBackup",
              String(e.target.checked),
            );
          }}
        />{" "}
        Back up license and settings when connecting
      </label>
      {!!deviceErrors.filter((line) => !ignoredErrors.includes(line))
        .length && (
        <div className="dialog-error" role="alert">
          <strong>TD1/S reported errors</strong>
          {deviceErrors
            .filter((line) => !ignoredErrors.includes(line))
            .map((line) => (
              <div key={line}>
                <p>{line}</p>
                <button
                  className="text-button"
                  onClick={() => dismissError(line)}
                >
                  Do not warn about this exact message again
                </button>
              </div>
            ))}
        </div>
      )}
      {!ports.length && !connected && (
        <p className="field-help">
          No compatible USB device is visible. Check its power/data cable, then
          refresh. On Linux, your account needs serial-port access.
        </p>
      )}
      {error && (
        <p className="dialog-error" role="alert">
          {error}
        </p>
      )}
      {state?.error && !error && (
        <p className="dialog-error" role="alert">
          {state.error}
        </p>
      )}
      {notice && (
        <p className="filament-notice" role="status">
          {notice}
        </p>
      )}
      <div className="td1-status">
        <strong>
          {connected ? "Connected" : "Disconnected"}
          {state?.busy ? " · Working…" : ""}
        </strong>
        <span>{state?.status}</span>
        {state?.identity && (
          <small>
            {state.identity} · Serial {state.port.serial}
          </small>
        )}
        {state?.version && <small>{state.version}</small>}
      </div>
      {!!state?.display?.length && (
        <div className="td1-display">
          <small>Device display</small>
          <svg
            role="img"
            aria-label="TD1 mirrored display"
            viewBox="0 0 128 64"
            style={{ display: "block", width: 320, maxWidth: "100%" }}
          >
            {state.display.map((d) => (
              <text
                key={`${d.x}-${d.y}`}
                x={d.x}
                y={d.y + 7}
                fontSize="7"
                fontFamily="monospace"
                fill="white"
              >
                {d.text}
              </text>
            ))}
          </svg>
        </div>
      )}
      {state?.latest && (
        <div className="td1-measurement">
          <span
            className="filament-color"
            style={{ background: state.latest.color }}
          />
          <div>
            <strong>
              {state.latest.td} <small>mm TD</small>
            </strong>
            <span>
              {state.latest.color} ·{" "}
              {new Date(state.latest.capturedAt).toLocaleTimeString()}
            </span>
          </div>
          <button
            className="button primary"
            disabled={disabled}
            onClick={() => onUse(state.latest!, false)}
          >
            Use TD for {selected}
          </button>
          <button
            className="button secondary"
            disabled={disabled}
            onClick={() => onUse(state.latest!, true)}
          >
            Add as new filament
          </button>
        </div>
      )}
      <div className="filament-toolbar">
        <button
          className="button secondary"
          disabled={unavailable}
          onClick={() => void operation("version")}
        >
          Read firmware version
        </button>
        <button
          className="button secondary"
          disabled={unavailable}
          onClick={() => void operation("settings")}
        >
          Load device settings
        </button>
        <button
          className="button secondary"
          disabled={unavailable}
          onClick={() => void operation("glow")}
        >
          Read glow value
        </button>
      </div>
      {settings && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            confirm(
              "Save TD1 settings",
              "Apply these device settings? The TD1/S will restart. Reconnect after it finishes.",
              "save-settings",
              { values: changes },
            );
          }}
        >
          <h3>Device settings</h3>
          <div className="td1-settings">
            {Object.entries(settings).map(([key, value]) =>
              value === "True" || value === "False" ? (
                <label className="check-field" key={key}>
                  <input
                    type="checkbox"
                    disabled={unavailable}
                    checked={value === "True"}
                    onChange={(e) =>
                      setSettings({
                        ...settings,
                        [key]: e.target.checked ? "True" : "False",
                      })
                    }
                  />
                  {settingLabels[key] ?? key}
                </label>
              ) : (
                <label className="select-field" key={key}>
                  {settingLabels[key] ?? key}
                  {key === "display_type" ? (
                    <select
                      disabled={unavailable}
                      value={value.replace(/["']/g, "")}
                      onChange={(e) =>
                        setSettings({ ...settings, [key]: e.target.value })
                      }
                    >
                      <option>SSD1306</option>
                      <option>SH1106</option>
                    </select>
                  ) : (
                    <input
                      type="number"
                      min={key === "sample_distance" ? "0.1" : "1"}
                      max={key === "sample_distance" ? "10" : "60"}
                      step={key === "sample_distance" ? "0.01" : "1"}
                      disabled={unavailable}
                      value={value}
                      onChange={(e) =>
                        setSettings({ ...settings, [key]: e.target.value })
                      }
                    />
                  )}
                </label>
              ),
            )}
          </div>
          <p className="field-help">
            Only settings supported by this firmware are shown. Continuous color
            requires continuous TD.
          </p>
          <button
            className="button primary"
            disabled={unavailable || !Object.keys(changes).length}
          >
            Save device settings
          </button>
        </form>
      )}
      <details className="td1-history">
        <summary>Measurement history ({state?.readings.length ?? 0})</summary>
        <div className="filament-toolbar">
          <button
            className="text-button"
            disabled={!state?.readings.length}
            onClick={() =>
              download(
                "td1-measurements.csv",
                "capturedAt,serial,td,color\n" +
                  (state?.readings ?? [])
                    .map((r) =>
                      [r.capturedAt, r.serial, String(r.td), r.color]
                        .map((v) => '"' + v.replaceAll('"', '""') + '"')
                        .join(","),
                    )
                    .join("\n"),
                "text/csv",
              )
            }
          >
            <Download size={14} /> Export measurements
          </button>
        </div>
        <div className="td1-history-list">
          {[...(state?.readings ?? [])].reverse().map((r) => (
            <button key={r.id} onClick={() => onUse(r, false)}>
              <span
                style={{ background: r.color }}
                className="filament-color"
              />
              {r.td} mm · {r.color} ·{" "}
              {new Date(r.capturedAt).toLocaleTimeString()}
            </button>
          ))}
        </div>
        <p className="field-help">
          The latest 100 measurements are kept for this connection. Select one
          to apply it to the filament editor.
        </p>
      </details>
      <details
        open={advanced}
        onToggle={(e) => {
          const open = e.currentTarget.open;
          setAdvanced(open);
          localStorage.setItem("colorninja.td1.advanced", String(open));
        }}
      >
        <summary>Advanced · calibration, diagnostics & maintenance</summary>
        <div className="td1-advanced">
          <section>
            <h3>Calibration</h3>
            <p className="field-help">
              Follow the device prompts. Lux calibration requires an empty
              filament path. Color calibration uses opaque black and white
              filaments with TD below 7 mm.
            </p>
            <div className="filament-toolbar">
              <button
                className="button secondary"
                disabled={unavailable}
                onClick={() =>
                  confirm(
                    "Calibrate empty lux",
                    "Remove all filament from the TD1/S before starting. Start LED/lux calibration?",
                    "calibrate-lux",
                  )
                }
              >
                Calibrate empty lux
              </button>
              <button
                className="button secondary"
                disabled={unavailable}
                onClick={() =>
                  confirm(
                    "Calibrate color",
                    "Have black and white calibration filament ready. Start RGB calibration and follow the device display?",
                    "calibrate-rgb",
                  )
                }
              >
                Calibrate RGB
              </button>
              <button
                className="button secondary"
                disabled={unavailable}
                onClick={() =>
                  confirm(
                    "Reset color calibration",
                    "Reset the color calibration matrix? Remove filament and follow the device prompts, then repeat RGB calibration.",
                    "reset-rgb",
                  )
                }
              >
                Reset RGB matrix
              </button>
              <button
                className="text-button"
                onClick={() => resource("calibration")}
              >
                Calibration guide ↗
              </button>
            </div>
            <div className="filament-fields">
              {["Red", "Green", "Blue"].map((name, i) => (
                <label className="select-field" key={name}>
                  {name} adjustment (%)
                  <input
                    type="number"
                    min="-100"
                    max="100"
                    step="1"
                    value={rgb[i]}
                    onChange={(e) =>
                      setRGB(
                        rgb.map((v, at) =>
                          at === i ? Number(e.target.value) : v,
                        ),
                      )
                    }
                  />
                </label>
              ))}
            </div>
            <button
              className="button secondary"
              disabled={
                unavailable ||
                rgb.some((v) => !Number.isInteger(v) || Math.abs(v) > 100)
              }
              onClick={() =>
                confirm(
                  "Adjust measured color",
                  `Apply RGB adjustments ${rgb.join(", ")}% to device calibration?`,
                  "adjust-rgb",
                  { red: rgb[0], green: rgb[1], blue: rgb[2] },
                )
              }
            >
              Apply RGB fine tuning
            </button>
          </section>
          <section>
            <h3>Diagnostics & backups</h3>
            <div className="filament-toolbar">
              {(
                [
                  ["errors", "Read error log"],
                  ["boot-info", "Boot information"],
                  ["license-request", "Prepare license request"],
                  ["rgb-offsets", "RGB offsets"],
                  ["empty-lux", "Stored empty lux"],
                  ["backup", "Back up device"],
                ] as const
              ).map(([a, label]) => (
                <button
                  className="button secondary"
                  key={a}
                  disabled={unavailable}
                  onClick={() => void operation(a)}
                >
                  {label}
                </button>
              ))}
              <button
                className="button secondary"
                disabled={unavailable}
                onClick={() =>
                  confirm(
                    "Restart TD1/S",
                    "Restart the device using its current settings? Reconnect when it is ready.",
                    "reboot",
                  )
                }
              >
                Restart
              </button>
            </div>
            {ignoredErrors.length > 0 && (
              <button
                className="text-button"
                onClick={() => {
                  setIgnoredErrors([]);
                  localStorage.removeItem(
                    `colorninja.td1.ignored.${state?.port.serial ?? ""}`,
                  );
                }}
              >
                Show ignored device errors again ({ignoredErrors.length})
              </button>
            )}
            {diagnostic && (
              <>
                <pre className="td1-log">{diagnostic}</pre>
                <button
                  className="text-button"
                  onClick={() => download("td1-diagnostics.txt", diagnostic)}
                >
                  Save diagnostics
                </button>
              </>
            )}
            <details>
              <summary>USB messages / raw readings</summary>
              <pre className="td1-log">
                {[
                  ...(state?.messages ?? []),
                  ...(state?.readings ?? []).map((r) => r.raw),
                ].join("\n")}
              </pre>
              <button
                className="text-button"
                onClick={() =>
                  download(
                    "td1-session.json",
                    JSON.stringify(state, null, 2),
                    "application/json",
                  )
                }
              >
                Export session
              </button>
            </details>
          </section>
          <section>
            <h3>Firmware & license</h3>
            <p className="field-help">
              Backups are stored by device serial number. Firmware files are
              reviewed before transfer; downloads are checked against the
              vendor’s SHA-256 checksum. Keep the USB cable connected during
              transfer.
            </p>
            <div className="filament-toolbar">
              <button
                className="button secondary"
                disabled={disabled}
                onClick={() => void operation("check-firmware")}
              >
                Check for firmware updates
              </button>
              <button
                className="button secondary"
                disabled={disabled}
                onClick={() => void operation("download-firmware")}
              >
                Download latest firmware
              </button>
              <button
                className="button secondary"
                disabled={disabled}
                onClick={async () => {
                  const path = await choose("firmware");
                  if (path) void operation("plan-firmware", { path });
                }}
              >
                Review firmware ZIP
              </button>
            </div>
            {firmware?.firmware && (
              <div className="filament-notice">
                <strong>
                  Latest firmware: {firmware.firmware.latestVersion}
                </strong>
                <ul>
                  {firmware.firmware.releaseNotes?.map((s, i) => (
                    <li key={i}>{s}</li>
                  ))}
                </ul>
              </div>
            )}
            {firmware?.plan && (
              <div className="td1-firmware-plan">
                <p>{firmware.path}</p>
                <p className="field-help">SHA-256: {firmware.plan.sha256}</p>
                <ul>
                  {firmware.plan.files.map((f) => (
                    <li key={f.name}>
                      {f.name} · {f.size} bytes
                    </li>
                  ))}
                </ul>
                {Object.keys(firmware.plan.prerequisites ?? {}).length > 0 && (
                  <p className="field-help">
                    Required versions:{" "}
                    {Object.entries(firmware.plan.prerequisites)
                      .map(([k, v]) => `${k}: ${v}`)
                      .join(" · ")}
                  </p>
                )}
                <button
                  className="button primary"
                  disabled={unavailable}
                  onClick={() =>
                    confirm(
                      "Install reviewed firmware",
                      `Back up device ${state?.port.serial}, then transfer ${firmware.plan!.files.length} reviewed files? Keep USB connected. Reconnect and read the version after restart to verify installation.`,
                      "apply-firmware",
                      { path: firmware.path, sha256: firmware.plan!.sha256 },
                    )
                  }
                >
                  Back up & install firmware
                </button>
              </div>
            )}
            <div className="filament-toolbar">
              <button
                className="button secondary"
                disabled={unavailable}
                onClick={async () => {
                  const path = await choose("license");
                  if (path)
                    confirm(
                      "Install TD1 license",
                      `Install the vendor license file ${path} on device ${state?.port.serial}?`,
                      "apply-license",
                      { path },
                    );
                }}
              >
                Install vendor license
              </button>
              <button
                className="button secondary"
                disabled={unavailable}
                onClick={async () => {
                  const path = await choose("backup");
                  if (path)
                    confirm(
                      "Restore TD1 backup",
                      `Restore license, settings, pins and RGB offsets from ${path} to device ${state?.port.serial}?`,
                      "restore-backup",
                      { path },
                    );
                }}
              >
                Restore device backup
              </button>
              <button
                className="button secondary"
                disabled={unavailable}
                onClick={() =>
                  confirm(
                    "Enter bootloader",
                    "Enter USB bootloader mode for vendor UF2 recovery? The USB serial connection will close. Use the recovery instructions for your exact TD1/S hardware.",
                    "bootloader",
                  )
                }
              >
                Enter bootloader
              </button>
              <button className="text-button" onClick={() => resource("td1")}>
                Vendor firmware, licensing & recovery ↗
              </button>
            </div>
            <p className="field-help">
              License requests and replacements are handled by the vendor.
            </p>
            <details>
              <summary>UF2 recovery</summary>
              <p className="field-help">
                Back up this device while connected, then enter its bootloader.
                Select its RPI-RP2 drive and a vendor TD1 UF2 image. Recovery
                replaces firmware; reconnect afterward and restore the backup.
                The bootloader identifies an RP2040 board but does not identify
                its TD1 serial number, so verify that the selected drive belongs
                to this device.
              </p>
              {!desktop && (
                <p className="field-help">
                  Selecting a recovery image, backup folder and bootloader drive
                  requires the desktop app.
                </p>
              )}
              <div className="filament-toolbar">
                <button
                  className="button secondary"
                  disabled={disabled}
                  onClick={() => void operation("download-recovery")}
                >
                  Download latest recovery UF2
                </button>
                {(
                  [
                    ["recovery", "Choose recovery UF2", setRecoveryPath],
                    ["backup", "Choose recovery backup", setRecoveryBackup],
                    ["volume", "Choose bootloader drive", setRecoveryVolume],
                  ] as const
                ).map(([kind, label, setValue]) => (
                  <button
                    className="button secondary"
                    key={kind}
                    disabled={disabled || !desktop}
                    onClick={async () => {
                      const path = await choose(kind);
                      if (path) {
                        setValue(path);
                        setRecovery(undefined);
                      }
                    }}
                  >
                    {label}
                  </button>
                ))}
              </div>
              <p className="field-help">
                Image: {recoveryPath || "Choose or download a UF2 image"}
                <br />
                Backup: {recoveryBackup || "Choose this device's saved backup"}
                <br />
                Bootloader drive:{" "}
                {recoveryVolume || "Choose the connected RPI-RP2 drive"}
              </p>
              <button
                className="button secondary"
                disabled={
                  disabled ||
                  !recoveryPath ||
                  !recoveryBackup ||
                  !recoveryVolume
                }
                onClick={() =>
                  void operation("plan-recovery", {
                    path: recoveryPath,
                    backup: recoveryBackup,
                    volume: recoveryVolume,
                  })
                }
              >
                Review recovery
              </button>
              {recovery && (
                <div className="td1-firmware-plan">
                  <p>
                    Device backup: {recovery.serial}
                    <br />
                    Destination: {recovery.volume}
                    <br />
                    Image: {recovery.path}
                    <br />
                    {recovery.size.toLocaleString()} bytes ·{" "}
                    {recovery.blocks.toLocaleString()} flash blocks
                  </p>
                  <pre className="td1-log">{recovery.board}</pre>
                  <p className="field-help">SHA-256: {recovery.sha256}</p>
                  <button
                    className="button primary"
                    disabled={disabled || connected}
                    onClick={() =>
                      confirm(
                        "Write TD1 recovery firmware",
                        `Write the reviewed UF2 image to ${recovery.volume}? Confirm that this drive is TD1/S ${recovery.serial}. Its backup is ${recovery.backup}. Keep USB connected until transfer finishes, then reconnect and restore that backup.`,
                        "apply-recovery",
                        { sha256: recovery.sha256 },
                      )
                    }
                  >
                    Write reviewed UF2 to bootloader drive
                  </button>
                </div>
              )}
            </details>
          </section>
        </div>
      </details>
      {confirmation && (
        <StudioDialog
          title={confirmation.title}
          onClose={() => setConfirmation(null)}
        >
          <p>{confirmation.text}</p>
          <div className="modal-actions">
            <button
              className="button secondary"
              onClick={() => setConfirmation(null)}
            >
              Cancel
            </button>
            <button
              className="button primary"
              onClick={() => {
                const run = confirmation.run;
                setConfirmation(null);
                run();
              }}
            >
              Continue
            </button>
          </div>
        </StudioDialog>
      )}
    </section>
  );
}
