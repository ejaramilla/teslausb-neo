package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ejaramilla/teslausb-neo/internal/archive"
	"github.com/ejaramilla/teslausb-neo/internal/config"
	"github.com/ejaramilla/teslausb-neo/internal/fsutil"
	"github.com/ejaramilla/teslausb-neo/internal/fswatch"
	"github.com/ejaramilla/teslausb-neo/internal/gadget"
	"github.com/ejaramilla/teslausb-neo/internal/health"
	"github.com/ejaramilla/teslausb-neo/internal/notify"
	"github.com/ejaramilla/teslausb-neo/internal/snapshot"
	"github.com/ejaramilla/teslausb-neo/internal/state"
	"github.com/ejaramilla/teslausb-neo/internal/sys"
	"github.com/ejaramilla/teslausb-neo/internal/tesla"
	"github.com/ejaramilla/teslausb-neo/internal/web"
	"github.com/ejaramilla/teslausb-neo/internal/wifi"
)

// version is set at build time via -ldflags.
var version = "dev"

// State represents a state machine state.
type State string

const (
	StateBooting        State = "booting"
	StateGadgetUp       State = "gadget_up"
	StateWaitingForWifi State = "waiting_for_wifi"
	StatePreArchive     State = "pre_archive"
	StateSnapshot       State = "snapshot"
	StateFsck           State = "fsck"
	StateReconnect      State = "reconnect"
	StateArchiving      State = "archiving"
	StateMediaSync      State = "media_sync"
	StateCleanup        State = "cleanup"
)

const (
	camPartition       = "/dev/mmcblk0p3"
	musicPartition     = "/dev/mmcblk0p4"
	lightshowPartition = "/dev/mmcblk0p5"
	boomboxPartition   = "/dev/mmcblk0p6"
	snapshotMountpoint = "/mnt/snap"
	mediaMountpoint    = "/mnt/media"
	configPath         = "/data/teslausb.toml"
	configFallback     = "/etc/teslausb.toml"
	dbPath             = "/data/teslausb.db"
)

// StateMachine orchestrates the main control loop.
type StateMachine struct {
	mu    sync.RWMutex
	state State

	cfg      config.Config
	gad      *gadget.Gadget
	archiver archive.Backend
	notifier notify.Notifier
	wake     tesla.WakeKeeper
	wifiMgr  *wifi.Manager
	watcher  *fswatch.Watcher
	db       *state.DB

	snap    *snapshot.Snapshot
	startUp time.Time
}

// CurrentState returns the current state (thread-safe).
func (sm *StateMachine) CurrentState() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

func (sm *StateMachine) transition(next State) {
	sm.mu.Lock()
	prev := sm.state
	sm.state = next
	sm.mu.Unlock()
	slog.Info("state transition", "from", string(prev), "to", string(next))
}

// Run executes the main state machine loop.
func (sm *StateMachine) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("state machine shutting down")
			sm.deactivateGadget()
			return ctx.Err()
		default:
		}

		switch sm.CurrentState() {
		case StateBooting:
			if err := sm.boot(); err != nil {
				slog.Error("boot failed", "error", err)
				time.Sleep(2 * time.Second)
				continue
			}
			sm.transition(StateGadgetUp)

		case StateGadgetUp:
			sm.transition(StateWaitingForWifi)

		case StateWaitingForWifi:
			if err := sm.waitForWifi(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				time.Sleep(5 * time.Second)
				continue
			}
			sm.transition(StatePreArchive)

		case StatePreArchive:
			// Wait for Tesla to stop writing before snapshotting.
			slog.Info("waiting for idle before archiving")
			if err := sm.watcher.WaitForIdle(ctx); err != nil {
				slog.Warn("idle wait failed or timed out, proceeding anyway", "error", err)
			}
			// Delay to let final writes flush.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(sm.cfg.Archive.DelaySeconds) * time.Second):
			}
			sm.transition(StateSnapshot)

		case StateSnapshot:
			if err := sm.createSnapshot(); err != nil {
				slog.Error("snapshot creation failed", "error", err)
				sm.transition(StateCleanup)
				continue
			}
			sm.transition(StateFsck)

		case StateFsck:
			if err := sm.runFsck(); err != nil {
				slog.Warn("fsck reported issues", "error", err)
			}
			sm.transition(StateReconnect)

		case StateReconnect:
			// Reconnect USB gadget so Tesla can resume writing immediately.
			// We archive from the snapshot, not the live partition.
			if err := sm.reactivateGadget(); err != nil {
				slog.Error("gadget reactivation failed", "error", err)
			}
			sm.transition(StateArchiving)

		case StateArchiving:
			if err := sm.runArchive(ctx); err != nil {
				slog.Error("archive failed", "error", err)
			}
			sm.transition(StateMediaSync)

		case StateMediaSync:
			if err := sm.runMediaSync(ctx); err != nil {
				slog.Error("media sync failed", "error", err)
			}
			sm.transition(StateCleanup)

		case StateCleanup:
			sm.cleanupSnapshot()
			sm.transition(StateWaitingForWifi)
			// Wait before next cycle.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(60 * time.Second):
			}
		}
	}
}

