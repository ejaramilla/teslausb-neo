package fsutil

import (
	"os"
	"testing"
)

func TestGetDiskUsage(t *testing.T) {
	// Use the temp directory — guaranteed to exist on all platforms.
	used, free, err := GetDiskUsage(os.TempDir())
	if err != nil {
		t.Fatalf("GetDiskUsage(%q) error: %v", os.TempDir(), err)
	}
	if used == 0 {
		t.Error("used bytes should be > 0")
	}
	if free == 0 {
		t.Error("free bytes should be > 0")
	}
	// Sanity: used + free should be roughly total (within reason).
	total := used + free
	if total < 1024*1024 {
		t.Errorf("total (%d) seems unreasonably small", total)
	}
}

func TestGetDiskUsageInvalidPath(t *testing.T) {
	_, _, err := GetDiskUsage("/nonexistent/path/that/should/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}
