#!/usr/bin/env bash
#
# Emulate a Raspberry Pi and boot the TeslaUSB Neo SD image under QEMU.
#
# What this proves (and what it can't):
#   * QEMU's raspi machine loads the kernel directly (-kernel/-dtb); it does
#     NOT run the Pi's GPU firmware (bootcode.bin -> start.elf). So this test
#     validates the *kernel + device tree + rootfs + systemd + teslausb*
#     boot path -- it cannot catch a missing/mis-selected firmware file.
#   * The firmware stage is validated separately and statically by
#     scripts/check_boot_files.py, which this harness runs first.
#
# Usage:
#   scripts/qemu_boot_test.sh path/to/sdcard.img[.xz]
#
# Env overrides:
#   QEMU_MACHINE   (default: raspi2b)   QEMU machine model
#   QEMU_BIN       (default: qemu-system-arm)
#   BOOT_TIMEOUT   (default: 120)       seconds to wait for a success marker
#   EXPECT_DTB     (default: bcm2710-rpi-zero-2-w.dtb)
#   SKIP_QEMU      (default: 0)         if 1, run only the static boot-file check
#
set -euo pipefail

IMG_IN="${1:?usage: qemu_boot_test.sh path/to/sdcard.img[.xz]}"
QEMU_MACHINE="${QEMU_MACHINE:-raspi2b}"
QEMU_BIN="${QEMU_BIN:-qemu-system-arm}"
BOOT_TIMEOUT="${BOOT_TIMEOUT:-120}"
EXPECT_DTB="${EXPECT_DTB:-bcm2710-rpi-zero-2-w.dtb}"
SKIP_QEMU="${SKIP_QEMU:-0}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"; [ -n "${QEMU_PID:-}" ] && kill "$QEMU_PID" 2>/dev/null || true' EXIT

log()  { printf '\033[1;36m[boot-test]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[boot-test] FAIL:\033[0m %s\n' "$*" >&2; exit 1; }

command -v mcopy   >/dev/null || fail "mtools (mcopy) not found -- install 'mtools'"
command -v python3 >/dev/null || fail "python3 not found"

# --- 1. Decompress if needed -------------------------------------------------
IMG="$WORK/sdcard.img"
case "$IMG_IN" in
  *.xz) log "Decompressing $IMG_IN"; xz -dc "$IMG_IN" > "$IMG" ;;
  *)    cp "$IMG_IN" "$IMG" ;;
esac

# --- 2. Parse the MBR to find the boot (p1) and rootfs (p2) partitions -------
# Each MBR entry is 16 bytes starting at offset 446; bytes 8-11 = start LBA,
# bytes 12-15 = sector count (little-endian). Sector size = 512.
read_mbr() {
  python3 - "$IMG" <<'PY'
import struct, sys
with open(sys.argv[1], "rb") as f:
    mbr = f.read(512)
if mbr[510:512] != b"\x55\xaa":
    sys.exit("no valid MBR signature -- not a partitioned disk image")
for i in range(4):
    e = mbr[446 + i*16 : 446 + (i+1)*16]
    ptype = e[4]
    start = struct.unpack("<I", e[8:12])[0]
    count = struct.unpack("<I", e[12:16])[0]
    if ptype != 0 and count != 0:
        print(f"{i+1} {ptype:#04x} {start} {count}")
PY
}

log "Partition table:"
read_mbr | while read -r idx ptype start count; do
  printf '    p%s  type=%s  start=%s sectors  size=%s MiB\n' \
    "$idx" "$ptype" "$start" "$(( count / 2048 ))"
done

BOOT_START="$(read_mbr | awk '$1==1 {print $3}')"
[ -n "$BOOT_START" ] || fail "could not locate boot partition (p1) in MBR"
BOOT_OFFSET=$(( BOOT_START * 512 ))
log "Boot partition starts at byte offset $BOOT_OFFSET"

# --- 3. Extract the boot partition contents with mtools (no mount/root) ------
BOOTDIR="$WORK/boot"
mkdir -p "$BOOTDIR"
# Recursively copy the whole FAT filesystem out of the image at its offset.
mcopy -s -n -i "${IMG}@@${BOOT_OFFSET}" "::/*" "$BOOTDIR/" 2>/dev/null || \
  fail "mcopy could not read the FAT boot partition at offset $BOOT_OFFSET"
log "Extracted boot files:"
( cd "$BOOTDIR" && find . -type f | sed 's/^/    /' | sort )