// boot configures the USB gadget and presents it to Tesla as fast as possible.
func (sm *StateMachine) boot() error {
	sm.gad = gadget.New("teslausb")

	if err := sm.gad.Configure(); err != nil {
		return fmt.Errorf("gadget configure: %w", err)
	}

	// Add LUNs for each partition that exists.
	partitions := []struct {
		device string
		label  string
	}{
		{camPartition, "TeslaUSB CAM"},
		{musicPartition, "TeslaUSB MUSIC"},
		{lightshowPartition, "TeslaUSB LIGHTSHOW"},
		{boomboxPartition, "TeslaUSB BOOMBOX"},
	}
	for _, p := range partitions {
		if _, err := os.Stat(p.device); err == nil {
			if err := sm.gad.AddLUN(p.device, p.label); err != nil {
				slog.Warn("failed to add LUN", "device", p.device, "error", err)
			}
		}
	}

	// Enable nofua on all LUNs for write caching performance.
	for i := range sm.gad.LUNs {
		if err := sm.gad.SetNofua(i, true); err != nil {
			slog.Warn("failed to set nofua", "lun", i, "error", err)
		}
	}

	if err := sm.gad.Activate(); err != nil {
		return fmt.Errorf("gadget activate: %w", err)
	}

	elapsed := time.Since(sm.startUp).Seconds()
	slog.Info(fmt.Sprintf("USB gadget presented at %.1f seconds uptime", elapsed))
	return nil
}

func (sm *StateMachine) deactivateGadget() {
	if sm.gad != nil {
		if err := sm.gad.Deactivate(); err != nil {
			slog.Warn("gadget deactivation failed", "error", err)
		}
	}
}

func (sm *StateMachine) reactivateGadget() error {
	if sm.gad != nil {
		return sm.gad.Activate()
	}
	return nil
}

// waitForWifi blocks until home WiFi is connected.
func (sm *StateMachine) waitForWifi(ctx context.Context) error {
	slog.Info("waiting for wifi", "ssid", sm.cfg.WiFi.HomeSSID)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if sm.wifiMgr.IsConnected() && sm.wifiMgr.IsHomeNetwork(sm.cfg.WiFi.HomeSSID) {
				slog.Info("connected to home wifi")
				return nil
			}
		}
	}
}

// createSnapshot deactivates the gadget, creates a dm-snapshot with zram COW.
func (sm *StateMachine) createSnapshot() error {
	slog.Info("deactivating gadget for snapshot")
	if err := sm.gad.Deactivate(); err != nil {
		return fmt.Errorf("deactivate gadget: %w", err)
	}

	slog.Info("creating dm-snapshot", "device", camPartition)
	snap, err := snapshot.Create(camPartition)
	if err != nil {
		// Try to reactivate gadget even if snapshot fails.
		_ = sm.gad.Activate()
		return fmt.Errorf("create snapshot: %w", err)
	}
	sm.snap = snap
	slog.Info("snapshot created successfully")
	return nil
}

// runFsck runs filesystem check on the live cam partition.
func (sm *StateMachine) runFsck() error {
	slog.Info("running fsck on cam partition", "device", camPartition)
	return fsutil.RunFsck(camPartition)
}

