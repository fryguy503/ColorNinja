# Filament library and TD1 / TD1S

Open **Filament library** in the top toolbar in any processing mode, or **Manage
library** in the Filaments inspector. The dialog has three sections: **My library**,
**Import profiles**, and **TD1 / TD1S**.

## Edit your library

Select an entry to edit its brand, name, material/finish, color, optional secondary
color, transmission distance (TD), ownership and tags. **New** adds a filament;
**Duplicate** starts a separate spool entry. **Save & add another** retains the
brand and material for repeated measurements. **Save & close** finishes editing.

TD is in millimeters. A value of **0 means unknown** and excludes the entry from
stack planning. Ownership and material filters affect processing eligibility;
the editor still shows all records, including duplicate or ineligible entries.
ColorNinja supports primary and secondary colors; a profile with more colors
imports its first two. Review multicolor spools before using them in a stack.

The first edit to an external/HueForge library creates a managed copy in
ColorNinja's configuration directory under `libraries/`. It becomes the selected
library. Later saves and deletions back up its previous JSON under
`libraries/backups/`. Unknown HueForge fields and unrelated records survive edits.
Concurrent changes to the same file require **Reload** before saving again.
**Export library** writes a separate HueForge-compatible JSON file.

Editing invalidates the current processed result so the next preview uses the
new values. Selected base, highlight and required filaments follow changed
identities. Save a project and reset existing Region Edit groups before changing
its library. Portable projects retain their embedded library snapshot.

## Find and import a profile

The preferred source is [3D Filament Profiles](https://3dfilamentprofiles.com/filaments)
(`3dfilamentprofiles.com`), named in **Import profiles**. Search its website in your
normal browser by brand, material/type or color. ColorNinja imports the content you
copy or save; it does not search or fetch profiles directly from the site.

1. Open **Import profiles**, select **Open 3D Filament Profiles**, and find the
   filament's detail page in English.
2. Under **Import a profile from your browser**, enter its URL or numeric ID.
   Click the page's content in your browser and copy its text (Ctrl+A then Ctrl+C
   on Windows/Linux, Cmd+A then Cmd+C on Mac).
3. Paste into **Copied profile page**, or select **Paste from clipboard**, then
   **Read copied profile**. Clipboard access happens only when you press the button.
4. Select **Use profile**, review the entry and choose **Save filament**. The
   source URL is retained. A profile without TD stays unknown until you enter or
   measure it with your TD1/S.

You can also save a loaded page as HTML or export My Spools CSV and choose
**Import saved HTML / CSV**. Exported CSV can be pasted under **Paste exported CSV**.
**Filter imported profiles** searches the imported records locally by brand,
material/type, color or hex. ColorNinja does not upload library edits or device
measurements to the website.

CSV accepts columns such as `Brand,Material,Type,Color,RGB,TD,Filament ID`.
Brand, material, color name and a six-digit RGB hex value are required. TD and ID
are optional. Website changes may require an update to the page importer.

## Measure your spool

1. Connect TD1 or TD1S with a USB data cable. Close other applications that hold
   its serial port, including HueForge.
2. Open **TD1 / TD1S**, refresh the device list if necessary, and select **Connect**.
   ColorNinja reads the identity, firmware version and error log, and by default
   backs up the device's license, settings, pins and RGB offsets.
3. Insert filament and follow the device display. The latest TD/color appears
   with a timestamp. **Use TD for…** applies it to the selected filament editor;
   **Add as new filament** starts a new entry using the measured color.
4. **Also replace filament color** is off initially. Leave it off to replace only
   TD on an imported profile. Review and save the filament.

Saved measurements include device serial, timestamp and measured color, even if
you retain the profile's color. Previous TD values are retained in the JSON's
`ColorNinjaTDHistory`. The latest 100 readings are available during the connection
and can be exported as CSV; raw session diagnostics can also be exported.

For batch work, optionally enable **Open each new scan in the filament editor
when it has no unsaved edits**, then use **Save & add another** between spools.
This does not automatically overwrite unsaved work. Applying a reading still
requires saving the entry.

Windows uses the native COM port. Linux requires permission to open the serial
device (commonly the `dialout` group). Native macOS desktop builds use IOKit USB
discovery with cgo enabled. Pure-Go macOS CLI builds retain image processing but
do not provide USB discovery. TD1/S uses USB VID `E4B2`, PID `0045`.

## Settings and maintenance

**Load device settings** displays the settings advertised by the attached firmware:
display controller/orientation, RGB/hex display, sensor enablement, continuous TD
and color scanning, scan delay/rate, sample distance, optical button, raw RGB,
scale controls and screen mirroring. Only changed supported values are sent.
Saving settings restarts the device; reconnect after it finishes.

Persistent **Advanced** contains:

- Empty-lux calibration, RGB calibration, matrix reset and RGB percentage fine
  tuning. Follow the linked [vendor calibration guide](https://ajax-3d.com/td1-td1s-color-calibration-guide/)
  and the device prompts. Empty-lux calibration needs the filament path empty.
- Glow-value reading, error log, boot information, stored offsets/empty lux,
  raw USB messages and a downloadable license-request draft. Exact recurring
  error messages can be suppressed per device and restored later.
- Serial-numbered device backups with checksums and restoration to the same
  device. Choose a timestamped backup folder inside ColorNinja's `td1/<serial>/`
  directory. An incomplete or altered backup is rejected.
- Vendor firmware checks and SHA-256-verified downloads. Review a ZIP's files,
  checksum and prerequisites before **Back up & install firmware**. Keep USB
  connected. Transfer acknowledgement does not prove installation: reconnect and
  read the firmware version afterward. Packages with prerequisites require the
  specified earlier module versions first.
- Vendor license installation and bootloader entry. The bootloader command
  requires Comms 1.0.4 or newer. Legacy firmware may require the vendor's physical
  bootloader procedure.
- **UF2 recovery**: first connect and back up the working TD1/S in this session.
  Choose/download a TD1 UF2, its backup and the device's RPI-RP2 bootloader drive.
  Review the image, destination, serial and checksum, disconnect serial, then
  confirm the write. The bootloader identifies an RP2040 board, not a TD1 serial;
  you must select the drive belonging to the backed-up device. Reconnect, restore
  its backup and verify the firmware version. For a device that cannot connect
  and be identified/backed up, use the [vendor recovery tools](https://ajax-3d.com/resources/).

Firmware and license operations are explicit actions. ColorNinja does not bundle
vendor firmware or issue licenses. Read the [implementation evidence and validation
limits](td1-research.md) before treating software checks as hardware acceptance.
