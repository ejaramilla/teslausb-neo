package fswatch

import (
	"testing"
	"time"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateUndetermined, "UNDETERMINED"},
		{StateWriting, "WRITING"},
		{StateIdle, "IDLE"},
		{State(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestNewWatcher(t *testing.T) {
	w := NewWatcher(1024, 300, 5)
	if w.WriteThreshold != 1024 {
		t.Errorf("WriteThreshold = %d, want 1024", w.WriteThreshold)
	}
	if w.Timeout != 300*time.Second {
		t.Errorf("Timeout = %v, want 300s", w.Timeout)
	}
	if w.PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want 5s", w.PollInterval)
	}
}

func TestNewWatcherDefaults(t *testing.T) {
	w := NewWatcher(0, 0, 0)
	if w.WriteThreshold != 0 {
		t.Errorf("WriteThreshold = %d, want 0", w.WriteThreshold)
	}
	if w.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0", w.Timeout)
	}
	if w.PollInterval != 0 {
		t.Errorf("PollInterval = %v, want 0", w.PollInterval)
	}
}

func TestStateConstants(t *testing.T) {
	// Ensure states are distinct.
	states := []State{StateUndetermined, StateWriting, StateIdle}
	seen := make(map[State]bool)
	for _, s := range states {
		if seen[s] {
			t.Errorf("duplicate state value: %d", s)
		}
		seen[s] = true
	}
}
