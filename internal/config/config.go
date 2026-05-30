package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ArchiveConfig defines archive behavior.
type ArchiveConfig struct {
	System             string `toml:"system"`
	DelaySeconds       int    `toml:"delay_seconds"`
	SavedClips         bool   `toml:"saved_clips"`
	SentryClips        bool   `toml:"sentry_clips"`
	RecentClips        bool   `toml:"recent_clips"`
	TrackModeClips     bool   `toml:"track_mode_clips"`
	SyncMusic          bool   `toml:"sync_music"`
	SyncLightShow      bool   `toml:"sync_lightshow"`
	SyncBoombox        bool   `toml:"sync_boombox"`
	FreeSpaceReserveMB int64  `toml:"free_space_reserve_mb"`
	ArchiveLogs        bool   `toml:"archive_logs"`
}

// CIFSConfig defines CIFS/SMB archive target settings.
type CIFSConfig struct {
	Server   string `toml:"server"`
	Share    string `toml:"share"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	Path     string `toml:"path"`
}

// RsyncConfig defines rsync archive target settings.
type RsyncConfig struct {
	Server   string `toml:"server"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	Path     string `toml:"path"`
}

// RcloneConfig defines rclone archive target settings.
type RcloneConfig struct {
	Path string `toml:"path"`
}

// NFSConfig defines NFS archive target settings.
type NFSConfig struct {
	Server string `toml:"server"`
	Share  string `toml:"share"`
	Path   string `toml:"path"`
}

// APConfig defines the access point sub-configuration.
type APConfig struct {
	Enabled  bool   `toml:"enabled"`
	SSID     string `toml:"ssid"`
	Password string `toml:"password"`
}

// WatchdogConfig defines WiFi connectivity monitoring settings.
type WatchdogConfig struct {
	Enabled         bool `toml:"enabled"`
	IntervalSeconds int  `toml:"interval_seconds"`
	MaxFailures     int  `toml:"max_failures"`
}

// WiFiConfig defines wireless network settings.
//
// HomePassword is the WPA passphrase for the home network. When both HomeSSID
// and HomePassword are set, the daemon provisions a NetworkManager connection
// profile from them at startup, so the Buildroot image (which has no Raspberry
// Pi Imager step to pre-seed WiFi) can join the network unattended.
type WiFiConfig struct {
	HomeSSID     string         `toml:"home_ssid"`
	HomePassword string         `toml:"home_password"`
	Hidden       bool           `toml:"hidden"`
	AP           APConfig       `toml:"ap"`
	Watchdog     WatchdogConfig `toml:"watchdog"`
}

// NtfyConfig defines ntfy notification settings.
type NtfyConfig struct {
	URL   string `toml:"url"`
	Topic string `toml:"topic"`
}

// AppriseConfig defines Apprise notification settings.
type AppriseConfig struct {
	URL string `toml:"url"`
}

// NotifyConfig defines notification settings.
type NotifyConfig struct {
	Ntfy    NtfyConfig    `toml:"ntfy"`
	Apprise AppriseConfig `toml:"apprise"`
}

// TessieConfig defines Tessie wake settings.
type TessieConfig struct {
	Token string `toml:"token"`
	VIN   string `toml:"vin"`
}

// WakeConfig defines vehicle wake settings.
type WakeConfig struct {
	Method     string       `toml:"method"`
	BLEVIN     string       `toml:"ble_vin"`
	Tessie     TessieConfig `toml:"tessie"`
	WebhookURL string       `toml:"webhook_url"`
}

// HealthConfig defines health monitoring thresholds and intervals.
type HealthConfig struct {
	TempWarningMC   int64 `toml:"temp_warning_mc"`
	TempCautionMC   int64 `toml:"temp_caution_mc"`
	IntervalSeconds int   `toml:"interval_seconds"`
}

// IdleConfig defines idle detection parameters.
type IdleConfig struct {
	WriteThresholdBytes int64 `toml:"write_threshold_bytes"`
	TimeoutSeconds      int   `toml:"timeout_seconds"`
}

// TuningConfig defines system tuning parameters.
type TuningConfig struct {
	DirtyRatio              int    `toml:"dirty_ratio"`
	DirtyBackgroundBytes    int64  `toml:"dirty_background_bytes"`
	DirtyWritebackCentisecs int    `toml:"dirty_writeback_centisecs"`
	CPUGovernor             string `toml:"cpu_governor"`
	CPUGovernorArchiving    string `toml:"cpu_governor_archiving"`
}

// WebConfig defines optional web UI HTTP basic authentication. When both
// Username and Password are set, the web server requires them for every
// request (the UI can delete footage, so it is unauthenticated otherwise).
type WebConfig struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

// Config is the top-level configuration.
type Config struct {
	Archive ArchiveConfig `toml:"archive"`
	CIFS    CIFSConfig    `toml:"cifs"`
	Rsync   RsyncConfig   `toml:"rsync"`
	Rclone  RcloneConfig  `toml:"rclone"`
	NFS     NFSConfig     `toml:"nfs"`
	WiFi    WiFiConfig    `toml:"wifi"`
	Notify  NotifyConfig  `toml:"notify"`
	Wake    WakeConfig    `toml:"wake"`
	Health  HealthConfig  `toml:"health"`
	Idle    IdleConfig    `toml:"idle"`
	Tuning  TuningConfig  `toml:"tuning"`
	Web     WebConfig     `toml:"web"`
}

