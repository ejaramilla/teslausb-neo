package archive

import (
	"context"
	"fmt"
	"os"
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

// ArchiveLog writes a log summary file to the rclone remote.
func (b *RcloneBackend) ArchiveLog(ctx context.Context, content []byte) error {
	tmp, err := os.CreateTemp("", "teslausb-log-*")
	if err != nil {
		return fmt.Errorf("rclone: create temp log: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("rclone: write temp log: %w", err)
	}
	tmp.Close()

	dst := fmt.Sprintf("%s%s", b.drive, b.path)
	cmd := exec.CommandContext(ctx, "rclone", "copy", tmp.Name(), dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rclone: log upload: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// SyncMedia synchronises a media folder from the rclone remote to the given
// mount point. mediaFolder is the root-level directory name (e.g. "Music",
// "LightShow", "Boombox").
func (b *RcloneBackend) SyncMedia(ctx context.Context, destMount string, mediaFolder string) error {
	src := fmt.Sprintf("%s%s/%s", b.drive, b.path, mediaFolder)
	dst := filepath.Join(destMount, mediaFolder)

	cmd := exec.CommandContext(ctx, "rclone", "sync", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rclone: %s sync: %s", mediaFolder, strings.TrimSpace(string(out)))
	}
	return nil
}
