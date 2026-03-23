package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// State represents a state machine state.
type State string

const (
	Booting        State = "booting"
	GadgetUp       State = "gadget_up"
	WaitingForWifi State = "waiting_for_wifi"
	PreArchive     State = "pre_archive"
	Snapshot       State = "snapshot"
	Fsck           State = "fsck"
	Reconnect      State = "reconnect"
	Archiving      State = "archiving"
	Cleanup        State = "cleanup"
)

// Config holds the application configuration loaded from TOML.
type Config struct {
	WiFiSSID       string `toml:"wifi_ssid"`
	WiFiPassword   string `toml:"wifi_password"`
	ArchiveBackend string `toml:"archive_backend"`
	ArchiveTarget  string `toml:"archive_target"`
	WebListenAddr  string `toml:"web_listen_addr"`
	DataDir        string `toml:"data_dir"`
	CamPartition   string `toml:"cam_partition"`
	MusicPartition string `toml:"music_partition"`
	UDCController  string `toml:"udc_controller"`
}

// StateMachine orchestrates the main control loop.
type StateMachine struct {
	mu      sync.RWMutex
	state   State
	config  Config
	cancel  context.CancelFunc
	startUp time.Time
}

// NewStateMachine creates a new state machine with the given config.
func NewStateMachine(cfg Config) *StateMachine {
	return &StateMachine{
		state:   Booting,
		config:  cfg,
		startUp: time.Now(),
	}
}

// CurrentState returns the current state (thread-safe).
func (sm *StateMachine) CurrentState() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

// transition moves the state machine to the next state.
func (sm *StateMachine) transition(next State) {
	sm.mu.Lock()
	prev := sm.state
	sm.state = next
	sm.mu.Unlock()
	slog.Info("state transition", "from", string(prev), "to", string(next))
}

// Run executes the main state machine loop. It blocks until the context is
// cancelled or an unrecoverable error occurs.
func (sm *StateMachine) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("state machine shutting down")
			return ctx.Err()
		default:
		}

		switch sm.CurrentState() {
		case Booting:
			sm.transition(GadgetUp)

		case GadgetUp:
			if err := sm.configureGadget(); err != nil {
				slog.Error("gadget setup failed", "error", err)
				time.Sleep(2 * time.Second)
				continue
			}
			elapsed := time.Since(sm.startUp).Seconds()
			slog.Info(fmt.Sprintf("USB gadget presented at %.1f seconds uptime", elapsed))
			sm.transition(WaitingForWifi)

		case WaitingForWifi:
			if err := sm.waitForWifi(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				slog.Warn("wifi connection attempt failed, retrying", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}
			sm.transition(PreArchive)

		case PreArchive:
			sm.transition(Snapshot)

		case Snapshot:
			if err := sm.createSnapshot(); err != nil {
				slog.Error("snapshot creation failed", "error", err)
				sm.transition(Cleanup)
				continue
			}
			sm.transition(Fsck)

		case Fsck:
			if err := sm.runFsck(); err != nil {
				slog.Warn("fsck reported issues", "error", err)
			}
			sm.transition(Reconnect)

		case Reconnect:
			sm.transition(Archiving)

		case Archiving:
			if err := sm.runArchive(ctx); err != nil {
				slog.Error("archive failed", "error", err)
			}
			sm.transition(Cleanup)

		case Cleanup:
			sm.cleanupSnapshot()
			sm.transition(WaitingForWifi)
			// Pause before next cycle to avoid busy-looping.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(60 * time.Second):
			}
		}
	}
}

