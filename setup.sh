#!/bin/bash
# TeslaUSB Neo — One-step setup script
# Run this on a Raspberry Pi Zero 2 W with Raspberry Pi OS Lite (64-bit, Bookworm)
#
# Usage:
#   1. Flash Raspberry Pi OS Lite (64-bit) to your SD card
#   2. Enable SSH and configure WiFi via Raspberry Pi Imager
#   3. Boot the Pi, SSH in
#   4. Copy this script and the teslausb binary to the Pi
#   5. Run: sudo bash setup.sh
#
# The script will:
#   - Install required packages (exfatprogs, rsync, rclone)
#   - Create the exFAT partitions for Tesla (cam, music, lightshow)
#   - Install the teslausb binary and systemd service
#   - Configure USB gadget modules
#   - Optimize boot time (disable unnecessary services)
#   - Enable hardware watchdog
#   - Create a default config if none exists
#   - Reboot

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[teslausb]${NC} $*"; }
warn() { echo -e "${YELLOW}[teslausb]${NC} $*"; }
err()  { echo -e "${RED}[teslausb]${NC} $*" >&2; }

if [ "$(id -u)" -ne 0 ]; then
    err "This script must be run as root (sudo bash setup.sh)"
    exit 1
fi

BOOT_DISK="/dev/mmcblk0"

if [ ! -b "$BOOT_DISK" ]; then
    err "Cannot find SD card at $BOOT_DISK"
    exit 1
fi

# Detect boot partition path (Bookworm uses /boot/firmware, older uses /boot)
if [ -d /boot/firmware ]; then
    BOOT_DIR=/boot/firmware
else
    BOOT_DIR=/boot
fi

log "=== TeslaUSB Neo Setup ==="
log "SD card: $BOOT_DISK"
log "Boot dir: $BOOT_DIR"

# ─── Step 1: Install packages ──────────────────────────
log "Installing required packages..."
apt-get update -qq
apt-get install -y -qq exfatprogs rsync rclone parted

# ─── Step 2: Check for teslausb binary ─────────────────
BINARY_SRC=""
if [ -f /tmp/teslausb ]; then
    BINARY_SRC=/tmp/teslausb
elif [ -f "$(dirname "$0")/teslausb-linux-arm64" ]; then
    BINARY_SRC="$(dirname "$0")/teslausb-linux-arm64"
elif [ -f /usr/bin/teslausb ]; then
    log "teslausb binary already installed"
    BINARY_SRC=""
else
    err "Cannot find teslausb binary."
    err "Please copy it to /tmp/teslausb or place teslausb-linux-arm64 next to this script."
    err "Build it on your Mac with: make binary-arm64"
    exit 1
fi

if [ -n "$BINARY_SRC" ]; then
    log "Installing teslausb binary from $BINARY_SRC"
    cp "$BINARY_SRC" /usr/bin/teslausb
    chmod +x /usr/bin/teslausb
fi

# ─── Step 3: Create data partition and Tesla partitions ─
# Layout (must match cmd/teslausb/main.go partition constants and CLAUDE.md):
#   p1 boot | p2 rootfs | p3 data(ext4,300M) | p4 extended container
#   p5 cam(exFAT) | p6 music(exFAT,30G) | p7 lightshow(exFAT,1G)
#
# parted output is parsed with LC_ALL=C and machine mode (-m) to avoid locale
# decimal separators and column-order fragility. Partition ends use 100% (never
# the raw reported disk size, which parted rounds UP and which therefore lands
# one MiB outside the device).

# Sizes in MiB.
DATA_SIZE=300
MUSIC_SIZE=30720    # 30 GiB
LIGHTSHOW_SIZE=1024 # 1 GiB
CAM_MIN_SIZE=8192   # require at least 8 GiB for the cam partition

# Idempotency: if the full TeslaUSB layout already exists, do nothing. If a
# PARTIAL/foreign layout exists past the rootfs, refuse rather than risk
# reformatting user data.
if [ -b "${BOOT_DISK}p5" ] && [ -b "${BOOT_DISK}p6" ] && [ -b "${BOOT_DISK}p7" ]; then
    log "TeslaUSB partitions (p5/p6/p7) already present. Skipping partitioning."
