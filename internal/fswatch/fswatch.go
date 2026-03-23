// Package fswatch detects when the USB gadget device becomes idle by
// monitoring kernel I/O statistics for the file-storage backing process.
package fswatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// State represents the idle-detection state machine.
type State int

const (
	StateUndetermined State = iota
	StateWriting
	StateIdle
)

// String returns a human-readable state name.
func (s State) String() string {
	switch s {
	case StateUndetermined:
		return "UNDETERMINED"
	case StateWriting:
		return "WRITING"
	case StateIdle:
		return "IDLE"
	default:
		return "UNKNOWN"
	}
}

// Watcher monitors /proc/{pid}/io for the file-storage kernel thread and
// detects when writes have stopped.
type Watcher struct {
	WriteThreshold int64         // Minimum write_bytes delta to be considered "writing".
	Timeout        time.Duration // How long writes must be idle before declaring idle.
	PollInterval   time.Duration // How often to sample /proc/{pid}/io.
}

// NewWatcher creates a Watcher with the given parameters.
func NewWatcher(writeThreshold int64, timeoutSec, pollIntervalSec int) *Watcher {
	return &Watcher{
		WriteThreshold: writeThreshold,
		Timeout:        time.Duration(timeoutSec) * time.Second,
		PollInterval:   time.Duration(pollIntervalSec) * time.Second,
	}
}

// WaitForIdle blocks until USB gadget writes are idle or the context is
// cancelled. It returns nil when idle is detected.
func (w *Watcher) WaitForIdle(ctx context.Context) error {
	pid, err := w.findGadgetPID()
	if err != nil {
		return fmt.Errorf("fswatch: %w", err)
	}

	state := StateUndetermined
	var lastBytes int64
	var idleSince time.Time

	lastBytes, err = w.readWriteBytes(pid)
	if err != nil {
		return fmt.Errorf("fswatch: initial read: %w", err)
	}

	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current, err := w.readWriteBytes(pid)
			if err != nil {
				return fmt.Errorf("fswatch: read io: %w", err)
			}

			delta := current - lastBytes
			lastBytes = current

			switch state {
			case StateUndetermined:
				if delta >= w.WriteThreshold {
					state = StateWriting
				} else {
					state = StateIdle
					idleSince = time.Now()
				}
			case StateWriting:
				if delta < w.WriteThreshold {
					state = StateIdle
					idleSince = time.Now()
				}
			case StateIdle:
				if delta >= w.WriteThreshold {
					state = StateWriting
				} else if time.Since(idleSince) >= w.Timeout {
					return nil
				}
			}
		}
	}
}

// readWriteBytes parses write_bytes from /proc/{pid}/io.
func (w *Watcher) readWriteBytes(pid string) (int64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "io"))
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "write_bytes:") {
			valStr := strings.TrimSpace(strings.TrimPrefix(line, "write_bytes:"))
			return strconv.ParseInt(valStr, 10, 64)
		}
	}

	return 0, fmt.Errorf("write_bytes not found in /proc/%s/io", pid)
}

// findGadgetPID locates the PID of the file_storage kernel thread by
// scanning /proc for processes whose comm matches "file-storage".
func (w *Watcher) findGadgetPID() (string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return "", fmt.Errorf("read /proc: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only numeric directory names are PIDs.
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if name == "file-storage" {
			return e.Name(), nil
		}
	}

	return "", fmt.Errorf("file-storage process not found")
}
