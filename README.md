# TeslaUSB Neo

A ground-up rewrite of [TeslaUSB](https://github.com/marcone/teslausb) in Go. A single 10 MB binary turns a Raspberry Pi Zero 2 W into a smart USB drive for Tesla dashcam, music, light shows, and boombox — with automatic WiFi archiving, a web UI, and zero-maintenance filesystem repair.

## What It Does

- Presents up to 4 USB drives to your Tesla (dashcam, music, light show, boombox)
- USB gadget appears in **under 4 seconds** after power-on
- Automatically archives dashcam/sentry clips to your NAS when you park at home
- Plays music without skipping (I/O scheduler tuning prevents write starvation)
- Repairs exFAT corruption on every archive cycle (Tesla cuts power without warning)
- Sends notifications when archiving starts/completes (ntfy, Apprise, or 130+ services)
- Serves a web UI for browsing and downloading recordings
- Provides AP mode for mobile access away from home WiFi
- Monitors temperature, storage, and system health with auto-reboot on hang

## How It Works

```
Tesla ──USB──> Pi Zero 2 W (USB gadget via configfs)
                 ├── LUN 0: /dev/mmcblk0p3 (exFAT) ── Dashcam + Sentry + Track Mode
                 ├── LUN 1: /dev/mmcblk0p4 (exFAT) ── Music
                 ├── LUN 2: /dev/mmcblk0p5 (exFAT) ── Light Shows
                 └── LUN 3: /dev/mmcblk0p6 (exFAT) ── Boombox

When parked at home (WiFi detected):
  1. Wait for Tesla to stop writing (idle detection)
  2. Create dm-snapshot of cam partition (zram COW in RAM, ~100ms)
  3. Run fsck.exfat on live partition
  4. Reconnect USB gadget (Tesla resumes writing immediately)
  5. Mount snapshot read-only, rsync new files to NAS
  6. Release snapshot, send notification
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
| p0 | boot | FAT32 | 50 MB | Kernel, firmware, config.txt |
| p1 | rootfs | ext4 | 200 MB | Read-only root (overlayfs), Go binary, systemd |
| p2 | data | ext4 | 300 MB | SQLite database, logs, teslausb.toml config |
| p3 | cam | exFAT | ~200 GB | Dashcam (TeslaCam/, TeslaTrackMode/) — USB LUN 0 |
| p4 | music | exFAT | ~30 GB | Music (Music/ folder) — USB LUN 1 |
| p5 | lightshow | exFAT | 1 GB | Light shows (LightShow/ folder) — USB LUN 2 |
| p6 | boombox | exFAT | 100 MB | Boombox sounds (Boombox/ folder) — USB LUN 3 |

Partitions p3–p6 are raw block devices presented directly to the Tesla via the Linux USB gadget subsystem. The Pi never mounts them during normal operation — only during archiving (via a read-only dm-snapshot).

## Installation

### Prerequisites

- A computer with an SD card reader
- [Go 1.22+](https://go.dev/dl/) installed (for building from source)
- A Raspberry Pi Zero 2 W
- A microSD card (64 GB+, high-endurance recommended)
- A network share or cloud storage for archiving (CIFS/SMB, NFS, rsync, or rclone)

### Option A: Flash a Pre-Built Image (Recommended)

> Pre-built images will be available on the [Releases](https://github.com/ejaramilla/teslausb-neo/releases) page once the Buildroot image pipeline is set up.

1. Download the latest `teslausb-neo-sdcard.img.xz` from Releases
2. Flash it to your SD card:
   ```bash
   # macOS
   xzcat teslausb-neo-sdcard.img.xz | sudo dd of=/dev/rdiskN bs=4m

   # Linux
   xzcat teslausb-neo-sdcard.img.xz | sudo dd of=/dev/sdX bs=4M status=progress

   # Or use Raspberry Pi Imager / balenaEtcher
   ```
3. Mount the `data` partition (p2) on your computer and create your config file:
   ```bash
   # The data partition should auto-mount. Create the config:
   cp teslausb.example.toml /Volumes/data/teslausb.toml   # macOS
   cp teslausb.example.toml /mnt/data/teslausb.toml       # Linux
   ```
4. Edit `teslausb.toml` (see [Configuration](#configuration) below)
5. Eject the SD card, insert it into the Pi Zero 2 W
6. Connect the Pi to your Tesla's glovebox USB port with a data cable
7. The dashcam icon should appear on the Tesla touchscreen within ~5 seconds

### Option B: Build from Source

#### 1. Clone and Build the Binary

```bash
git clone https://github.com/ejaramilla/teslausb-neo.git
cd teslausb-neo

# Build for Pi Zero 2 W (ARM64)
make binary-arm64

# Output: build/teslausb-linux-arm64 (10 MB, statically linked)
```

#### 2. Prepare the SD Card

Partition your SD card with the layout shown above. On Linux:

```bash
export DEVICE=/dev/sdX   # YOUR SD CARD - double check!

# Create partition table
sudo parted $DEVICE --script mktable msdos

# Create partitions
sudo parted $DEVICE --script mkpart primary fat32 1MiB 51MiB
sudo parted $DEVICE --script set 1 boot on
sudo parted $DEVICE --script mkpart primary ext4 51MiB 251MiB
sudo parted $DEVICE --script mkpart primary ext4 251MiB 551MiB
sudo parted $DEVICE --script mkpart primary 551MiB 200GiB      # cam
sudo parted $DEVICE --script mkpart primary 200GiB 230GiB      # music
# Note: MBR only supports 4 primary. For lightshow + boombox,
# create an extended partition or skip them for now.

# Format partitions
sudo mkfs.vfat -n boot ${DEVICE}1
sudo mkfs.ext4 -L rootfs ${DEVICE}2
sudo mkfs.ext4 -L data ${DEVICE}3
sudo mkfs.exfat -L cam ${DEVICE}4
sudo mkfs.exfat -L music ${DEVICE}5
```

#### 3. Install Raspberry Pi OS Lite (Temporary Base)

Until the Buildroot image is available, you can use Raspberry Pi OS Lite (64-bit, Bookworm) as the base:

1. Flash **Raspberry Pi OS Lite (64-bit)** to the SD card using [Raspberry Pi Imager](https://www.raspberrypi.com/software/)
2. Enable SSH: create an empty file named `ssh` on the boot partition
3. Configure WiFi: create `wpa_supplicant.conf` on the boot partition:
   ```
   country=US
   ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
   update_config=1

   network={
       ssid="YourNetworkName"
       psk="YourPassword"
   }
   ```
4. Boot the Pi, SSH in: `ssh pi@raspberrypi.local` (default password: `raspberry`)
5. Install dependencies:
   ```bash
   sudo apt update
   sudo apt install -y exfatprogs rsync rclone
   ```

#### 4. Install the Binary and Configure

```bash
# Copy binary to the Pi
scp build/teslausb-linux-arm64 pi@raspberrypi.local:/tmp/teslausb

# SSH into the Pi
ssh pi@raspberrypi.local

# Install
sudo mv /tmp/teslausb /usr/bin/teslausb
sudo chmod +x /usr/bin/teslausb

# Create data directory
sudo mkdir -p /data

# Create config
sudo nano /data/teslausb.toml
# (paste your config - see Configuration section below)

# Install systemd service
sudo tee /etc/systemd/system/teslausb.service << 'EOF'
[Unit]
Description=TeslaUSB Neo Daemon
After=local-fs.target
DefaultDependencies=no

[Service]
Type=notify
ExecStart=/usr/bin/teslausb
Restart=always
RestartSec=2
WatchdogSec=30
NotifyAccess=main
ReadWritePaths=/data /sys/kernel/config /sys/class/leds /proc/sys/vm /sys/block
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable teslausb.service

# Load USB gadget modules at boot
echo "dtoverlay=dwc2" | sudo tee -a /boot/firmware/config.txt
echo "modules-load=dwc2,libcomposite" | sudo tee -a /boot/firmware/cmdline.txt

# Reboot
sudo reboot
```

#### 5. Partition and Format the Tesla Drives

After reboot, create the exFAT partitions for Tesla (if not already done):

```bash
# Check your partition layout
lsblk

# Format cam and music partitions as exFAT
sudo mkfs.exfat -L cam /dev/mmcblk0p3
sudo mkfs.exfat -L music /dev/mmcblk0p4

# Create required folder structure on cam partition
sudo mkdir -p /mnt/cam
sudo mount /dev/mmcblk0p3 /mnt/cam
sudo mkdir -p /mnt/cam/TeslaCam/RecentClips
sudo mkdir -p /mnt/cam/TeslaCam/SavedClips
sudo mkdir -p /mnt/cam/TeslaCam/SentryClips
sudo umount /mnt/cam

# Create Music folder on music partition
sudo mkdir -p /mnt/music
sudo mount /dev/mmcblk0p4 /mnt/music
sudo mkdir -p /mnt/music/Music
sudo umount /mnt/music
```

#### 6. Connect to Tesla

Plug the Pi Zero 2 W into the Tesla glovebox USB-A port using a **data cable** (not a charge-only cable). The dashcam icon should appear on the Tesla touchscreen within a few seconds.

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
2. Run: `sudo fsck.exfat -p /dev/sdX3` (cam partition)
3. Run: `sudo fsck.ext4 -fy /dev/sdX2` (data partition)
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
| **Boot to USB** | 45-60 seconds | <4 seconds |
| **Storage** | .bin image files on XFS | Raw exFAT partitions (no file-in-file) |
| **Snapshots** | XFS reflink + loop device | dm-snapshot + zram (no disk I/O) |
| **State tracking** | Flat text files + sort/comm | SQLite WAL database |
| **Web server** | nginx + CGI shell scripts | Embedded Go HTTP server |
| **Notifications** | 12 separate curl implementations | ntfy native + Apprise (130+ services) |
| **OS** | Raspberry Pi OS (900 MB) | Buildroot custom image (~50 MB) |
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
