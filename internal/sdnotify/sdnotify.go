// Package sdnotify implements the small subset of the systemd notify protocol
// the daemon needs: telling the service manager it is READY, and answering the
// watchdog. It is a no-op when not running under systemd (NOTIFY_SOCKET unset),
// so the daemon behaves identically when run by hand.
//
// Protocol reference: sd_notify(3) and sd_watchdog_enabled(3).
package sdnotify

import (
	"net"
	"os"
	"strconv"
	"time"
)

// notify sends a state line to the systemd notify socket ($NOTIFY_SOCKET).
func notify(state string) {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return
	}
	conn, err := net.Dial("unixgram", socket)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(state))
}

// Ready tells the service manager that startup is complete (READY=1). This is
// REQUIRED for Type=notify units; without it systemd treats the service as
// still starting and kills it after TimeoutStartSec.
func Ready() { notify("READY=1") }

// Watchdog sends a watchdog keep-alive ping (WATCHDOG=1).
func Watchdog() { notify("WATCHDOG=1") }

// WatchdogInterval returns how often Watchdog should be called: half of the
// WATCHDOG_USEC value the service manager exported (the interval recommended
// by sd_watchdog_enabled(3)). It returns 0 when the watchdog is not enabled
// for this process, in which case no pings are needed.
func WatchdogInterval() time.Duration {
	usec := os.Getenv("WATCHDOG_USEC")
	if usec == "" {
		return 0
	}
	// Honor WATCHDOG_PID: the keep-alive is expected only from that PID (when
	// set). If it names a different process, this isn't our watchdog.
	if pid := os.Getenv("WATCHDOG_PID"); pid != "" {
		if p, err := strconv.Atoi(pid); err != nil || p != os.Getpid() {
			return 0
		}
	}
	us, err := strconv.ParseInt(usec, 10, 64)
	if err != nil || us <= 0 {
		return 0
	}
	return time.Duration(us) * time.Microsecond / 2
}