// runArchive mounts the snapshot, connects to the archive backend, and transfers files.
func (sm *StateMachine) runArchive(ctx context.Context) error {
	if sm.snap == nil {
		return fmt.Errorf("no snapshot available")
	}

	// Mount snapshot read-only.
	if err := sm.snap.Mount(snapshotMountpoint); err != nil {
		return fmt.Errorf("mount snapshot: %w", err)
	}
	defer func() {
		if err := fsutil.Unmount(snapshotMountpoint); err != nil {
			slog.Warn("failed to unmount snapshot", "error", err)
		}
	}()

	// Start Tesla wake to keep car awake during archiving.
	if err := sm.wake.Start(ctx); err != nil {
		slog.Warn("failed to start wake keeper", "error", err)
	}
	defer func() {
		if err := sm.wake.Stop(ctx); err != nil {
			slog.Warn("failed to stop wake keeper", "error", err)
		}
	}()

	// Switch to performance CPU governor during archiving.
	sys.SetCPUGovernor(sm.cfg.Tuning.CPUGovernorArchiving)
	defer sys.SetCPUGovernor(sm.cfg.Tuning.CPUGovernor)

	// Connect to archive backend.
	if !sm.archiver.IsReachable(ctx) {
		return fmt.Errorf("archive backend %s is not reachable", sm.archiver.Name())
	}
	if err := sm.archiver.Connect(ctx); err != nil {
		return fmt.Errorf("connect to archive: %w", err)
	}
	defer func() {
		if err := sm.archiver.Disconnect(ctx); err != nil {
			slog.Warn("failed to disconnect archive", "error", err)
		}
	}()

	// Send start notification.
	_ = sm.notifier.Send(ctx, "TeslaUSB", "Archiving started", notify.EventStart)

	// Create archive session in SQLite.
	sessionID, err := sm.db.CreateSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	// Scan snapshot for files to archive.
	allFiles, err := scanFilesForArchive(snapshotMountpoint, sm.cfg.Archive)
	if err != nil {
		sm.db.FailSession(sessionID, err.Error())
		return fmt.Errorf("scan files: %w", err)
	}

	// Filter out already-archived files.
	newFiles := sm.db.ListUnarchived(allFiles)
	slog.Info("files to archive", "total", len(allFiles), "new", len(newFiles))

	if len(newFiles) == 0 {
		sm.db.CompleteSession(sessionID, 0, 0)
		_ = sm.notifier.Send(ctx, "TeslaUSB", "No new files to archive", notify.EventFinish)
		return nil
	}

	// Archive files.
	var archived int64
	progressFn := archive.ProgressFunc(func(current, total int, filename string) {
		archived = int64(current)
		if err := sm.db.MarkArchived(filename, 0, sessionID); err != nil {
			slog.Warn("failed to mark archived", "path", filename, "error", err)
		}
	})

	err = sm.archiver.ArchiveFiles(ctx, snapshotMountpoint, newFiles, progressFn)
	if err != nil {
		sm.db.FailSession(sessionID, err.Error())
		_ = sm.notifier.Send(ctx, "TeslaUSB", fmt.Sprintf("Archive failed: %v", err), notify.EventError)
		return err
	}

	sm.db.CompleteSession(sessionID, archived, 0)
	_ = sm.notifier.Send(ctx, "TeslaUSB", fmt.Sprintf("Archived %d files", archived), notify.EventFinish)
	return nil
}

