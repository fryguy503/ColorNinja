package studio

import (
	"bytes"
	"colorninja/internal/engine"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func discoveryFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "engine", "testdata", "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeDiscoveryFixture(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestHueForgeLibraryDiscovery(t *testing.T) {
	raw := discoveryFixture(t)
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	appData := filepath.Join(root, "AppData", "Roaming")
	macRelative := filepath.FromSlash("Library/Containers/com.thehueforge.hueforge/Data/Documents/HueForge/Filaments/personal_library.json")
	windowsRelative := filepath.FromSlash("HueForge/Filaments/personal_library.json")
	macLibrary := filepath.Join(homeDir, macRelative)
	windowsLibrary := filepath.Join(appData, windowsRelative)
	writeDiscoveryFixture(t, macLibrary, raw)
	writeDiscoveryFixture(t, windowsLibrary, raw)
	// These decoys must never be picked when the user's directory is unavailable.
	writeDiscoveryFixture(t, filepath.Join(root, macRelative), raw)
	writeDiscoveryFixture(t, filepath.Join(root, windowsRelative), raw)
	t.Chdir(root)
	directoryRoot := filepath.Join(root, "directories")
	for _, relative := range []string{macRelative, windowsRelative} {
		if err := os.MkdirAll(filepath.Join(directoryRoot, relative), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, goos, homeDir, appData, want string
	}{
		{"macOS container", "darwin", homeDir, appData, macLibrary},
		{"Windows roaming", "windows", homeDir, appData, windowsLibrary},
		{"macOS missing library", "darwin", filepath.Join(root, "missing"), appData, ""},
		{"Windows missing library", "windows", homeDir, filepath.Join(root, "missing"), ""},
		{"macOS missing home", "darwin", "", appData, ""},
		{"Windows missing APPDATA", "windows", homeDir, "", ""},
		{"macOS relative home", "darwin", "home", appData, ""},
		{"Windows relative APPDATA", "windows", homeDir, filepath.Join("AppData", "Roaming"), ""},
		{"macOS directory is not a library", "darwin", directoryRoot, appData, ""},
		{"Windows directory is not a library", "windows", homeDir, directoryRoot, ""},
		{"unsupported platform", "linux", homeDir, appData, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectHueForgeLibrary(tc.goos, tc.homeDir, tc.appData); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStartupLibraryDiscoveryAndSavedSelection(t *testing.T) {
	raw := discoveryFixture(t)
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	appData := filepath.Join(root, "AppData", "Roaming")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("APPDATA", appData)
	defaults := map[string]string{
		"darwin":  filepath.Join(homeDir, filepath.FromSlash("Library/Containers/com.thehueforge.hueforge/Data/Documents/HueForge/Filaments/personal_library.json")),
		"windows": filepath.Join(appData, "HueForge", "Filaments", "personal_library.json"),
	}
	for _, path := range defaults {
		writeDiscoveryFixture(t, path, raw)
	}
	config := filepath.Join(root, "settings.json")
	first := New(context.Background(), config)
	snap := first.Snapshot()
	want := defaults[runtime.GOOS]
	if snap.Settings.LibraryPath != want {
		t.Fatalf("first launch: got %q, want %q", snap.Settings.LibraryPath, want)
	}
	if want != "" && (snap.Library == nil || len(snap.Library.Filaments) != 4) {
		t.Fatal("first launch did not load the detected library")
	}
	first.Shutdown()
	// Detection must also work after quitting without saving any preferences.
	restarted := New(context.Background(), config)
	if restarted.settings.LibraryPath != want {
		t.Fatal("unsaved automatic detection did not survive restart")
	}
	manual := filepath.Join(root, "Custom library", "personal_library.json")
	writeDiscoveryFixture(t, manual, raw)
	if _, err := restarted.SetLibrary(manual, engine.LibraryFilter{}); err != nil {
		t.Fatal(err)
	}
	restarted.Shutdown()
	reopened := New(context.Background(), config)
	snap = reopened.Snapshot()
	if snap.Settings.LibraryPath != manual || snap.Library == nil || len(snap.Library.Filaments) != 4 {
		t.Fatal("saved manual selection did not take priority over automatic discovery")
	}
	reopened.Shutdown()
	for _, path := range append([]string{manual}, defaults["darwin"], defaults["windows"]) {
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, raw) {
			t.Fatalf("library was modified: %s: %v", path, err)
		}
	}
	// A missing saved file should still surface the load error for that choice,
	// rather than silently switching the user to a different filament inventory.
	if err := os.Remove(manual); err != nil {
		t.Fatal(err)
	}
	missing := New(context.Background(), config)
	defer missing.Shutdown()
	snap = missing.Snapshot()
	if snap.Settings.LibraryPath != manual || snap.Library != nil || snap.Warning == "" {
		t.Fatal("a missing saved selection was silently replaced")
	}
}
