// Package wifi manages wireless network connections via NetworkManager (nmcli).
package wifi

import (
	"fmt"
	"os/exec"
	"strings"
)

// Network represents a WiFi network discovered during scanning.
type Network struct {
	SSID     string
	Signal   string
	Security string
}

// Manager controls WiFi connections via nmcli.
type Manager struct{}

// NewManager creates a new WiFi Manager.
func NewManager() *Manager {
	return &Manager{}
}

// IsConnected returns true if the system has an active WiFi connection.
func (m *Manager) IsConnected() bool {
	out, err := exec.Command("nmcli", "-t", "-f", "TYPE,STATE", "device").CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "wifi:connected") {
			return true
		}
	}
	return false
}

// GetSSID returns the SSID of the currently connected WiFi network, or an
// empty string if not connected.
func (m *Manager) GetSSID() string {
	out, err := exec.Command("nmcli", "-t", "-f", "active,ssid", "dev", "wifi").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "yes:") {
			return strings.TrimPrefix(line, "yes:")
		}
	}
	return ""
}

// IsHomeNetwork returns true if the given ssid matches the currently
// connected network.
func (m *Manager) IsHomeNetwork(ssid string) bool {
	return m.GetSSID() == ssid
}

// ConnectToHome attempts to activate an existing NetworkManager connection
// profile for the given SSID.
func (m *Manager) ConnectToHome(ssid string) error {
	cmd := exec.Command("nmcli", "connection", "up", ssid)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wifi: connect %s: %s", ssid, strings.TrimSpace(string(out)))
	}
	return nil
}

// StartAP creates and activates a WiFi access point with the given SSID and
// password using NetworkManager.
func (m *Manager) StartAP(ssid, password string) error {
	// Delete any previous AP connection with the same name.
	_ = exec.Command("nmcli", "connection", "delete", "teslausb-ap").Run()

	cmd := exec.Command("nmcli", "device", "wifi", "hotspot",
		"ifname", "wlan0",
		"con-name", "teslausb-ap",
		"ssid", ssid,
		"password", password,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wifi: start AP: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// StopAP deactivates the access point.
func (m *Manager) StopAP() error {
	cmd := exec.Command("nmcli", "connection", "down", "teslausb-ap")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wifi: stop AP: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ScanNetworks returns a list of visible WiFi networks.
func (m *Manager) ScanNetworks() []Network {
	// Trigger a fresh scan (best effort).
	_ = exec.Command("nmcli", "device", "wifi", "rescan").Run()

	out, err := exec.Command("nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY", "dev", "wifi", "list").CombinedOutput()
	if err != nil {
		return nil
	}

	var networks []Network
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		if seen[parts[0]] {
			continue
		}
		seen[parts[0]] = true
		networks = append(networks, Network{
			SSID:     parts[0],
			Signal:   parts[1],
			Security: parts[2],
		})
	}
	return networks
}
