package td1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type RecoveryPlan struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
	Blocks int    `json:"blocks"`
	Volume string `json:"volume"`
	Board  string `json:"board"`
	Serial string `json:"serial"`
	Backup string `json:"backup"`
}

// A TD1 UF2 recovery image contains RP2040 flash blocks. Never copy an arbitrary
// download to an arbitrary directory under the name of a firmware operation.
func ValidateUF2(raw []byte) error {
	if len(raw) == 0 || len(raw) > 16<<20 || len(raw)%512 != 0 {
		return fmt.Errorf("invalid UF2 file size")
	}
	seen := map[uint32]bool{}
	addresses := map[uint32]bool{}
	count := uint32(len(raw) / 512)
	for at := 0; at < len(raw); at += 512 {
		b := raw[at : at+512]
		u := func(i int) uint32 { return binary.LittleEndian.Uint32(b[i : i+4]) }
		if u(0) != 0x0a324655 || u(4) != 0x9e5d5157 || u(508) != 0x0ab16f30 || u(8) != 0x2000 || u(16) != 256 || u(24) != count || u(28) != 0xe48bff56 {
			return fmt.Errorf("UF2 is not a supported RP2040 flash image")
		}
		address, index := u(12), u(20)
		if address < 0x10000000 || address > 0x101fff00 || address%256 != 0 || index >= count || seen[index] || addresses[address] {
			return fmt.Errorf("invalid or duplicate UF2 flash block")
		}
		seen[index] = true
		addresses[address] = true
	}
	return nil
}
func RecoveryVolume(volume string) (string, string, error) {
	dir, e := filepath.Abs(volume)
	if e != nil {
		return "", "", e
	}
	resolved, e := filepath.EvalSymlinks(dir)
	if e != nil || !strings.EqualFold(dir, resolved) {
		return "", "", fmt.Errorf("select the actual RPI-RP2 bootloader volume")
	}
	f, e := os.Open(filepath.Join(dir, "INFO_UF2.TXT"))
	if e != nil {
		return "", "", fmt.Errorf("selected folder is not a UF2 bootloader volume")
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(raw) > 4096 {
		return "", "", fmt.Errorf("cannot read bootloader identification")
	}
	info := string(raw)
	if !strings.Contains(info, "Board-ID: RPI-RP2") || !strings.Contains(info, "UF2 Bootloader") {
		return "", "", fmt.Errorf("selected volume is not an RP2040 bootloader")
	}
	return dir, info, nil
}
func PlanRecovery(raw []byte, volume string) (RecoveryPlan, error) {
	p := RecoveryPlan{SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Size: len(raw), Blocks: len(raw) / 512}
	if e := ValidateUF2(raw); e != nil {
		return p, e
	}
	var e error
	p.Volume, p.Board, e = RecoveryVolume(volume)
	return p, e
}
func ApplyRecovery(ctx context.Context, raw []byte, p RecoveryPlan) error {
	fresh, e := PlanRecovery(raw, p.Volume)
	if e != nil {
		return e
	}
	if fresh.SHA256 != p.SHA256 || fresh.Board != p.Board {
		return fmt.Errorf("recovery image or bootloader changed since review")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	// Bootloader volumes consume UF2 writes immediately; atomic rename is not a
	// valid protocol here. Refuse an existing target and write only reviewed data.
	f, e := os.OpenFile(filepath.Join(p.Volume, "ColorNinja-TD1.uf2"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(f, bytes.NewReader(raw))
	closeErr := f.Close()
	if e != nil {
		return fmt.Errorf("UF2 write interrupted; device recovery may be incomplete: %w", e)
	}
	if closeErr != nil {
		return fmt.Errorf("UF2 transfer ended with an error; reconnect and verify firmware: %w", closeErr)
	}
	return nil
}