// runMediaSync deactivates the gadget, mounts each media partition, and syncs
// content from the archive backend. This mirrors the Music/, LightShow/, and
// Boombox/ folders from the user's server to the corresponding USB partitions.
func (sm *StateMachine) runMediaSync(ctx context.Context) error {
	type mediaTarget struct {
		enabled   bool
		partition string
		folder    string
	}
	targets := []mediaTarget{
		{sm.cfg.Archive.SyncMusic, musicPartition, "Music"},
		{sm.cfg.Archive.SyncLightShow, lightshowPartition, "LightShow"},
		{sm.cfg.Archive.SyncBoombox, boomboxPartition, "Boombox"},
	}

	// Check if any sync is enabled.
	var anyEnabled bool
	for _, t := range targets {
		if t.enabled {
			anyEnabled = true
			break
		}
	}
	if !anyEnabled {
		return nil
	}

	// Connect to archive backend (may already be disconnected after runArchive).
	if !sm.archiver.IsReachable(ctx) {
		return fmt.Errorf("archive backend %s is not reachable for media sync", sm.archiver.Name())
	}
	if err := sm.archiver.Connect(ctx); err != nil {
		return fmt.Errorf("connect for media sync: %w", err)
	}
	defer func() {
		if err := sm.archiver.Disconnect(ctx); err != nil {
			slog.Warn("failed to disconnect after media sync", "error", err)
		}
	}()

	// Deactivate gadget so we can mount the media partitions.
	slog.Info("deactivating gadget for media sync")
	if err := sm.gad.Deactivate(); err != nil {
		return fmt.Errorf("deactivate gadget for media sync: %w", err)
	}
	defer func() {
		if err := sm.gad.Activate(); err != nil {
			slog.Error("failed to reactivate gadget after media sync", "error", err)
		}
	}()

	for _, t := range targets {
		if !t.enabled {
			continue
		}
		if _, err := os.Stat(t.partition); err != nil {
			slog.Warn("media partition not found, skipping sync", "partition", t.partition, "folder", t.folder)
			continue
		}

		slog.Info("syncing media", "folder", t.folder, "partition", t.partition)

		mountpoint := mediaMountpoint + "/" + t.folder
		if err := os.MkdirAll(mountpoint, 0o755); err != nil {
			slog.Error("failed to create media mountpoint", "path", mountpoint, "error", err)
			continue
		}

		if err := fsutil.Mount(t.partition, mountpoint, "exfat", false); err != nil {
			slog.Error("failed to mount media partition", "partition", t.partition, "error", err)
			continue
		}

		if err := sm.archiver.SyncMedia(ctx, mountpoint, t.folder); err != nil {
			slog.Error("media sync failed", "folder", t.folder, "error", err)
		} else {
			slog.Info("media sync complete", "folder", t.folder)
		}

		if err := fsutil.Unmount(mountpoint); err != nil {
			slog.Error("failed to unmount media partition", "mountpoint", mountpoint, "error", err)
		}
	}

	return nil
}

// cleanupSnapshot releases the dm-snapshot and frees the zram device.
func (sm *StateMachine) cleanupSnapshot() {
	if sm.snap != nil {
		slog.Info("releasing snapshot")
		if err := sm.snap.Release(); err != nil {
			slog.Error("failed to release snapshot", "error", err)
		}
		sm.snap = nil
	}
}

// scanFilesForArchive walks the snapshot mount and returns file paths to archive.
func scanFilesForArchive(root string, cfg config.ArchiveConfig) ([]string, error) {
	var files []string
	dirs := []struct {
		path    string
		enabled bool
	}{
		{"TeslaCam/SavedClips", cfg.SavedClips},
		{"TeslaCam/SentryClips", cfg.SentryClips},
		{"TeslaCam/RecentClips", cfg.RecentClips},
		{"TeslaTrackMode", cfg.TrackModeClips},
	}

	for _, d := range dirs {
		if !d.enabled {
			continue
		}
		dirPath := root + "/" + d.path
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			slog.Warn("failed to read directory", "path", dirPath, "error", err)
			continue
		}
		for _, e := range entries {
			files = append(files, d.path+"/"+e.Name())
		}
	}
	return files, nil
}

// buildArchiveBackend creates the appropriate archive backend from config.
func buildArchiveBackend(cfg config.Config) archive.Backend {
	switch cfg.Archive.System {
	case "cifs":
		return archive.NewCIFS(cfg.CIFS)
	case "rsync":
		return archive.NewRsync(cfg.Rsync.Server, cfg.Rsync.User, cfg.Rsync.Path, "")
	case "rclone":
		return archive.NewRclone("teslausb", cfg.Rclone.Path)
	case "nfs":
		return archive.NewNFS(cfg.NFS.Server, cfg.NFS.Share)
	default:
		slog.Warn("unknown archive system, using noop", "system", cfg.Archive.System)
		return &noopBackend{}
	}
}

