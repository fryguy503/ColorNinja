package studio

import (
	"os"
	"path/filepath"
)

// Only discover a default library when no library path was saved. Platform and
// user directories are arguments so every supported layout can be tested on CI.
func detectHueForgeLibrary(goos, homeDir, appData string) string {
	var directory string
	switch goos {
	case "windows":
		directory = appData
	case "darwin":
		directory = filepath.Join(homeDir, "Library", "Containers", "com.thehueforge.hueforge", "Data", "Documents")
	default:
		return ""
	}
	// Missing environment variables must not turn discovery into a search of the
	// application's working directory.
	if !filepath.IsAbs(directory) {
		return ""
	}
	candidate := filepath.Join(directory, "HueForge", "Filaments", "personal_library.json")
	if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
		return candidate
	}
	return ""
}
