// Package archive provides backend interfaces and implementations for
// archiving TeslaCam footage to various storage targets.
package archive

import (
	"context"
	"errors"
)

// Common errors returned by archive backends.
var (
	ErrNotReachable     = errors.New("archive: backend is not reachable")
	ErrConnectionFailed = errors.New("archive: connection failed")
)

// ProgressFunc is called by ArchiveFiles to report progress.
// current is the index of the file being processed (0-based) and total is
// the total number of files to archive.
type ProgressFunc func(current, total int, filename string)

// Backend is the interface that every archive target must implement.
type Backend interface {
	// Name returns a human-readable identifier for this backend (e.g. "cifs").
	Name() string

	// IsReachable returns true when the target host or service is reachable.
	IsReachable(ctx context.Context) bool

	// Connect establishes the connection (e.g. mounts a share).
	Connect(ctx context.Context) error

	// Disconnect tears down the connection (e.g. unmounts).
	Disconnect(ctx context.Context) error

	// ArchiveFiles copies files from srcRoot to the backend.
	// files is a list of paths relative to srcRoot.
	// progressFn may be nil.
	ArchiveFiles(ctx context.Context, srcRoot string, files []string, progressFn ProgressFunc) error

	// SyncMusic synchronises the music library from the backend to srcMount.
	SyncMusic(ctx context.Context, srcMount string) error
}
