package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ejaramilla/teslausb-neo/internal/config"
)

func TestBackendNames(t *testing.T) {
	tests := []struct {
		name    string
		backend Backend
		want    string
	}{
		{"cifs", NewCIFS(config.CIFSConfig{}), "cifs"},
		{"nfs", NewNFS("", ""), "nfs"},
		{"rsync", NewRsync("", "", "", ""), "rsync"},
		{"rclone", NewRclone("", ""), "rclone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.backend.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAllBackendsImplementLogArchiver(t *testing.T) {
	backends := []Backend{
		NewCIFS(config.CIFSConfig{}),
		NewNFS("", ""),
		NewRsync("", "", "", ""),
		NewRclone("", ""),
	}
	for _, b := range backends {
		if _, ok := b.(LogArchiver); !ok {
			t.Errorf("%s backend does not implement LogArchiver", b.Name())
		}
	}
}

func TestCIFSArchiveLog(t *testing.T) {
	dir := t.TempDir()
	b := &CIFSBackend{mountpoint: dir}

	content := []byte("test log content\nsession 42\n")
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

func TestNFSArchiveLog(t *testing.T) {
	dir := t.TempDir()
	b := &NFSBackend{mountpoint: dir}

	content := []byte("nfs log test\n")
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

func TestCIFSArchiveLogOverwrites(t *testing.T) {
	dir := t.TempDir()
	b := &CIFSBackend{mountpoint: dir}

	// Write first log.
	_ = b.ArchiveLog(context.Background(), []byte("first"))

	// Write second log — should overwrite.
	_ = b.ArchiveLog(context.Background(), []byte("second"))

	got, _ := os.ReadFile(filepath.Join(dir, "teslausb.log"))
	if string(got) != "second" {
		t.Errorf("log content = %q, want %q (should overwrite)", got, "second")
	}
}

func TestNewCIFS(t *testing.T) {
	cfg := config.CIFSConfig{
		Server:   "nas.local",
		Share:    "teslacam",
		User:     "admin",
		Password: "secret",
	}
	b := NewCIFS(cfg)
	if b.server != "nas.local" {
		t.Errorf("server = %q, want %q", b.server, "nas.local")
	}
	if b.share != "teslacam" {
		t.Errorf("share = %q, want %q", b.share, "teslacam")
	}
	if b.username != "admin" {
		t.Errorf("username = %q, want %q", b.username, "admin")
	}
	if b.mountpoint != "/tmp/archive_cifs" {
		t.Errorf("mountpoint = %q, want %q", b.mountpoint, "/tmp/archive_cifs")
	}
}

func TestNewRsync(t *testing.T) {
	b := NewRsync("host.local", "user", "/data/cam", "/root/.ssh/id_rsa")
	if b.server != "host.local" {
		t.Errorf("server = %q, want %q", b.server, "host.local")
	}
	if b.sshKey != "/root/.ssh/id_rsa" {
		t.Errorf("sshKey = %q, want %q", b.sshKey, "/root/.ssh/id_rsa")
	}
}

func TestRsyncConnectDisconnectAreNoops(t *testing.T) {
	b := NewRsync("", "", "", "")
	ctx := context.Background()
	if err := b.Connect(ctx); err != nil {
		t.Errorf("Connect() error: %v", err)
	}
	if err := b.Disconnect(ctx); err != nil {
		t.Errorf("Disconnect() error: %v", err)
	}
}

func TestRcloneConnectDisconnectAreNoops(t *testing.T) {
	b := NewRclone("", "")
	ctx := context.Background()
	if err := b.Connect(ctx); err != nil {
		t.Errorf("Connect() error: %v", err)
	}
	if err := b.Disconnect(ctx); err != nil {
		t.Errorf("Disconnect() error: %v", err)
	}
}
