import { useEffect, useRef, useState } from "react";
import {
  Plus,
  Usb,
  Download,
  RefreshCw,
  Pencil,
  Trash2,
  SwatchBook,
} from "lucide-react";
import { invoke, desktop } from "./bridge";
import { StudioDialog } from "./StudioDialog";
import { TD1Panel } from "./TD1Panel";
import {
  newFilament,
  measuredEntry,
  profileEntry,
  profileDetailURL,
  type Catalog,
  type FilamentEntry,
  type Reading,
  type ProfileResults,
  type Profile,
} from "./filamentTypes";
import "./filaments.css";

export function FilamentManager({
  path,
  onChanged,
  onClose,
}: {
  path: string;
  onChanged: (c: Catalog) => void;
  onClose: () => void;
}) {
  const [catalog, setCatalog] = useState<Catalog | null>(null),
    [tab, setTab] = useState("library"),
    [query, setQuery] = useState(""),
    [draft, setDraft] = useState<FilamentEntry>(newFilament),
    [saved, setSaved] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState(""),
    [confirm, setConfirm] = useState<null | { text: string; run: () => void }>(
      null,
    );
  const [latest, setLatest] = useState<Reading>(),
    [updateColor, setUpdateColor] = useState(
      () => localStorage.getItem("colorninja.td1.updateColor") === "true",
    ),
    [captureScans, setCaptureScans] = useState(false),
    [visibleCount, setVisibleCount] = useState(200);
  const currentPath = useRef(path);
  const select = (entry: FilamentEntry) => {
    setDraft(structuredClone(entry));
    setSaved(JSON.stringify(entry));
    setMessage("");
  };
  const dirty = saved !== "" && JSON.stringify(draft) !== saved;
  const guard = (run: () => void) =>
    dirty
      ? setConfirm({ text: "Discard the unsaved filament edits?", run })
      : run();
  const load = async () => {
    setBusy(true);
    setError("");
    try {
      const c = await invoke<Catalog>("FilamentCatalog", currentPath.current);
      setCatalog(c);
      select(
        c.entries.find((e) => draft.uuid !== "" && e.uuid === draft.uuid) ??
          newFilament(),
      );
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    void load();
  }, []);
  const save = async (
    remove = false,
    addAnother = false,
    closeAfter = false,
  ) => {
    if (!catalog) return;
    setBusy(true);
    setError("");
    try {
      const c = await invoke<Catalog>("EditFilament", {
        path: catalog.path,
        sha256: catalog.sha256,
        entry: draft,
        delete: remove,
      });
      currentPath.current = c.path;
      setCatalog(c);
      onChanged(c);
      select(
        remove || addAnother
          ? { ...newFilament(), brand: draft.brand, material: draft.material }
          : (c.entries.find(
              (e) =>
                e.index ===
                (draft.index < 0 ? c.entries.at(-1)?.index : draft.index),
            ) ?? newFilament()),
      );
      setMessage(
        remove
          ? "Filament removed. A backup was saved."
          : "Filament saved. Generate a new preview to use the updated library.",
      );
      if (closeAfter) onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  const applyReading = (reading: Reading, create = false) => {
    const entry = create
      ? { ...newFilament(), brand: draft.brand, material: draft.material }
      : draft;
    setDraft(measuredEntry(entry, reading, create || updateColor));
    if (create) setSaved(JSON.stringify(entry));
    setTab("library");
    setMessage(
      "Measurement copied into the editor. Review the filament and save.",
    );
  };
  useEffect(() => {
    if (latest && captureScans && !dirty && !busy) applyReading(latest, true);
  }, [latest?.id]);
  useEffect(() => setVisibleCount(200), [query]);
  const entries = (catalog?.entries ?? []).filter((e) =>
    [e.brand, e.name, e.material, e.color, ...e.tags]
      .join(" ")
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  const field = (
    key: "brand" | "name" | "material" | "color" | "secondaryColor",
    label: string,
  ) => (
    <label className="select-field">
      {label}
      <input
        aria-label={label}
        required={key !== "secondaryColor"}
        value={draft[key]}
        onChange={(e) => setDraft({ ...draft, [key]: e.target.value })}
      />
    </label>
  );
  const importProfile = (p: Profile) =>
    guard(() => {
      select(profileEntry(p));
      setSaved(JSON.stringify(newFilament()));
      setTab("library");
      setMessage(
        p.td
          ? "Imported profile ready to review and save."
          : "This profile has no TD. Measure it with TD1/S or enter a TD before using it in a stack.",
      );
    });
  return (
    <StudioDialog
      title="Filament library & devices"
      className="filament-manager"
      onClose={() => {
        if (!busy) guard(onClose);
      }}
    >
      <nav className="filament-tabs" aria-label="Filament manager sections">
        {[
          ["library", "My library", SwatchBook],
          ["profiles", "Import profiles", Download],
          ["td1", "TD1 / TD1S", Usb],
        ].map(([id, label, Icon]) => (
          <button
            key={String(id)}
            className={tab === id ? "active" : ""}
            aria-pressed={tab === id}
            onClick={() => setTab(String(id))}
          >
            {typeof Icon !== "string" && <Icon size={16} />} {String(label)}
          </button>
        ))}
      </nav>
      {error && (
        <p role="alert" className="dialog-error">
          {error}
        </p>
      )}
      {catalog?.warning && (
        <p className="field-help" role="status">
          {catalog.warning}
        </p>
      )}
      {message && (
        <p role="status" className="filament-notice">
          {message}
        </p>
      )}
      {tab === "library" && (
        <>
          <div className="filament-toolbar">
            <span>
              {catalog?.entries.length ?? 0} entries ·{" "}
              {catalog?.managed
                ? "ColorNinja library"
                : "Edits save to a ColorNinja copy"}
            </span>
            <button
              className="text-button"
              disabled={busy}
              onClick={() => guard(() => void load())}
            >
              <RefreshCw size={14} /> Reload
            </button>
            <button
              className="text-button"
              disabled={busy || !catalog}
              onClick={async () => {
                try {
                  if (desktop) {
                    const dest = await invoke<string>(
                      "ExportFilamentLibrary",
                      catalog!.path,
                    );
                    if (dest) setMessage(`Exported ${dest}`);
                  } else {
                    setError("Export library is available in the desktop app.");
                  }
                } catch (e) {
                  setError(String(e));
                }
              }}
            >
              <Download size={14} /> Export library
            </button>
          </div>
          <div className="filament-management-grid">
            <section
              className="filament-collection"
              aria-label="All library entries"
            >
              <div className="filament-toolbar">
                <input
                  aria-label="Search all library entries"
                  placeholder="Brand, material, name, color…"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
                <button
                  className="button secondary"
                  disabled={busy}
                  onClick={() => guard(() => select(newFilament()))}
                >
                  <Plus size={14} /> New
                </button>
              </div>
              <div className="managed-filament-list">
                {entries.slice(0, visibleCount).map((e) => (
                  <button
                    className={`managed-filament ${e.index === draft.index ? "active" : ""}`}
                    key={e.index}
                    onClick={() => guard(() => select(e))}
                    disabled={busy}
                  >
                    <span
                      className="filament-color"
                      style={{ background: e.color }}
                    />
                    <span>
                      <strong>{e.name || "Unnamed filament"}</strong>
                      <small>
                        {e.brand} · {e.material} ·{" "}
                        {e.owned ? "Owned" : "Not owned"}
                      </small>
                    </span>
                    <span className="filament-td">
                      {e.td > 0 ? `${e.td} mm` : "TD unknown"}
                    </span>
                  </button>
                ))}
              </div>
              {entries.length > visibleCount && (
                <button
                  className="text-button"
                  onClick={() => setVisibleCount((n) => n + 200)}
                >
                  Show more ({entries.length - visibleCount} remaining)
                </button>
              )}
              {!entries.length && (
                <p className="field-help">
                  No matching filaments. Add one manually, import a profile, or
                  scan with TD1/S.
                </p>
              )}
            </section>
            <form
              className="filament-editor"
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              <h3>
                <Pencil size={16} />
                {draft.index < 0 ? "Add filament" : "Edit filament"}
              </h3>
              <div className="filament-fields">
                {field("brand", "Brand")}
                {field("name", "Filament name")}
                {field("material", "Material / type")}
                <label className="select-field">
                  Transmission distance (mm)
                  <input
                    aria-label="Transmission distance (mm)"
                    type="number"
                    min="0"
                    max="1000"
                    step="any"
                    required
                    value={draft.td}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        td: Number(e.target.value),
                        measurement: undefined,
                      })
                    }
                  />
                </label>
                {field("color", "Color hex")}
                {field("secondaryColor", "Secondary color hex (optional)")}
              </div>
              <div className="filament-toolbar">
                <input
                  type="color"
                  aria-label="Pick filament color"
                  value={
                    /^#[0-9a-f]{6}$/i.test(draft.color)
                      ? draft.color
                      : "#808080"
                  }
                  onChange={(e) =>
                    setDraft({ ...draft, color: e.target.value })
                  }
                />
                <label className="check-field">
                  <input
                    type="checkbox"
                    checked={draft.owned}
                    onChange={(e) =>
                      setDraft({ ...draft, owned: e.target.checked })
                    }
                  />{" "}
                  I own this filament
                </label>
              </div>
              <label className="select-field">
                Tags
                <input
                  aria-label="Filament tags"
                  value={draft.tags.join(",")}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      tags: e.target.value.split(","),
                    })
                  }
                />
              </label>
              <p className="field-help">
                TD 0 means unknown and excludes this filament from stack
                planning. Use the exact material and finish printed on your
                spool.
              </p>
              {draft.sourceURL && (
                <p className="field-help filament-source">
                  Source: {draft.sourceURL}
                </p>
              )}
              {draft.measurement && (
                <p className="filament-notice">
                  Measured TD: {draft.measurement.td} mm ·{" "}
                  {new Date(draft.measurement.capturedAt).toLocaleString()} ·
                  Device {draft.measurement.serial}
                </p>
              )}
              {latest && (
                <div className="measurement-apply">
                  <strong>Latest TD1/S reading: {latest.td} mm</strong>
                  <small>
                    {new Date(latest.capturedAt).toLocaleString()} ·{" "}
                    {latest.color}
                  </small>
                  <label className="check-field">
                    <input
                      type="checkbox"
                      checked={updateColor}
                      onChange={(e) => {
                        setUpdateColor(e.target.checked);
                        localStorage.setItem(
                          "colorninja.td1.updateColor",
                          String(e.target.checked),
                        );
                      }}
                    />{" "}
                    Also replace filament color
                  </label>
                  <button
                    className="button secondary"
                    type="button"
                    disabled={busy}
                    onClick={() => applyReading(latest)}
                  >
                    Use measured TD
                  </button>
                </div>
              )}
              <div className="filament-toolbar">
                <button
                  className="button primary"
                  disabled={busy || !catalog}
                  type="submit"
                >
                  {busy ? "Saving…" : "Save filament"}
                </button>
                <button
                  className="button secondary"
                  type="button"
                  disabled={
                    busy || !draft.brand || !draft.name || !draft.material
                  }
                  onClick={() => void save(false, true)}
                >
                  Save & add another
                </button>
                <button
                  className="button secondary"
                  type="button"
                  disabled={busy || !catalog}
                  onClick={() => void save(false, false, true)}
                >
                  Save & close
                </button>
                {draft.index >= 0 && (
                  <button
                    className="text-button"
                    type="button"
                    disabled={busy}
                    onClick={() =>
                      guard(() => {
                        select({
                          ...draft,
                          index: -1,
                          uuid: "",
                          name: draft.name + " (copy)",
                        });
                        setSaved(JSON.stringify(newFilament()));
                      })
                    }
                  >
                    Duplicate
                  </button>
                )}
                {draft.index >= 0 && (
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="Delete filament"
                    disabled={busy}
                    onClick={() =>
                      setConfirm({
                        text: `Remove ${draft.brand} ${draft.name} from this library? A backup will be saved.`,
                        run: () => void save(true),
                      })
                    }
                  >
                    <Trash2 size={17} />
                  </button>
                )}
              </div>
            </form>
          </div>
        </>
      )}
      {tab === "profiles" && <ProfileImport onImport={importProfile} />}
      <div hidden={tab !== "td1"}>
        <label className="check-field">
          <input
            type="checkbox"
            checked={captureScans}
            onChange={(e) => setCaptureScans(e.target.checked)}
          />{" "}
          Open each new scan in the filament editor when it has no unsaved edits
        </label>
        <TD1Panel
          onReading={setLatest}
          onUse={(r, create) =>
            create ? guard(() => applyReading(r, true)) : applyReading(r)
          }
          selected={
            draft.index < 0 ? "new filament" : `${draft.brand} ${draft.name}`
          }
        />
      </div>
      {confirm && (
        <StudioDialog
          title="Confirm library change"
          onClose={() => setConfirm(null)}
        >
          <p>{confirm.text}</p>
          <div className="modal-actions">
            <button
              className="button secondary"
              onClick={() => setConfirm(null)}
            >
              Cancel
            </button>
            <button
              className="button primary"
              onClick={() => {
                const action = confirm.run;
                setConfirm(null);
                action();
              }}
            >
              Continue
            </button>
          </div>
        </StudioDialog>
      )}
    </StudioDialog>
  );
}