// configureGadget sets up the USB mass storage gadget via configfs.
func (sm *StateMachine) configureGadget() error {
	gadgetBase := "/sys/kernel/config/usb_gadget/teslausb"

	// Create gadget directory structure.
	dirs := []string{
		gadgetBase,
		filepath.Join(gadgetBase, "strings/0x409"),
		filepath.Join(gadgetBase, "configs/c.1"),
		filepath.Join(gadgetBase, "configs/c.1/strings/0x409"),
		filepath.Join(gadgetBase, "functions/mass_storage.usb0"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	// Write gadget descriptor attributes.
	attrs := map[string]string{
		filepath.Join(gadgetBase, "idVendor"):                          "0x1d6b",
		filepath.Join(gadgetBase, "idProduct"):                         "0x0104",
		filepath.Join(gadgetBase, "strings/0x409/serialnumber"):        "teslausb-neo",
		filepath.Join(gadgetBase, "strings/0x409/manufacturer"):        "TeslaUSB",
		filepath.Join(gadgetBase, "strings/0x409/product"):             "TeslaUSB Mass Storage",
		filepath.Join(gadgetBase, "configs/c.1/strings/0x409/configuration"): "Config 1",
	}
	for path, value := range attrs {
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	// Bind cam partition to LUN 0.
	lun0 := filepath.Join(gadgetBase, "functions/mass_storage.usb0/lun.0")
	if err := os.MkdirAll(lun0, 0755); err != nil {
		return fmt.Errorf("mkdir lun0: %w", err)
	}

	camPart := sm.config.CamPartition
	if camPart == "" {
		camPart = "/dev/mmcblk0p4" // default cam partition
	}
	if err := os.WriteFile(filepath.Join(lun0, "file"), []byte(camPart), 0644); err != nil {
		return fmt.Errorf("bind cam partition: %w", err)
	}
	// Set nofua to reduce unnecessary flush commands.
	if err := os.WriteFile(filepath.Join(lun0, "nofua"), []byte("1"), 0644); err != nil {
		slog.Warn("failed to set nofua on lun0", "error", err)
	}

	// Symlink function into config.
	funcLink := filepath.Join(gadgetBase, "configs/c.1/mass_storage.usb0")
	funcTarget := filepath.Join(gadgetBase, "functions/mass_storage.usb0")
	_ = os.Remove(funcLink) // remove stale symlink if present
	if err := os.Symlink(funcTarget, funcLink); err != nil {
		return fmt.Errorf("symlink function: %w", err)
	}

	// Activate UDC.
	udc := sm.config.UDCController
	if udc == "" {
		// Auto-detect UDC controller.
		entries, err := os.ReadDir("/sys/class/udc")
		if err != nil || len(entries) == 0 {
			return fmt.Errorf("no UDC controllers found")
		}
		udc = entries[0].Name()
	}
	if err := os.WriteFile(filepath.Join(gadgetBase, "UDC"), []byte(udc), 0644); err != nil {
		return fmt.Errorf("activate UDC: %w", err)
	}

	return nil
}

// waitForWifi blocks until a wifi connection is established or the context is cancelled.
func (sm *StateMachine) waitForWifi(ctx context.Context) error {
	slog.Info("waiting for wifi connection", "ssid", sm.config.WiFiSSID)

	// Poll for connectivity. In production this would invoke wpa_supplicant
	// or networkd and check for an IP address.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if sm.checkConnectivity() {
				slog.Info("wifi connected")
				return nil
			}
		}
	}
}

// checkConnectivity returns true if the system has network connectivity.
func (sm *StateMachine) checkConnectivity() bool {
	// Stub: check for default route or IP on wlan0.
	_, err := os.Stat("/sys/class/net/wlan0/operstate")
	if err != nil {
		return false
	}
	data, err := os.ReadFile("/sys/class/net/wlan0/operstate")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "up"
}

// createSnapshot creates a dm-snapshot of the cam partition for safe archiving.
func (sm *StateMachine) createSnapshot() error {
	slog.Info("creating dm-snapshot for archive")
	// Stub: would invoke dmsetup to create a snapshot with a zram COW device.
	return nil
}

// runFsck performs a filesystem check on the snapshot.
func (sm *StateMachine) runFsck() error {
	slog.Info("running fsck on snapshot")
	// Stub: would invoke fsck.exfat / fsck.vfat on the snapshot device.
	return nil
}

// runArchive copies new/changed files from the snapshot to the archive backend.
func (sm *StateMachine) runArchive(ctx context.Context) error {
	slog.Info("starting archive", "backend", sm.config.ArchiveBackend, "target", sm.config.ArchiveTarget)
	// Stub: would iterate files and upload via the configured backend
	// (rclone, cifs, ssh, etc).
	return nil
}