// DefaultConfig returns a Config with sensible defaults applied.
func DefaultConfig() Config {
	return Config{
		Archive: ArchiveConfig{
			System:             "cifs",
			DelaySeconds:       60,
			SavedClips:         true,
			SentryClips:        true,
			RecentClips:        false,
			TrackModeClips:     false,
			FreeSpaceReserveMB: 10240, // 10 GiB
			ArchiveLogs:        false,
		},
		CIFS: CIFSConfig{
			Path: "TeslaCam",
		},
		Rsync: RsyncConfig{
			Path: "TeslaCam",
		},
		Rclone: RcloneConfig{
			Path: "TeslaCam",
		},
		NFS: NFSConfig{
			Path: "TeslaCam",
		},
		WiFi: WiFiConfig{
			AP: APConfig{
				Enabled:  false,
				SSID:     "teslausb",
				Password: "teslausb",
			},
			Watchdog: WatchdogConfig{
				Enabled:         true,
				IntervalSeconds: 60,
				MaxFailures:     5,
			},
		},
		Wake: WakeConfig{
			Method: "none",
		},
		Health: HealthConfig{
			TempWarningMC:   80000,
			TempCautionMC:   70000,
			IntervalSeconds: 60,
		},
		Idle: IdleConfig{
			WriteThresholdBytes: 1024,
			TimeoutSeconds:      300,
		},
		Tuning: TuningConfig{
			DirtyRatio:              10,
			DirtyBackgroundBytes:    65536,
			DirtyWritebackCentisecs: 25,
			CPUGovernor:             "conservative",
			CPUGovernorArchiving:    "ondemand",
		},
	}
}

// Load reads TOML configuration from path and applies defaults for any
// fields not present in the file.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// validArchiveSystems and validWakeMethods are the only accepted values for
// archive.system and wake.method. They are kept here (next to Validate) so a
// newly added backend or wake keeper that is wired in main.go but forgotten
// here trips the test suite rather than silently degrading to a no-op.
var validArchiveSystems = map[string]bool{
	"": true, "none": true, "cifs": true, "rsync": true, "rclone": true, "nfs": true,
}

var validWakeMethods = map[string]bool{
	"": true, "none": true, "ble": true, "tessie": true, "webhook": true,
}

// Validate returns a list of human-readable configuration warnings. It never
// fails the process — the USB gadget must come up even with a broken archive
// config — but every warning describes a setting that is selected yet cannot
// take effect, which is exactly the class of "silent no-op" bug this guards
// against. main logs each warning loudly at startup.
func (c Config) Validate() []string {
	var w []string

	// Archive backend: known system + required fields present.
	if !validArchiveSystems[c.Archive.System] {
		w = append(w, fmt.Sprintf("archive.system = %q is not recognized (valid: cifs, rsync, rclone, nfs, none); archiving is disabled", c.Archive.System))
	}
	switch c.Archive.System {
	case "cifs":
		if c.CIFS.Server == "" || c.CIFS.Share == "" {
			w = append(w, "archive.system = cifs but [cifs] server/share are not set; archiving will fail")
		}
	case "rsync":
		if c.Rsync.Server == "" || c.Rsync.User == "" {
			w = append(w, "archive.system = rsync but [rsync] server/user are not set; archiving will fail")
		}
	case "nfs":
		if c.NFS.Server == "" || c.NFS.Share == "" {
			w = append(w, "archive.system = nfs but [nfs] server/share are not set; archiving will fail")
		}
	case "rclone":
		if c.Rclone.Path == "" {
			w = append(w, "archive.system = rclone but [rclone] path is not set; archiving will fail")
		}
	}

	// Wake keeper: known method + required fields present.
	if !validWakeMethods[c.Wake.Method] {
		w = append(w, fmt.Sprintf("wake.method = %q is not recognized (valid: none, ble, tessie, webhook); the car will not be kept awake", c.Wake.Method))
	}
	switch c.Wake.Method {
	case "ble":
		if c.Wake.BLEVIN == "" {
			w = append(w, "wake.method = ble but wake.ble_vin is not set")
		}
	case "tessie":
		if c.Wake.Tessie.Token == "" || c.Wake.Tessie.VIN == "" {
			w = append(w, "wake.method = tessie but [wake.tessie] token/vin are not set")
		}
	case "webhook":
		if c.Wake.WebhookURL == "" {
			w = append(w, "wake.method = webhook but wake.webhook_url is not set")
		}
	}

	// WiFi: archiving is gated on home WiFi, so a missing SSID means it never
	// runs. A set SSID with no password relies on an externally-provisioned
	// NetworkManager profile (Raspberry Pi Imager on the Pi OS path) — fine if
	// intentional, worth flagging because it is the exact gap that left the
	// Buildroot path unable to connect.
	if c.WiFi.HomeSSID == "" {
		w = append(w, "wifi.home_ssid is not set; the archive cycle will never trigger (it waits for home WiFi)")
	} else if c.WiFi.HomePassword == "" {
		w = append(w, "wifi.home_ssid is set but wifi.home_password is empty; relying on a pre-existing NetworkManager profile (e.g. seeded by Raspberry Pi Imager). On the Buildroot image set wifi.home_password so the daemon can create the connection itself")
	}

	return w
}
