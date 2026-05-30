package wifi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderHomeKeyfile(t *testing.T) {
	out := RenderHomeKeyfile("MyNet", "p@ss#word!", false)

	wants := []string{
		"[connection]",
		"id=MyNet",
		"type=wifi",
		"autoconnect=true",
		"[wifi]",
		"ssid=MyNet",
		"mode=infrastructure",
		"[wifi-security]",
		"key-mgmt=wpa-psk",
		"psk=p@ss#word!",
		"method=auto",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("keyfile missing %q\n---\n%s", w, out)
		}
	}

	// A non-hidden network must NOT carry hidden=true.
	if strings.Contains(out, "hidden=true") {
		t.Error("non-hidden network should not set hidden=true")
	}

	// The id must equal the SSID so `nmcli connection up <ssid>` resolves it.
	if !strings.Contains(out, "id=MyNet") {
		t.Error("connection id must equal SSID for ConnectToHome to work")
	}
}

func TestRenderHomeKeyfile_Hidden(t *testing.T) {
	out := RenderHomeKeyfile("Cloaked", "secret", true)
	if !strings.Contains(out, "hidden=true") {
		t.Error("hidden network should set hidden=true")
	}
}

// TestRenderHomeKeyfile_SpecialCharsVerbatim verifies passphrases with shell/
// URL-special characters survive verbatim (keyfile values are literal to EOL).
func TestRenderHomeKeyfile_SpecialCharsVerbatim(t *testing.T) {
	pw := `aB3^d7)Xz!#Qm0` // synthetic; exercises ^ ) ! # which break naive quoting
	out := RenderHomeKeyfile("example-ssid", pw, false)
	if !strings.Contains(out, "psk="+pw+"\n") {
		t.Errorf("passphrase not emitted verbatim; got:\n%s", out)
	}
}

// TestEnsureHomeConnection_NoCredsIsNoop ensures we never write a half-formed
// profile when credentials are absent.
func TestEnsureHomeConnection_NoCredsIsNoop(t *testing.T) {
	m := NewManager()
	if err := m.EnsureHomeConnection("", "", false); err != nil {
		t.Errorf("expected no-op with empty creds, got %v", err)
	}
	if err := m.EnsureHomeConnection("ssidonly", "", false); err != nil {
		t.Errorf("expected no-op when password missing, got %v", err)
	}
}

// TestWriteKeyfilePermissions checks the rendered file is written 0600. It
// exercises the write/permission path directly (without invoking nmcli) so it
// runs on any OS.
func TestWriteKeyfilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, homeConnectionFile)
	content := RenderHomeKeyfile("net", "pw", false)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("keyfile perms = %o, want 600 (NetworkManager ignores world-readable profiles)", perm)
	}
}
