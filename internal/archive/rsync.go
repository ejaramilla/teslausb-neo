package archive

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RsyncBackend archives footage to a remote server via rsync over SSH.
type RsyncBackend struct {
	server string
	user   string
	path   string
	sshKey string
}

// NewRsync creates an RsyncBackend with the given parameters.
func NewRsync(server, user, path, sshKey string) *RsyncBackend {
	return &RsyncBackend{
		server: server,
		user:   user,
		path:   path,
		sshKey: sshKey,
	}
}

func (b *RsyncBackend) Name() string { return "rsync" }

// IsReachable checks whether the SSH port on the remote server is open.
func (b *RsyncBackend) IsReachable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "3", b.server)
	return cmd.Run() == nil
}

// Connect is a no-op for the rsync backend (SSH connections are per-command).
func (b *RsyncBackend) Connect(ctx context.Context) error { return nil }

// Disconnect is a no-op for the rsync backend.
func (b *RsyncBackend) Disconnect(ctx context.Context) error { return nil }

// ArchiveFiles copies files from srcRoot to the remote server via rsync/SSH.
func (b *RsyncBackend) ArchiveFiles(ctx context.Context, srcRoot string, files []string, progressFn ProgressFunc) error {
	total := len(files)
	for i, f := range files {
		if progressFn != nil {
			progressFn(i, total, f)
		}

		src := filepath.Join(srcRoot, f)
		dst := fmt.Sprintf("%s@%s:%s", b.user, b.server, filepath.Join(b.path, f))

		args := []string{"-a", "--remove-source-files", "--mkpath"}
		if b.sshKey != "" {
			args = append(args, "-e", fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no", b.sshKey))
		}
		args = append(args, src, dst)

		cmd := exec.CommandContext(ctx, "rsync", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("rsync: %s: %s", f, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// SyncMedia synchronises a media folder from the remote server to the given
// mount point. mediaFolder is the root-level directory name (e.g. "Music",
// "LightShow", "Boombox").
func (b *RsyncBackend) SyncMedia(ctx context.Context, destMount string, mediaFolder string) error {
	src := fmt.Sprintf("%s@%s:%s/", b.user, b.server, filepath.Join(b.path, mediaFolder))
	dst := filepath.Join(destMount, mediaFolder) + "/"

	args := []string{"-a", "--delete"}
	if b.sshKey != "" {
		args = append(args, "-e", fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no", b.sshKey))
	}
	args = append(args, src, dst)

	cmd := exec.CommandContext(ctx, "rsync", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync: %s sync: %s", mediaFolder, strings.TrimSpace(string(out)))
	}
	return nil
}
