package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

// GadgetConfig defines USB mass storage gadget sizes and options.
type GadgetConfig struct {
	CamSize       string `toml:"cam_size"`
	MusicSize     string `toml:"music_size"`
	LightshowSize string `toml:"lightshow_size"`
	BoomboxSize   string `toml:"boombox_size"`
	UseExFAT      bool   `toml:"use_exfat"`
}

// ArchiveConfig defines archive behavior.
type ArchiveConfig struct {
	System         string `toml:"system"`
	DelaySeconds   int    `toml:"delay_seconds"`
	SavedClips     bool   `toml:"saved_clips"`
	SentryClips    bool   `toml:"sentry_clips"`
	RecentClips    bool   `toml:"recent_clips"`
	TrackModeClips bool   `toml:"track_mode_clips"`
	SyncMusic      bool   `toml:"sync_music"`
	SyncLightShow  bool   `toml:"sync_lightshow"`
	SyncBoombox    bool   `toml:"sync_boombox"`
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

// WiFiConfig defines wireless network settings.
type WiFiConfig struct {
	HomeSSID string   `toml:"home_ssid"`
	AP       APConfig `toml:"ap"`
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

// TeslaFiConfig defines TeslaFi wake settings.
type TeslaFiConfig struct {
	Token string `toml:"token"`
}

// WakeConfig defines vehicle wake settings.
type WakeConfig struct {
	Method     string        `toml:"method"`
	BLEVIN     string        `toml:"ble_vin"`
	Tessie     TessieConfig  `toml:"tessie"`
	TeslaFi    TeslaFiConfig `toml:"teslafi"`
	WebhookURL string        `toml:"webhook_url"`
}

// HealthConfig defines health monitoring thresholds and intervals.
type HealthConfig struct {
	TempWarningMC  int64 `toml:"temp_warning_mc"`
	TempCautionMC  int64 `toml:"temp_caution_mc"`
	IntervalSeconds int   `toml:"interval_seconds"`
}

// IdleConfig defines idle detection parameters.
type IdleConfig struct {
	WriteThresholdBytes     int64 `toml:"write_threshold_bytes"`
	TimeoutSeconds          int   `toml:"timeout_seconds"`
	SnapshotIntervalSeconds int   `toml:"snapshot_interval_seconds"`
}

// TuningConfig defines system tuning parameters.
type TuningConfig struct {
	DirtyRatio              int    `toml:"dirty_ratio"`
	DirtyBackgroundBytes    int64  `toml:"dirty_background_bytes"`
	DirtyWritebackCentisecs int    `toml:"dirty_writeback_centisecs"`
	CPUGovernor             string `toml:"cpu_governor"`
	CPUGovernorArchiving    string `toml:"cpu_governor_archiving"`
}

// Config is the top-level configuration.
type Config struct {
	Gadget  GadgetConfig  `toml:"gadget"`
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
}

// DefaultConfig returns a Config with sensible defaults applied.
func DefaultConfig() Config {
	return Config{
		Gadget: GadgetConfig{
			CamSize:       "16G",
			MusicSize:     "0",
			LightshowSize: "0",
			BoomboxSize:   "0",
			UseExFAT:      false,
		},
		Archive: ArchiveConfig{
			System:         "cifs",
			DelaySeconds:   60,
			SavedClips:     true,
			SentryClips:    true,
			RecentClips:    false,
			TrackModeClips: false,
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
		},
		Wake: WakeConfig{
			Method: "none",
		},
		Health: HealthConfig{
			TempWarningMC:  80000,
			TempCautionMC:  70000,
			IntervalSeconds: 60,
		},
		Idle: IdleConfig{
			WriteThresholdBytes:     1024,
			TimeoutSeconds:          300,
			SnapshotIntervalSeconds: 5,
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