function ProfileImport({ onImport }: { onImport: (p: Profile) => void }) {
  const [paste, setPaste] = useState("");
  const [copiedPage, setCopiedPage] = useState(""),
    [browserImport, setBrowserImport] = useState(true),
    [results, setResults] = useState<ProfileResults>(),
    [filter, setFilter] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [detail, setDetail] = useState("");
  const showResults = (next: ProfileResults) => {
    setResults(next);
    setFilter("");
    setBrowserImport(false);
  };
  const openWebsite = () => {
    if (desktop)
      void invoke("OpenFilamentResource", "profiles").catch((e) =>
        setError(String(e)),
      );
    else
      window.open(
        "https://3dfilamentprofiles.com/filaments",
        "_blank",
        "noopener,noreferrer",
      );
  };
  const openProfile = () => {
    const url = profileDetailURL(detail);
    if (!url) return;
    if (desktop)
      void invoke("OpenFilamentProfile", detail).catch((e) =>
        setError(String(e)),
      );
    else window.open(url, "_blank", "noopener,noreferrer");
  };
  const visible = (results?.profiles ?? []).filter((p) =>
    [p.brand, p.material, p.type, p.color, p.hex]
      .join(" ")
      .toLowerCase()
      .includes(filter.trim().toLowerCase()),
  );
  return (
    <section className="profile-import">
      <p className="profile-source">
        <strong>Preferred source: 3D Filament Profiles</strong>
        <span>3dfilamentprofiles.com</span>
      </p>
      <p className="field-help">
        Search for filaments on the website in your browser, then import a
        copied profile page or saved file here. Use your TD1/S to measure your
        spool’s TD.
      </p>
      <div className="filament-toolbar">
        <button className="button primary" onClick={openWebsite}>
          Open 3D Filament Profiles ↗
        </button>
        <label className="button secondary import-file">
          Import saved HTML / CSV
          <input
            type="file"
            accept=".html,.htm,.csv"
            aria-label="Import saved profile page or CSV"
            disabled={busy}
            onChange={async (e) => {
              const file = e.target.files?.[0];
              if (!file) return;
              setBusy(true);
              setError("");
              try {
                if (file.size > 8 * 1024 * 1024)
                  throw new Error("File exceeds 8 MiB");
                showResults(
                  await invoke<ProfileResults>(
                    "ImportFilamentProfiles",
                    await file.text(),
                    /\.csv$/i.test(file.name) ? "csv" : "html",
                  ),
                );
              } catch (e) {
                setError(String(e));
              } finally {
                setBusy(false);
              }
            }}
          />
        </label>
      </div>
      <details
        className="profile-browser-import"
        open={browserImport}
        onToggle={(e) => setBrowserImport(e.currentTarget.open)}
      >
        <summary>Import a profile from your browser</summary>
        <ol>
          <li>
            Open <strong>3D Filament Profiles (3dfilamentprofiles.com)</strong>,
            search for your filament, and open its detail page.
          </li>
          <li>
            Paste that page’s URL or ID below. Click the English profile page,
            select all page text and copy it (Ctrl+A, Ctrl+C on Windows/Linux;
            Cmd+A, Cmd+C on Mac).
          </li>
          <li>
            Paste the text below and choose <strong>Read copied profile</strong>
            . Review the result, then choose <strong>Use profile</strong>.
          </li>
        </ol>
        <form
          className="filament-toolbar"
          onSubmit={(e) => {
            e.preventDefault();
            openProfile();
          }}
        >
          <input
            aria-label="Profile URL or ID"
            placeholder="3dfilamentprofiles.com profile URL or numeric ID"
            value={detail}
            onChange={(e) => setDetail(e.target.value)}
          />
          <button
            className="button secondary"
            disabled={busy || !profileDetailURL(detail)}
          >
            Open profile on website ↗
          </button>
        </form>
        <label className="select-field">
          Copied profile page
          <textarea
            aria-label="Copied profile page"
            rows={5}
            value={copiedPage}
            onChange={(e) => setCopiedPage(e.target.value)}
            placeholder="Paste the complete profile page text here. No HTML or CSV needed."
          />
        </label>
        <div className="filament-toolbar">
          <button
            className="button secondary"
            disabled={busy}
            onClick={async () => {
              setError("");
              try {
                setCopiedPage(
                  desktop
                    ? await invoke<string>("ReadFilamentClipboard")
                    : await navigator.clipboard.readText(),
                );
              } catch {
                setError(
                  "Clipboard access was unavailable. Paste directly into the copied profile page box.",
                );
              }
            }}
          >
            Paste from clipboard
          </button>
          <button
            className="button primary"
            disabled={busy || !copiedPage.trim() || !profileDetailURL(detail)}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                showResults({
                  profiles: [
                    await invoke<Profile>(
                      "ImportCopiedFilamentProfile",
                      detail,
                      copiedPage,
                    ),
                  ],
                });
              } catch (e) {
                setError(String(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            Read copied profile
          </button>
        </div>
      </details>
      {error && (
        <p role="alert" className="dialog-error">
          {error}
        </p>
      )}
      <p className="field-help">
        Imports are read locally. ColorNinja does not upload library edits or
        device measurements to the website.
      </p>
      <details>
        <summary>Paste exported CSV</summary>
        <label className="select-field">
          CSV content
          <textarea
            aria-label="CSV content"
            rows={5}
            value={paste}
            onChange={(e) => setPaste(e.target.value)}
            placeholder="Brand,Material,Color,RGB,TD"
          />
        </label>
        <button
          className="button secondary"
          disabled={busy || !paste}
          onClick={async () => {
            setBusy(true);
            setError("");
            try {
              showResults(
                await invoke<ProfileResults>(
                  "ImportFilamentProfiles",
                  paste,
                  "csv",
                ),
              );
            } catch (e) {
              setError(String(e));
            } finally {
              setBusy(false);
            }
          }}
        >
          Import pasted CSV
        </button>
      </details>
      {results && (
        <>
          <div className="filament-toolbar">
            <input
              aria-label="Filter imported profiles"
              placeholder="Filter imported profiles by brand, material or color…"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
            <span>
              {visible.length} imported{" "}
              {visible.length === 1 ? "profile" : "profiles"}
            </span>
          </div>
          <div className="profile-results">
            {visible.slice(0, 100).map((p, i) => (
              <div className="profile-card" key={`${p.id}-${i}`}>
                <span
                  className="filament-color"
                  style={{ background: p.hex }}
                />
                <div>
                  <strong>{p.color}</strong>
                  <small>
                    {p.brand} · {p.material} {p.type} · {p.hex}
                  </small>
                  <small>
                    {p.td ? `TD ${p.td} mm` : "TD unknown — measure your spool"}
                  </small>
                </div>
                <button
                  className="button secondary"
                  disabled={busy}
                  onClick={() => onImport(p)}
                >
                  Use profile
                </button>
              </div>
            ))}
          </div>
          {visible.length > 100 && (
            <p className="field-help">
              Showing 100 results. Refine the imported-profile filter above.
            </p>
          )}
          {!visible.length && (
            <p className="field-help">No matching imported profiles.</p>
          )}
        </>
      )}
    </section>
  );
}
