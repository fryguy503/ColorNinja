package studio

import (
	"colorninja/internal/engine"
	"colorninja/internal/filamentprofiles"
	"colorninja/internal/td1"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

func (s *Studio) ImportCopiedFilamentProfile(value, content string) (filamentprofiles.Profile, error) {
	return filamentprofiles.ParseCopiedProfile(value, content)
}
func (s *Studio) ImportFilamentProfiles(content, format string) (filamentprofiles.Results, error) {
	if format == "csv" {
		return filamentprofiles.ParseCSV([]byte(content))
	}
	if format == "html" {
		return filamentprofiles.ParseHTML([]byte(content))
	}
	return filamentprofiles.Results{}, fmt.Errorf("choose a CSV or saved HTML page")
}

type TD1Request struct {
	Action string            `json:"action"`
	Values map[string]string `json:"values"`
	Red    int               `json:"red"`
	Green  int               `json:"green"`
	Blue   int               `json:"blue"`
	Path   string            `json:"path"`
	SHA256 string            `json:"sha256"`
	Volume string            `json:"volume"`
	Backup string            `json:"backup"`
}
type TD1Response struct {
	State    td1.State         `json:"state"`
	Text     string            `json:"text"`
	Settings map[string]string `json:"settings,omitempty"`
	Files    []string          `json:"files,omitempty"`
	Firmware *td1.Firmware     `json:"firmware,omitempty"`
	Plan     *td1.UpdatePlan   `json:"plan,omitempty"`
	Path     string            `json:"path,omitempty"`
	Recovery *td1.RecoveryPlan `json:"recovery,omitempty"`
}

func (s *Studio) td1Backup() ([]string, error) {
	state := s.TD1.State()
	if !state.Connected {
		return nil, fmt.Errorf("connect your TD1 first")
	}
	serial := state.Port.Serial
	if serial == "" || strings.ContainsAny(serial, "/\\:.") {
		return nil, fmt.Errorf("device has no usable serial number")
	}
	dir := filepath.Join(filepath.Dir(s.configPath), "td1", serial, time.Now().UTC().Format("20060102T150405.000000000"))
	files := []string{}
	manifest := td1BackupManifest{Serial: serial, Files: map[string]string{}}
	for _, name := range []string{"license.bin", "settings.py", "pins.py", "lib/rgbOffset.py"} {
		raw, err := s.TD1.ReadFile(name)
		if errors.Is(err, td1.ErrNoFile) && (name == "license.bin" || name == "lib/rgbOffset.py") {
			continue
		}
		if err != nil {
			return files, fmt.Errorf("backup %s: %w", name, err)
		}
		if len(raw) == 0 {
			return files, fmt.Errorf("device returned an empty %s; backup incomplete", name)
		}
		if name == "license.bin" && len(raw) != 256 {
			return files, fmt.Errorf("device license has an unexpected size; backup incomplete")
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err = engine.AtomicWrite(path, false, func(w io.Writer) error { _, e := w.Write(raw); return e }); err != nil {
			return files, err
		}
		files = append(files, path)
		manifest.Files[name] = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(dir, "backup.json")
	if err := engine.AtomicWrite(manifestPath, false, func(w io.Writer) error { _, e := w.Write(raw); return e }); err != nil {
		return files, err
	}
	files = append(files, manifestPath)
	return files, nil
}
func (s *Studio) TD1Operation(req TD1Request) (TD1Response, error) {
	s.deviceMu.Lock()
	defer s.deviceMu.Unlock()
	r := TD1Response{}
	var err error
	switch req.Action {
	case "download-recovery":
		fw, e := td1.CheckFirmware(s.ctx)
		if e != nil {
			return r, e
		}
		raw, e := td1.DownloadFirmware(s.ctx, fw.Downloads["uf2Update"])
		if e != nil {
			return r, e
		}
		if e = td1.ValidateUF2(raw); e != nil {
			return r, e
		}
		r.Path = filepath.Join(filepath.Dir(s.configPath), "td1", "downloads", "TD1-"+fw.Version+".uf2")
		err = engine.AtomicWrite(r.Path, true, func(w io.Writer) error { _, e := w.Write(raw); return e })
		r.Text = "Recovery image downloaded and SHA-256 verified"
		r.Firmware = &fw
	case "plan-recovery":
		s.recoveryPlan = nil
		serial := s.TD1.State().Port.Serial
		if serial == "" || strings.ContainsAny(serial, "/\\:.") {
			return r, fmt.Errorf("connect and back up this TD1 in this session before entering bootloader mode")
		}
		if _, e := s.readTD1Backup(req.Backup, serial); e != nil {
			return r, e
		}
		raw, e := readBounded(req.Path, 16<<20)
		if e != nil {
			return r, e
		}
		plan, e := td1.PlanRecovery(raw, req.Volume)
		if e != nil {
			return r, e
		}
		plan.Serial = serial
		plan.Backup = req.Backup
		plan.Path = req.Path
		s.recoveryPlan = &plan
		r.Recovery = &plan
		r.Text = "Review the connected bootloader volume and backed-up device before recovery"
	case "apply-recovery":
		if s.recoveryPlan == nil || req.SHA256 != s.recoveryPlan.SHA256 {
			return r, fmt.Errorf("review the recovery image, device backup and bootloader volume first")
		}
		plan := *s.recoveryPlan
		if s.TD1.State().Connected {
			return r, fmt.Errorf("disconnect the serial connection before writing to the bootloader volume")
		}
		if _, e := s.readTD1Backup(plan.Backup, plan.Serial); e != nil {
			return r, e
		}
		raw, e := readBounded(plan.Path, 16<<20)
		if e != nil {
			return r, e
		}
		s.recoveryPlan = nil
		err = td1.ApplyRecovery(s.ctx, raw, plan)
		r.Text = "UF2 transfer finished. Reconnect the TD1/S, restore its saved backup, then read its firmware version to verify recovery."
	case "settings":
		var raw []byte
		raw, err = s.TD1.ReadFile("settings.py")
		if err == nil {
			r.Settings = td1.ParseSettings(raw)
			r.Text = "Device settings loaded"
		}
	case "save-settings":
		// Only fields actually advertised by this firmware can be changed.
		var raw []byte
		raw, err = s.TD1.ReadFile("settings.py")
		if err == nil {
			current := td1.ParseSettings(raw)
			for key := range req.Values {
				if _, ok := current[key]; !ok {
					return r, fmt.Errorf("device firmware does not support %s", key)
				}
			}
			merged := map[string]string{}
			for k, v := range current {
				merged[k] = v
			}
			for k, v := range req.Values {
				merged[k] = v
			}
			if _, e := td1.SettingsCommand(merged); e != nil {
				return r, e
			}
			r.Text, err = s.TD1.ApplySettings(req.Values)
		}
	case "adjust-rgb":
		r.Text, err = s.TD1.AdjustRGB(req.Red, req.Green, req.Blue)
	case "errors", "boot-info", "rgb-offsets", "empty-lux":
		file := map[string]string{"errors": "errors.txt", "boot-info": "boot_out.txt", "rgb-offsets": "lib/rgbOffset.py", "empty-lux": "emptyLux.txt"}[req.Action]
		var raw []byte
		raw, err = s.TD1.ReadFile(file)
		r.Text = string(raw)
		if req.Action == "errors" && errors.Is(err, td1.ErrNoFile) {
			err = nil
			r.Text = ""
		}
	case "license-request":
		var raw []byte
		raw, err = s.TD1.ReadFile("boot_out.txt")
		if err == nil {
			r.Text = "TD1 / TD1S license request\nSerial: " + s.TD1.State().Port.Serial + "\n\n" + string(raw)
		}
	case "backup":
		r.Files, err = s.td1Backup()
		if err == nil && len(r.Files) > 0 {
			r.Path = filepath.Dir(r.Files[len(r.Files)-1])
		}
		r.Text = "Available device license, settings, pins, and RGB offsets backed up with checksums"
	case "check-firmware":
		var fw td1.Firmware
		fw, err = td1.CheckFirmware(s.ctx)
		if err == nil {
			r.Firmware = &fw
			r.Text = "Latest TD1 firmware: " + fw.Version
		}
	case "download-firmware":
		var fw td1.Firmware
		fw, err = td1.CheckFirmware(s.ctx)
		if err == nil {
			var raw []byte
			raw, err = td1.DownloadFirmware(s.ctx, fw.Downloads["zipUpdate"])
			if err == nil {
				r.Path = filepath.Join(filepath.Dir(s.configPath), "td1", "downloads", "update-"+fw.Version+".zip")
				err = engine.AtomicWrite(r.Path, true, func(w io.Writer) error { _, e := w.Write(raw); return e })
				if err == nil {
					var plan td1.UpdatePlan
					plan, err = td1.PlanUpdate(raw)
					r.Plan = &plan
					r.Firmware = &fw
					r.Text = "Firmware downloaded and SHA-256 verified. Review before applying."
				}
			}
		}
	case "plan-firmware":
		var raw []byte
		raw, err = readBounded(req.Path, 16<<20)
		if err == nil {
			var plan td1.UpdatePlan
			plan, err = td1.PlanUpdate(raw)
			r.Plan = &plan
			r.Path = req.Path
		}
	case "apply-firmware":
		var raw []byte
		raw, err = readBounded(req.Path, 16<<20)
		if err != nil {
			break
		}
		var plan td1.UpdatePlan
		plan, err = td1.PlanUpdate(raw)
		if err != nil {
			break
		}
		if plan.SHA256 != req.SHA256 {
			return r, fmt.Errorf("firmware changed since review; inspect it again")
		}
		// Prerequisite-aware staged updates must not skip a required earlier update.
		var version string
		version, err = s.TD1.Action("version")
		if err != nil {
			break
		}
		if err = td1.CheckPrerequisites(plan, version); err != nil {
			break
		}
		r.Files, err = s.td1Backup()
		if err == nil {
			r.Text, err = s.TD1.Upload(plan.Files)
		}
	case "apply-license":
		var raw []byte
		raw, err = readBounded(req.Path, 4096)
		if err != nil {
			break
		}
		base := filepath.Base(req.Path)
		serial := s.TD1.State().Port.Serial
		if !strings.EqualFold(base, "license.bin") && !strings.EqualFold(base, serial+".bin") {
			return r, fmt.Errorf("select license.bin or the license named for this device's serial number")
		}
		if len(raw) != 256 {
			return r, fmt.Errorf("TD1 license must be a 256-byte vendor license file")
		}
		r.Text, err = s.TD1.Upload([]td1.UpdateFile{{Name: "license.bin", Size: len(raw), Data: raw}})
	case "restore-backup":
		// The selected backup belongs to this connected device, never another TD1.
		serial := s.TD1.State().Port.Serial
		if serial == "" || strings.ContainsAny(serial, "/\\:.") {
			return r, fmt.Errorf("connect the TD1 with a usable serial number first")
		}
		files, e := s.readTD1Backup(req.Path, serial)
		if e != nil {
			return r, e
		}
		r.Text, err = s.TD1.Upload(files)
	default:
		r.Text, err = s.TD1.Action(req.Action)
	}
	r.State = s.TD1.State()
	return r, err
}

func (s *Studio) ConnectTD1(port string) (td1.State, error) {
	s.deviceMu.Lock()
	defer s.deviceMu.Unlock()
	return s.TD1.Connect(port)
}
func (s *Studio) DisconnectTD1() { s.TD1.Disconnect() }

func (s *Studio) SaveTD1Diagnostics(path string) error {
	state := s.TD1.State()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return engine.AtomicWrite(path, false, func(w io.Writer) error { _, e := w.Write(raw); return e })
}
