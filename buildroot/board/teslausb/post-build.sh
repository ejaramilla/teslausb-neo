#!/bin/sh
# Post-build script — runs after rootfs is assembled, before image creation.
# $1 = TARGET_DIR (the root filesystem directory)

set -eu

TARGET_DIR="$1"

# Create mount points
mkdir -p "${TARGET_DIR}/data"
mkdir -p "${TARGET_DIR}/mnt/snap"
mkdir -p "${TARGET_DIR}/mnt/cam"
mkdir -p "${TARGET_DIR}/mnt/music"
mkdir -p "${TARGET_DIR}/mnt/lightshow"

# Add fstab entry for data partition
if ! grep -q "LABEL=data" "${TARGET_DIR}/etc/fstab" 2>/dev/null; then
    echo "LABEL=data /data ext4 defaults,noatime 0 2" >> "${TARGET_DIR}/etc/fstab"
fi

# Load dm-snapshot and zram modules at boot
mkdir -p "${TARGET_DIR}/etc/modules-load.d"
cat > "${TARGET_DIR}/etc/modules-load.d/teslausb.conf" << 'EOF'
dm-snapshot
zram
EOF

# Enable systemd watchdog
if [ -f "${TARGET_DIR}/etc/systemd/system.conf" ]; then
    sed -i 's/#RuntimeWatchdogSec=off/RuntimeWatchdogSec=14/' \
        "${TARGET_DIR}/etc/systemd/system.conf"
fi

# Enable NTP. The Pi Zero 2 W has no RTC, so without this the clock boots at
# the epoch and breaks TLS to notification services. Enabling via a symlink
# (rather than relying on systemd presets) makes it deterministic. The unit
# lives in /usr/lib/systemd/system on Buildroot.
if [ -f "${TARGET_DIR}/usr/lib/systemd/system/systemd-timesyncd.service" ]; then
    mkdir -p "${TARGET_DIR}/etc/systemd/system/sysinit.target.wants"
    ln -sf ../../../../usr/lib/systemd/system/systemd-timesyncd.service \
        "${TARGET_DIR}/etc/systemd/system/sysinit.target.wants/systemd-timesyncd.service"
else
    echo "WARNING: systemd-timesyncd.service not found; NTP will not be enabled" >&2
fi

# Set up serial console on tty1 (standard Buildroot Pi pattern)
if [ -e "${TARGET_DIR}/etc/inittab" ]; then
    grep -qE '^tty1::' "${TARGET_DIR}/etc/inittab" || \
        sed -i '/GENERIC_SERIAL/a tty1::respawn:/sbin/getty -L tty1 0 vt100' \
            "${TARGET_DIR}/etc/inittab"
fi

echo "TeslaUSB Neo post-build complete"
