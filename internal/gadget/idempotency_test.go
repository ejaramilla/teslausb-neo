package gadget

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestGadget returns a Gadget rooted at a temp dir so the configfs teardown
// logic can be exercised without a real /sys/kernel/config.
func newTestGadget(t *testing.T) *Gadget {
	t.Helper()
	return &Gadget{root: t.TempDir(), name: "teslausb"}
}

// buildFakeTree creates the directory/symlink structure a real gadget would
// have (without the virtual attribute files, which only exist in configfs).
func buildFakeTree(t *testing.T, g *Gadget) string {
	t.Helper()
	gp := g.gadgetPath()
	mustMkdir(t, filepath.Join(gp, "functions", "mass_storage.usb0", "lun.0"))
	mustMkdir(t, filepath.Join(gp, "functions", "mass_storage.usb0", "lun.1"))
	mustMkdir(t, filepath.Join(gp, "configs", "c.1", "strings", "0x409"))
	mustMkdir(t, filepath.Join(gp, "strings", "0x409"))
	// Function linked into the config.
	link := filepath.Join(gp, "configs", "c.1", "mass_storage.usb0")
	if err := os.Symlink(filepath.Join(gp, "functions", "mass_storage.usb0"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return gp
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func TestTeardownExisting_NoopWhenAbsent(t *testing.T) {
	g := newTestGadget(t)
	// Must not panic or error when there is nothing to tear down (the common
	// case on a clean first boot).
	g.teardownExisting()
}

func TestTeardownExisting_RemovesStaleTree(t *testing.T) {
	g := newTestGadget(t)
	gp := buildFakeTree(t, g)

	g.teardownExisting()

	if _, err := os.Stat(gp); !os.IsNotExist(err) {
		t.Errorf("gadget tree still present after teardown: stat err = %v", err)
	}
	// Idempotent: a second teardown is safe.
	g.teardownExisting()
}

func TestClearUDC(t *testing.T) {
	g := newTestGadget(t)
	gp := g.gadgetPath()
	mustMkdir(t, gp)
	udcPath := filepath.Join(gp, "UDC")
	if err := os.WriteFile(udcPath, []byte("fe980000.usb\n"), 0o644); err != nil {
		t.Fatalf("write UDC: %v", err)
	}

	if err := clearUDC(gp); err != nil {
		t.Fatalf("clearUDC: %v", err)
	}
	got, _ := os.ReadFile(udcPath)
	if string(got) != "" {
		t.Errorf("UDC = %q after clear, want empty", string(got))
	}

	// Clearing an already-empty UDC is a no-op (no error).
	if err := clearUDC(gp); err != nil {
		t.Errorf("clearUDC on empty: %v", err)
	}
}
