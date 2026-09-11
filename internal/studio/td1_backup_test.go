package studio

import (
	"colorninja/internal/td1"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDeviceBackupRequiresSerialAndChecksums(t *testing.T) {
	s := New(context.Background(), filepath.Join(t.TempDir(), "settings.json"))
	defer s.Shutdown()
	serial := "TEST123"
	dir := filepath.Join(filepath.Dir(s.configPath), "td1", serial, "backup-test")
	os.MkdirAll(dir, 0700)
	m := td1BackupManifest{Serial: serial, Files: map[string]string{}}
	for _, name := range []string{"settings.py", "pins.py"} {
		raw := []byte("sample file\n")
		os.WriteFile(filepath.Join(dir, name), raw, 0600)
		m.Files[name] = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	raw, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "backup.json"), raw, 0600)
	files, e := s.readTD1Backup(dir, serial)
	if e != nil || len(files) != 2 {
		t.Fatal(files, e)
	}
	if _, e = s.readTD1Backup(dir, "ANOTHER123"); e == nil {
		t.Fatal("other device backup accepted")
	}
	os.WriteFile(filepath.Join(dir, "settings.py"), []byte("changed"), 0600)
	if _, e = s.readTD1Backup(dir, serial); e == nil {
		t.Fatal("changed backup accepted")
	}
	s.recoveryPlan = &td1.RecoveryPlan{SHA256: "reviewed"}
	if _, e = s.TD1Operation(TD1Request{Action: "apply-recovery", SHA256: "different"}); e == nil {
		t.Fatal("unreviewed recovery accepted")
	}
}
