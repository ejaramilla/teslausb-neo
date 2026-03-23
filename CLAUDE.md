# TeslaUSB Neo

TeslaUSB Neo is a ground-up Go rewrite of TeslaUSB. It runs as a single static binary on a Buildroot-based Linux image for Raspberry Pi Zero 2 W, acting as a USB mass storage gadget for Tesla dashcam/sentry footage with automatic archiving.

## Build Commands

```bash
make binary-arm64    # Cross-compile for Pi Zero 2 W (linux/arm64)
make binary-local    # Native build for development
make test            # Run all tests
make vet             # Run go vet
make clean           # Remove build artifacts
```

## Architecture

- **Single Go binary** -- no shell scripts, no Python, no runtime dependencies
- **Raw partition USB gadget** -- configfs mass_storage gadget binds /dev/mmcblk0pN directly to LUNs; no filesystem mount on the Pi side during gadget mode
- **dm-snapshot + zram** -- copy-on-write snapshots allow safe archiving while the car continues writing; zram backs the COW device to avoid SD wear
- **SQLite state DB** -- tracks archive sessions, file hashes, and operational state in /data/teslausb.db
- **Buildroot image** -- minimal Linux (~50 MB rootfs) built with Buildroot; boots to USB gadget in under 3 seconds
- **Partition layout** -- boot (50M), rootfs (200M, ext4, read-only), data (300M, ext4), cam (exFAT), music (exFAT), lightshow (exFAT), boombox (exFAT)

## Package Responsibilities

| Package | Role |
|---------|------|
| `cmd/teslausb` | Entry point, state machine, signal handling |
| `internal/config` | TOML configuration loading and validation |
| `internal/gadget` | USB gadget setup via configfs (create, bind LUNs, activate UDC) |
| `internal/sys` | System tuning (VM params, BFQ scheduler, CPU governor, LED control) |
| `internal/state` | SQLite database for sessions, file tracking, and operational state |
| `internal/archive` | Archive orchestration with pluggable backends (rclone, cifs, ssh) |
| `internal/snapshot` | dm-snapshot creation/teardown with zram COW backing |
| `internal/notify` | Push notifications (Pushover, Telegram, webhooks) |
| `internal/tesla` | Tesla Owner API integration for sleep/wake detection |
| `internal/wifi` | WiFi management via wpa_supplicant/networkd |
| `internal/fswatch` | inotify-based file watcher for new dashcam clips |
| `internal/fsutil` | Filesystem utilities (exFAT creation, fsck, mount helpers) |
| `internal/health` | Health monitoring (temperature, disk space, SD health via mmc-utils) |
| `internal/web` | Embedded HTTP server with REST API and static web UI |

## Key Design Decisions

1. **Gadget-first boot order** -- The USB gadget is configured via configfs before wifi or any other subsystem, ensuring the car sees the drive within seconds of power-on. This prevents Tesla from flagging the USB device as faulty.

2. **Raw partition binding** -- LUNs bind directly to block devices (`/dev/mmcblk0p4`) rather than file-backed images. This eliminates a layer of indirection and improves I/O throughput.

3. **dm-snapshot for safe archiving** -- Instead of unmounting the gadget to archive, a device-mapper snapshot captures a point-in-time view of the partition. The car continues writing to the origin device while the archive reads from the frozen snapshot.

4. **zram COW device** -- The snapshot's copy-on-write store lives in compressed RAM (zram) rather than on the SD card, reducing write amplification and extending SD card lifespan.

5. **Single binary, no scripts** -- All logic lives in compiled Go. This eliminates shell script fragility, enables static type checking, and produces a single deployable artifact.

6. **Buildroot over Raspbian** -- A minimal Buildroot image boots faster, uses less storage, and has a smaller attack surface than a full Raspbian installation. The rootfs is mounted read-only in production.

7. **SQLite for state** -- Lightweight, single-file database avoids external dependencies while providing ACID guarantees for tracking archive progress across power cycles.

8. **systemd notify integration** -- The service uses Type=notify with a watchdog, allowing systemd to restart the daemon if it becomes unresponsive.
