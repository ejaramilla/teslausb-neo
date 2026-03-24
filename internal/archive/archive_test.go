package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ejaramilla/teslausb-neo/internal/config"
)

// Compile-time interface guards: every backend must implement both interfaces.
var (
	_ Backend     = (*CIFSBackend)(nil)
	_ Backend     = (*NFSBackend)(nil)
	_ Backend     = (*RsyncBackend)(nil)
	_ Backend     = (*RcloneBackend)(nil)
	_ LogArchiver = (*CIFSBackend)(nil)
	_ LogArchiver = (*NFSBackend)(nil)
	_ LogArchiver = (*RsyncBackend)(nil)
	_ LogArchiver = (*RcloneBackend)(nil)
)

func TestCIFSArchiveLog_WritesFileToMountpoint(t *testing.T) {
	dir := t.TempDir()
	b := &CIFSBackend{mountpoint: dir}

	content := []byte("TeslaUSB Neo v0.1.0 - Archive Summary\nSession: 42\nFiles: 17\n")
	if err := b.ArchiveLog(context.Background(), content); err != nil {
		t.Fatalf("ArchiveLog() error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "teslausb.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("log content = %q, want %q", got, content)
	}
}

func TestNFSArchiveLog_WritesFileToMountpoint(t *testing.T) {
	dir := t.TempDir()
	b := &NFSBackend{mountpoint: dir}

	content := []byte("NFS log test\n")
	if err := b.ArchiveLog(context.Background(), content); err != nil {
		t.Fatalf("ArchiveLog() error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "teslausb.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("log content = %q, want %q", got, content)
	}
}

func TestArchiveLog_OverwritesPreviousLog(t *testing.T) {
	dir := t.TempDir()
	b := &CIFSBackend{mountpoint: dir}
	ctx := context.Background()

	_ = b.ArchiveLog(ctx, []byte("session 1: archived 10 files"))
	_ = b.ArchiveLog(ctx, []byte("session 2: archived 25 files"))

	got, _ := os.ReadFile(filepath.Join(dir, "teslausb.log"))
	if string(got) != "session 2: archived 25 files" {
		t.Errorf("log should overwrite, got %q", got)
	}
}

func TestArchiveLog_InvalidMountpoint(t *testing.T) {
	b := &CIFSBackend{mountpoint: "/nonexistent/path/that/does/not/exist"}
	err := b.ArchiveLog(context.Background(), []byte("test"))
	if err == nil {
		t.Error("expected error when mountpoint doesn't exist")
	}
}

func TestCIFS_MountpointDefaultPath(t *testing.T) {
	// NewCIFS must set a fixed mountpoint — the rest of the code assumes it.
	b := NewCIFS(config.CIFSConfig{Server: "nas"})
	if b.mountpoint == "" {
		t.Error("CIFS mountpoint should not be empty")
	}
}

func TestNFS_MountpointDefaultPath(t *testing.T) {
	b := NewNFS("server", "/share")
	if b.mountpoint == "" {
		t.Error("NFS mountpoint should not be empty")
	}
}
