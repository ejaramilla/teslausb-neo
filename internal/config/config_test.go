package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

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
	if cfg.Archive.SyncMusic {
		t.Error("Archive.SyncMusic should default to false")
	}
	if cfg.Archive.SyncLightShow {
		t.Error("Archive.SyncLightShow should default to false")
	}
	if cfg.Archive.SyncBoombox {
		t.Error("Archive.SyncBoombox should default to false")
	}

	// Archive new defaults
	if cfg.Archive.FreeSpaceReserveMB != 10240 {
		t.Errorf("Archive.FreeSpaceReserveMB = %d, want 10240", cfg.Archive.FreeSpaceReserveMB)
	}
	if cfg.Archive.ArchiveLogs {
		t.Error("Archive.ArchiveLogs should default to false")
	}

	// Default path is honored by all four backends.
	if cfg.CIFS.Path != "TeslaCam" {
		t.Errorf("CIFS.Path = %q, want %q", cfg.CIFS.Path, "TeslaCam")
	}
	if cfg.NFS.Path != "TeslaCam" {
		t.Errorf("NFS.Path = %q, want %q", cfg.NFS.Path, "TeslaCam")
	}

	// Tuning defaults
	if cfg.Tuning.DirtyRatio != 10 {
		t.Errorf("Tuning.DirtyRatio = %d, want 10", cfg.Tuning.DirtyRatio)
	}
	if cfg.Tuning.DirtyBackgroundBytes != 65536 {
		t.Errorf("Tuning.DirtyBackgroundBytes = %d, want 65536", cfg.Tuning.DirtyBackgroundBytes)
	}
	if cfg.Tuning.CPUGovernor != "conservative" {
		t.Errorf("Tuning.CPUGovernor = %q, want %q", cfg.Tuning.CPUGovernor, "conservative")
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

	// WiFi Watchdog defaults
	if !cfg.WiFi.Watchdog.Enabled {
		t.Error("WiFi.Watchdog.Enabled should default to true")
	}
	if cfg.WiFi.Watchdog.IntervalSeconds != 60 {
		t.Errorf("WiFi.Watchdog.IntervalSeconds = %d, want 60", cfg.WiFi.Watchdog.IntervalSeconds)
	}
	if cfg.WiFi.Watchdog.MaxFailures != 5 {
		t.Errorf("WiFi.Watchdog.MaxFailures = %d, want 5", cfg.WiFi.Watchdog.MaxFailures)
	}
}

func TestLoadConfig(t *testing.T) {
	tomlContent := `
[archive]
system = "rsync"
delay_seconds = 120
saved_clips = false

[rsync]
server = "nas.local"
user = "admin"
path = "/mnt/tank/TeslaCam"

[wifi]
home_ssid = "MyNetwork"
home_password = "hunter2"
hidden = true

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

	if cfg.Archive.System != "rsync" {
		t.Errorf("Archive.System = %q, want %q", cfg.Archive.System, "rsync")
	}
	if cfg.Archive.DelaySeconds != 120 {
		t.Errorf("Archive.DelaySeconds = %d, want 120", cfg.Archive.DelaySeconds)
	}
	if cfg.Archive.SavedClips {
		t.Error("Archive.SavedClips should be false")
	}
	if cfg.Rsync.Server != "nas.local" {
		t.Errorf("Rsync.Server = %q, want %q", cfg.Rsync.Server, "nas.local")
	}
	if cfg.WiFi.HomeSSID != "MyNetwork" {
		t.Errorf("WiFi.HomeSSID = %q, want %q", cfg.WiFi.HomeSSID, "MyNetwork")
	}
	if cfg.WiFi.HomePassword != "hunter2" {
		t.Errorf("WiFi.HomePassword = %q, want %q", cfg.WiFi.HomePassword, "hunter2")
	}
	if !cfg.WiFi.Hidden {
		t.Error("WiFi.Hidden should be true")
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
	// Sync flags should remain at default (false) since not set in TOML.
	if cfg.Archive.SyncMusic {
		t.Error("Archive.SyncMusic should be false when not set")
	}
}

func TestLoadConfigWithMediaSync(t *testing.T) {
	tomlContent := `
[archive]
system = "cifs"
sync_music = true
sync_lightshow = true
sync_boombox = true
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

	if !cfg.Archive.SyncMusic {
		t.Error("Archive.SyncMusic should be true")
	}
	if !cfg.Archive.SyncLightShow {
		t.Error("Archive.SyncLightShow should be true")
	}
	if !cfg.Archive.SyncBoombox {
		t.Error("Archive.SyncBoombox should be true")
	}
}