# --- 4. STATIC firmware-completeness check (catches the real boot bug) -------
log "Running static boot-file completeness check..."
python3 "$SCRIPT_DIR/check_boot_files.py" "$BOOTDIR" --expect-dtb "$EXPECT_DTB" \
  || fail "boot-file check failed -- the firmware would not be able to boot"

if [ "$SKIP_QEMU" = "1" ]; then
  log "SKIP_QEMU=1 -- static check passed, skipping dynamic QEMU boot."
  exit 0
fi

# --- 5. Locate kernel, dtb, cmdline for the direct QEMU boot -----------------
KERNEL="$BOOTDIR/zImage"
[ -f "$KERNEL" ] || KERNEL="$(find "$BOOTDIR" -maxdepth 1 -name 'zImage' -o -name 'kernel*.img' | head -1)"
[ -f "$KERNEL" ] || fail "no kernel (zImage) found in boot partition"
DTB="$BOOTDIR/$EXPECT_DTB"
[ -f "$DTB" ] || fail "expected dtb $EXPECT_DTB not found in boot partition"
CMDLINE="$(cat "$BOOTDIR/cmdline.txt")"
# Force serial console so QEMU output is captured regardless of image defaults.
case "$CMDLINE" in
  *console=ttyAMA0*) ;;
  *) CMDLINE="$CMDLINE console=ttyAMA0,115200" ;;
esac

command -v "$QEMU_BIN" >/dev/null || fail "$QEMU_BIN not found -- install qemu"

# --- 6. QEMU's SD card must be a power-of-two size; pad a working copy --------
QIMG="$WORK/qemu-sd.img"
cp "$IMG" "$QIMG"
SIZE=$(python3 -c "import os,sys;print(os.path.getsize(sys.argv[1]))" "$QIMG")
POW=1
while [ "$POW" -lt "$SIZE" ]; do POW=$(( POW * 2 )); done
if [ "$POW" -ne "$SIZE" ]; then
  log "Padding SD image $SIZE -> $POW bytes (QEMU requires power-of-two SD size)"
  python3 -c "import sys;f=open(sys.argv[1],'r+b');f.truncate(int(sys.argv[2]))" "$QIMG" "$POW"
fi

# --- 7. Boot under QEMU, capture serial, watch for success/failure markers ---
SERIAL="$WORK/serial.log"
log "Booting under $QEMU_BIN -M $QEMU_MACHINE (timeout ${BOOT_TIMEOUT}s)"
log "  cmdline: $CMDLINE"

"$QEMU_BIN" \
  -M "$QEMU_MACHINE" \
  -kernel "$KERNEL" \
  -dtb "$DTB" \
  -append "$CMDLINE" \
  -drive "file=${QIMG},if=sd,format=raw" \
  -no-reboot \
  -nographic \
  -serial "file:$SERIAL" \
  >/dev/null 2>&1 &
QEMU_PID=$!

SUCCESS_RE='Welcome to|TeslaUSB Neo|teslausb\.service|Reached target.*[Mm]ulti|login:|Starting TeslaUSB'
FAILURE_RE='Kernel panic|Unable to mount root|VFS: Cannot open root|Attempted to kill init|---\[ end Kernel panic'

deadline=$(( $(date +%s) + BOOT_TIMEOUT ))
result="timeout"
while [ "$(date +%s)" -lt "$deadline" ]; do
  if ! kill -0 "$QEMU_PID" 2>/dev/null; then result="exited"; break; fi
  if [ -f "$SERIAL" ] && grep -Eq "$SUCCESS_RE" "$SERIAL"; then result="success"; break; fi
  if [ -f "$SERIAL" ] && grep -Eq "$FAILURE_RE" "$SERIAL"; then result="panic"; break; fi
  sleep 2
done
kill "$QEMU_PID" 2>/dev/null || true
wait "$QEMU_PID" 2>/dev/null || true
QEMU_PID=""

echo "---------------- last 40 lines of serial log ----------------"
tail -n 40 "$SERIAL" 2>/dev/null || echo "(no serial output captured)"
echo "-------------------------------------------------------------"

case "$result" in
  success) log "PASS: kernel + rootfs booted to userspace under QEMU." ;;
  panic)   fail "kernel panic / rootfs failure during QEMU boot (see log above)" ;;
  *)       fail "no userspace boot marker within ${BOOT_TIMEOUT}s (result=$result). See serial log above." ;;
esac
