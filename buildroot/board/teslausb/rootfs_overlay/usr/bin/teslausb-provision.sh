#!/bin/sh
# TeslaUSB Neo — first-boot partition provisioning (Buildroot image).
#
# The flashed image contains only p1(boot)/p2(rootfs)/p3(data). The Tesla
# exFAT partitions do not exist yet, so the daemon would have no storage to
# present. This runs once on first boot to create and format them, matching
# the layout in cmd/teslausb/main.go and setup.sh:
#
#   p3 data(ext4) | p4 extended | p5 cam | p6 music(30G) | p7 lightshow(1G)
#
# Idempotent: if p5/p6/p7 already exist it does nothing (the systemd unit also
# guards on ConditionPathExists=!/dev/mmcblk0p5).
set -eu

DISK="/dev/mmcblk0"

log() { echo "[provision] $*"; }

if [ ! -b "$DISK" ]; then
    log "no $DISK; nothing to do"
    exit 0
fi

if [ -b "${DISK}p5" ] && [ -b "${DISK}p6" ] && [ -b "${DISK}p7" ]; then
    log "Tesla partitions already present; nothing to do"
    exit 0
fi

# Sizes in MiB (keep in sync with setup.sh).
MUSIC_SIZE=30720
LIGHTSHOW_SIZE=1024
CAM_MIN_SIZE=8192

DISK_SIZE_MIB=$(LC_ALL=C parted -m -s "$DISK" unit MiB print | awk -F: 'NR==2{gsub(/MiB/,"",$2); print int($2)}')
LAST_END=$(LC_ALL=C parted -m -s "$DISK" unit MiB print | awk -F: '/^[0-9]+:/{gsub(/MiB/,"",$3); e=int($3)} END{print e}')

if [ -z "$DISK_SIZE_MIB" ] || [ -z "$LAST_END" ]; then
    log "could not parse disk geometry; aborting"
    exit 1
fi

EXTENDED_START=$((LAST_END + 1))
LIGHTSHOW_START=$((DISK_SIZE_MIB - LIGHTSHOW_SIZE))
MUSIC_START=$((LIGHTSHOW_START - MUSIC_SIZE))
CAM_START=$((EXTENDED_START + 1))
CAM_END=$((MUSIC_START - 1))
CAM_SIZE=$((CAM_END - CAM_START))

if [ "$CAM_SIZE" -lt "$CAM_MIN_SIZE" ]; then
    log "SD card too small: only $((DISK_SIZE_MIB - LAST_END)) MiB free after rootfs/data"
    exit 1
fi

log "creating p4(extended) p5(cam ${CAM_SIZE}MiB) p6(music) p7(lightshow)"
parted -s "$DISK" mkpart extended "${EXTENDED_START}MiB" 100%
parted -s "$DISK" mkpart logical "${CAM_START}MiB" "${CAM_END}MiB"
parted -s "$DISK" mkpart logical "${MUSIC_START}MiB" "$((LIGHTSHOW_START - 1))MiB"
parted -s "$DISK" mkpart logical "${LIGHTSHOW_START}MiB" 100%

partprobe "$DISK" || true
udevadm settle || true
for n in 5 6 7; do
    i=0
    while [ ! -b "${DISK}p${n}" ] && [ "$i" -lt 50 ]; do
        sleep 0.2
        i=$((i + 1))
    done
    if [ ! -b "${DISK}p${n}" ]; then
        log "partition ${DISK}p${n} did not appear; aborting"
        exit 1
    fi
done

# Format each Tesla partition as a PARTITIONED disk: an MBR with a single
# exFAT partition starting at LBA 2048. The Tesla requires this layout — its
# own "Format USB Drive" produces exactly it, and a bare exFAT written directly
# to the device is not recognized (the car offers to reformat and may grab the
# wrong drive). The exFAT is created on an offset loop so its PartitionOffset is
# consistent with a standalone drive (matches upstream teslausb's image layout).
PART_OFFSET=1048576   # 2048 sectors * 512; keep in sync with the sfdisk start

format_tesla_drive() {
    dev="$1"; label="$2"; make_teslacam="$3"
    log "partitioning + formatting $dev ($label)"
    printf 'label: dos\nstart=2048, type=07\n' | sfdisk -q --wipe always "$dev"
    sync
    udevadm settle 2>/dev/null || true
    loop=$(losetup -o "$PART_OFFSET" --find --show "$dev")
    mkfs.exfat -L "$label" "$loop"
    if [ "$make_teslacam" = "1" ]; then
        mkdir -p /mnt/provision
        if mount -t exfat "$loop" /mnt/provision; then
            mkdir -p /mnt/provision/TeslaCam/RecentClips \
                     /mnt/provision/TeslaCam/SavedClips \
                     /mnt/provision/TeslaCam/SentryClips
            umount /mnt/provision || umount -l /mnt/provision || true
        fi
        rmdir /mnt/provision 2>/dev/null || true
    fi
    losetup -d "$loop"
}

format_tesla_drive "${DISK}p5" cam 1
format_tesla_drive "${DISK}p6" music 0
format_tesla_drive "${DISK}p7" lightshow 0

log "provisioning complete"