elif [ -b "${BOOT_DISK}p3" ] || [ -b "${BOOT_DISK}p4" ] || [ -b "${BOOT_DISK}p5" ]; then
    err "Found existing partition(s) past the rootfs but the TeslaUSB layout is incomplete."
    err "Refusing to repartition to avoid destroying data. Remove p3 and higher"
    err "manually (e.g. 'sudo parted $BOOT_DISK') and re-run this script."
    exit 1
else
    log "Creating partitions for TeslaUSB Neo..."

    # Disk size and end of the last existing partition (the rootfs), in MiB.
    DISK_SIZE_MIB=$(LC_ALL=C parted -m -s "$BOOT_DISK" unit MiB print | awk -F: 'NR==2{gsub(/MiB/,"",$2); print int($2)}')
    LAST_END=$(LC_ALL=C parted -m -s "$BOOT_DISK" unit MiB print | awk -F: '/^[0-9]+:/{gsub(/MiB/,"",$3); e=int($3)} END{print e}')

    if [ -z "$DISK_SIZE_MIB" ] || [ -z "$LAST_END" ]; then
        err "Could not parse disk geometry from parted. Aborting."
        exit 1
    fi

    # Lay partitions out from the end so the cam partition absorbs the
    # remaining space: [ data ][ cam .......... ][ music ][ lightshow ]
    DATA_START=$((LAST_END + 1))
    DATA_END=$((DATA_START + DATA_SIZE))
    EXTENDED_START=$((DATA_END + 1))

    LIGHTSHOW_START=$((DISK_SIZE_MIB - LIGHTSHOW_SIZE))
    MUSIC_START=$((LIGHTSHOW_START - MUSIC_SIZE))
    CAM_START=$((EXTENDED_START + 1))
    CAM_END=$((MUSIC_START - 1))
    CAM_SIZE=$((CAM_END - CAM_START))

    # Guard: not enough room. The usual cause is Raspberry Pi OS having
    # auto-expanded the root filesystem to fill the whole card on first boot.
    if [ "$CAM_SIZE" -lt "$CAM_MIN_SIZE" ]; then
        err "Not enough free space after the rootfs to create the Tesla partitions."
        err "Free space after p2: $((DISK_SIZE_MIB - LAST_END)) MiB; need >= $((DATA_SIZE + MUSIC_SIZE + LIGHTSHOW_SIZE + CAM_MIN_SIZE)) MiB."
        err ""
        err "Raspberry Pi OS expands the root filesystem to fill the SD card on"
        err "first boot. Shrink it first, e.g.:"
        err "  sudo systemctl disable --now ... ; sudo raspi-config (Advanced > Expand) is the OPPOSITE;"
        err "  boot a PC, open the card in GParted, and shrink partition 2 to ~8 GiB,"
        err "  then re-run this script."
        exit 1
    fi

    log "  p3: data     ${DATA_START}-${DATA_END} MiB (${DATA_SIZE} MiB, ext4)"
    log "  p4: extended ${EXTENDED_START} MiB - 100%"
    log "  p5: cam      ${CAM_START}-${CAM_END} MiB ($((CAM_SIZE / 1024)) GiB, exFAT)"
    log "  p6: music    ${MUSIC_START}-$((LIGHTSHOW_START - 1)) MiB ($((MUSIC_SIZE / 1024)) GiB, exFAT)"
    log "  p7: lightshow ${LIGHTSHOW_START} MiB - 100% ($((LIGHTSHOW_SIZE / 1024)) GiB, exFAT)"

    warn "This will create new partitions on $BOOT_DISK."
    warn "Existing partitions (boot, rootfs) will NOT be modified."
    echo ""
    read -r -p "Continue? (y/N) " CONFIRM
    if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
        err "Aborted by user."
        exit 1
    fi

    # Create partitions. The extended container and the last logical end at
    # 100% so we never run past the device.
    parted -s "$BOOT_DISK" mkpart primary ext4 "${DATA_START}MiB" "${DATA_END}MiB"
    parted -s "$BOOT_DISK" mkpart extended "${EXTENDED_START}MiB" 100%
    parted -s "$BOOT_DISK" mkpart logical "${CAM_START}MiB" "${CAM_END}MiB"
    parted -s "$BOOT_DISK" mkpart logical "${MUSIC_START}MiB" "$((LIGHTSHOW_START - 1))MiB"
    parted -s "$BOOT_DISK" mkpart logical "${LIGHTSHOW_START}MiB" 100%

    # Wait for the kernel + udev to create the new device nodes (a fixed sleep
    # races on slow cards).
    partprobe "$BOOT_DISK"
    udevadm settle || true
    for n in 3 5 6 7; do
        for _ in $(seq 1 50); do
            [ -b "${BOOT_DISK}p${n}" ] && break
            sleep 0.2
        done
        if [ ! -b "${BOOT_DISK}p${n}" ]; then
            err "Partition ${BOOT_DISK}p${n} did not appear after partitioning. Aborting."
            exit 1
        fi
    done

    # Format. Each partition was just created empty, but guard against
    # formatting anything that unexpectedly already holds a filesystem.
    format_if_empty() {
        local dev="$1" type="$2" label="$3"
        if blkid "$dev" >/dev/null 2>&1; then
            warn "$dev already has a filesystem; leaving it untouched."
            return
        fi
        log "Formatting $label ($type)..."
        if [ "$type" = "ext4" ]; then
            mkfs.ext4 -F -L "$label" "$dev"
        else
            mkfs.exfat -L "$label" "$dev"
        fi
    }
    format_if_empty "${BOOT_DISK}p3" ext4 data
    format_if_empty "${BOOT_DISK}p5" exfat cam
    format_if_empty "${BOOT_DISK}p6" exfat music
    format_if_empty "${BOOT_DISK}p7" exfat lightshow

    log "Partitions created and formatted."
