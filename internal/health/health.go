// Package health provides system health monitoring including CPU temperature,
// storage usage, and systemd watchdog integration.
package health

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ejaramilla/teslausb-neo/internal/config"
	"github.com/ejaramilla/teslausb-neo/internal/notify"
	"github.com/ejaramilla/teslausb-neo/internal/sdnotify"
)

// Monitor periodically checks system health and notifies on warnings.
type Monitor struct {
	cfg      config.HealthConfig
	notifier notify.Notifier
	interval time.Duration
}

// NewMonitor creates a health Monitor.
func NewMonitor(cfg config.HealthConfig, notifier notify.Notifier) *Monitor {
	interval := time.Duration(cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Monitor{
		cfg:      cfg,
		notifier: notifier,
		interval: interval,
	}
}

// Start begins the health monitoring loop. It blocks until ctx is cancelled.
func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CheckTemperature(ctx)
		}
	}
}

// GetCPUTemp reads the CPU temperature in millidegrees Celsius from the
// thermal zone sysfs interface.
func GetCPUTemp() (int64, error) {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, fmt.Errorf("health: read cpu temp: %w", err)
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("health: parse cpu temp: %w", err)
	}
	return val, nil
}

// GetStorageUsage returns used and free bytes for the filesystem at the
// given mountpoint.
func GetStorageUsage(mountpoint string) (used, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountpoint, &stat); err != nil {
		return 0, 0, fmt.Errorf("health: statfs %s: %w", mountpoint, err)
	}
	total := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bfree * uint64(stat.Bsize)
	return total - freeBytes, freeBytes, nil
}

// NotifyWatchdog sends a watchdog keep-alive to the systemd notify socket.
//
// Deprecated: watchdog pings are driven by main on a dedicated ticker at
// WATCHDOG_USEC/2 (see internal/sdnotify); this remains for compatibility and
// simply delegates. The health monitor's own tick must NOT be used for the
// watchdog, since its interval (default 60s) can exceed WatchdogSec.
func (m *Monitor) NotifyWatchdog() {
	sdnotify.Watchdog()
}

// CheckTemperature reads the CPU temperature and sends a notification if
// it exceeds the configured warning threshold.
func (m *Monitor) CheckTemperature(ctx context.Context) {
	temp, err := GetCPUTemp()
	if err != nil {
		return
	}

	if temp >= m.cfg.TempWarningMC {
		msg := fmt.Sprintf("CPU temperature is %d.%03d C (warning threshold: %d.%03d C)",
			temp/1000, temp%1000,
			m.cfg.TempWarningMC/1000, m.cfg.TempWarningMC%1000,
		)
		if m.notifier != nil {
			_ = m.notifier.Send(ctx, "Temperature Warning", msg, notify.EventWarning)
		}
	}
}
