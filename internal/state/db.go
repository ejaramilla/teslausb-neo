package state

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB connection to the SQLite state store.
type DB struct {
	db *sql.DB
}

// ArchiveSession represents a single archive run.
type ArchiveSession struct {
	ID            int64
	StartedAt     time.Time
	CompletedAt   sql.NullTime
	FilesArchived int64
	BytesArchived int64
	Status        string
	ErrorMessage  string
}

// HealthMetric represents a point-in-time health measurement.
type HealthMetric struct {
	ID               int64
	Timestamp        time.Time
	CPUTempMC        int64
	StorageUsedBytes int64
	StorageFreeBytes int64
}

// Open opens (or creates) the SQLite database at the given path, enables WAL
// mode, and runs any pending migrations.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Set a busy timeout so concurrent access doesn't immediately fail.
	if _, err := sqlDB.Exec("PRAGMA busy_timeout=5000"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	d := &DB{db: sqlDB}
	if err := d.Migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Migrate creates the database tables if they do not already exist.
func (d *DB) Migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS archive_sessions (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	started_at      DATETIME NOT NULL DEFAULT (datetime('now')),
	completed_at    DATETIME,
	files_archived  INTEGER NOT NULL DEFAULT 0,
	bytes_archived  INTEGER NOT NULL DEFAULT 0,
	status          TEXT NOT NULL DEFAULT 'running',
	error_message   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS archived_files (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	path        TEXT NOT NULL UNIQUE,
	size        INTEGER NOT NULL,
	archived_at DATETIME NOT NULL DEFAULT (datetime('now')),
	session_id  INTEGER NOT NULL REFERENCES archive_sessions(id)
);

CREATE TABLE IF NOT EXISTS health_metrics (
	id                 INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp          DATETIME NOT NULL DEFAULT (datetime('now')),
	cpu_temp_mc        INTEGER NOT NULL,
	storage_used_bytes INTEGER NOT NULL,
	storage_free_bytes INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_archived_files_path ON archived_files(path);
CREATE INDEX IF NOT EXISTS idx_health_metrics_timestamp ON health_metrics(timestamp);
`
	_, err := d.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}
	return nil
}