// cleanupSnapshot removes the dm-snapshot after archiving completes.
func (sm *StateMachine) cleanupSnapshot() {
	slog.Info("cleaning up snapshot")
	// Stub: would invoke dmsetup remove.
}

// applySystemTuning sets kernel parameters for optimal USB gadget performance.
func applySystemTuning() {
	tunings := map[string]string{
		"/proc/sys/vm/dirty_background_ratio": "5",
		"/proc/sys/vm/dirty_ratio":            "10",
		"/proc/sys/vm/dirty_writeback_centisecs": "100",
	}
	for path, value := range tunings {
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			slog.Warn("failed to apply tuning", "path", path, "error", err)
		}
	}

	// Set BFQ I/O scheduler on mmcblk0 if available.
	schedulerPath := "/sys/block/mmcblk0/queue/scheduler"
	if err := os.WriteFile(schedulerPath, []byte("bfq"), 0644); err != nil {
		slog.Warn("failed to set BFQ scheduler", "error", err)
	}

	// Set CPU governor to ondemand.
	governorPath := "/sys/devices/system/cpu/cpufreq/policy0/scaling_governor"
	if err := os.WriteFile(governorPath, []byte("ondemand"), 0644); err != nil {
		slog.Warn("failed to set CPU governor", "error", err)
	}
}

// loadConfig reads configuration from a TOML file, trying the primary path
// first and falling back to an alternative.
func loadConfig() Config {
	cfg := Config{
		WebListenAddr: ":8080",
		DataDir:       "/data",
		ArchiveBackend: "rclone",
	}

	primary := "/data/teslausb.toml"
	fallback := "/etc/teslausb.toml"

	configPath := primary
	if _, err := os.Stat(primary); os.IsNotExist(err) {
		configPath = fallback
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		slog.Warn("config file not found, using defaults", "tried", configPath, "error", err)
		return cfg
	}

	// Minimal TOML-like parser for key = "value" lines.
	// A real implementation would use a TOML library.
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		switch key {
		case "wifi_ssid":
			cfg.WiFiSSID = value
		case "wifi_password":
			cfg.WiFiPassword = value
		case "archive_backend":
			cfg.ArchiveBackend = value
		case "archive_target":
			cfg.ArchiveTarget = value
		case "web_listen_addr":
			cfg.WebListenAddr = value
		case "data_dir":
			cfg.DataDir = value
		case "cam_partition":
			cfg.CamPartition = value
		case "music_partition":
			cfg.MusicPartition = value
		case "udc_controller":
			cfg.UDCController = value
		}
	}

	slog.Info("configuration loaded", "path", configPath)
	return cfg
}

func main() {
	// Structured logging to stderr.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("teslausb-neo starting")

	// Load configuration.
	cfg := loadConfig()

	// Apply system tuning (VM params, BFQ, CPU governor).
	applySystemTuning()

	// Create cancellable context for clean shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGTERM/SIGINT for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, initiating shutdown", "signal", sig)
		cancel()
	}()

	// Create state machine.
	sm := NewStateMachine(cfg)

	// Start health monitor goroutine.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				slog.Debug("health check", "state", string(sm.CurrentState()))
			}
		}
	}()

	// Start web server in background goroutine.
	go func() {
		// Import is not used directly here since the web package has its own
		// initialization. In a full build, we'd wire the status channel and
		// state DB through here. For now, log a placeholder.
		slog.Info("web server goroutine placeholder", "addr", cfg.WebListenAddr)
		// In production:
		// webSrv := web.NewServer(web.Config{ArchiveDir: cfg.DataDir}, stateDB, statusCh)
		// if err := webSrv.Start(cfg.WebListenAddr); err != nil {
		//     slog.Error("web server failed", "error", err)
		// }
	}()

	// Run the main state machine loop.
	if err := sm.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("state machine exited with error", "error", err)
		os.Exit(1)
	}

	slog.Info("teslausb-neo shutdown complete")
}