fi

# ─── Step 4: Mount and set up data partition ───────────
mkdir -p /data
if ! mountpoint -q /data; then
    mount "${BOOT_DISK}p3" /data 2>/dev/null || mount LABEL=data /data
fi

# Add to fstab if not already there
if ! grep -q "LABEL=data" /etc/fstab; then
    echo "LABEL=data /data ext4 defaults,noatime 0 2" >> /etc/fstab
fi

# ─── Step 5: Create Tesla folder structure ─────────────
log "Creating Tesla folder structure on cam partition..."
mkdir -p /mnt/cam
mount "${BOOT_DISK}p5" /mnt/cam 2>/dev/null || mount LABEL=cam /mnt/cam || true
if mountpoint -q /mnt/cam; then
    mkdir -p /mnt/cam/TeslaCam/RecentClips
    mkdir -p /mnt/cam/TeslaCam/SavedClips
    mkdir -p /mnt/cam/TeslaCam/SentryClips
    umount /mnt/cam
    log "Tesla folder structure created."
else
    warn "Could not mount cam partition to create folders. You may need to create TeslaCam/ manually."
fi

log "Creating Music folder on music partition..."
mkdir -p /mnt/music
mount "${BOOT_DISK}p6" /mnt/music 2>/dev/null || mount LABEL=music /mnt/music || true
if mountpoint -q /mnt/music; then
    mkdir -p /mnt/music/Music
    umount /mnt/music
fi

log "Creating LightShow folder on lightshow partition..."
mkdir -p /mnt/lightshow
mount "${BOOT_DISK}p7" /mnt/lightshow 2>/dev/null || mount LABEL=lightshow /mnt/lightshow || true
if mountpoint -q /mnt/lightshow; then
    mkdir -p /mnt/lightshow/LightShow
    umount /mnt/lightshow
fi

# ─── Step 6: Create snapshot mountpoint ────────────────
mkdir -p /mnt/snap

# ─── Step 7: Install systemd service ──────────────────
log "Installing systemd service..."
cat > /etc/systemd/system/teslausb.service << 'EOF'
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
ReadWritePaths=/data /mnt /sys/kernel/config /sys/class/leds /proc/sys/vm /sys/block /sys/class/zram-control /dev/mapper
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable teslausb.service

# ─── Step 8: Configure USB gadget modules ─────────────
log "Configuring USB gadget kernel modules..."

