package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StatusInfo represents the current system status returned by the API.
type StatusInfo struct {
	State     string    `json:"state"`
	Uptime    string    `json:"uptime"`
	Timestamp time.Time `json:"timestamp"`
}

// FileInfo represents a file entry returned by the files API.
type FileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// SessionInfo represents an archive session entry.
type SessionInfo struct {
	ID        int       `json:"id"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	Status    string    `json:"status"`
}

// Config holds web server configuration.
type Config struct {
	ArchiveDir string
}

// StateDB is the interface the web server requires for state persistence.
type StateDB interface {
	GetCurrentState() string
	ListSessions() ([]SessionInfo, error)
}

// Server is the HTTP server for the TeslaUSB Neo web UI and API.
type Server struct {
	config   Config
	stateDB  StateDB
	statusCh <-chan StatusInfo

	mu         sync.RWMutex
	lastStatus StatusInfo

	startTime time.Time
	mux       *http.ServeMux
}

// NewServer creates a new web server instance.
func NewServer(cfg Config, db StateDB, statusCh <-chan StatusInfo) *Server {
	s := &Server{
		config:    cfg,
		stateDB:   db,
		statusCh:  statusCh,
		startTime: time.Now(),
		mux:       http.NewServeMux(),
		lastStatus: StatusInfo{
			State:     "booting",
			Timestamp: time.Now(),
		},
	}
	s.registerRoutes()
	return s
}

// Start begins listening on the given address. It blocks until the server
// is shut down or encounters a fatal error.
func (s *Server) Start(addr string) error {
	if s.statusCh != nil {
		go s.consumeStatus()
	}

	slog.Info("web server starting", "addr", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/v1/files", s.handleFiles)
	s.mux.HandleFunc("GET /api/v1/files/download", s.handleDownload)
	s.mux.HandleFunc("DELETE /api/v1/files", s.handleDeleteFile)
	s.mux.HandleFunc("POST /api/v1/sync", s.handleSync)
	s.mux.HandleFunc("GET /api/v1/archive/sessions", s.handleSessions)
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	// Serve embedded static files for the web UI.
	webFS, err := fs.Sub(WebAssets, "web")
	if err != nil {
		slog.Error("failed to create sub filesystem for embedded web assets", "error", err)
		return
	}
	s.mux.Handle("/", http.FileServer(http.FS(webFS)))
}

func (s *Server) consumeStatus() {
	for status := range s.statusCh {
		s.mu.Lock()
		s.lastStatus = status
		s.mu.Unlock()
	}
}

// handleStatus returns the current system state.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := s.lastStatus
	s.mu.RUnlock()

	status.Uptime = time.Since(s.startTime).Truncate(time.Second).String()
	writeJSON(w, http.StatusOK, status)
}

// handleFiles lists files in the archive directory.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	dir := s.config.ArchiveDir
	if dir == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "archive directory not configured"})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read archive directory"})
		return
	}

	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    e.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	writeJSON(w, http.StatusOK, files)
}

// handleDownload serves a single file from the archive directory.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing name parameter"})
		return
	}

	safePath, err := validatePath(s.config.ArchiveDir, name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file path"})
		return
	}

	http.ServeFile(w, r, safePath)
}

// handleDeleteFile removes a file from the archive directory.
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing name parameter"})
		return
	}

	safePath, err := validatePath(s.config.ArchiveDir, name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file path"})
		return
	}

	if err := os.Remove(safePath); err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete file"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleSync triggers an archive sync cycle.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	// In a full implementation this would signal the state machine to begin
	// an archive cycle. For now, acknowledge the request.
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync requested"})
}

// handleSessions returns the list of archive sessions from the state DB.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.stateDB == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state database not available"})
		return
	}

	sessions, err := s.stateDB.ListSessions()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query sessions"})
		return
	}

	writeJSON(w, http.StatusOK, sessions)
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	state := "unknown"
	if s.stateDB != nil {
		state = s.stateDB.GetCurrentState()
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"state":  state,
		"uptime": time.Since(s.startTime).Truncate(time.Second).String(),
	})
}

// validatePath ensures the requested filename resolves to a path inside baseDir,
// preventing directory traversal attacks. It returns the absolute safe path.
func validatePath(baseDir, requested string) (string, error) {
	// Clean the requested path and reject anything with path separators or
	// obvious traversal components.
	cleaned := filepath.Clean(requested)
	if strings.Contains(cleaned, string(filepath.Separator)) || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("invalid path: contains directory traversal")
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}

	candidate := filepath.Join(absBase, cleaned)

	// Use filepath.Rel to verify the candidate is within the base directory.
	rel, err := filepath.Rel(absBase, candidate)
	if err != nil {
		return "", fmt.Errorf("path resolution failed: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes base directory")
	}

	return candidate, nil
}

// writeJSON marshals v as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
