import { useEffect, useRef, useState, type ReactNode } from "react";
import { ArrowUpRight, RefreshCw, X } from "lucide-react";
import { desktop, invoke } from "./bridge";
import type { Preferences, UpdateResult } from "./types";

export function Updates({
  preferences,
  savePreferences,
  ready,
  workspaceControls,
}: {
  preferences: Preferences;
  savePreferences: (p: Preferences) => Promise<void>;
  ready: boolean;
  workspaceControls?: ReactNode;
}) {
  const [version, setVersion] = useState("");
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [saving, setSaving] = useState(false);
  const [result, setResult] = useState<UpdateResult | null>(null);
  const [error, setError] = useState("");
  const initialized = useRef(false);
  const requestID = useRef(0);
  const panel = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const close = () => {
    setOpen(false);
    trigger.current?.focus();
  };
  const check = async (force: boolean) => {
    const id = ++requestID.current;
    setBusy(true);
    setError("");
    try {
      const next = await invoke<UpdateResult>(
        "CheckUpdates",
        preferences.includePrereleases,
        force,
      );
      if (id === requestID.current) setResult(next);
    } catch (e) {
      if (id === requestID.current) {
        setResult(null);
        setError(String(e instanceof Error ? e.message : e));
      }
    } finally {
      if (id === requestID.current) setBusy(false);
    }
  };
  useEffect(() => {
    invoke<string>("Version")
      .then(setVersion)
      .catch(() => {});
  }, []);
  useEffect(() => {
    if (!ready || initialized.current) return;
    initialized.current = true;
    if (preferences.checkOnStartup) void check(false);
  }, [ready]);
  useEffect(() => {
    if (!open) return;
    panel.current?.focus();
    const dismiss = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
      if (e.key === "Tab" && panel.current) {
        const controls = [
          ...panel.current.querySelectorAll<HTMLElement>(
            "button:not(:disabled), input, a[href]",
          ),
        ];
        const first = controls[0],
          last = controls.at(-1);
        if (
          e.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === panel.current)
        ) {
          e.preventDefault();
          last?.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener("keydown", dismiss);
    return () => document.removeEventListener("keydown", dismiss);
  }, [open]);
  const save = async (next: Preferences) => {
    setSaving(true);
    setError("");
    try {
      await savePreferences(next);
      if (next.includePrereleases !== preferences.includePrereleases) {
        requestID.current++;
        setBusy(false);
        setResult(null);
      }
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setSaving(false);
    }
  };
  const releaseLink = (tag: string, label: string) =>
    desktop ? (
      <button
        className="button secondary"
        onClick={() =>
          invoke("OpenReleasePage", tag).catch((e) => setError(String(e)))
        }
      >
        {label}
        <ArrowUpRight size={14} />
      </button>
    ) : (
      <a
        className="button secondary"
        href={
          "https://github.com/fryguy503/ColorNinja/releases" +
          (tag ? "/tag/" + encodeURIComponent(tag) : "")
        }
        target="_blank"
        rel="noreferrer"
      >
        {label}
        <ArrowUpRight size={14} />
      </a>
    );
  return (
    <>
      <button
        ref={trigger}
        className={"update-trigger " + (result?.available ? "available" : "")}
        onClick={() => setOpen(true)}
        title="Preferences, version and updates"
      >
        {result?.available
          ? "Update available"
          : `Preferences · ${version || "…"}`}{" "}
        <ArrowUpRight size={12} />
      </button>
      {open && (
        <div
          className="modal-backdrop"
          onPointerDown={(e) => {
            if (e.target === e.currentTarget) close();
          }}
        >
          <div
            ref={panel}
            className="modal updates-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="updates-title"
            tabIndex={-1}
          >
            <div className="updates-heading">
              <h2 id="updates-title">Preferences & updates</h2>
              <button
                className="icon-button"
                aria-label="Close updates"
                onClick={close}
              >
                <X size={18} />
              </button>
            </div>
            {workspaceControls && (
              <section className="preferences-workspace">
                <h3>Workspace</h3>
                {workspaceControls}
              </section>
            )}
            <p>
              Installed version: <strong>{version || "Loading…"}</strong>
            </p>
            <label className="check-field">
              <input
                type="checkbox"
                disabled={saving || !ready}
                checked={preferences.checkOnStartup}
                onChange={(e) =>
                  void save({
                    ...preferences,
                    checkOnStartup: e.target.checked,
                  })
                }
              />
              Check for updates on startup
            </label>
            <label className="check-field">
              <input
                type="checkbox"
                disabled={saving || !ready}
                checked={preferences.includePrereleases}
                onChange={(e) =>
                  void save({
                    ...preferences,
                    includePrereleases: e.target.checked,
                  })
                }
              />
              Include release candidates, beta and alpha builds
            </label>
            <p className="field-help">
              {preferences.includePrereleases
                ? "Includes stable releases and preview builds, which may have unfinished features."
                : "Only stable releases are offered. Enable preview builds to follow release candidates, betas and alphas."}
            </p>
            <div className="update-status" aria-live="polite">
              {busy
                ? "Checking GitHub…"
                : error ||
                  (result
                    ? result.available
                      ? `New version: ${result.latestVersion}${result.prerelease ? " · Preview build" : ""}`
                      : result.latestVersion
                        ? "No newer release in this channel."
                        : "No releases have been published in this channel yet."
                    : "Choose Check now to look for a newer release.")}
              {result && !busy && (
                <small>
                  Checked {new Date(result.checkedAt).toLocaleString()}
                </small>
              )}
            </div>
            <div className="updates-actions">
              <button
                className="button primary"
                disabled={busy || saving || !ready}
                onClick={() => void check(true)}
              >
                <RefreshCw size={14} className={busy ? "spin" : ""} />
                Check now
              </button>
              {releaseLink(
                result?.available ? result.latestVersion : "",
                result?.available
                  ? "Download & release notes"
                  : "View releases",
              )}
            </div>
            <p className="field-help">
              Checks public GitHub releases. Your images and filament library
              stay on this computer. Download and replace the portable app
              manually; checks never install anything. Repeated checks within a
              minute use the last result.
            </p>
          </div>
        </div>
      )}
    </>
  );
}
