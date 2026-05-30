package archive

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NFSBackend archives footage to an NFS share.
type NFSBackend struct {
	server     string
	share      string
	path       string
	mountpoint string
}

// NewNFS creates an NFSBackend with the given parameters. path is a
// subdirectory within the export for dashcam footage (e.g. "TeslaCam").
func NewNFS(server, share, path string) *NFSBackend {
	return &NFSBackend{
		server:     server,
		share:      share,
		path:       path,
		mountpoint: "/tmp/archive_nfs",
	}
}

// archiveRoot is the destination directory for dashcam files: the mounted
// export plus the configured subpath. Media sync stays at the export root.
func (b *NFSBackend) archiveRoot() string {
	return filepath.Join(b.mountpoint, b.path)
}

func (b *NFSBackend) Name() string { return "nfs" }

// IsReachable pings the NFS server to check basic network connectivity.
func (b *NFSBackend) IsReachable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "3", b.server)
	return cmd.Run() == nil
}

// Connect mounts the NFS share at the configured mountpoint.
func (b *NFSBackend) Connect(ctx context.Context) error {
	if err := os.MkdirAll(b.mountpoint, 0o755); err != nil {
		return fmt.Errorf("nfs: mkdir %s: %w", b.mountpoint, err)
	}

	src := fmt.Sprintf("%s:%s", b.server, b.share)
	cmd := exec.CommandContext(ctx, "mount.nfs", src, b.mountpoint, "-o", "nolock")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: mount.nfs: %s", ErrConnectionFailed, strings.TrimSpace(string(out)))
	}

	return nil
}

// Disconnect unmounts the NFS share.
func (b *NFSBackend) Disconnect(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "umount", b.mountpoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nfs: umount: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ArchiveFiles uses rsync to copy files from srcRoot to the mounted NFS share.
func (b *NFSBackend) ArchiveFiles(ctx context.Context, srcRoot string, files []string, progressFn ProgressFunc) error {
	total := len(files)
	for i, f := range files {
		if progressFn != nil {
			progressFn(i, total, f)
		}

		src := filepath.Join(srcRoot, f)
		dst := filepath.Join(b.archiveRoot(), f)

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("nfs: mkdir for %s: %w", f, err)
		}

		cmd := exec.CommandContext(ctx, "rsync", "-a", "--append-verify", "--remove-source-files", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("nfs: rsync %s: %s", f, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// ArchiveLog writes a log summary file alongside the archived footage.
func (b *NFSBackend) ArchiveLog(ctx context.Context, content []byte) error {
	if err := os.MkdirAll(b.archiveRoot(), 0o755); err != nil {
		return fmt.Errorf("nfs: mkdir for log: %w", err)
	}
	dst := filepath.Join(b.archiveRoot(), "teslausb.log")
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		return fmt.Errorf("nfs: write log: %w", err)
	}
	return nil
}

// SyncMedia synchronises a media folder from the NFS share to the given
// mount point. mediaFolder is the root-level directory name (e.g. "Music",
// "LightShow", "Boombox").
func (b *NFSBackend) SyncMedia(ctx context.Context, destMount string, mediaFolder string) error {
	src := filepath.Join(b.mountpoint, mediaFolder) + "/"
	dst := filepath.Join(destMount, mediaFolder) + "/"

	cmd := exec.CommandContext(ctx, "rsync", "-a", "--delete", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nfs: %s sync: %s", mediaFolder, strings.TrimSpace(string(out)))
	}
	return nil
}
