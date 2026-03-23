package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Gadget defaults
	if cfg.Gadget.CamSize != "16G" {
		t.Errorf("CamSize = %q, want %q", cfg.Gadget.CamSize, "16G")
	}
	if cfg.Gadget.MusicSize != "0" {
		t.Errorf("MusicSize = %q, want %q", cfg.Gadget.MusicSize, "0")
	}
	if cfg.Gadget.LightshowSize != "0" {
		t.Errorf("LightshowSize = %q, want %q", cfg.Gadget.LightshowSize, "0")
	}
	if cfg.Gadget.BoomboxSize != "0" {
		t.Errorf("BoomboxSize = %q, want %q", cfg.Gadget.BoomboxSize, "0")
	}
	if cfg.Gadget.UseExFAT {
		t.Error("UseExFAT should default to false")
	}

	// Archive defaults
	if cfg.Archive.System != "cifs" {
		t.Errorf("Archive.System = %q, want %q", cfg.Archive.System, "cifs")
	}
	if cfg.Archive.DelaySeconds != 60 {
		t.Errorf("Archive.DelaySeconds = %d, want 60", cfg.Archive.DelaySeconds)
	}
	if !cfg.Archive.SavedClips {
		t.Error("Archive.SavedClips should default to true")
	}
	if !cfg.Archive.SentryClips {
		t.Error("Archive.SentryClips should default to true")
	}
	if cfg.Archive.RecentClips {
		t.Error("Archive.RecentClips should default to false")
	}
	if cfg.Archive.TrackModeClips {
		t.Error("Archive.TrackModeClips should default to false")
	}

	// CIFS default path
	if cfg.CIFS.Path != "TeslaCam" {
		t.Errorf("CIFS.Path = %q, want %q", cfg.CIFS.Path, "TeslaCam")
	}

	// Tuning defaults
	if cfg.Tuning.DirtyRatio != 80 {
		t.Errorf("Tuning.DirtyRatio = %d, want 80", cfg.Tuning.DirtyRatio)
	}
	if cfg.Tuning.DirtyBackgroundBytes != 65536 {
		t.Errorf("Tuning.DirtyBackgroundBytes = %d, want 65536", cfg.Tuning.DirtyBackgroundBytes)
	}
	if cfg.Tuning.CPUGovernor != "powersave" {
		t.Errorf("Tuning.CPUGovernor = %q, want %q", cfg.Tuning.CPUGovernor, "powersave")
	}

	// Health defaults
	if cfg.Health.TempWarningMC != 80000 {
		t.Errorf("Health.TempWarningMC = %d, want 80000", cfg.Health.TempWarningMC)
	}
	if cfg.Health.IntervalSeconds != 60 {
		t.Errorf("Health.IntervalSeconds = %d, want 60", cfg.Health.IntervalSeconds)
	}

	// Idle defaults
	if cfg.Idle.WriteThresholdBytes != 1024 {
		t.Errorf("Idle.WriteThresholdBytes = %d, want 1024", cfg.Idle.WriteThresholdBytes)
	}
	if cfg.Idle.TimeoutSeconds != 300 {
		t.Errorf("Idle.TimeoutSeconds = %d, want 300", cfg.Idle.TimeoutSeconds)
	}

	// Wake default
	if cfg.Wake.Method != "none" {
		t.Errorf("Wake.Method = %q, want %q", cfg.Wake.Method, "none")
	}

	// WiFi AP defaults
	if cfg.WiFi.AP.Enabled {
		t.Error("WiFi.AP.Enabled should default to false")
	}
	if cfg.WiFi.AP.SSID != "teslausb" {
		t.Errorf("WiFi.AP.SSID = %q, want %q", cfg.WiFi.AP.SSID, "teslausb")
	}
}

