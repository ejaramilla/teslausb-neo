#!/bin/sh
# Post-image script — generates the final sdcard.img using genimage.
# This runs after all filesystem images are built.

set -eu

BOARD_DIR="$(dirname "$0")"
GENIMAGE_CFG="$BOARD_DIR/genimage.cfg"
GENIMAGE_TMP="$(mktemp -d)"

# Build the boot partition file list for genimage.
# Enumerate all DTB files and firmware files from the images directory.
BOOT_FILES=""
for f in "${BINARIES_DIR}"/*.dtb "${BINARIES_DIR}"/rpi-firmware/*; do
    [ -e "$f" ] || continue
    base="$(basename "$f")"
    case "$f" in
        */rpi-firmware/*)
            BOOT_FILES="${BOOT_FILES}\"rpi-firmware/${base}\","
            ;;
        *)
            BOOT_FILES="${BOOT_FILES}\"${base}\","
            ;;
    esac
done

# Add kernel image
KERNEL_NAME="zImage"
if [ -f "${BINARIES_DIR}/Image" ]; then
    KERNEL_NAME="Image"
fi
BOOT_FILES="${BOOT_FILES}\"${KERNEL_NAME}\","

# Add cmdline.txt
BOOT_FILES="${BOOT_FILES}\"cmdline.txt\""

# Run genimage
genimage \
    --rootpath "${TARGET_DIR}" \
    --tmppath "${GENIMAGE_TMP}" \
    --inputpath "${BINARIES_DIR}" \
    --outputpath "${BINARIES_DIR}" \
    --config "${GENIMAGE_CFG}"

rm -rf "${GENIMAGE_TMP}"

echo ""
echo "=== TeslaUSB Neo SD card image built ==="
echo "Output: ${BINARIES_DIR}/sdcard.img"
echo ""
echo "Flash with:"
echo "  dd if=${BINARIES_DIR}/sdcard.img of=/dev/sdX bs=4M status=progress"
echo ""
echo "IMPORTANT: After flashing, the cam/music/lightshow partitions are"
echo "unformatted. On first boot, the teslausb daemon will format them"
echo "as exFAT and create the Tesla folder structure automatically."
