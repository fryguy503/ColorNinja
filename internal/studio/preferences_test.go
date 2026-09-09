package studio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesPersistWithoutProcessingOrChangingProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := New(context.Background(), path)
	defer s.Shutdown()
	if !s.settings.Options.TotalColors || !s.settings.Preferences.CheckOnStartup || s.settings.Preferences.IncludePrereleases || s.settings.Preferences.Advanced {
		t.Fatal("wrong new-install defaults")
	}
	before := s.settings.Options
	p := Preferences{Advanced: true, IncludePrereleases: true, CheckOnStartup: false}
	if _, err := s.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	reopened := New(context.Background(), path)
	defer reopened.Shutdown()
	if reopened.settings.Preferences != p || reopened.settings.Options != before {
		t.Fatal("preferences or image settings changed")
	}
	// An old settings file lacks preferences and totalColors. Retain its image
	// settings while adding startup checks and keeping prereleases opt-in.
	b, _ := json.Marshal(s.settings)
	var legacy map[string]any
	_ = json.Unmarshal(b, &legacy)
	delete(legacy, "preferences")
	delete(legacy["options"].(map[string]any), "totalColors")
	b, _ = json.Marshal(legacy)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	old := New(context.Background(), path)
	defer old.Shutdown()
	if old.settings.Options.TotalColors || !old.settings.Preferences.CheckOnStartup || old.settings.Preferences.Advanced || old.settings.Preferences.IncludePrereleases {
		t.Fatal("old settings did not migrate safely")
	}
}
