package fsutil

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeFakeMBR writes a 512-byte MBR to a temp file with the first partition
// entry's start LBA set to startLBA, and returns the path. A zero startLBA
// writes an empty entry; a negative signature flag omits the 0x55AA marker.
func writeFakeMBR(t *testing.T, startLBA uint32, withSignature bool) string {
	t.Helper()
	buf := make([]byte, 512)
	if startLBA != 0 {
		binary.LittleEndian.PutUint32(buf[mbrPart1EntryOffset+mbrPartStartLBAOffset:], startLBA)
		buf[mbrPart1EntryOffset+4] = 0x07 // partition type: exFAT
	}
	if withSignature {
		buf[mbrSignatureOffset] = 0x55
		buf[mbrSignatureOffset+1] = 0xAA
	}
	path := filepath.Join(t.TempDir(), "disk.img")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write fake MBR: %v", err)
	}
	return path
}

func TestPartitionOffset_StandardAlignment(t *testing.T) {
	// LBA 2048 is the conventional first-partition start; offset must be 1 MiB.
	path := writeFakeMBR(t, 2048, true)
	got, err := PartitionOffset(path)
	if err != nil {
		t.Fatalf("PartitionOffset: %v", err)
	}
	if got != 1048576 {
		t.Errorf("PartitionOffset = %d, want 1048576 (2048 sectors * 512)", got)
	}
}

func TestPartitionOffset_NonStandardAlignment(t *testing.T) {
	// We parse the table rather than assume 2048, so a different start works.
	path := writeFakeMBR(t, 4096, true)
	got, err := PartitionOffset(path)
	if err != nil {
		t.Fatalf("PartitionOffset: %v", err)
	}
	if got != 4096*512 {
		t.Errorf("PartitionOffset = %d, want %d", got, 4096*512)
	}
}

func TestPartitionOffset_NoSignature(t *testing.T) {
	// A bare exFAT volume (no MBR) must be rejected — that's the exact broken
	// state we're moving away from.
	path := writeFakeMBR(t, 2048, false)
	if _, err := PartitionOffset(path); err == nil {
		t.Error("expected error for a device without an MBR signature")
	}
}

func TestPartitionOffset_EmptyPartitionEntry(t *testing.T) {
	path := writeFakeMBR(t, 0, true)
	if _, err := PartitionOffset(path); err == nil {
		t.Error("expected error when the first partition entry is empty")
	}
}
