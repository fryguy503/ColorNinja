package td1

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type FirmwareDownload struct {
	URL    string `json:"downloadUrl"`
	Name   string `json:"fileName"`
	Size   int64  `json:"fileSize"`
	SHA256 string `json:"sha256"`
}
type Firmware struct {
	Version           string                      `json:"latestVersion"`
	Minimum           string                      `json:"minimumSupportedVersion"`
	MinimumBootloader string                      `json:"minimumBootloaderCommandVersion"`
	Notes             []string                    `json:"releaseNotes"`
	Downloads         map[string]FirmwareDownload `json:"downloads"`
}

func CheckFirmware(ctx context.Context) (Firmware, error) {
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return fmt.Errorf("unexpected firmware manifest redirect")
	}}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://version.ajax-3d.com/version.json", nil)
	if err != nil {
		return Firmware{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Firmware{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Firmware{}, fmt.Errorf("firmware server returned HTTP %d", resp.StatusCode)
	}
	var manifest struct {
		Schema   int                 `json:"schemaVersion"`
		Firmware map[string]Firmware `json:"firmware"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&manifest); err != nil {
		return Firmware{}, err
	}
	fw, ok := manifest.Firmware["TD1"]
	if !ok || manifest.Schema != 1 || !versionPattern.MatchString(fw.Version) {
		return fw, fmt.Errorf("unrecognized TD1 firmware manifest")
	}
	return fw, nil
}
func ValidateDownload(d FirmwareDownload) error {
	u, err := url.Parse(d.URL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || !strings.HasPrefix(u.Path, "/AJAX-3D-LLC/TD1iTs-version/releases/download/") || u.User != nil || u.RawQuery != "" {
		return fmt.Errorf("firmware download must come from AJAX-3D's official releases")
	}
	if d.Size <= 0 || d.Size > 16<<20 || len(d.SHA256) != 64 {
		return fmt.Errorf("firmware manifest has no usable size/checksum")
	}
	return nil
}
func DownloadFirmware(ctx context.Context, d FirmwareDownload) ([]byte, error) {
	if err := ValidateDownload(d); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		host := r.URL.Host
		if len(via) > 4 || r.URL.Scheme != "https" || (host != "github.com" && host != "release-assets.githubusercontent.com" && host != "objects.githubusercontent.com") {
			return fmt.Errorf("unexpected firmware download redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, "GET", d.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("firmware download returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, d.Size+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != d.Size || !strings.EqualFold(fmt.Sprintf("%x", sha256.Sum256(raw)), d.SHA256) {
		return nil, fmt.Errorf("firmware download failed size/SHA-256 verification")
	}
	return raw, nil
}

type UpdateFile struct {
	Name string `json:"name"`
	Size int    `json:"size"`
	Data []byte `json:"-"`
}
type UpdatePlan struct {
	SHA256        string            `json:"sha256"`
	Files         []UpdateFile      `json:"files"`
	Prerequisites map[string]string `json:"prerequisites"`
}

func PlanUpdate(raw []byte) (UpdatePlan, error) {
	p := UpdatePlan{SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Files: []UpdateFile{}, Prerequisites: map[string]string{}}
	if len(raw) > 16<<20 {
		return p, fmt.Errorf("firmware ZIP exceeds 16 MiB")
	}
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return p, err
	}
	total := uint64(0)
	seen := map[string]bool{}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
		if strings.ContainsAny(name, "\\:\r\n\x00") || strings.HasPrefix(name, "/") || path.Clean(name) != name || strings.HasPrefix(name, "../") || f.Mode()&os.ModeType != 0 {
			return p, fmt.Errorf("unsafe firmware path")
		}
		// Official ZIP files may have one enclosing update directory.
		if strings.HasPrefix(name, "update/") {
			name = strings.TrimPrefix(name, "update/")
		}
		if strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(path.Base(name), ".") {
			continue
		}
		if name != "prereqs.json" && name != "code.py" && name != "boot.py" && (!strings.HasPrefix(name, "lib/") || strings.Count(name, "/") != 1 || !(strings.HasSuffix(name, ".mpy") || strings.HasSuffix(name, ".bin"))) {
			return p, fmt.Errorf("unexpected firmware file %s", name)
		}
		if seen[name] {
			return p, fmt.Errorf("duplicate firmware file")
		}
		seen[name] = true
		total += f.UncompressedSize64
		if total > 16<<20 || len(p.Files) >= 256 {
			return p, fmt.Errorf("firmware expands beyond supported size")
		}
		r, e := f.Open()
		if e != nil {
			return p, e
		}
		data, e := io.ReadAll(io.LimitReader(r, 1<<20+1))
		r.Close()
		if e != nil {
			return p, e
		}
		if len(data) > 1<<20 {
			return p, fmt.Errorf("firmware file is too large")
		}
		if name == "prereqs.json" {
			if e = json.Unmarshal(data, &p.Prerequisites); e != nil {
				return p, e
			}
			continue
		}
		p.Files = append(p.Files, UpdateFile{Name: name, Size: len(data), Data: data})
	}
	if len(p.Files) == 0 {
		return p, fmt.Errorf("firmware ZIP has no update files")
	}
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Name < p.Files[j].Name })
	return p, nil
}
func (m *Manager) Upload(files []UpdateFile) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no files to send")
	}
	for _, f := range files {
		if len(f.Data) == 0 || len(f.Data) > 1<<20 || strings.ContainsAny(f.Name, "\\:\r\n\x00") || path.Clean(f.Name) != f.Name || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") {
			return "", fmt.Errorf("invalid TD1 update file")
		}
	}
	v, err := m.run(3*time.Minute, func(ctx context.Context, w *wire) (any, error) {
		if err := w.send([]byte("update\n")); err != nil {
			return nil, err
		}
		if err := w.ready(ctx); err != nil {
			return nil, err
		}
		for _, f := range files {
			m.mu.Lock()
			m.state.Status = "Sending " + f.Name
			m.mu.Unlock()
			if err := w.sendFile(ctx, f.Name, f.Data); err != nil {
				return nil, err
			}
		}
		if err := w.send([]byte("done\n")); err != nil {
			return nil, err
		}
		// Firmware commits and can reboot without another serial reply. Every
		// block was acknowledged; reconnect/version checking confirms install.
		return "Transfer acknowledged. Reconnect the TD1/S after restart and read its firmware version to verify installation.", nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var reportedVersion = regexp.MustCompile(`(?i)([a-z0-9]+)\s+Version:\s*[vV]?([0-9]+\.[0-9]+\.[0-9]+)`)

func Versions(reply string) map[string]string {
	result := map[string]string{}
	for _, m := range reportedVersion.FindAllStringSubmatch(reply, -1) {
		result[strings.ToLower(m[1])+".mpy"] = m[2]
	}
	return result
}
func versionAtLeast(current, minimum string) bool {
	if !versionPattern.MatchString(current) || !versionPattern.MatchString(minimum) {
		return false
	}
	a, b := strings.Split(current, "."), strings.Split(minimum, ".")
	for i := range a {
		av, ea := strconv.Atoi(a[i])
		bv, eb := strconv.Atoi(b[i])
		if ea != nil || eb != nil {
			return false
		}
		if av != bv {
			return av > bv
		}
	}
	return true
}
func CheckPrerequisites(p UpdatePlan, reply string) error {
	versions := Versions(reply)
	for name, minimum := range p.Prerequisites {
		// HueForge uses this value to gate the optional bootloader command,
		// not as the version of a file installed by a serial update.
		if name == "bootloaderMinimum" {
			continue
		}
		current := versions[strings.ToLower(name)]
		if !versionAtLeast(current, minimum) {
			return fmt.Errorf("%s requires version %s first (device reports %q); apply the vendor prerequisite update before this ZIP", name, minimum, current)
		}
	}
	return nil
}
