# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TeslaUSB Neo is a complete Go rewrite of [marcone/teslausb](https://github.com/marcone/teslausb). A single 10 MB static binary on a Raspberry Pi Zero 2 W acts as a USB mass storage gadget for Tesla dashcam, music, light shows, and boombox — with automatic WiFi archiving, health monitoring, and a web UI.

## Build Commands

```bash
make binary-arm64    # Cross-compile for Pi Zero 2 W (linux/arm64)
make binary-local    # Native build for development/testing
make test            # Run all tests (33 tests across 6 packages)
make vet             # Run go vet
make clean           # Remove build artifacts
```

Cross-compile target: `GOOS=linux GOARCH=arm64`. Binary is statically linked, ~10 MB stripped.

## Architecture

### Core Design

- **Single Go binary** — replaces 60+ bash scripts. No Python, no nginx, no shell script daemons
- **Raw partition USB gadget** — exFAT partitions (p3-p6) bound directly to USB mass storage LUNs via configfs. No `.bin` image files, no XFS, no loop devices
- **dm-snapshot + zram** — archiving uses device-mapper snapshots with zram-backed COW in RAM. Power loss at any point causes zero on-disk corruption from the snapshot mechanism
- **SQLite WAL state** — crash-safe tracking of archived files, sessions, and health metrics in `/data/teslausb.db`
- **nofua=1** — write caching enabled on all USB LUNs for ~15-20 MB/s throughput (vs ~2 MB/s without)
- **BFQ I/O scheduler** — prevents dashcam writes from starving music reads on the same SD card

### Partition Layout (SD Card)

| Part | FS | Size | Purpose |
|------|----|------|---------|
| p1 | FAT32 | 64 MB | Boot: kernel, firmware, config.txt, cmdline.txt |
| p2 | ext4 | 200 MB | Root filesystem (read-only via overlayfs) |
| p3 | ext4 | 300 MB | Data: SQLite DB, logs, teslausb.toml |
| p5 | exFAT | ~200 GB | Cam partition → USB LUN 0 (TeslaCam/) |
| p6 | exFAT | ~30 GB | Music → USB LUN 1 (Music/) |
| p7 | exFAT | ~1 GB | LightShow → USB LUN 2 (LightShow/) |

p4 is an extended partition container for p5-p7 (MBR 4-primary limit).

### State Machine Flow

```
Boot → GadgetUp → WaitingForWifi → PreArchive (idle detection)
  → Snapshot (dm-snapshot + zram) → Fsck (fsck.exfat -p)
  → Reconnect (gadget back up) → Archiving (rsync from snapshot)
  → MediaSync (gadget down, mount+sync Music/LightShow/Boombox, gadget up)
  → Cleanup (release snapshot) → WaitingForWifi (loop)
```

USB gadget is presented FIRST on boot before any other initialization. Tesla gadget downtime during archive = dm-snapshot setup (~100ms) + fsck time (~5-30s). Media sync causes a second brief gadget downtime to mount and sync media partitions.

### Boot Sequence (target: <4s on Buildroot, ~10s on Pi OS Lite)

1. Kernel loads `dwc2` + `libcomposite` modules via `modules-load=` cmdline param
2. systemd starts `teslausb.service` (Type=notify, After=local-fs.target)
3. Go binary configures USB gadget via configfs, binds raw partitions to LUNs, sets nofua=1, activates UDC
4. Background: WiFi, health monitor, web server start in goroutines

## Package Responsibilities

