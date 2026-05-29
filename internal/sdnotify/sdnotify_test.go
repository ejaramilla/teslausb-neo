package sdnotify

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestWatchdogInterval(t *testing.T) {
	// Save and restore the environment.
	for _, k := range []string{"WATCHDOG_USEC", "WATCHDOG_PID"} {
		old, ok := os.LookupEnv(k)
		t.Cleanup(func() {
			if ok {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}

	t.Run("unset means disabled", func(t *testing.T) {
		os.Unsetenv("WATCHDOG_USEC")
		os.Unsetenv("WATCHDOG_PID")
		if got := WatchdogInterval(); got != 0 {
			t.Errorf("WatchdogInterval() = %v, want 0 when WATCHDOG_USEC unset", got)
		}
	})

	t.Run("returns half of WATCHDOG_USEC", func(t *testing.T) {
		os.Setenv("WATCHDOG_USEC", "30000000") // 30s
		os.Unsetenv("WATCHDOG_PID")
		if got := WatchdogInterval(); got != 15*time.Second {
			t.Errorf("WatchdogInterval() = %v, want 15s (half of 30s)", got)
		}
	})

	t.Run("honors WATCHDOG_PID for this process", func(t *testing.T) {
		os.Setenv("WATCHDOG_USEC", "30000000")
		os.Setenv("WATCHDOG_PID", fmt.Sprintf("%d", os.Getpid()))
		if got := WatchdogInterval(); got != 15*time.Second {
			t.Errorf("WatchdogInterval() = %v, want 15s for our PID", got)
		}
	})

	t.Run("disabled when WATCHDOG_PID is another process", func(t *testing.T) {
		os.Setenv("WATCHDOG_USEC", "30000000")
		os.Setenv("WATCHDOG_PID", fmt.Sprintf("%d", os.Getpid()+1))
		if got := WatchdogInterval(); got != 0 {
			t.Errorf("WatchdogInterval() = %v, want 0 when PID is another process", got)
		}
	})

	t.Run("invalid value is disabled", func(t *testing.T) {
		os.Setenv("WATCHDOG_USEC", "notanumber")
		os.Unsetenv("WATCHDOG_PID")
		if got := WatchdogInterval(); got != 0 {
			t.Errorf("WatchdogInterval() = %v, want 0 for invalid value", got)
		}
	})
}

// TestNotifyNoSocketIsNoOp ensures the notify helpers are safe (no panic) when
// not running under systemd.
func TestNotifyNoSocketIsNoOp(t *testing.T) {
	old, ok := os.LookupEnv("NOTIFY_SOCKET")
	os.Unsetenv("NOTIFY_SOCKET")
	t.Cleanup(func() {
		if ok {
			os.Setenv("NOTIFY_SOCKET", old)
		}
	})
	Ready()
	Watchdog()
}
