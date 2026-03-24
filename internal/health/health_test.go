package health

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ejaramilla/teslausb-neo/internal/config"
)

// mockNotifier records notification calls.
type mockNotifier struct {
	sendCount int32
	lastTitle string
	lastMsg   string
}

func (m *mockNotifier) Send(_ context.Context, title, msg string, _ string) error {
	atomic.AddInt32(&m.sendCount, 1)
	m.lastTitle = title
	m.lastMsg = msg
	return nil
}

func TestNewMonitor_ZeroIntervalDefaultsTo60s(t *testing.T) {
	m := NewMonitor(config.HealthConfig{IntervalSeconds: 0}, nil)
	if m.interval != 60*time.Second {
		t.Errorf("interval = %v, want 60s default for zero input", m.interval)
	}
}

func TestNewMonitor_NegativeIntervalDefaultsTo60s(t *testing.T) {
	m := NewMonitor(config.HealthConfig{IntervalSeconds: -5}, nil)
	if m.interval != 60*time.Second {
		t.Errorf("interval = %v, want 60s default for negative input", m.interval)
	}
}

func TestGetStorageUsage_RealFilesystem(t *testing.T) {
	used, free, err := GetStorageUsage(os.TempDir())
	if err != nil {
		t.Fatalf("GetStorageUsage() error: %v", err)
	}
	if used == 0 {
		t.Error("used should be > 0")
	}
	if free == 0 {
		t.Error("free should be > 0")
	}
	if used > used+free {
		t.Error("used exceeds total — math is wrong")
	}
}

func TestGetStorageUsage_InvalidPath(t *testing.T) {
	_, _, err := GetStorageUsage("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestCheckTemperature_DoesNotPanicOnReadFailure(t *testing.T) {
	// On macOS, /sys/class/thermal doesn't exist. CheckTemperature should
	// silently return without sending any notification.
	n := &mockNotifier{}
	m := NewMonitor(config.HealthConfig{TempWarningMC: 80000}, n)

	m.CheckTemperature(context.Background())

	if atomic.LoadInt32(&n.sendCount) != 0 {
		t.Error("should not send notification when temp read fails")
	}
}

func TestCheckTemperature_NilNotifierDoesNotPanic(t *testing.T) {
	m := NewMonitor(config.HealthConfig{TempWarningMC: 80000}, nil)
	// This must not panic even though notifier is nil.
	m.CheckTemperature(context.Background())
}

func TestNotifyWatchdog_NoSocketIsNoOp(t *testing.T) {
	os.Unsetenv("NOTIFY_SOCKET")
	m := NewMonitor(config.HealthConfig{}, nil)
	// Must not panic or error when NOTIFY_SOCKET is unset.
	m.NotifyWatchdog()
}

func TestStart_ExitsOnContextCancel(t *testing.T) {
	m := NewMonitor(config.HealthConfig{IntervalSeconds: 1}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Good — goroutine exited.
	case <-time.After(3 * time.Second):
		t.Error("Start() did not exit after context cancel — goroutine leak")
	}
}
