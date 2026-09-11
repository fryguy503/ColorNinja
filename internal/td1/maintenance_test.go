package td1

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

func updateZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range files {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(data)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestFirmwarePlanAndPrerequisites(t *testing.T) {
	p, e := PlanUpdate(updateZIP(t, map[string]string{"lib/TD1.mpy": "firmware", "lib/TD1.bin": strings.Repeat("x", 256), "prereqs.json": `{"Comms.mpy":"2.0.1","bootloaderMinimum":"1.0.4"}`}))
	if e != nil || len(p.Files) != 2 || len(p.SHA256) != 64 {
		t.Fatal(p, e)
	}
	if e = CheckPrerequisites(p, "version,TD1 Version: V2.0.0,Comms Version: V2.0.2"); e != nil {
		t.Fatal(e)
	}
	for _, version := range []string{"version,Comms Version: 2.0.0", "version,TD1 Version: 2.0.2", "garbage"} {
		if e = CheckPrerequisites(p, version); e == nil {
			t.Fatal("accepted missing prerequisite", version)
		}
	}
}
func TestFirmwareRejectsUnsafeOrUnboundedPaths(t *testing.T) {
	for _, name := range []string{"../license.bin", "lib/../../settings.py", "lib\\TD1.mpy", "lib/TD1.mpy\nupdate", "/code.py", "settings.py", "lib/sub/TD1.mpy", "lib/firmware.exe"} {
		if _, e := PlanUpdate(updateZIP(t, map[string]string{name: "bad"})); e == nil {
			t.Fatal("unsafe ZIP accepted", name)
		}
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "lib/TD1.mpy"}
	h.SetMode(os.ModeSymlink | 0777)
	w, _ := z.CreateHeader(h)
	w.Write([]byte("/etc/passwd"))
	z.Close()
	if _, e := PlanUpdate(b.Bytes()); e == nil {
		t.Fatal("symlink accepted")
	}
	if _, e := PlanUpdate(updateZIP(t, map[string]string{"lib/TD1.mpy": strings.Repeat("x", (1<<20)+1)})); e == nil {
		t.Fatal("oversized expanded file accepted")
	}
}
func TestFirmwareDownloadOriginAndVersion(t *testing.T) {
	d := FirmwareDownload{URL: "https://github.com/AJAX-3D-LLC/TD1iTs-version/releases/download/td1-firmware-v2.0.2/update-2.0.2.zip", SHA256: strings.Repeat("A", 64), Size: 31527}
	if e := ValidateDownload(d); e != nil {
		t.Fatal(e)
	}
	for _, u := range []string{"http://github.com/AJAX-3D-LLC/TD1iTs-version/releases/download/test.zip", "https://evil.example/update.zip", "https://github.com/other/repo/releases/download/x.zip", "https://user@github.com/AJAX-3D-LLC/TD1iTs-version/releases/download/test.zip"} {
		d.URL = u
		if e := ValidateDownload(d); e == nil {
			t.Fatal("unsafe URL", u)
		}
	}
	if !versionAtLeast("2.10.0", "2.9.9") || versionAtLeast("2.0.0", "2.0.1") || versionAtLeast("2.0", "2.0.0") {
		t.Fatal("version comparison")
	}
}
