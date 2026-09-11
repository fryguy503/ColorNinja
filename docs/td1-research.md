# TD1 / TD1S feature and protocol evidence

Research and Windows hardware verification: 2026-09-10 (measurement timestamp
2026-09-11 UTC). Scope is TD1 and TD1S. ColorNinja is independent of HueForge,
AJAX-3D, BIQU and 3D Filament Profiles.

## Sources

- [AJAX-3D HueForge TD1 guide](https://ajax-3d.com/td-1-hueforge-user-guide/):
  connect, filament capture/editing and device workflow; the guide describes older
  HueForge 0.7.1 UI and is not the sole feature inventory.
- [TD1S product information](https://ajax-3d.com/td1s-ajax-3d-biqu/),
  [resources](https://ajax-3d.com/resources/) and
  [color calibration guide](https://ajax-3d.com/td1-td1s-color-calibration-guide/).
- [Vendor version manifest](https://version.ajax-3d.com/version.json), schema 1:
  TD1 firmware 2.0.2, minimum supported 2.0.1, bootloader command minimum 1.0.4.
  Official ZIP and UF2 were downloaded and validated against its SHA-256 values.
- [Vendor-authored Moonraker TD1 integration](https://raw.githubusercontent.com/Arksine/moonraker/master/moonraker/components/td1.py):
  USB identity, handshake, reading fields and settings. Used as protocol reference;
  its GPL implementation was not copied into ColorNinja.
- Installed HueForge 0.9.4.3 `libTD1.dll`: exported manager/settings/receiver,
  firmware checker and simulator symbols, strings and Ghidra control-flow inspection.
  DLL SHA-256: `e712afbdc2ebdf59abb1b3268114ba779f9cd258f5fc643c8b2c71b6d28f4384`.
  Research artifacts and vendor binaries
  remain outside tracked source.
- Live [3D Filament Profiles list](https://3dfilamentprofiles.com/filaments?q=red&page=2)
  and [profile 13036](https://3dfilamentprofiles.com/filament/details/13036).
  No documented public catalogue API was found. The importer reads table rows and
  bounded profile objects in saved Next.js payloads without executing scripts.

## Feature mapping

| HueForge / TD1 feature family | ColorNinja location and behavior |
| --- | --- |
| USB discovery, connect, license identity, disconnect | Filament library → TD1 / TD1S; recognized USB ports and one serialized connection |
| Live TD and color, continuous scans | Latest reading, bounded history, continuous settings, measurement CSV |
| Capture into new/existing filament, save / continue / close | My library editor; TD-only default for existing profiles, optional measured color, batch capture, provenance |
| Brand, material, name, color, TD and ownership management | My library; add/edit/duplicate/delete, tags and secondary color, backed-up managed copy, JSON export |
| Device settings | Load settings; supported literals only, restart/reconnect after saving |
| Screen simulator/mirror | SVG display preserving device text coordinates and clear-screen commands |
| Lux and RGB calibration, matrix reset, RGB adjustments | Advanced → Calibration with preparation prompts |
| Glow reading | Read glow value (a sensor reading, not an identification LED command) |
| Firmware versions, error notices, ignored messages | Version and error read on connect, exact per-device suppression with reset |
| File retrieval, diagnostics | Error log, boot information, RGB offsets, empty lux and raw USB/session export |
| License request/install and device backup/restore | Downloadable request draft, vendor binary installation, serial-bound checksummed backups |
| Firmware check, staged ZIP transfer and bootloader | Vendor manifest/download verification, review, prerequisite checks, automatic pre-update backup, gated bootloader command |
| UF2 recovery | Explicit drive/image/backup review and RP2040 validation; recovery write followed by restore/version verification |
| Reboot | Existing supported settings command using unchanged RGB_Enabled |

Shared library symbols also include a misspelled `adjust color brighness` command.
It was absent from inspected TD1 firmware 2.0.1 and 2.0.2; ColorNinja does not expose
that shared/TD0 control as a supported TD1 feature. This is feature coverage for
the inspected TD1/S surface, not a guarantee about future HueForge or firmware.

## Wire protocol

115200 baud, 8N1, DTR/RTS enabled. Host sends `connect\n`, waits for `ready`, then
`PY\n`; the device replies `connected to PY licensed` for the tested licensed unit.
Measurements have six or nine comma-separated fields; TD and color are fields
4 and 5 (zero-based), including records with empty sensor fields.

The real TD1S supplied `11267973560,,,,6.9,530723`. The app displayed **6.9 mm** and
**#530723**. `clearScreen` and `display,ClearScreen` clear the mirror;
`display,<text>,<x>,<y>` updates a coordinate slot, including text containing commas.

`retrieve file` sends a basename; replies contain `ready`, total size, block count,
then each length line followed by exactly that many binary bytes. The host
acknowledges each block with `ready`. There is no extra newline after binary data.
Updates use `update`, then `file`, relative filename, size/count and blocks up to
1024 bytes, each acknowledged. `done` commits and may restart without another
reply. Received sizes, expanded ZIP contents, paths, history and diagnostics are
bounded. Unexpected framing or timeout closes the connection.

Settings use `change settings`, a ready handshake, known literal assignments and
`done`. RGB fine tuning uses `rgb adj` and three integer percentages. Calibration
uses `calibrate emptyLux`, or `rgb cal` followed by `rgb` / `matrix`. Version replies
are labeled module versions, without a leading `version` token.

## Validation and limits

The user's attached TD1S was detected on COM3 with firmware **2.0.2** and a licensed
identity. Real USB checks covered connection, version, settings, boot information,
pins, RGB offsets, license retrieval and a physical filament measurement. The live
measurement was applied to a profile imported in an isolated test configuration:
profile 13036's TD changed from 2.6 to **6.9**, its original **#C72E22** color and source
URL stayed intact, and measured **#530723**, serial and timestamp were saved separately.
The source HueForge library remained unchanged.

Protocol regression tests cover fragmented reads, short writes, binary boundaries,
interleaved messages, cancellation, corrupt frames, settings validation, update
paths/prerequisites, UF2 headers/blocks and reviewed backup checksums. Library tests
cover lossless records, unknown TD, filtering, provenance, conflicts, backups,
constraint remapping and preview invalidation. Frontend tests cover import and
measured-TD replacement. UF2 write tests use temporary directories, not hardware.

Direct HTTP search encountered the site's **HTTP 429 and 403 browser-verification responses**
in this environment. The Mia White report also exposed a misleading fallback:
a locally chosen five-minute cooldown was described as a server-requested wait.
Because direct access was unreliable, the direct search, URL fetching and retry
features were removed. **Import profiles** now links to the preferred website in
the user's normal browser and parses copied page text or saved HTML/CSV locally.
Public pages were inspected through the ordinary browser; file/CSV import and the
import-to-measurement workflow were verified. No challenge bypass or private API
is used.

The English visible text of [Anycubic PLA Basic Mia White, profile 23212](https://3dfilamentprofiles.com/filament/details/23212)
was inspected in an ordinary browser and used to validate copied-page import:
nominal color `#F1E9E0`, no published TD. Copied text is parsed locally with a
user-supplied profile URL, without browser credentials or further HTTP requests.
The user's Ctrl+A/Ctrl+C report omitted the Export button and TD links that were
present in the earlier browser DOM check. Its complete pasted text is now a
regression fixture: heading detection accepts the Color section boundary when
Export is absent, and a missing TD remains unknown.

Calibration, device setting changes, license installation, firmware flashing and
recovery were not physically exercised. Original TD1 and native Linux/macOS USB
acceptance remain unverified. Confirm firmware installation by reconnecting and
reading its version; transfer acknowledgement alone is insufficient. Software
checks do not establish physical print accuracy or complete future HueForge parity.
