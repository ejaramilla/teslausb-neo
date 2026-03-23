package archive

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RcloneBackend archives footage using rclone (supports Google Drive, S3,
// and many other cloud storage providers).
type RcloneBackend struct {
	drive string
	path  string
}

// NewRclone creates an RcloneBackend. drive is the rclone remote name
// (e.g. "gdrive:") and path is the destination directory on that remote.
func NewRclone(drive, path string) *RcloneBackend {
	return &RcloneBackend{
		drive: drive,
		path:  path,
	}
}

func (b *RcloneBackend) Name() string { return "rclone" }

// IsReachable checks whether the rclone remote is accessible.
func (b *RcloneBackend) IsReachable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "rclone", "lsd", b.drive)
	return cmd.Run() == nil
}

// Connect is a no-op for rclone (each command authenticates independently).
func (b *RcloneBackend) Connect(ctx context.Context) error { return nil }

// Disconnect is a no-op for rclone.
func (b *RcloneBackend) Disconnect(ctx context.Context) error { return nil }

// ArchiveFiles copies files from srcRoot to the rclone remote.
func (b *RcloneBackend) ArchiveFiles(ctx context.Context, srcRoot string, files []string, progressFn ProgressFunc) error {
	total := len(files)
	for i, f := range files {
		if progressFn != nil {
			progressFn(i, total, f)
		}

		src := filepath.Join(srcRoot, f)
		dst := fmt.Sprintf("%s%s", b.drive, filepath.Join(b.path, filepath.Dir(f)))

		cmd := exec.CommandContext(ctx, "rclone", "copy", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("rclone: copy %s: %s", f, strings.TrimSpace(string(out)))
		}

		// Remove source file after successful upload.
		if err := exec.CommandContext(ctx, "rm", "-f", src).Run(); err != nil {
			return fmt.Errorf("rclone: remove source %s: %w", f, err)
		}
	}
	return nil
}

// SyncMusic synchronises the music library from the rclone remote to
// the given mount point.
func (b *RcloneBackend) SyncMusic(ctx context.Context, srcMount string) error {
	src := fmt.Sprintf("%s%s/Music", b.drive, b.path)
	dst := filepath.Join(srcMount, "Music")

	cmd := exec.CommandContext(ctx, "rclone", "sync", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rclone: music sync: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
