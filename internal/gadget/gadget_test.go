package gadget

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestNewGadget(t *testing.T) {
	g := New("teslausb")

	if g.name != "teslausb" {
		t.Errorf("name = %q, want %q", g.name, "teslausb")
	}
	if g.root != configFSRoot {
		t.Errorf("root = %q, want %q", g.root, configFSRoot)
	}

	wantPath := filepath.Join(configFSRoot, "teslausb")
	if got := g.gadgetPath(); got != wantPath {
		t.Errorf("gadgetPath() = %q, want %q", got, wantPath)
	}
}

func TestGadgetPathConstruction(t *testing.T) {
	tests := []struct {
		name     string
		wantPath string
	}{
		{"teslausb", "/sys/kernel/config/usb_gadget/teslausb"},
		{"mydevice", "/sys/kernel/config/usb_gadget/mydevice"},
		{"test-gadget", "/sys/kernel/config/usb_gadget/test-gadget"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(tt.name)
			if got := g.gadgetPath(); got != tt.wantPath {
				t.Errorf("gadgetPath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestLUNPath(t *testing.T) {
	g := New("teslausb")

	// Verify LUN directory paths are built correctly for various indices.
	for i := 0; i < 4; i++ {
		wantLUN := filepath.Join(
			configFSRoot, "teslausb",
			"functions", "mass_storage.usb0",
			fmt.Sprintf("lun.%d", i),
		)

		gotLUN := filepath.Join(
			g.gadgetPath(),
			"functions", "mass_storage.usb0",
			fmt.Sprintf("lun.%d", i),
		)

		if gotLUN != wantLUN {
			t.Errorf("LUN %d path = %q, want %q", i, gotLUN, wantLUN)
		}
	}
}

func TestGadgetStructDefaults(t *testing.T) {
	g := New("test")

	if g.active {
		t.Error("new gadget should not be active")
	}
	if len(g.LUNs) != 0 {
		t.Errorf("new gadget should have 0 LUNs, got %d", len(g.LUNs))
	}
}

func TestConfigFSConstants(t *testing.T) {
	if configFSRoot != "/sys/kernel/config/usb_gadget" {
		t.Errorf("configFSRoot = %q, want %q", configFSRoot, "/sys/kernel/config/usb_gadget")
	}
	if defaultIDVendor != "0x1d6b" {
		t.Errorf("defaultIDVendor = %q, want %q", defaultIDVendor, "0x1d6b")
	}
	if defaultIDProduct != "0x0104" {
		t.Errorf("defaultIDProduct = %q, want %q", defaultIDProduct, "0x0104")
	}
}

func TestFunctionSubdirectoryPaths(t *testing.T) {
	g := New("teslausb")
	gp := g.gadgetPath()

	// Verify the string path for the mass_storage function directory.
	funcDir := filepath.Join(gp, "functions", "mass_storage.usb0")
	want := "/sys/kernel/config/usb_gadget/teslausb/functions/mass_storage.usb0"
	if funcDir != want {
		t.Errorf("function dir = %q, want %q", funcDir, want)
	}

	// Verify config directory path.
	configDir := filepath.Join(gp, "configs", "c.1")
	wantConfig := "/sys/kernel/config/usb_gadget/teslausb/configs/c.1"
	if configDir != wantConfig {
		t.Errorf("config dir = %q, want %q", configDir, wantConfig)
	}

	// Verify strings directory path.
	stringsDir := filepath.Join(gp, "strings", "0x409")
	wantStrings := "/sys/kernel/config/usb_gadget/teslausb/strings/0x409"
	if stringsDir != wantStrings {
		t.Errorf("strings dir = %q, want %q", stringsDir, wantStrings)
	}
}