func TestLoadConfig(t *testing.T) {
	tomlContent := `
[gadget]
cam_size = "32G"
music_size = "4G"
use_exfat = true

[archive]
system = "rsync"
delay_seconds = 120
saved_clips = false

[cifs]
server = "nas.local"
share = "teslacam"
user = "admin"
password = "secret"

[tuning]
dirty_ratio = 50
cpu_governor = "performance"

[notify.ntfy]
url = "https://ntfy.sh/my-topic"
topic = "teslausb"
`

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Gadget.CamSize != "32G" {
		t.Errorf("CamSize = %q, want %q", cfg.Gadget.CamSize, "32G")
	}
	if cfg.Gadget.MusicSize != "4G" {
		t.Errorf("MusicSize = %q, want %q", cfg.Gadget.MusicSize, "4G")
	}
	if !cfg.Gadget.UseExFAT {
		t.Error("UseExFAT should be true")
	}
	if cfg.Archive.System != "rsync" {
		t.Errorf("Archive.System = %q, want %q", cfg.Archive.System, "rsync")
	}
	if cfg.Archive.DelaySeconds != 120 {
		t.Errorf("Archive.DelaySeconds = %d, want 120", cfg.Archive.DelaySeconds)
	}
	if cfg.Archive.SavedClips {
		t.Error("Archive.SavedClips should be false")
	}
	if cfg.CIFS.Server != "nas.local" {
		t.Errorf("CIFS.Server = %q, want %q", cfg.CIFS.Server, "nas.local")
	}
	if cfg.CIFS.User != "admin" {
		t.Errorf("CIFS.User = %q, want %q", cfg.CIFS.User, "admin")
	}
	if cfg.Tuning.DirtyRatio != 50 {
		t.Errorf("Tuning.DirtyRatio = %d, want 50", cfg.Tuning.DirtyRatio)
	}
	if cfg.Tuning.CPUGovernor != "performance" {
		t.Errorf("Tuning.CPUGovernor = %q, want %q", cfg.Tuning.CPUGovernor, "performance")
	}
	if cfg.Notify.Ntfy.URL != "https://ntfy.sh/my-topic" {
		t.Errorf("Notify.Ntfy.URL = %q, want %q", cfg.Notify.Ntfy.URL, "https://ntfy.sh/my-topic")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")

	// Load returns an error for missing file but still returns defaults.
	if err == nil {
		t.Fatal("expected error for missing file")
	}

	// Verify the returned config has default values.
	dflt := DefaultConfig()
	if cfg.Gadget.CamSize != dflt.Gadget.CamSize {
		t.Errorf("CamSize = %q, want default %q", cfg.Gadget.CamSize, dflt.Gadget.CamSize)
	}
	if cfg.Archive.System != dflt.Archive.System {
		t.Errorf("Archive.System = %q, want default %q", cfg.Archive.System, dflt.Archive.System)
	}
	if cfg.Tuning.DirtyRatio != dflt.Tuning.DirtyRatio {
		t.Errorf("Tuning.DirtyRatio = %d, want default %d", cfg.Tuning.DirtyRatio, dflt.Tuning.DirtyRatio)
	}
}

func TestLoadConfigPartial(t *testing.T) {
	// Only set [archive] section; everything else should remain default.
	tomlContent := `
[archive]
system = "nfs"
delay_seconds = 30
`

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Overridden values
	if cfg.Archive.System != "nfs" {
		t.Errorf("Archive.System = %q, want %q", cfg.Archive.System, "nfs")
	}
	if cfg.Archive.DelaySeconds != 30 {
		t.Errorf("Archive.DelaySeconds = %d, want 30", cfg.Archive.DelaySeconds)
	}

	// Default values should be preserved
	dflt := DefaultConfig()
	if cfg.Gadget.CamSize != dflt.Gadget.CamSize {
		t.Errorf("CamSize = %q, want default %q", cfg.Gadget.CamSize, dflt.Gadget.CamSize)
	}
	if cfg.Tuning.DirtyRatio != dflt.Tuning.DirtyRatio {
		t.Errorf("Tuning.DirtyRatio = %d, want default %d", cfg.Tuning.DirtyRatio, dflt.Tuning.DirtyRatio)
	}
	if cfg.Health.TempWarningMC != dflt.Health.TempWarningMC {
		t.Errorf("Health.TempWarningMC = %d, want default %d", cfg.Health.TempWarningMC, dflt.Health.TempWarningMC)
	}
	if cfg.CIFS.Path != dflt.CIFS.Path {
		t.Errorf("CIFS.Path = %q, want default %q", cfg.CIFS.Path, dflt.CIFS.Path)
	}
	if cfg.WiFi.AP.SSID != dflt.WiFi.AP.SSID {
		t.Errorf("WiFi.AP.SSID = %q, want default %q", cfg.WiFi.AP.SSID, dflt.WiFi.AP.SSID)
	}
	if cfg.Wake.Method != dflt.Wake.Method {
		t.Errorf("Wake.Method = %q, want default %q", cfg.Wake.Method, dflt.Wake.Method)
	}
}