# Add a single [all]-scoped TeslaUSB block to config.txt. The explicit [all]
# filter is essential: appending bare lines would inherit whatever conditional
# filter the stock Bookworm config.txt ends in (e.g. [cm5]) and not apply to a
# Pi Zero 2 W. For the same reason we do NOT `grep dtoverlay=dwc2` to decide
# whether dwc2 is present — the stock config.txt has a `[cm5] dtoverlay=dwc2,
# dr_mode=host` line that would false-positive and leave the gadget disabled.
if ! grep -q "# TeslaUSB Neo settings" "$BOOT_DIR/config.txt"; then
    log "Adding TeslaUSB settings to config.txt..."
    cat >> "$BOOT_DIR/config.txt" << 'BOOTCFG'

[all]
# TeslaUSB Neo settings
dtoverlay=dwc2,dr_mode=otg
hdmi_blanking=2
dtparam=audio=off
gpu_mem=16
dtparam=watchdog=on
BOOTCFG
fi

# cmdline.txt is a SINGLE line. Edit only the first line; merge into an
# existing modules-load= (a second modules-load= is ignored by the kernel).
CMDLINE_FILE="$BOOT_DIR/cmdline.txt"
CMDLINE=$(head -n1 "$CMDLINE_FILE")
case "$CMDLINE" in
    *modules-load=*dwc2*)   ;;
    *modules-load=*)        CMDLINE=$(printf '%s' "$CMDLINE" | sed 's/modules-load=/modules-load=dwc2,libcomposite,/') ;;
    *)                      CMDLINE="$CMDLINE modules-load=dwc2,libcomposite" ;;
esac
case " $CMDLINE " in *" quiet "*) ;; *) CMDLINE="$CMDLINE quiet" ;; esac
printf '%s\n' "$CMDLINE" > "$CMDLINE_FILE"

# ─── Step 9: Optimize boot time ──────────────────────
log "Optimizing boot time..."

# Disable unnecessary services
systemctl disable apt-daily.timer 2>/dev/null || true
systemctl disable apt-daily-upgrade.timer 2>/dev/null || true
systemctl disable man-db.timer 2>/dev/null || true
systemctl disable triggerhappy.service 2>/dev/null || true

# (HDMI/audio/gpu_mem/watchdog overlay settings are written once to config.txt
# under the [all] block in Step 8.)

# ─── Step 10: Enable hardware watchdog ────────────────
log "Enabling hardware watchdog..."

# Configure systemd watchdog
if ! grep -q "RuntimeWatchdogSec" /etc/systemd/system.conf; then
    sed -i 's/#RuntimeWatchdogSec=off/RuntimeWatchdogSec=14/' /etc/systemd/system.conf
fi

# ─── Step 11: Create default config if needed ─────────
if [ ! -f /data/teslausb.toml ]; then
    log "Creating default configuration at /data/teslausb.toml"
    cat > /data/teslausb.toml << 'TOML'
# TeslaUSB Neo Configuration
# Edit this file with your settings, then reboot.

[wifi]
home_ssid = "YourNetworkName"
# Leave home_password blank if you configured WiFi in Raspberry Pi Imager;
# otherwise set it and the daemon will create the connection on boot.
home_password = ""

[archive]
system = "cifs"

[cifs]
server = "192.168.1.100"
share = "TeslaCam"
user = "tesla"
password = "changeme"
TOML
    warn "IMPORTANT: Edit /data/teslausb.toml with your WiFi and archive settings!"
fi

# ─── Step 12: Load dm-mod for snapshots ───────────────
if ! grep -q "dm-snapshot" /etc/modules-load.d/*.conf 2>/dev/null; then
    echo "dm-snapshot" > /etc/modules-load.d/teslausb.conf
    echo "zram" >> /etc/modules-load.d/teslausb.conf
fi

# ─── Done ─────────────────────────────────────────────
log ""
log "=== Setup Complete ==="
log ""
log "Before rebooting, edit your config:"
log "  sudo nano /data/teslausb.toml"
log ""
log "Then reboot:"
log "  sudo reboot"
log ""
log "After reboot:"
log "  - The dashcam icon should appear on your Tesla touchscreen"
log "  - Web UI available at http://$(hostname).local"
log "  - Logs: sudo journalctl -u teslausb -f"
log ""
warn "Remember to edit /data/teslausb.toml with your WiFi SSID and archive server!"
