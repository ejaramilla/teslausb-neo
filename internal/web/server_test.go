package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockStateDB satisfies the StateDB interface for testing.
type mockStateDB struct {
	state    string
	sessions []SessionInfo
}

func (m *mockStateDB) GetCurrentState() string {
	return m.state
}

func (m *mockStateDB) ListSessions() ([]SessionInfo, error) {
	return m.sessions, nil
}

func TestValidatePath(t *testing.T) {
	baseDir := t.TempDir()

	tests := []struct {
		name      string
		requested string
		wantErr   bool
	}{
		{"simple filename", "video.mp4", false},
		{"dotdot traversal", "../etc/passwd", true},
		{"deep traversal", "../../etc/shadow", true},
		{"mid-path traversal", "foo/../../../etc/passwd", true},
		{"clean filename", "clip-2024.mp4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePath(baseDir, tt.requested)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q, %q) error = %v, wantErr = %v", baseDir, tt.requested, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePathEdgeCases(t *testing.T) {
	baseDir := t.TempDir()

	tests := []struct {
		name      string
		requested string
		wantErr   bool
	}{
		{"dot only", ".", false},
		{"empty string", "", false},
		{"absolute path", "/etc/passwd", true},
		{"dotdot only", "..", true},
		{"slash prefix", "/foo", true},
		{"backslash in name", "foo\\bar.mp4", false}, // On POSIX this is a valid filename
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePath(baseDir, tt.requested)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q, %q) error = %v, wantErr = %v", baseDir, tt.requested, err, tt.wantErr)
			}
		})
	}
}

func TestStatusEndpoint(t *testing.T) {
	db := &mockStateDB{state: "idle"}
	srv := NewServer(Config{ArchiveDir: t.TempDir()}, db, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var status StatusInfo
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}

	if status.State != "booting" {
		t.Errorf("state = %q, want %q", status.State, "booting")
	}
	if status.Uptime == "" {
		t.Error("uptime should not be empty")
	}
}

func TestFilesEndpointTraversal(t *testing.T) {
	db := &mockStateDB{state: "idle"}
	srv := NewServer(Config{ArchiveDir: t.TempDir()}, db, nil)

	// The files endpoint uses handleDownload for ?name= parameter, but
	// handleFiles just lists the archive dir. Test download traversal.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/download?name=../../etc/passwd", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d (path traversal should be rejected)", rec.Code, http.StatusBadRequest)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if body["error"] != "invalid file path" {
		t.Errorf("error = %q, want %q", body["error"], "invalid file path")
	}
}

func TestHealthEndpoint(t *testing.T) {
	db := &mockStateDB{state: "archiving"}
	srv := NewServer(Config{ArchiveDir: t.TempDir()}, db, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
	if body["state"] != "archiving" {
		t.Errorf("state = %q, want %q", body["state"], "archiving")
	}
}

func TestSyncEndpoint(t *testing.T) {
	db := &mockStateDB{state: "idle"}
	srv := NewServer(Config{ArchiveDir: t.TempDir()}, db, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusAccepted)
	}
}
