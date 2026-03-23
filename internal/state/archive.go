package state

import (
	"fmt"
	"time"
)

// MarkArchived records that a file has been successfully archived as part of
// the given session.
func (d *DB) MarkArchived(path string, size int64, sessionID int64) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO archived_files (path, size, archived_at, session_id)
		 VALUES (?, ?, datetime('now'), ?)`,
		path, size, sessionID,
	)
	if err != nil {
		return fmt.Errorf("mark archived %s: %w", path, err)
	}
	return nil
}

// IsArchived returns true if the given file path has already been archived.
func (d *DB) IsArchived(path string) bool {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM archived_files WHERE path = ?`, path,
	).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// ListUnarchived returns the subset of allFiles that have not yet been
// archived.
func (d *DB) ListUnarchived(allFiles []string) []string {
	var unarchived []string
	for _, f := range allFiles {
		if !d.IsArchived(f) {
			unarchived = append(unarchived, f)
		}
	}
	return unarchived
}

// CreateSession inserts a new archive session and returns its ID.
func (d *DB) CreateSession() (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO archive_sessions (started_at, status) VALUES (datetime('now'), 'running')`,
	)
	if err != nil {
		return 0, fmt.Errorf("create session: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get session id: %w", err)
	}
	return id, nil
}

// CompleteSession marks a session as successfully completed with summary
// statistics.
func (d *DB) CompleteSession(id int64, filesArchived, bytesArchived int64) error {
	_, err := d.db.Exec(
		`UPDATE archive_sessions
		 SET completed_at = datetime('now'),
		     files_archived = ?,
		     bytes_archived = ?,
		     status = 'completed'
		 WHERE id = ?`,
		filesArchived, bytesArchived, id,
	)
	if err != nil {
		return fmt.Errorf("complete session %d: %w", id, err)
	}
	return nil
}

// FailSession marks a session as failed with an error message.
func (d *DB) FailSession(id int64, errMsg string) error {
	_, err := d.db.Exec(
		`UPDATE archive_sessions
		 SET completed_at = datetime('now'),
		     status = 'failed',
		     error_message = ?
		 WHERE id = ?`,
		errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("fail session %d: %w", id, err)
	}
	return nil
}

// RecentSessions returns the most recent archive sessions, ordered by start
// time descending.
func (d *DB) RecentSessions(limit int) ([]ArchiveSession, error) {
	rows, err := d.db.Query(
		`SELECT id, started_at, completed_at, files_archived, bytes_archived, status, error_message
		 FROM archive_sessions
		 ORDER BY started_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent sessions: %w", err)
	}
	defer rows.Close()

	var sessions []ArchiveSession
	for rows.Next() {
		var s ArchiveSession
		var startedStr string
		var completedStr *string
		if err := rows.Scan(&s.ID, &startedStr, &completedStr, &s.FilesArchived, &s.BytesArchived, &s.Status, &s.ErrorMessage); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		s.StartedAt, _ = time.Parse("2006-01-02 15:04:05", startedStr)
		if completedStr != nil {
			t, _ := time.Parse("2006-01-02 15:04:05", *completedStr)
			s.CompletedAt.Time = t
			s.CompletedAt.Valid = true
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessions, nil
}
