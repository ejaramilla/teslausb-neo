package tesla

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Compile-time interface guards.
var (
	_ WakeKeeper = NoopWakeKeeper{}
	_ WakeKeeper = (*BLEWakeKeeper)(nil)
	_ WakeKeeper = (*TessieWakeKeeper)(nil)
	_ WakeKeeper = (*WebhookWakeKeeper)(nil)
)

func TestBLEWakeKeeper_DefaultBinaryPath(t *testing.T) {
	b := NewBLEWakeKeeper("VIN", "")
	if b.BinaryPath != "tesla-control" {
		t.Errorf("BinaryPath = %q, want %q when empty string passed", b.BinaryPath, "tesla-control")
	}
}

func TestBLEWakeKeeper_StartStopIdempotent(t *testing.T) {
	// Uses "true" binary (exists on all UNIX) to avoid exec failures.
	b := NewBLEWakeKeeper("VIN", "true")
	ctx := context.Background()

	// Double-start must not deadlock or error.
	if err := b.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if err := b.Start(ctx); err != nil {
		t.Fatalf("second Start() error: %v", err)
	}

	// Double-stop must not deadlock or panic.
	if err := b.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
	if err := b.Stop(ctx); err != nil {
		t.Fatalf("second Stop() error: %v", err)
	}
}

func TestBLEWakeKeeper_StopWithoutStart(t *testing.T) {
	b := NewBLEWakeKeeper("VIN", "true")
	// Stop on never-started keeper must not panic.
	if err := b.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() on unstarted keeper: %v", err)
	}
}

func TestTessieNudge_SendsCorrectHTTPRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer mytoken" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer mytoken")
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want %q", got, "application/json")
		}
		if r.URL.Path != "/5YJ3E1EA1NF000001/wake" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/5YJ3E1EA1NF000001/wake")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tw := &TessieWakeKeeper{APIToken: "mytoken", VIN: "5YJ3E1EA1NF000001"}
	err := tw.nudgeWithURL(context.Background(), server.URL+"/5YJ3E1EA1NF000001/wake")
	if err != nil {
		t.Errorf("nudgeWithURL() error: %v", err)
	}
}

func TestTessieNudge_ReturnsErrorOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tw := &TessieWakeKeeper{APIToken: "token", VIN: "VIN"}
	err := tw.nudgeWithURL(context.Background(), server.URL+"/VIN/wake")
	if err == nil {
		t.Error("expected error for HTTP 500 response")
	}
}

func TestTessieNudge_ReturnsErrorOnConnectionFailure(t *testing.T) {
	tw := &TessieWakeKeeper{APIToken: "token", VIN: "VIN"}
	// Use a URL that will definitely refuse connections.
	err := tw.nudgeWithURL(context.Background(), "http://127.0.0.1:1/VIN/wake")
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestWebhookWakeKeeper_PostsActions(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		seen = append(seen, string(buf))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	k := NewWebhookWakeKeeper(server.URL)
	ctx := context.Background()
	if err := k.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := k.Nudge(ctx); err != nil {
		t.Fatalf("Nudge: %v", err)
	}
	if err := k.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	want := []string{`{"action":"start"}`, `{"action":"nudge"}`, `{"action":"stop"}`}
	if len(seen) != len(want) {
		t.Fatalf("got %d requests, want %d: %v", len(seen), len(want), seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("request %d body = %q, want %q", i, seen[i], want[i])
		}
	}
}

func TestWebhookWakeKeeper_ErrorOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	k := NewWebhookWakeKeeper(server.URL)
	if err := k.Nudge(context.Background()); err == nil {
		t.Error("expected error for HTTP 500 response")
	}
}