func TestLoadConfigNewFeatures(t *testing.T) {
	tomlContent := `
[archive]
system = "cifs"
free_space_reserve_mb = 20480
archive_logs = true

[wifi]
home_ssid = "MyNetwork"

[wifi.watchdog]
enabled = false
interval_seconds = 120
max_failures = 10

[web]
username = "admin"
password = "letmein"
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

	if cfg.Archive.FreeSpaceReserveMB != 20480 {
		t.Errorf("FreeSpaceReserveMB = %d, want 20480", cfg.Archive.FreeSpaceReserveMB)
	}
	if !cfg.Archive.ArchiveLogs {
		t.Error("ArchiveLogs should be true")
	}
	if cfg.WiFi.Watchdog.Enabled {
		t.Error("Watchdog.Enabled should be false")
	}
	if cfg.WiFi.Watchdog.IntervalSeconds != 120 {
		t.Errorf("Watchdog.IntervalSeconds = %d, want 120", cfg.WiFi.Watchdog.IntervalSeconds)
	}
	if cfg.WiFi.Watchdog.MaxFailures != 10 {
		t.Errorf("Watchdog.MaxFailures = %d, want 10", cfg.WiFi.Watchdog.MaxFailures)
	}
	if cfg.Web.Username != "admin" || cfg.Web.Password != "letmein" {
		t.Errorf("Web auth = %q/%q, want admin/letmein", cfg.Web.Username, cfg.Web.Password)
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

// hasWarning reports whether any warning contains substr.
func hasWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestValidate_FullyWiredConfigIsClean(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Archive.System = "cifs"
	cfg.CIFS.Server = "192.168.1.100"
	cfg.CIFS.Share = "TeslaCam"
	cfg.CIFS.User = "tesla"
	cfg.WiFi.HomeSSID = "home"
	cfg.WiFi.HomePassword = "pw"

	if w := cfg.Validate(); len(w) != 0 {
		t.Errorf("expected no warnings for a complete config, got: %v", w)
	}
}

func TestValidate_UnknownArchiveSystem(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WiFi.HomeSSID = "home"
	cfg.WiFi.HomePassword = "pw"
	cfg.Archive.System = "smbv1"

	if !hasWarning(cfg.Validate(), "archive.system") {
		t.Error("expected a warning for an unrecognized archive.system")
	}
}

func TestValidate_BackendSelectedButUnconfigured(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WiFi.HomeSSID = "home"
	cfg.WiFi.HomePassword = "pw"
	cfg.Archive.System = "cifs" // server/share intentionally empty

	if !hasWarning(cfg.Validate(), "cifs") {
		t.Error("expected a warning when cifs is selected but server/share are unset")
	}
}

// TestValidate_AllWakeMethodsRecognized is the regression guard for the
// original bug class: a wake method that main.go handles must be accepted by
// Validate, and vice versa. If someone adds a method to one and not the other,
// this fails.
func TestValidate_AllWakeMethodsRecognized(t *testing.T) {
	for _, method := range []string{"none", "ble", "tessie", "webhook"} {
		cfg := DefaultConfig()
		cfg.WiFi.HomeSSID = "home"
		cfg.WiFi.HomePassword = "pw"
		cfg.Wake.Method = method
		if hasWarning(cfg.Validate(), "is not recognized") {
			t.Errorf("wake.method %q should be recognized", method)
		}
	}
}

func TestValidate_UnknownWakeMethod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WiFi.HomeSSID = "home"
	cfg.WiFi.HomePassword = "pw"
	cfg.Wake.Method = "teslafi" // removed stub — must now warn, not silently no-op

	if !hasWarning(cfg.Validate(), "wake.method") {
		t.Error("expected a warning for an unrecognized wake.method")
	}
}

func TestValidate_MissingWiFi(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Archive.System = "none"

	if !hasWarning(cfg.Validate(), "wifi.home_ssid is not set") {
		t.Error("expected a warning when home_ssid is unset")
	}
}

func TestValidate_SSIDWithoutPassword(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Archive.System = "none"
	cfg.WiFi.HomeSSID = "home" // no password

	if !hasWarning(cfg.Validate(), "home_password is empty") {
		t.Error("expected a warning when SSID is set without a password")
	}
}
