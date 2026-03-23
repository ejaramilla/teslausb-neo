package sys

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	btHCIPath = "/sys/class/bluetooth/hci0"
)

// DisableHDMI turns off the HDMI output to save power. This uses tvservice
// on Raspberry Pi systems.
func DisableHDMI() error {
	cmd := exec.Command("tvservice", "-o")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("disable HDMI: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DisableBluetooth disables the Bluetooth controller to save power. The
// useBLEWake parameter indicates whether BLE is needed for vehicle wake;
// if true this function is a no-op.
func DisableBluetooth(useBLEWake bool) error {
	if useBLEWake {
		return nil
	}

	cmd := exec.Command("rfkill", "block", "bluetooth")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("disable bluetooth: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
