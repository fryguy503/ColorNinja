package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"colorninja/internal/engine"
	"colorninja/internal/studio"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

type App struct {
	ctx    context.Context
	studio *studio.Studio
	config string
}

func main() {
	dev := flag.String("dev-server", "", "serve the real app on a loopback address for browser integration testing")
	config := flag.String("config-dir", "", "override the settings directory")
	flag.Parse()
	if *config == "" {
		dir, e := os.UserConfigDir()
		if e != nil {
			log.Fatal(e)
		}
		*config = filepath.Join(dir, "ColorNinja")
	}
	configPath := filepath.Join(*config, "settings.json")
	if *dev != "" {
		root, e := fs.Sub(assets, "frontend/dist")
		if e != nil {
			log.Fatal(e)
		}
		if e = serveDevelopment(*dev, configPath, root); e != nil {
			log.Fatal(e)
		}
		return
	}
	app := &App{config: configPath}
	err := wails.Run(&options.App{Title: "ColorNinja Studio", Width: 1440, Height: 980, MinWidth: 1080, MinHeight: 720, BackgroundColour: &options.RGBA{R: 22, G: 24, B: 25, A: 255}, AssetServer: &assetserver.Options{Assets: assets, Handler: mediaHandler{app}}, OnStartup: app.startup, OnShutdown: func(context.Context) {
		if app.studio != nil {
			app.studio.Shutdown()
		}
	}, Bind: []interface{}{app}, DragAndDrop: &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true}, Windows: &windows.Options{WebviewIsTransparent: false, WindowIsTranslucent: false, Theme: windows.Dark, WebviewUserDataPath: filepath.Join(*config, "webview")}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.studio = studio.New(ctx, a.config)
	a.studio.Emit = func(name string, v any) { runtime.EventsEmit(ctx, name, v) }
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		if len(paths) > 0 {
			runtime.EventsEmit(ctx, "file-dropped", paths[0])
		}
	})
}
func (a *App) emitCommand(command string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "command", command)
	}
}
func (a *App) Initialize() (studio.Snapshot, error)           { return a.studio.Initialize() }
func (a *App) UseDemo() (studio.Snapshot, error)              { return a.studio.UseDemo() }
func (a *App) LoadImage(path string) (studio.Snapshot, error) { return a.studio.LoadImage(path) }
func (a *App) OpenImage() (*studio.Snapshot, error) {
	path, e := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open an image", Filters: []runtime.FileFilter{{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.gif;*.tif;*.tiff;*.bmp"}}})
	if e != nil || path == "" {
		return nil, e
	}
	s, e := a.studio.LoadImage(path)
	return &s, e
}
func (a *App) ChooseLibrary() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose your HueForge filament library", Filters: []runtime.FileFilter{{DisplayName: "Filament library", Pattern: "*.json"}}})
}
func (a *App) SetLibrary(path string, filter engine.LibraryFilter) (*engine.Library, error) {
	return a.studio.SetLibrary(path, filter)
}
func (a *App) Process(req studio.Request) (*studio.Preview, error) { return a.studio.Process(req) }
func (a *App) Cancel()                                             { a.studio.Cancel() }
func (a *App) SavePreset(name string, o engine.Options) ([]studio.Preset, error) {
	return a.studio.SavePreset(name, o)
}
func (a *App) DeletePreset(name string) ([]studio.Preset, error) { return a.studio.DeletePreset(name) }
func (a *App) SaveProject(req studio.Request) (string, error) {
	path, e := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Save ColorNinja project", DefaultFilename: "Untitled.colorninja.json", Filters: []runtime.FileFilter{{DisplayName: "ColorNinja project", Pattern: "*.json"}}, CanCreateDirectories: true})
	if e != nil || path == "" {
		return "", e
	}
	if filepath.Ext(path) == "" {
		path += ".colorninja.json"
	}
	overwrite, e := a.confirmOverwrite(path)
	if e != nil {
		return "", e
	}
	if e = a.studio.SaveProject(path, req, overwrite); e != nil {
		return "", e
	}
	return path, nil
}
func (a *App) OpenProject() (*studio.Snapshot, error) {
	path, e := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open ColorNinja project", Filters: []runtime.FileFilter{{DisplayName: "ColorNinja project", Pattern: "*.json"}}})
	if e != nil || path == "" {
		return nil, e
	}
	s, e := a.studio.OpenProject(path)
	return &s, e
}
func (a *App) confirmOverwrite(path string) (bool, error) {
	if _, e := os.Stat(path); os.IsNotExist(e) {
		return false, nil
	} else if e != nil {
		return false, e
	}
	choice, e := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{Type: runtime.QuestionDialog, Title: "Replace existing file?", Message: filepath.Base(path) + " already exists. Replace it?", Buttons: []string{"Replace", "Cancel"}, DefaultButton: "Cancel", CancelButton: "Cancel"})
	if e != nil {
		return false, e
	}
	if choice != "Replace" {
		return false, fmt.Errorf("export canceled")
	}
	return true, nil
}
func (a *App) Export(kind string, id, revision uint64) (string, error) {
	s := a.studio.Snapshot()
	base := strings.TrimSuffix(s.Source.Name, filepath.Ext(s.Source.Name))
	ext, suffix, label := ".png", "-colorninja", "PNG image"
	if kind == "palette" {
		ext, suffix, label = ".json", "-palette", "Palette report"
	} else if kind == "layers" {
		suffix, label = "-layers", "16-bit layer map"
	}
	path, e := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Export " + label, DefaultFilename: base + suffix + ext, Filters: []runtime.FileFilter{{DisplayName: label, Pattern: "*" + ext}}, CanCreateDirectories: true})
	if e != nil || path == "" {
		return "", e
	}
	if !strings.EqualFold(filepath.Ext(path), ext) {
		path += ext
	}
	overwrite, e := a.confirmOverwrite(path)
	if e != nil {
		return "", e
	}
	if e = a.studio.Export(kind, path, id, revision, overwrite); e != nil {
		return "", e
	}
	return path, nil
}
