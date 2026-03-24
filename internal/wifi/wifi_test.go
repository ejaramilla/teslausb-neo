package wifi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// mockConn implements Connectivity for testing the watchdog logic.
type mockConn struct {
	connected   bool
	homeNetwork bool
	ssidVisible bool
	connectErr  error
}

func (m *mockConn) IsConnected() bool                { return m.connected }
func (m *mockConn) IsHomeNetwork(_ string) bool       { return m.homeNetwork }
func (m *mockConn) IsSSIDVisible(_ string) bool       { return m.ssidVisible }
func (m *mockConn) ConnectToHome(_ string) error      { return m.connectErr }

func TestWatchdogTick_ConnectedToHome(t *testing.T) {
	conn := &mockConn{connected: true, homeNetwork: true}
	got := watchdogTick(conn, "Home", 3, 5, nil)
	if got != 0 {
		t.Errorf("failures = %d, want 0 (should reset on connected)", got)
	}
}

func TestWatchdogTick_SSIDNotVisible(t *testing.T) {
	conn := &mockConn{connected: false, ssidVisible: false}
	got := watchdogTick(conn, "Home", 3, 5, nil)
	if got != 0 {
		t.Errorf("failures = %d, want 0 (should reset when SSID not visible)", got)
	}
}

func TestWatchdogTick_ReconnectSucceeds(t *testing.T) {
	conn := &mockConn{connected: false, ssidVisible: true, connectErr: nil}
	got := watchdogTick(conn, "Home", 2, 5, nil)
	if got != 0 {
		t.Errorf("failures = %d, want 0 (reconnect succeeded)", got)
	}
}

func TestWatchdogTick_ReconnectFails_IncrementsFailures(t *testing.T) {
	conn := &mockConn{connected: false, ssidVisible: true, connectErr: errors.New("fail")}
	got := watchdogTick(conn, "Home", 0, 5, func() error { return nil })
	if got != 1 {
		t.Errorf("failures = %d, want 1", got)
	}
}

func TestWatchdogTick_ReconnectFails_AccumulatesFailures(t *testing.T) {
	conn := &mockConn{connected: false, ssidVisible: true, connectErr: errors.New("fail")}
	got := watchdogTick(conn, "Home", 3, 5, func() error { return nil })
	if got != 4 {
		t.Errorf("failures = %d, want 4", got)
	}
}

func TestWatchdogTick_MaxFailures_Reboots(t *testing.T) {
	rebooted := false
	rebootFn := func() error { rebooted = true; return nil }
	conn := &mockConn{connected: false, ssidVisible: true, connectErr: errors.New("fail")}
	got := watchdogTick(conn, "Home", 4, 5, rebootFn)
	if got != 5 {
		t.Errorf("failures = %d, want 5", got)
	}
	if !rebooted {
		t.Error("expected reboot to be called at max failures")
	}
}

func TestWatchdogTick_BelowMaxFailures_NoReboot(t *testing.T) {
	rebooted := false
	rebootFn := func() error { rebooted = true; return nil }
	conn := &mockConn{connected: false, ssidVisible: true, connectErr: errors.New("fail")}
	watchdogTick(conn, "Home", 2, 5, rebootFn)
	if rebooted {
		t.Error("should not reboot below max failures")
	}
}

func TestWatchdogLoop_EmptySSID_ReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		runWatchdogLoop(ctx, &mockConn{}, "", 1, 5, nil)
		close(done)
	}()

	select {
	case <-done:
		// Good — returned immediately.
	case <-time.After(200 * time.Millisecond):
		t.Error("RunWatchdog with empty SSID should return immediately")
	}
}

func TestWatchdogLoop_CancelStopsLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	conn := &mockConn{connected: true, homeNetwork: true}

	done := make(chan struct{})
	go func() {
		runWatchdogLoop(ctx, conn, "Home", 1, 5, nil)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Good — loop exited on cancel.
	case <-time.After(3 * time.Second):
		t.Error("watchdog loop did not exit after context cancel")
	}
}

func TestWatchdogLoop_RebootsAfterMaxFailures(t *testing.T) {
	var rebootCount int32
	rebootFn := func() error {
		atomic.AddInt32(&rebootCount, 1)
		return nil
	}
	conn := &mockConn{connected: false, ssidVisible: true, connectErr: errors.New("fail")}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		// interval=1s, maxFailures=3 — should reboot after 3 ticks.
		runWatchdogLoop(ctx, conn, "Home", 1, 3, rebootFn)
		close(done)
	}()

	// Wait for enough ticks to trigger reboot.
	time.Sleep(4 * time.Second)
	cancel()
	<-done

	if got := atomic.LoadInt32(&rebootCount); got == 0 {
		t.Error("expected reboot to be called, but it was not")
	}
}

// Verify Manager implements Connectivity interface.
var _ Connectivity = (*Manager)(nil)
