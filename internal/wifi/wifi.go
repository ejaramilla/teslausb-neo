// Package wifi manages wireless network connections via NetworkManager (nmcli).
package wifi

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
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

// IsSSIDVisible returns true if the given SSID is found in a WiFi scan.
func (m *Manager) IsSSIDVisible(ssid string) bool {
	for _, n := range m.ScanNetworks() {
		if n.SSID == ssid {
			return true
		}
	}
	return false
}

// Connectivity abstracts the WiFi operations the watchdog needs, making
// the watchdog logic testable without nmcli.
type Connectivity interface {
	IsConnected() bool
	IsHomeNetwork(ssid string) bool
	IsSSIDVisible(ssid string) bool
	ConnectToHome(ssid string) error
}

// RebootFunc is called when the watchdog decides a reboot is needed.
// Production code passes systemdReboot; tests pass a stub.
type RebootFunc func() error

func systemdReboot() error {
	return exec.Command("systemctl", "reboot").Run()
}

// RunWatchdog monitors WiFi connectivity and attempts recovery when the home
// network is visible but the Pi is not connected. It uses a graduated response:
// first it tries to reconnect, then reboots after maxFailures consecutive
// failures. It blocks until ctx is cancelled.
func (m *Manager) RunWatchdog(ctx context.Context, homeSSID string, intervalSeconds, maxFailures int) {
	runWatchdogLoop(ctx, m, homeSSID, intervalSeconds, maxFailures, systemdReboot)
}

// runWatchdogLoop contains the core watchdog logic, parameterised for testing.
func runWatchdogLoop(ctx context.Context, conn Connectivity, homeSSID string, intervalSeconds, maxFailures int, reboot RebootFunc) {
	if homeSSID == "" {
		slog.Warn("wifi watchdog: no home SSID configured, watchdog disabled")
		return
	}

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	consecutiveFailures := 0
	slog.Info("wifi watchdog started", "interval_seconds", intervalSeconds, "max_failures", maxFailures)

	for {
		select {
		case <-ctx.Done():
			slog.Info("wifi watchdog stopped")
			return
		case <-ticker.C:
			consecutiveFailures = watchdogTick(conn, homeSSID, consecutiveFailures, maxFailures, reboot)
		}
	}
}

// watchdogTick runs one iteration of the watchdog check and returns the
// updated consecutive failure count.
func watchdogTick(conn Connectivity, homeSSID string, consecutiveFailures, maxFailures int, reboot RebootFunc) int {
	if conn.IsConnected() && conn.IsHomeNetwork(homeSSID) {
		if consecutiveFailures > 0 {
			slog.Info("wifi watchdog: connectivity restored", "after_failures", consecutiveFailures)
		}
		return 0
	}

	// Not connected to home WiFi — check if SSID is even visible.
	if !conn.IsSSIDVisible(homeSSID) {
		// Home network not in range; this is normal (car is away).
		return 0
	}

	// Home SSID is visible but we're not connected.
	consecutiveFailures++
	slog.Warn("wifi watchdog: home SSID visible but not connected",
		"ssid", homeSSID,
		"consecutive_failures", consecutiveFailures,
		"max_failures", maxFailures)

	// Graduated response: try reconnect first.
	if err := conn.ConnectToHome(homeSSID); err != nil {
		slog.Warn("wifi watchdog: reconnect attempt failed", "error", err)
	} else {
		slog.Info("wifi watchdog: reconnect succeeded")
		return 0
	}

	// Reboot after max consecutive failures.
	if consecutiveFailures >= maxFailures {
		slog.Error("wifi watchdog: max failures reached, rebooting",
			"consecutive_failures", consecutiveFailures)
		if err := reboot(); err != nil {
			slog.Error("wifi watchdog: reboot failed", "error", err)
		}
	}

	return consecutiveFailures
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
