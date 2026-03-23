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
#   - Create the exFAT partitions for Tesla (cam, music, lightshow, boombox)
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
# Check what partitions already exist
LAST_PART=$(lsblk -rno NAME "$BOOT_DISK" | tail -1)
LAST_PART_NUM=${LAST_PART: -1}

if [ "$LAST_PART_NUM" -ge 6 ]; then
    log "Partitions already exist (found p$LAST_PART_NUM). Skipping partitioning."
else
    log "Creating partitions for TeslaUSB Neo..."

    # Get the end of the last existing partition
    LAST_END=$(parted -s "$BOOT_DISK" unit MiB print | grep "^ " | tail -1 | awk '{print $3}' | tr -d 'MiB')

    # Calculate partition layout
    # p3: data (300 MB, ext4)
    # p4 is extended partition (container for p5-p7)
    # p5: cam (remainder minus music/lightshow/boombox)
    # p6: music (30 GB)
    # p7: lightshow + boombox (1.1 GB)
    #
    # Note: MBR only allows 4 primary partitions. Pi OS uses p1 (boot) and p2 (rootfs).
    # We need p3 (data) as primary, then an extended partition for the rest.

    DISK_SIZE_MIB=$(parted -s "$BOOT_DISK" unit MiB print | grep "Disk $BOOT_DISK" | awk '{print $3}' | tr -d 'MiB')

    DATA_START=$((LAST_END + 1))
    DATA_END=$((DATA_START + 300))

    EXTENDED_START=$((DATA_END + 1))
    EXTENDED_END=$((DISK_SIZE_MIB))

    # Music, lightshow, boombox at the end of disk
    BOOMBOX_SIZE=100    # MiB
    LIGHTSHOW_SIZE=1024 # MiB
    MUSIC_SIZE=30720    # MiB (30 GB)

    BOOMBOX_END=$((DISK_SIZE_MIB))
    BOOMBOX_START=$((BOOMBOX_END - BOOMBOX_SIZE))
    LIGHTSHOW_END=$((BOOMBOX_START))
    LIGHTSHOW_START=$((LIGHTSHOW_END - LIGHTSHOW_SIZE))
    MUSIC_END=$((LIGHTSHOW_START))
    MUSIC_START=$((MUSIC_END - MUSIC_SIZE))

    CAM_START=$((EXTENDED_START + 1))
    CAM_END=$((MUSIC_START))

    log "  p3: data     ${DATA_START}-${DATA_END} MiB (300 MiB, ext4)"
    log "  p4: extended ${EXTENDED_START}-${EXTENDED_END} MiB"
    log "  p5: cam      ${CAM_START}-${CAM_END} MiB ($(( (CAM_END - CAM_START) / 1024 )) GiB, exFAT)"
    log "  p6: music    ${MUSIC_START}-${MUSIC_END} MiB (30 GiB, exFAT)"
    log "  p7: liteshw  ${LIGHTSHOW_START}-${LIGHTSHOW_END} MiB (1 GiB, exFAT)"
    # Boombox shares p7 or we skip it to stay within MBR limits

    warn "This will create new partitions on $BOOT_DISK."
    warn "Existing partitions (boot, rootfs) will NOT be modified."
    echo ""
    read -r -p "Continue? (y/N) " CONFIRM
    if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
        err "Aborted by user."
        exit 1
    fi

    # Create partitions
    parted -s "$BOOT_DISK" mkpart primary ext4 "${DATA_START}MiB" "${DATA_END}MiB"
    parted -s "$BOOT_DISK" mkpart extended "${EXTENDED_START}MiB" "${EXTENDED_END}MiB"
    parted -s "$BOOT_DISK" mkpart logical "${CAM_START}MiB" "${CAM_END}MiB"
    parted -s "$BOOT_DISK" mkpart logical "${MUSIC_START}MiB" "${MUSIC_END}MiB"
    parted -s "$BOOT_DISK" mkpart logical "${LIGHTSHOW_START}MiB" "${LIGHTSHOW_END}MiB"

    # Wait for kernel to recognize new partitions
    partprobe "$BOOT_DISK"
    sleep 2

    # Format partitions
    log "Formatting data partition (ext4)..."
    mkfs.ext4 -F -L data "${BOOT_DISK}p3"

    log "Formatting cam partition (exFAT)..."
    mkfs.exfat -L cam "${BOOT_DISK}p5"

    log "Formatting music partition (exFAT)..."
    mkfs.exfat -L music "${BOOT_DISK}p6"

    log "Formatting lightshow partition (exFAT)..."
    mkfs.exfat -L lightshow "${BOOT_DISK}p7"

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

# Add dwc2 overlay if not present
if ! grep -q "dtoverlay=dwc2" "$BOOT_DIR/config.txt"; then
    echo "dtoverlay=dwc2" >> "$BOOT_DIR/config.txt"
fi

# Add modules-load to cmdline.txt if not present
CMDLINE_FILE="$BOOT_DIR/cmdline.txt"
if ! grep -q "modules-load=dwc2,libcomposite" "$CMDLINE_FILE"; then
    sed -i 's/$/ modules-load=dwc2,libcomposite/' "$CMDLINE_FILE"
fi

# ─── Step 9: Optimize boot time ──────────────────────
log "Optimizing boot time..."

# Disable unnecessary services
systemctl disable apt-daily.timer 2>/dev/null || true
systemctl disable apt-daily-upgrade.timer 2>/dev/null || true
systemctl disable man-db.timer 2>/dev/null || true
systemctl disable triggerhappy.service 2>/dev/null || true

# Disable HDMI in config.txt (saves ~25mA and boot time)
if ! grep -q "hdmi_blanking=2" "$BOOT_DIR/config.txt"; then
    cat >> "$BOOT_DIR/config.txt" << 'BOOTCFG'

# TeslaUSB Neo optimizations
hdmi_blanking=2
dtparam=audio=off
gpu_mem=16
BOOTCFG
fi

# Add quiet to kernel cmdline for faster boot
if ! grep -q " quiet" "$CMDLINE_FILE"; then
    sed -i 's/$/ quiet/' "$CMDLINE_FILE"
fi

# ─── Step 10: Enable hardware watchdog ────────────────
log "Enabling hardware watchdog..."
if ! grep -q "dtparam=watchdog=on" "$BOOT_DIR/config.txt"; then
    echo "dtparam=watchdog=on" >> "$BOOT_DIR/config.txt"
fi

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

[archive]
system = "cifs"

[cifs]
server = "192.168.1.100"
share = "TeslaCam"
user = "tesla"
password = "changeme"
TOML
    warn "IMPORTANT: Edit /data/teslausb.toml with your WiFi SSID and archive settings!"
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
