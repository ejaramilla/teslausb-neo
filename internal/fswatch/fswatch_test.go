package fswatch

import (
	"testing"
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
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestStateString_UnknownState(t *testing.T) {
	// Any value beyond the defined iota range must return "UNKNOWN",
	// not panic or return empty string.
	for _, s := range []State{-1, 100, 255} {
		if got := s.String(); got != "UNKNOWN" {
			t.Errorf("State(%d).String() = %q, want %q", s, got, "UNKNOWN")
		}
	}
}
