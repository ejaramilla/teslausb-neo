package state

import (
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", dbPath, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndMigrate(t *testing.T) {
	db := openTestDB(t)

	// Verify all three expected tables exist by querying sqlite_master.
	tables := map[string]bool{
		"archive_sessions": false,
		"archived_files":   false,
		"health_metrics":   false,
	}

	rows, err := db.db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		if _, ok := tables[name]; ok {
			tables[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rows: %v", err)
	}

	for name, found := range tables {
		if !found {
			t.Errorf("table %q not found after migration", name)
		}
	}
}

func TestMarkAndIsArchived(t *testing.T) {
	db := openTestDB(t)

	// Create a session first (required by foreign key).
	sessionID, err := db.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	path := "TeslaCam/SavedClips/2024-01-01_12-00-00/front.mp4"

	if db.IsArchived(path) {
		t.Error("IsArchived should return false before marking")
	}

	if err := db.MarkArchived(path, 1024, sessionID); err != nil {
		t.Fatalf("MarkArchived: %v", err)
	}

	if !db.IsArchived(path) {
		t.Error("IsArchived should return true after marking")
	}

	// A different path should still be unarchived.
	if db.IsArchived("some/other/file.mp4") {
		t.Error("IsArchived should return false for a different path")
	}
}

func TestListUnarchived(t *testing.T) {
	db := openTestDB(t)

	sessionID, err := db.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Mark two files as archived.
	archivedFiles := []string{"a.mp4", "b.mp4"}
	for _, f := range archivedFiles {
		if err := db.MarkArchived(f, 100, sessionID); err != nil {
			t.Fatalf("MarkArchived(%q): %v", f, err)
		}
	}

	// Pass a superset including archived and new files.
	allFiles := []string{"a.mp4", "b.mp4", "c.mp4", "d.mp4"}
	unarchived := db.ListUnarchived(allFiles)

	if len(unarchived) != 2 {
		t.Fatalf("ListUnarchived returned %d files, want 2", len(unarchived))
	}

	expected := map[string]bool{"c.mp4": true, "d.mp4": true}
	for _, f := range unarchived {
		if !expected[f] {
			t.Errorf("unexpected unarchived file: %q", f)
		}
	}
}

func TestCreateAndCompleteSession(t *testing.T) {
	db := openTestDB(t)

	sessionID, err := db.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sessionID <= 0 {
		t.Fatalf("CreateSession returned invalid ID: %d", sessionID)
	}

	if err := db.CompleteSession(sessionID, 10, 5000); err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}

	sessions, err := db.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("RecentSessions returned %d sessions, want 1", len(sessions))
	}

	s := sessions[0]
	if s.ID != sessionID {
		t.Errorf("session ID = %d, want %d", s.ID, sessionID)
	}
	if s.Status != "completed" {
		t.Errorf("session status = %q, want %q", s.Status, "completed")
	}
	if s.FilesArchived != 10 {
		t.Errorf("files_archived = %d, want 10", s.FilesArchived)
	}
	if s.BytesArchived != 5000 {
		t.Errorf("bytes_archived = %d, want 5000", s.BytesArchived)
	}
	if !s.CompletedAt.Valid {
		t.Error("completed_at should be set")
	}
}

func TestFailSession(t *testing.T) {
	db := openTestDB(t)

	sessionID, err := db.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	errMsg := "connection timed out"
	if err := db.FailSession(sessionID, errMsg); err != nil {
		t.Fatalf("FailSession: %v", err)
	}

	sessions, err := db.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("RecentSessions returned %d sessions, want 1", len(sessions))
	}

	s := sessions[0]
	if s.Status != "failed" {
		t.Errorf("session status = %q, want %q", s.Status, "failed")
	}
	if s.ErrorMessage != errMsg {
		t.Errorf("error_message = %q, want %q", s.ErrorMessage, errMsg)
	}
	if !s.CompletedAt.Valid {
		t.Error("completed_at should be set for failed session")
	}
}

func TestRecordAndRecentHealth(t *testing.T) {
	db := openTestDB(t)

	// Record several health metrics.
	temps := []int64{45000, 50000, 55000}
	for _, temp := range temps {
		if err := db.RecordHealth(temp, 8000000000, 2000000000); err != nil {
			t.Fatalf("RecordHealth: %v", err)
		}
	}

	metrics, err := db.RecentHealth(10)
	if err != nil {
		t.Fatalf("RecentHealth: %v", err)
	}
	if len(metrics) != 3 {
		t.Fatalf("RecentHealth returned %d metrics, want 3", len(metrics))
	}

	// RecentHealth returns newest first; the last recorded temp (55000) should
	// be first.
	if metrics[0].CPUTempMC != 55000 {
		t.Errorf("first metric CPUTempMC = %d, want 55000", metrics[0].CPUTempMC)
	}
	if metrics[0].StorageUsedBytes != 8000000000 {
		t.Errorf("StorageUsedBytes = %d, want 8000000000", metrics[0].StorageUsedBytes)
	}
	if metrics[0].StorageFreeBytes != 2000000000 {
		t.Errorf("StorageFreeBytes = %d, want 2000000000", metrics[0].StorageFreeBytes)
	}

	// Verify limit works.
	limited, err := db.RecentHealth(2)
	if err != nil {
		t.Fatalf("RecentHealth(2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("RecentHealth(2) returned %d metrics, want 2", len(limited))
	}
}