// buildNotifier creates a multi-notifier from config.
func buildNotifier(cfg config.NotifyConfig) notify.Notifier {
	var notifiers []notify.Notifier
	if cfg.Ntfy.URL != "" {
		notifiers = append(notifiers, notify.NewNtfy(cfg.Ntfy.URL, "default", ""))
	}
	if cfg.Apprise.URL != "" {
		notifiers = append(notifiers, notify.NewApprise(cfg.Apprise.URL))
	}
	if len(notifiers) == 0 {
		return &noopNotifier{}
	}
	return notify.NewMulti(notifiers...)
}

// buildWakeKeeper creates the appropriate Tesla wake keeper from config.
func buildWakeKeeper(cfg config.WakeConfig) tesla.WakeKeeper {
	switch cfg.Method {
	case "ble":
		return tesla.NewBLEWakeKeeper(cfg.BLEVIN, "/usr/bin/tesla-control")
	case "tessie":
		return tesla.NewTessieWakeKeeper(cfg.Tessie.Token, cfg.Tessie.VIN)
	default:
		return tesla.NoopWakeKeeper{}
	}
}

// noopBackend is a no-op archive backend.
type noopBackend struct{}

func (n *noopBackend) Name() string                                   { return "none" }
func (n *noopBackend) IsReachable(_ context.Context) bool             { return true }
func (n *noopBackend) Connect(_ context.Context) error                { return nil }
func (n *noopBackend) Disconnect(_ context.Context) error             { return nil }
func (n *noopBackend) ArchiveFiles(_ context.Context, _ string, _ []string, _ archive.ProgressFunc) error {
	return nil
}
func (n *noopBackend) SyncMedia(_ context.Context, _ string, _ string) error { return nil }

// noopNotifier is a no-op notifier.
type noopNotifier struct{}

func (n *noopNotifier) Send(_ context.Context, _, _ string, _ string) error { return nil }

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("teslausb-neo starting", "version", version)
	startUp := time.Now()

	// Load configuration.
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Warn("config load failed, trying fallback", "error", err)
		cfg, err = config.Load(configFallback)
		if err != nil {
			slog.Warn("fallback config also failed, using defaults", "error", err)
			cfg = config.DefaultConfig()
		}
	}

	// Apply system tuning FIRST (VM params, BFQ, CPU governor).
	sys.ApplyAll(sys.TuningConfig{
		DirtyRatio:              cfg.Tuning.DirtyRatio,
		DirtyBackgroundBytes:    cfg.Tuning.DirtyBackgroundBytes,
		DirtyWritebackCentisecs: cfg.Tuning.DirtyWritebackCentisecs,
		CPUGovernor:             cfg.Tuning.CPUGovernor,
	})
	sys.DisableHDMI()

	// Open SQLite state database.
	db, err := state.Open(dbPath)
	if err != nil {
		slog.Error("failed to open state database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Build components.
	archiver := buildArchiveBackend(cfg)
	notifier := buildNotifier(cfg.Notify)
	wakeKeeper := buildWakeKeeper(cfg.Wake)
	wifiMgr := wifi.NewManager()
	watcher := fswatch.NewWatcher(cfg.Idle.WriteThresholdBytes, cfg.Idle.TimeoutSeconds, 1)
	healthMon := health.NewMonitor(cfg.Health, notifier)

	// Create context for clean shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		slog.Info("received signal", "signal", sig)
		cancel()
	}()

	// Create state machine with all components wired in.
	sm := &StateMachine{
		state:    StateBooting,
		cfg:      cfg,
		archiver: archiver,
		notifier: notifier,
		wake:     wakeKeeper,
		wifiMgr:  wifiMgr,
		watcher:  watcher,
		db:       db,
		startUp:  startUp,
	}

	// Start background goroutines.
	go healthMon.Start(ctx)
	go func() {
		statusCh := make(chan web.StatusInfo, 1)
		srv := web.NewServer(web.Config{ArchiveDir: snapshotMountpoint}, nil, statusCh)
		if err := srv.Start(":80"); err != nil {
			slog.Error("web server failed", "error", err)
		}
	}()

	// Run the main state machine loop.
	if err := sm.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("state machine exited with error", "error", err)
		os.Exit(1)
	}

	slog.Info("teslausb-neo shutdown complete")
}
