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
	mountpoint string
}

// NewNFS creates an NFSBackend with the given parameters.
func NewNFS(server, share string) *NFSBackend {
	return &NFSBackend{
		server:     server,
		share:      share,
		mountpoint: "/tmp/archive_nfs",
	}
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
		dst := filepath.Join(b.mountpoint, f)

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("nfs: mkdir for %s: %w", f, err)
		}

		cmd := exec.CommandContext(ctx, "rsync", "-a", "--remove-source-files", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("nfs: rsync %s: %s", f, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// SyncMusic synchronises the music library from the NFS share to the
// given mount point.
func (b *NFSBackend) SyncMusic(ctx context.Context, srcMount string) error {
	src := filepath.Join(b.mountpoint, "Music") + "/"
	dst := filepath.Join(srcMount, "Music") + "/"

	cmd := exec.CommandContext(ctx, "rsync", "-a", "--delete", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nfs: music sync: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
