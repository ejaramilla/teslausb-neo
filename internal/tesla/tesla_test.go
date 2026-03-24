package tesla

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Verify interface compliance at compile time.
var (
	_ WakeKeeper = NoopWakeKeeper{}
	_ WakeKeeper = (*BLEWakeKeeper)(nil)
	_ WakeKeeper = (*TessieWakeKeeper)(nil)
)

func TestNoopWakeKeeper(t *testing.T) {
	noop := NoopWakeKeeper{}
	ctx := context.Background()

	if err := noop.Start(ctx); err != nil {
		t.Errorf("Start() error: %v", err)
	}
	if err := noop.Stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
	if err := noop.Nudge(ctx); err != nil {
		t.Errorf("Nudge() error: %v", err)
	}
}

func TestNewBLEWakeKeeper(t *testing.T) {
	b := NewBLEWakeKeeper("VIN123", "/usr/bin/tesla-control")
	if b.VIN != "VIN123" {
		t.Errorf("VIN = %q, want %q", b.VIN, "VIN123")
	}
	if b.BinaryPath != "/usr/bin/tesla-control" {
		t.Errorf("BinaryPath = %q, want %q", b.BinaryPath, "/usr/bin/tesla-control")
	}
}

func TestNewBLEWakeKeeperDefaultPath(t *testing.T) {
	b := NewBLEWakeKeeper("VIN123", "")
	if b.BinaryPath != "tesla-control" {
		t.Errorf("BinaryPath = %q, want %q (default)", b.BinaryPath, "tesla-control")
	}
}

func TestBLEStartStopIdempotent(t *testing.T) {
	b := NewBLEWakeKeeper("VIN123", "true") // "true" binary exists on all UNIX systems
	ctx := context.Background()

	// Start twice — should not error.
	if err := b.Start(ctx); err != nil {
		t.Errorf("Start() error: %v", err)
	}
	if err := b.Start(ctx); err != nil {
		t.Errorf("second Start() error: %v", err)
	}

	// Stop twice — should not error.
	if err := b.Stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
	if err := b.Stop(ctx); err != nil {
		t.Errorf("second Stop() error: %v", err)
	}
}

func TestNewTessieWakeKeeper(t *testing.T) {
	tw := NewTessieWakeKeeper("token123", "VIN456")
	if tw.APIToken != "token123" {
		t.Errorf("APIToken = %q, want %q", tw.APIToken, "token123")
	}
	if tw.VIN != "VIN456" {
		t.Errorf("VIN = %q, want %q", tw.VIN, "VIN456")
	}
}

func TestTessieStartStopNoops(t *testing.T) {
	tw := NewTessieWakeKeeper("", "")
	ctx := context.Background()
	if err := tw.Start(ctx); err != nil {
		t.Errorf("Start() error: %v", err)
	}
	if err := tw.Stop(ctx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestTessieNudgeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format.
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer testtoken" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer testtoken")
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want %q", got, "application/json")
		}
		// Path should end with /VIN/wake.
		if r.URL.Path != "/TESTVIN/wake" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/TESTVIN/wake")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tw := &TessieWakeKeeper{APIToken: "testtoken", VIN: "TESTVIN"}
	// Override the base URL by constructing URL manually.
	// Since tessieBaseURL is a package const, we test via the mock server
	// by temporarily testing the HTTP request construction.
	// For a proper integration test, we'd need to make the URL configurable.
	// Instead, test request construction indirectly via the mock.

	// Test with the real tessieBaseURL — this will fail to connect (expected).
	// We primarily test the httptest server path.
	err := tw.nudgeWithURL(context.Background(), server.URL+"/TESTVIN/wake")
	if err != nil {
		t.Errorf("Nudge() error: %v", err)
	}
}

func TestTessieNudgeServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tw := &TessieWakeKeeper{APIToken: "token", VIN: "VIN"}
	err := tw.nudgeWithURL(context.Background(), server.URL+"/VIN/wake")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}
