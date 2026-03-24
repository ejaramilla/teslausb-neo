#!/bin/sh
# Post-build script — runs after the rootfs is assembled but before the image is created.
# $1 = TARGET_DIR (the root filesystem directory)

set -eu

TARGET_DIR="$1"
BOARD_DIR="$(dirname "$0")"

# Install our custom config.txt and cmdline.txt
cp "$BOARD_DIR/config.txt" "$TARGET_DIR/../images/rpi-firmware/config.txt"
cp "$BOARD_DIR/cmdline.txt" "$TARGET_DIR/../images/cmdline.txt"

# Create mount points
mkdir -p "$TARGET_DIR/data"
mkdir -p "$TARGET_DIR/mnt/snap"

# Add fstab entries for data partition
cat >> "$TARGET_DIR/etc/fstab" << 'EOF'
LABEL=data /data ext4 defaults,noatime 0 2
EOF

# Load dm-snapshot and zram modules at boot
mkdir -p "$TARGET_DIR/etc/modules-load.d"
cat > "$TARGET_DIR/etc/modules-load.d/teslausb.conf" << 'EOF'
dm-snapshot
zram
EOF

# Enable systemd watchdog
if [ -f "$TARGET_DIR/etc/systemd/system.conf" ]; then
    sed -i 's/#RuntimeWatchdogSec=off/RuntimeWatchdogSec=14/' "$TARGET_DIR/etc/systemd/system.conf"
fi

# Create default config if overlay didn't provide one
if [ ! -f "$TARGET_DIR/data/teslausb.toml" ]; then
    mkdir -p "$TARGET_DIR/data"
    cat > "$TARGET_DIR/data/teslausb.toml" << 'TOML'
# TeslaUSB Neo - Edit this file with your settings.
# See: https://github.com/ejaramilla/teslausb-neo

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
fi

echo "TeslaUSB Neo post-build complete"
