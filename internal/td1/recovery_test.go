package td1

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func testUF2() []byte {
	raw := make([]byte, 1024)
	for block := 0; block < 2; block++ {
		b := raw[block*512:]
		for offset, value := range map[int]uint32{0: 0x0a324655, 4: 0x9e5d5157, 8: 0x2000, 12: 0x10000000 + uint32(block*256), 16: 256, 20: uint32(block), 24: 2, 28: 0xe48bff56, 508: 0x0ab16f30} {
			binary.LittleEndian.PutUint32(b[offset:offset+4], value)
		}
	}
	return raw
}
func TestUF2RecoveryRequiresReviewedBoardAndImage(t *testing.T) {
	dir := t.TempDir()
	raw := testUF2()
	if e := ValidateUF2(raw); e != nil {
		t.Fatal(e)
	}
	if _, e := PlanRecovery(raw, dir); e == nil {
		t.Fatal("accepted ordinary folder")
	}
	os.WriteFile(filepath.Join(dir, "INFO_UF2.TXT"), []byte("UF2 Bootloader v3.0\nBoard-ID: RPI-RP2\n"), 0600)
	p, e := PlanRecovery(raw, dir)
	if e != nil || p.Blocks != 2 {
		t.Fatal(p, e)
	}
	changed := bytes.Clone(raw)
	changed[40] = 1
	if e = ApplyRecovery(context.Background(), changed, p); e == nil {
		t.Fatal("image changed after approval")
	}
	if e = ApplyRecovery(context.Background(), raw, p); e != nil {
		t.Fatal(e)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, "ColorNinja-TD1.uf2"))
	if !bytes.Equal(raw, saved) {
		t.Fatal("recovery image changed on write")
	}
	if e = ApplyRecovery(context.Background(), raw, p); e == nil {
		t.Fatal("existing recovery target overwritten")
	}
}
func TestRecoveryVolumeResolvesParentAliasesButRejectsLinkedVolume(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "volumes")
	volume := filepath.Join(parent, "RPI-RP2")
	if err := os.MkdirAll(volume, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volume, "INFO_UF2.TXT"), []byte("UF2 Bootloader v3.0\nBoard-ID: RPI-RP2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "parent-alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Skipf("directory links unavailable: %v", err)
	}
	plan, err := PlanRecovery(testUF2(), filepath.Join(alias, "RPI-RP2"))
	resolved, resolveErr := filepath.EvalSymlinks(volume)
	if err != nil || resolveErr != nil || plan.Volume != resolved {
		t.Fatalf("parent alias was not resolved: plan=%+v error=%v resolve=%v", plan, err, resolveErr)
	}
	linked := filepath.Join(dir, "volume-alias")
	if err := os.Symlink(volume, linked); err != nil {
		t.Fatal(err)
	}
	if _, _, err := RecoveryVolume(linked); err == nil {
		t.Fatal("linked recovery volume accepted")
	}
}

func TestUF2RejectsInvalidBlocks(t *testing.T) {
	for _, offset := range []int{0, 4, 8, 12, 16, 20, 24, 28, 508} {
		raw := testUF2()
		binary.LittleEndian.PutUint32(raw[offset:offset+4], 0xffffffff)
		if e := ValidateUF2(raw); e == nil {
			t.Fatal("invalid block accepted", offset)
		}
	}
	raw := testUF2()
	copy(raw[512:], raw[:512])
	if e := ValidateUF2(raw); e == nil {
		t.Fatal("duplicate flash block")
	}
}
