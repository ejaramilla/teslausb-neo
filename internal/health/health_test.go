package health

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ejaramilla/teslausb-neo/internal/config"
)

// mockNotifier records calls for testing.
type mockNotifier struct {
	sendCount int32
	lastMsg   string
}

func (m *mockNotifier) Send(_ context.Context, _, msg string, _ string) error {
	atomic.AddInt32(&m.sendCount, 1)
	m.lastMsg = msg
	return nil
}

func TestNewMonitor(t *testing.T) {
	cfg := config.HealthConfig{
		TempWarningMC:  80000,
		TempCautionMC:  70000,
		IntervalSeconds: 30,
	}
	m := NewMonitor(cfg, nil)
	if m.interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", m.interval)
	}
}

func TestNewMonitorZeroInterval(t *testing.T) {
	cfg := config.HealthConfig{IntervalSeconds: 0}
	m := NewMonitor(cfg, nil)
	if m.interval != 60*time.Second {
		t.Errorf("interval = %v, want 60s (default)", m.interval)
	}
}

func TestNewMonitorNegativeInterval(t *testing.T) {
	cfg := config.HealthConfig{IntervalSeconds: -5}
	m := NewMonitor(cfg, nil)
	if m.interval != 60*time.Second {
		t.Errorf("interval = %v, want 60s (default for negative)", m.interval)
	}
}

func TestGetStorageUsage(t *testing.T) {
	used, free, err := GetStorageUsage(os.TempDir())
	if err != nil {
		t.Fatalf("GetStorageUsage() error: %v", err)
	}
	if used == 0 {
		t.Error("used bytes should be > 0")
	}
	if free == 0 {
		t.Error("free bytes should be > 0")
	}
}

func TestGetStorageUsageInvalidPath(t *testing.T) {
	_, _, err := GetStorageUsage("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestCheckTemperatureBelowThreshold(t *testing.T) {
	// CheckTemperature reads /sys/class/thermal which doesn't exist on macOS.
	// We test that it doesn't panic or send notifications when it can't read temp.
	n := &mockNotifier{}
	cfg := config.HealthConfig{TempWarningMC: 80000}
	m := NewMonitor(cfg, n)

	m.CheckTemperature(context.Background())

	// On macOS, GetCPUTemp() will fail — no notification should be sent.
	if atomic.LoadInt32(&n.sendCount) != 0 {
		t.Error("should not send notification when temp read fails")
	}
}

func TestCheckTemperatureNilNotifier(t *testing.T) {
	// Ensure no panic when notifier is nil.
	cfg := config.HealthConfig{TempWarningMC: 80000}
	m := NewMonitor(cfg, nil)
	// Should not panic.
	m.CheckTemperature(context.Background())
}

func TestNotifyWatchdogNoSocket(t *testing.T) {
	// Ensure NotifyWatchdog is a no-op when NOTIFY_SOCKET is unset.
	cfg := config.HealthConfig{IntervalSeconds: 60}
	m := NewMonitor(cfg, nil)
	os.Unsetenv("NOTIFY_SOCKET")
	// Should not panic or error.
	m.NotifyWatchdog()
}

func TestStartCancellation(t *testing.T) {
	cfg := config.HealthConfig{IntervalSeconds: 1}
	m := NewMonitor(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Good — exited on cancel.
	case <-time.After(3 * time.Second):
		t.Error("Start() did not exit after context cancel")
	}
}
