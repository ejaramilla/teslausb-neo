package snapshot

import "testing"

// TestDefaultCOWSize guards the zram copy-on-write size against drift. It was
// briefly 512 MB in code while the design (CLAUDE.md #5) specifies 64 MB; on a
// 512 MB-RAM Pi Zero 2 W an oversized COW ceiling is a real risk.
func TestDefaultCOWSize(t *testing.T) {
	if defaultCOWSizeMB != 64 {
		t.Errorf("defaultCOWSizeMB = %d, want 64 (per design)", defaultCOWSizeMB)
	}
}
