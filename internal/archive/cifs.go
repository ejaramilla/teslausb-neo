package archive

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ejaramilla/teslausb-neo/internal/config"
)

// CIFSBackend archives footage to a CIFS/SMB share.
type CIFSBackend struct {
	server          string
	share           string
	username        string
	password        string
	mountpoint      string
	credentialsFile string
}

// NewCIFS creates a CIFSBackend from the provided configuration.
func NewCIFS(cfg config.CIFSConfig) *CIFSBackend {
	return &CIFSBackend{
		server:     cfg.Server,
		share:      cfg.Share,
		username:   cfg.User,
		password:   cfg.Password,
		mountpoint: "/tmp/archive_cifs",
	}
}

func (b *CIFSBackend) Name() string { return "cifs" }

// IsReachable pings the CIFS server to check basic network connectivity.
func (b *CIFSBackend) IsReachable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "3", b.server)
	return cmd.Run() == nil
}

// Connect mounts the CIFS share at the configured mountpoint.
func (b *CIFSBackend) Connect(ctx context.Context) error {
	if err := os.MkdirAll(b.mountpoint, 0o755); err != nil {
		return fmt.Errorf("cifs: mkdir %s: %w", b.mountpoint, err)
	}

	// Write a temporary credentials file so the password is not visible in
	// /proc/PID/cmdline.
	cred, err := os.CreateTemp("", "cifs-cred-*")
	if err != nil {
		return fmt.Errorf("cifs: create credentials file: %w", err)
	}
	b.credentialsFile = cred.Name()

	content := fmt.Sprintf("username=%s\npassword=%s\n", b.username, b.password)
	if _, err := cred.WriteString(content); err != nil {
		cred.Close()
		return fmt.Errorf("cifs: write credentials: %w", err)
	}
	cred.Close()

	src := fmt.Sprintf("//%s/%s", b.server, b.share)
	opts := fmt.Sprintf("credentials=%s,iocharset=utf8", b.credentialsFile)

	cmd := exec.CommandContext(ctx, "mount.cifs", src, b.mountpoint, "-o", opts)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: mount.cifs: %s", ErrConnectionFailed, strings.TrimSpace(string(out)))
	}

	return nil
}

// Disconnect unmounts the CIFS share and cleans up the credentials file.
func (b *CIFSBackend) Disconnect(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "umount", b.mountpoint)
	out, err := cmd.CombinedOutput()

	if b.credentialsFile != "" {
		os.Remove(b.credentialsFile)
		b.credentialsFile = ""
	}

	if err != nil {
		return fmt.Errorf("cifs: umount: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ArchiveFiles uses rsync to copy files from srcRoot to the mounted share.
func (b *CIFSBackend) ArchiveFiles(ctx context.Context, srcRoot string, files []string, progressFn ProgressFunc) error {
	total := len(files)
	for i, f := range files {
		if progressFn != nil {
			progressFn(i, total, f)
		}

		src := filepath.Join(srcRoot, f)
		dst := filepath.Join(b.mountpoint, f)

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("cifs: mkdir for %s: %w", f, err)
		}

		cmd := exec.CommandContext(ctx, "rsync", "-a", "--remove-source-files", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cifs: rsync %s: %s", f, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// SyncMusic synchronises the music library from the CIFS share to the
// given mount point.
func (b *CIFSBackend) SyncMusic(ctx context.Context, srcMount string) error {
	src := filepath.Join(b.mountpoint, "Music") + "/"
	dst := filepath.Join(srcMount, "Music") + "/"

	cmd := exec.CommandContext(ctx, "rsync", "-a", "--delete", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cifs: music sync: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