| Package | Role |
|---------|------|
| `cmd/teslausb` | Entry point, state machine orchestrator, component wiring |
| `internal/config` | TOML config loading via BurntSushi/toml, defaults, validation |
| `internal/gadget` | USB gadget via configfs: create dirs, write descriptors, add LUNs, set nofua, activate/deactivate UDC |
| `internal/sys` | VM tuning (dirty_ratio, dirty_writeback_centisecs), BFQ scheduler, CPU governor, ZRAM setup, LED control, HDMI/BT disable |
| `internal/state` | SQLite WAL database: archived_files, archive_sessions, health_metrics tables |
| `internal/archive` | Backend interface + 4 implementations: CIFS (mount.cifs+rsync), rsync (SSH), rclone, NFS (mount.nfs+rsync). Supports ArchiveFiles (cam→server) and SyncMedia (server→Music/LightShow/Boombox partitions) |
| `internal/snapshot` | dm-snapshot lifecycle: zram alloc, dmsetup create origin+snapshot, mount RO, release, validity check |
| `internal/notify` | Notifier interface + ntfy (HTTP POST), Apprise (REST API), Multi fan-out dispatcher |
| `internal/tesla` | WakeKeeper interface + BLE (tesla-control binary), Tessie (HTTP API), Noop |
| `internal/wifi` | nmcli wrapper: IsConnected, GetSSID, IsHomeNetwork, ConnectToHome, StartAP/StopAP, ScanNetworks |
| `internal/fswatch` | Idle detection: reads /proc/{pid}/io for file-storage kernel thread, state machine UNDETERMINED→WRITING→IDLE |
| `internal/fsutil` | fsck.exfat -p, fstrim, mount/unmount, GetDiskUsage via syscall.Statfs |
| `internal/health` | CPU temp via thermal_zone0, storage usage, systemd watchdog (sd_notify via unix socket), temperature alerts |
| `internal/web` | net/http server, REST API (status/files/download/delete/sync/sessions/health), go:embed static assets, filepath.Rel path validation |

## Key Design Decisions

1. **Gadget-first boot** — USB gadget configured before WiFi or anything else. Configfs writes are ~100ms. Raw partition binding needs no mount.

2. **Raw partitions, not .bin files** — Eliminates XFS host filesystem, loop devices, and file-in-file overhead. Direct block I/O path between Tesla and SD card.

3. **dm-snapshot with zram COW** — Transient (non-persistent) snapshots store COW data in RAM only. Power loss destroys the snapshot harmlessly — the origin partition is always consistent. Rejected alternatives: LVM classic (20-77x write penalty), LVM thin (corrupts on power loss), BTRFS (write amplification on SD cards).

4. **exFAT for Tesla partitions** — Tesla's own formatter uses exFAT. Reliable fsck via exfatprogs >= 1.2.2. ext4 technically better for power loss but Tesla dropped ext4 support once in firmware history.

5. **64 MB zram COW** — Tesla writes sequentially to free space (new blocks, not overwrites). Actual COW captures are minimal (exFAT metadata only). 64 MB sufficient for ~15 min archive window. If COW fills, snapshot invalidates but origin continues serving Tesla normally.

6. **dirty_ratio=10, BFQ, nofua=1** — Three-pronged approach to music skipping. Low dirty_ratio prevents large write stalls. BFQ prevents write starvation of reads. nofua allows write caching for throughput.

7. **ntfy + Apprise** — ntfy native (just HTTP POST, zero dependencies). Apprise REST API covers 130+ notification backends for users who want Telegram/Discord/Pushover/etc.

## CI/CD

Three GitHub Actions workflows:

| Workflow | Trigger | Output |
|----------|---------|--------|
| `ci.yml` | push/PR to master | Tests + ARM64 binary artifact |
| `release.yml` | `git tag v*` | GitHub Release with binary + setup.sh + example config |
| `buildroot.yml` | `git tag v*` + manual | Bootable `sdcard.img.xz` (~250 MB compressed) |

## Two Install Paths

1. **Pi OS Lite** (easy): Flash Pi OS Lite → SCP binary + setup.sh → `sudo bash setup.sh` → edit config → reboot. ~10s boot.
2. **Buildroot** (advanced): Flash `sdcard.img.xz` from Releases → edit config on data partition → plug into Tesla. ~4s boot.

## Testing

Tests cover config loading (87.5%), SQLite CRUD (76.1%), web API + path traversal prevention (42.0%), notification dispatch (83.3%), gadget path construction, and snapshot device paths. All tests run on macOS — Linux-specific packages (gadget, snapshot, sys, health, wifi, fswatch) require Pi hardware or QEMU for integration testing.
