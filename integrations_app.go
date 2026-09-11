package main

import (
	"colorninja/internal/filamentprofiles"
	"colorninja/internal/studio"
	"colorninja/internal/td1"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) FilamentCatalog(path string) (studio.FilamentCatalog, error) {
	return a.studio.FilamentCatalog(path)
}
func (a *App) EditFilament(edit studio.FilamentEdit) (studio.FilamentCatalog, error) {
	return a.studio.EditFilament(edit)
}
func (a *App) ImportCopiedFilamentProfile(value, content string) (filamentprofiles.Profile, error) {
	return a.studio.ImportCopiedFilamentProfile(value, content)
}
func (a *App) ReadFilamentClipboard() (string, error) { return runtime.ClipboardGetText(a.ctx) }
func (a *App) OpenFilamentProfile(value string) error {
	target, e := filamentprofiles.ProfileURL(value)
	if e != nil {
		return e
	}
	runtime.BrowserOpenURL(a.ctx, target)
	return nil
}
func (a *App) ImportFilamentProfiles(content, format string) (filamentprofiles.Results, error) {
	return a.studio.ImportFilamentProfiles(content, format)
}
func (a *App) TD1Ports() ([]td1.Port, error)             { return td1.Ports() }
func (a *App) TD1State() td1.State                       { return a.studio.TD1.State() }
func (a *App) ConnectTD1(port string) (td1.State, error) { return a.studio.ConnectTD1(port) }
func (a *App) DisconnectTD1()                            { a.studio.DisconnectTD1() }
func (a *App) TD1Operation(req studio.TD1Request) (studio.TD1Response, error) {
	return a.studio.TD1Operation(req)
}
func (a *App) ChooseTD1File(kind string) (string, error) {
	switch kind {
	case "recovery":
		return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose TD1 recovery UF2", Filters: []runtime.FileFilter{{DisplayName: "TD1 RP2040 firmware", Pattern: "*.uf2"}}})
	case "volume":
		return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select this TD1's RPI-RP2 bootloader drive"})
	case "firmware":
		return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose TD1 firmware ZIP", Filters: []runtime.FileFilter{{DisplayName: "TD1 firmware ZIP", Pattern: "*.zip"}}})
	case "license":
		return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose the vendor license for this TD1", Filters: []runtime.FileFilter{{DisplayName: "TD1 license", Pattern: "*.bin"}}})
	case "backup":
		return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose this TD1's backup folder"})
	}
	return "", fmt.Errorf("unknown TD1 file type")
}
func (a *App) ExportFilamentLibrary(path string) (string, error) {
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Export HueForge-compatible filament library", DefaultFilename: "colorninja-filaments.json", Filters: []runtime.FileFilter{{DisplayName: "Filament library", Pattern: "*.json"}}})
	if err != nil || target == "" {
		return "", err
	}
	// The native Save As dialog already confirms replacement of this exact path.
	err = a.studio.ExportFilamentLibrary(path, target, true)
	return target, err
}
func (a *App) OpenFilamentResource(kind string) error {
	var target string
	switch kind {
	case "profiles":
		target = filamentprofiles.Origin + "/filaments"
	case "td1":
		target = "https://ajax-3d.com/resources/"
	case "calibration":
		target = "https://ajax-3d.com/td1-td1s-color-calibration-guide/"
	default:
		return fmt.Errorf("unknown filament resource")
	}
	runtime.BrowserOpenURL(a.ctx, target)
	return nil
}
