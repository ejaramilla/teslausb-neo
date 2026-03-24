# TeslaUSB Neo

[![CI](https://github.com/ejaramilla/teslausb-neo/actions/workflows/ci.yml/badge.svg)](https://github.com/ejaramilla/teslausb-neo/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ejaramilla/teslausb-neo)](https://github.com/ejaramilla/teslausb-neo/releases/latest)

A ground-up rewrite of [TeslaUSB](https://github.com/marcone/teslausb) in Go. A single 10 MB binary turns a Raspberry Pi Zero 2 W into a smart USB drive for Tesla dashcam, music, light shows, and boombox — with automatic WiFi archiving, a web UI, and zero-maintenance filesystem repair.

## What It Does

- Presents up to 4 USB drives to your Tesla (dashcam, music, light show, boombox)
- USB gadget appears in **under 4 seconds** after power-on
- Automatically archives dashcam/sentry clips to your NAS when you park at home
- Automatically syncs music, light shows, and boombox sounds from your NAS
- Plays music without skipping (I/O scheduler tuning prevents write starvation)
- Repairs exFAT corruption on every archive cycle (Tesla cuts power without warning)
- Sends notifications when archiving starts/completes (ntfy, Apprise, or 130+ services)
- Serves a web UI for browsing and downloading recordings
- Provides AP mode for mobile access away from home WiFi
- Monitors temperature, storage, and system health with auto-reboot on hang

## How It Works

```
Tesla ──USB──> Pi Zero 2 W (USB gadget via configfs)
                 ├── LUN 0: /dev/mmcblk0p5 (exFAT) ── Dashcam + Sentry + Track Mode
                 ├── LUN 1: /dev/mmcblk0p6 (exFAT) ── Music
                 └── LUN 2: /dev/mmcblk0p7 (exFAT) ── Light Shows

When parked at home (WiFi detected):
  1. Wait for Tesla to stop writing (idle detection)
  2. Create dm-snapshot of cam partition (zram COW in RAM, ~100ms)
  3. Run fsck.exfat on live partition
  4. Reconnect USB gadget (Tesla resumes writing immediately)
  5. Mount snapshot read-only, rsync new files to NAS
  6. Sync media: mirror Music/, LightShow/, Boombox/ from NAS to USB
  7. Release snapshot, send notification
```

Raw partitions are passed directly to the USB gadget as block devices — no `.bin` image files, no XFS, no loop devices. Snapshots use Linux device-mapper with a zram-backed copy-on-write device stored entirely in RAM, so a power loss at any point causes zero on-disk corruption from the snapshot mechanism.

## Hardware Requirements

| Component | Requirement |
|-----------|-------------|
| **Board** | Raspberry Pi Zero 2 W (quad-core ARM64, 512 MB RAM) |
| **SD Card** | 64 GB minimum, 256 GB recommended. Use a high-endurance card (Samsung PRO Endurance or SanDisk High Endurance) |
| **Cable** | USB-A to Micro-USB (data, not charge-only) |
| **WiFi** | 2.4 GHz network in range of your parking spot |

The Pi Zero 2 W draws ~100 mA idle — well within the Tesla glovebox USB port's power budget.

## SD Card Layout

| Partition | Label | Filesystem | Size | Purpose |
|-----------|-------|------------|------|---------|
| p1 | boot | FAT32 | 64 MB | Kernel, firmware, config.txt, cmdline.txt |
| p2 | rootfs | ext4 | 200 MB | Read-only root (overlayfs), Go binary, systemd |
| p3 | data | ext4 | 300 MB | SQLite database, logs, teslausb.toml config |
| p4 | (extended) | — | — | MBR extended partition container |
| p5 | cam | exFAT | ~200 GB | Dashcam (TeslaCam/, TeslaTrackMode/) — USB LUN 0 |
| p6 | music | exFAT | ~30 GB | Music (Music/ folder) — USB LUN 1 |
| p7 | lightshow | exFAT | ~1 GB | Light shows (LightShow/ folder) — USB LUN 2 |

MBR only allows 4 primary partitions, so p4 is an extended partition containing the Tesla exFAT partitions as logical volumes. The Pi never mounts p5–p7 during normal operation — they are raw block devices passed directly to the USB gadget. They are only mounted during archiving (via a read-only dm-snapshot on the cam partition).

## Installation

There are two install paths:
- **Easy path (recommended):** Flash Raspberry Pi OS Lite, run the setup script. Works today, ~10 second boot.
- **Advanced path:** Flash a custom Buildroot image from the Releases page. Minimal OS, ~4 second boot. Built automatically by GitHub Actions.

### What You Need

- A Raspberry Pi Zero 2 W
- A microSD card (64 GB minimum, 256 GB recommended). Use a **high-endurance** card: Samsung PRO Endurance or SanDisk High Endurance
- A USB-A to Micro-USB **data** cable (not a charge-only cable)
- A computer with an SD card reader (Mac, Windows, or Linux)
- A network share for archiving: CIFS/SMB (Synology, Windows share), NFS, rsync server, or rclone remote
- 2.4 GHz WiFi network in range of where you park

### Step 1: Flash Raspberry Pi OS Lite

1. Download and install [Raspberry Pi Imager](https://www.raspberrypi.com/software/) on your computer
2. Insert your SD card
3. In Raspberry Pi Imager:
   - **Device**: Raspberry Pi Zero 2 W
   - **OS**: Raspberry Pi OS Lite (64-bit, Bookworm) — under "Raspberry Pi OS (other)"
   - **Storage**: Your SD card
4. Click the **gear icon** (or "Edit Settings") before writing:
   - **Set hostname**: `teslausb`
   - **Enable SSH**: Yes, use password authentication
   - **Set username/password**: `pi` / your chosen password
   - **Configure WiFi**: Enter your home WiFi SSID and password, select your country
5. Click **Write** and wait for it to finish

### Step 2: Get the Binary

**Option A — Download from Releases (no build tools needed):**

Go to [Releases](https://github.com/ejaramilla/teslausb-neo/releases/latest) and download `teslausb-linux-arm64` and `setup.sh`.

**Option B — Build from source:**

```bash
# Install Go if you don't have it: https://go.dev/dl/
# macOS: brew install go

git clone https://github.com/ejaramilla/teslausb-neo.git
cd teslausb-neo
make binary-arm64

# Output: build/teslausb-linux-arm64 (10 MB)
```

### Step 3: Boot the Pi and Run Setup

1. Insert the SD card into the Pi Zero 2 W
2. Connect the Pi to **any USB power source** (not the Tesla yet — use a phone charger or computer USB port)
3. Wait ~30 seconds for first boot, then SSH in:
   ```bash
   ssh pi@teslausb.local
   ```
4. Copy the binary and setup script to the Pi (from a second terminal on your Mac):
   ```bash
   scp build/teslausb-linux-arm64 pi@teslausb.local:/tmp/teslausb
   scp setup.sh pi@teslausb.local:/tmp/setup.sh
   ```
5. Back in the Pi SSH session, run the setup script:
   ```bash
   sudo bash /tmp/setup.sh
   ```
   The script will:
   - Install required packages (exfatprogs, rsync, rclone)
   - Create partitions (data, cam, music, lightshow) on the SD card
   - Format them as exFAT with the Tesla folder structure (TeslaCam/, Music/, LightShow/)
   - Install the teslausb binary and systemd service
   - Configure USB gadget kernel modules (dwc2, libcomposite)
   - Optimize boot (disable HDMI, audio, unnecessary services)
   - Enable hardware watchdog
   - Create a default config at `/data/teslausb.toml`

### Step 4: Edit Your Config

```bash
sudo nano /data/teslausb.toml
```

At minimum, set your WiFi SSID and archive server. See [Configuration](#configuration) for examples.

**CIFS/SMB example** (Synology, Windows share, etc.):
```toml
[wifi]
home_ssid = "MyHomeNetwork"

[archive]
system = "cifs"

[cifs]
server = "192.168.1.100"
share = "TeslaCam"
user = "tesla"
password = "your_password"
```

### Step 5: Reboot and Install in Car

```bash
sudo reboot
```

After reboot, unplug the Pi from the power source and connect it to your **Tesla's glovebox USB-A port** with a data cable. The dashcam icon should appear on the Tesla touchscreen within ~10 seconds.

### Step 6: Verify

From your home WiFi (while the car is parked):
```bash
# Check the web UI
open http://teslausb.local       # macOS
# or visit http://teslausb.local in a browser

# Check logs via SSH
ssh pi@teslausb.local
sudo journalctl -u teslausb -f

# You should see:
#   teslausb-neo starting
#   USB gadget presented at X.X seconds uptime
#   waiting for wifi
#   connected to home wifi
#   archiving started...
```

### Alternative: Flash the Buildroot Image (Advanced)

A pre-built minimal Buildroot image is available on the [Releases page](https://github.com/ejaramilla/teslausb-neo/releases). This skips Steps 1-3 entirely:

1. Download `sdcard.img.xz` from the [latest release](https://github.com/ejaramilla/teslausb-neo/releases/latest)
2. Flash it:
   ```bash
   # macOS
   xzcat sdcard.img.xz | sudo dd of=/dev/rdiskN bs=4m

   # Linux
   xzcat sdcard.img.xz | sudo dd of=/dev/sdX bs=4M status=progress
   ```
3. Mount the `data` partition on your computer, edit `teslausb.toml` with your WiFi and archive settings
4. Eject, insert into Pi, plug into Tesla

The Buildroot image is a minimal ~50 MB Linux with only what TeslaUSB Neo needs — no Raspberry Pi OS, no apt, no desktop. It boots to USB gadget in ~4 seconds. The image is built automatically by GitHub Actions on every release tag.

> To trigger a new image build manually: go to Actions > "Build SD Card Image" > "Run workflow" on GitHub.

### Adding Music, Light Shows, and Boombox Sounds

**Automatic sync (recommended):** Place files on your archive server and enable sync in the config. Files are synced from your NAS every archive cycle:

```toml
[archive]
sync_music = true       # Sync Music/ folder to music partition
sync_lightshow = true   # Sync LightShow/ folder to lightshow partition
sync_boombox = true     # Sync Boombox/ folder to boombox partition
```

On your archive server, create these folders alongside your TeslaCam archive:
- `Music/` — MP3, FLAC, WAV, OGG files (subfolders for artist/album OK)
- `LightShow/` — paired `.fseq` + `.mp3`/`.wav` files (filenames must match, e.g. `show.fseq` + `show.wav`). Audio must be 44.1 kHz sample rate.
- `Boombox/` — `.mp3` or `.wav` custom horn sounds (max 5 selectable, alphabetical order). Filenames: alphanumeric, dashes, underscores only — no spaces.

The sync mirrors these folders to the USB partitions with `--delete`, so removing a file from the server removes it from the car.

**Manual copy (alternative):**

1. SSH into the Pi: `ssh pi@teslausb.local`
2. Stop the teslausb service: `sudo systemctl stop teslausb`
3. Mount the partition: `sudo mount /dev/mmcblk0p6 /mnt/music`
4. Copy your files: `scp -r ~/Music/* pi@teslausb.local:/mnt/music/Music/`
5. Unmount and restart: `sudo umount /mnt/music && sudo systemctl start teslausb`

Supported music formats: MP3, FLAC, WAV, OGG. Tesla reads ID3 metadata tags.

**Sources:** [Tesla Light Show repo](https://github.com/teslamotors/light-show), [Tesla Owner's Manual — Boombox](https://www.tesla.com/ownersmanual/models/en_us/GUID-79A49D40-A028-435B-A7F6-8E48846AB9E9.html)

## Configuration

Create `/data/teslausb.toml` with your settings. Only include sections you need — all values have sensible defaults.

### Minimal Config (CIFS/SMB Archive)

```toml
[wifi]
home_ssid = "MyHomeNetwork"

[archive]
system = "cifs"

[cifs]
server = "192.168.1.100"
share = "TeslaCam"
user = "tesla"
password = "your_password"
```

### Minimal Config (rsync over SSH)

```toml
[wifi]
home_ssid = "MyHomeNetwork"

[archive]
system = "rsync"

[rsync]
server = "192.168.1.100"
user = "tesla"
path = "/mnt/nas/TeslaCam"
```

### Minimal Config (rclone)

```toml
[wifi]
home_ssid = "MyHomeNetwork"

[archive]
system = "rclone"

[rclone]
# Configure rclone remote first: rclone config
# Then reference it here:
path = "gdrive:TeslaCam"
```

### Minimal Config (NFS)

```toml
[wifi]
home_ssid = "MyHomeNetwork"

[archive]
system = "nfs"

[nfs]
server = "192.168.1.100"
share = "/exports/teslacam"
```

### Full Config Reference

```toml
# ─── WiFi ───────────────────────────────────────────────
[wifi]
home_ssid = "MyHomeNetwork"

[wifi.ap]
enabled = false          # Enable AP hotspot when away from home
ssid = "teslausb"        # AP network name
password = "teslausb"    # AP password (min 8 chars)

# ─── Archive ────────────────────────────────────────────
[archive]
system = "cifs"          # cifs, rsync, rclone, nfs
delay_seconds = 60       # Wait this long after idle before archiving
saved_clips = true       # Archive SavedClips (manual saves, honk)
sentry_clips = true      # Archive SentryClips
recent_clips = false     # Archive RecentClips (rolling buffer)
track_mode_clips = false # Archive TeslaTrackMode telemetry
sync_music = false       # Mirror Music/ from server to music partition
sync_lightshow = false   # Mirror LightShow/ from server to lightshow partition
sync_boombox = false     # Mirror Boombox/ from server to boombox partition

[cifs]
server = "192.168.1.100"
share = "TeslaCam"
user = "tesla"
password = "secret"
path = "TeslaCam"       # Subfolder on the share

[rsync]
server = "192.168.1.100"
user = "tesla"
path = "/mnt/nas/TeslaCam"

[rclone]
path = "remote:TeslaCam"

[nfs]
server = "192.168.1.100"
share = "/exports/teslacam"
path = "TeslaCam"

# ─── Notifications ──────────────────────────────────────
[notify.ntfy]
url = "https://ntfy.sh/my-teslausb"   # ntfy server URL + topic

[notify.apprise]
url = "http://localhost:8000/notify"   # Apprise REST API (optional)

# ─── Tesla Wake ─────────────────────────────────────────
[wake]
method = "none"          # none, ble, tessie
ble_vin = ""             # Your Tesla VIN (for BLE wake)

[wake.tessie]
token = ""               # Tessie API token
vin = ""                 # Tesla VIN

# ─── Health Monitoring ──────────────────────────────────
[health]
temp_warning_mc = 80000  # Warning at 80C (millicelsius)
temp_caution_mc = 70000  # Caution at 70C
interval_seconds = 60    # Check interval

# ─── Idle Detection ────────────────────────────────────
[idle]
write_threshold_bytes = 1024  # Below this = idle
timeout_seconds = 300         # Max wait for idle
snapshot_interval_seconds = 5 # Snapshot frequency

# ─── System Tuning ──────────────────────────────────────
[tuning]
dirty_ratio = 10              # % of RAM for dirty pages (low = less music skip)
dirty_background_bytes = 65536
dirty_writeback_centisecs = 25 # Flush interval (250ms)
cpu_governor = "conservative"  # Normal operation
cpu_governor_archiving = "ondemand"  # During archive transfers

# ─── Gadget Sizes ───────────────────────────────────────
# These are only used during initial SD card setup.
# After partitioning, sizes are fixed by the partition table.
[gadget]
cam_size = "200G"
music_size = "30G"
lightshow_size = "1G"
boombox_size = "100M"
```

## Web UI

The built-in web server runs on port 80. Access it at:

```
http://teslausb.local
```

### REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/status` | System state, uptime, temperature |
| GET | `/api/v1/files?path=TeslaCam/SavedClips` | List files in a directory |
| GET | `/api/v1/files/download?path=TeslaCam/SavedClips/2025-01-15_12-30/front.mp4` | Download a file (supports byte-range for video streaming) |
| DELETE | `/api/v1/files?path=...` | Delete a file or directory |
| POST | `/api/v1/sync` | Trigger an archive cycle immediately |
| GET | `/api/v1/archive/sessions` | Archive history (from SQLite) |
| GET | `/api/v1/health` | Health check |

All file paths are validated against directory traversal attacks.

## Troubleshooting

### Tesla shows "Dashcam Unavailable"

- Verify the cable is a **data cable**, not charge-only. Try a known-good cable.
- Check that `dwc2` and `libcomposite` modules are loaded: `lsmod | grep dwc2`
- Check gadget status: `ls /sys/kernel/config/usb_gadget/teslausb/UDC`
- Check the service: `sudo journalctl -u teslausb -f`

### Music skips or stutters

The system tunes the I/O scheduler (BFQ) and dirty page ratio to minimize read/write contention. If skipping persists:
- Verify BFQ is active: `cat /sys/block/mmcblk0/queue/scheduler` (should show `[bfq]`)
- Try a high-endurance SD card (Samsung PRO Endurance, SanDisk High Endurance) — consumer SD cards have unpredictable garbage collection stalls
- Check CPU governor: `cat /sys/devices/system/cpu/cpufreq/policy0/scaling_governor`

### Archiving not working

- Check WiFi: `nmcli device wifi list` — is your home SSID visible?
- Check archive logs: `sudo journalctl -u teslausb | grep -i archive`
- Test reachability manually: `ping <your-nas-ip>`
- For CIFS: test mount manually: `sudo mount.cifs //server/share /mnt/test -o user=tesla,password=secret`

### SD card corruption after power loss

This is expected and handled automatically. The system runs `fsck.exfat -p` on the cam partition during every archive cycle. The root filesystem is read-only (overlayfs) and immune to corruption. The data partition (SQLite) uses WAL mode for crash safety.

If the system won't boot:
1. Remove the SD card and mount it on another computer
2. Run: `sudo fsck.exfat -p /dev/sdX5` (cam partition, p5)
3. Run: `sudo fsck.ext4 -fy /dev/sdX3` (data partition, p3)
4. Reinsert and boot

### Checking logs

```bash
# Live service logs
sudo journalctl -u teslausb -f

# Boot timing (how fast the gadget appeared)
sudo journalctl -u teslausb | grep "gadget presented"

# Archive history
sudo journalctl -u teslausb | grep -i "archiv"
```

## Development

### Building

```bash
make binary-arm64    # Cross-compile for Pi Zero 2 W
make binary-local    # Build for your machine (development)
make test            # Run all tests (33 tests, 6 packages)
make vet             # Run go vet
make clean           # Remove build artifacts
```

### Testing

```bash
# Run all tests with race detector
go test -race ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Project Structure

```
teslausb-neo/
├── cmd/teslausb/main.go        # Entry point + state machine
├── internal/
│   ├── archive/                # CIFS, rsync, rclone, NFS backends
│   ├── config/                 # TOML config loading + defaults
│   ├── fsutil/                 # fsck.exfat, fstrim, mount helpers
│   ├── fswatch/                # USB gadget idle detection
│   ├── gadget/                 # USB configfs gadget management
│   ├── health/                 # Temperature, storage, watchdog
│   ├── notify/                 # ntfy + Apprise notifications
│   ├── snapshot/               # dm-snapshot + zram COW
│   ├── state/                  # SQLite state database
│   ├── sys/                    # VM tuning, BFQ, CPU governor, LED
│   ├── tesla/                  # BLE + Tessie wake APIs
│   ├── web/                    # Embedded HTTP server + REST API
│   └── wifi/                   # nmcli WiFi/AP management
├── web/                        # Frontend assets (embedded in binary)
├── buildroot/                  # Buildroot external tree for custom image
├── Makefile
└── go.mod
```

## How It Differs from Original TeslaUSB

| | Original TeslaUSB | TeslaUSB Neo |
|---|---|---|
| **Language** | 60+ bash scripts | Single Go binary (10 MB) |
| **Boot to USB** | 45-60 seconds | ~10s (Pi OS Lite) / ~4s (Buildroot) |
| **Storage** | .bin image files on XFS | Raw exFAT partitions (no file-in-file) |
| **Snapshots** | XFS reflink + loop device | dm-snapshot + zram (no disk I/O) |
| **State tracking** | Flat text files + sort/comm | SQLite WAL database |
| **Web server** | nginx + CGI shell scripts | Embedded Go HTTP server |
| **Notifications** | 12 separate curl implementations | ntfy native + Apprise (130+ services) |
| **OS** | Raspberry Pi OS (900 MB) | Pi OS Lite (430 MB) or Buildroot (~50 MB) |
| **Root filesystem** | Manual read-only hacks | overlayfs (built-in) |
| **Music skipping** | Common (I/O contention) | Mitigated (BFQ + nofua + dirty_ratio tuning) |
| **Security** | CGI path traversal vulnerabilities | filepath.Rel validation on all paths |
| **Watchdog** | None | Hardware BCM2835 + systemd WatchdogSec |

## License

MIT

## Acknowledgments

- [marcone/teslausb](https://github.com/marcone/teslausb) — the original project that proved a Pi could be a Tesla dashcam drive
- [KittenLabs](https://kittenlabs.de/blog/2024/09/01/extreme-pi-boot-optimization/) — Pi Zero 2 W boot optimization research
- [Linux kernel dm-snapshot](https://docs.kernel.org/admin-guide/device-mapper/snapshot.html) — the snapshot mechanism that makes safe archiving possible
- [ntfy.sh](https://ntfy.sh/) — simple, self-hostable push notifications
