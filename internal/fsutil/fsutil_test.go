package fsutil

import (
	"os"
	"testing"
)

func TestGetDiskUsage_RealFilesystem(t *testing.T) {
	used, free, err := GetDiskUsage(os.TempDir())
	if err != nil {
		t.Fatalf("GetDiskUsage(%q) error: %v", os.TempDir(), err)
	}

	// Sanity checks on a real filesystem.
	if used == 0 {
		t.Error("used bytes should be > 0 on a real filesystem")
	}
	if free == 0 {
		t.Error("free bytes should be > 0 on a real filesystem")
	}
	total := used + free
	if total < 1024*1024 {
		t.Errorf("total (%d bytes) seems unreasonably small for any real disk", total)
	}
	if used > total {
		t.Errorf("used (%d) > total (%d), math is wrong", used, total)
	}
}

func TestGetDiskUsage_InvalidPath(t *testing.T) {
	_, _, err := GetDiskUsage("/nonexistent/path/that/should/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}
