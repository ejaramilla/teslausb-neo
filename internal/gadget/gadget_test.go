package gadget

import (
	"testing"
)

func TestNew_GadgetPathIncludesName(t *testing.T) {
	// The gadget path must contain the name — this is how configfs
	// namespaces multiple gadgets.
	tests := []struct {
		name string
		want string
	}{
		{"teslausb", "/sys/kernel/config/usb_gadget/teslausb"},
		{"mydevice", "/sys/kernel/config/usb_gadget/mydevice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(tt.name)
			if got := g.gadgetPath(); got != tt.want {
				t.Errorf("gadgetPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNew_StartsInactive(t *testing.T) {
	g := New("test")
	if g.active {
		t.Error("new gadget should start inactive — USB must not be presented before Configure()")
	}
	if len(g.LUNs) != 0 {
		t.Errorf("new gadget should have 0 LUNs, got %d", len(g.LUNs))
	}
}
