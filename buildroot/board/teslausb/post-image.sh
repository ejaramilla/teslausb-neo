#!/bin/sh
# Post-image script — generates the final sdcard.img using genimage.

set -eu

BOARD_DIR="$(dirname "$0")"

# Copy our cmdline.txt to the images directory where genimage expects it.
cp "${BOARD_DIR}/cmdline.txt" "${BINARIES_DIR}/cmdline.txt"

# Use Buildroot's genimage wrapper if available, otherwise call genimage directly.
GENIMAGE_CFG="${BOARD_DIR}/genimage.cfg"

support/scripts/genimage.sh -c "${GENIMAGE_CFG}"

echo ""
echo "=== TeslaUSB Neo SD card image built ==="
echo "Output: ${BINARIES_DIR}/sdcard.img"
echo ""
echo "Flash with:"
echo "  xzcat sdcard.img.xz | sudo dd of=/dev/sdX bs=4M status=progress"
echo ""
echo "NOTE: cam/music/lightshow partitions are unformatted."
echo "The teslausb daemon formats them as exFAT on first boot."
