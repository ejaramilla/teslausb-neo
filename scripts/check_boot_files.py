#!/usr/bin/env python3
"""Static boot-file completeness checker for the TeslaUSB Neo SD image.

The Raspberry Pi boot ROM and GPU firmware (bootcode.bin -> start*.elf) run
*before* any CPU/Linux emulator gets involved, so a missing or mis-selected
firmware file produces a board that "compiles fine but never boots" with no
diagnostic output. A CPU emulator like QEMU cannot catch this because it skips
the firmware stage entirely and boots the kernel directly.

This script closes that gap: it parses the boot partition's config.txt (and
cmdline.txt), works out exactly which files the firmware will try to load, and
verifies every one of them is actually present on the boot partition.

It is deliberately dependency-free (stdlib only) so it runs anywhere.

Usage:
    check_boot_files.py <boot_dir> [--expect-dtb bcm2710-rpi-zero-2-w.dtb]

<boot_dir> is a directory containing the extracted contents of the FAT boot
partition (config.txt, start.elf, zImage, overlays/, ...). The QEMU harness
extracts this for you with mtools; you can also point it at a mounted boot
partition.

Exit code 0 = all required boot files present, non-zero = something missing.
"""

import argparse
import os
import sys


def parse_config_txt(path):
    """Return a dict of key->value for simple `key=value` lines and a list of
    (key, value) for repeatable directives like dtoverlay."""
    scalars = {}
    overlays = []
    if not os.path.isfile(path):
        return scalars, overlays
    with open(path, "r", errors="replace") as fh:
        for raw in fh:
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            if "=" not in line:
                continue
            key, _, value = line.partition("=")
            key = key.strip()
            value = value.strip()
            if key == "dtoverlay":
                # dtoverlay=name[,param=val,...] -- we only need the overlay name.
                overlays.append(value.split(",")[0])
            else:
                scalars[key] = value
    return scalars, overlays


def required_firmware_files(scalars):
    """Determine the start_file/fixup_file the firmware will load.

    Mirrors the Pi firmware's selection logic: if start_file/fixup_file are set
    explicitly they win; otherwise the variant is inferred from gpu_mem
    (gpu_mem<=16 -> the cut-down start_cd.elf/fixup_cd.dat) and start_x.
    """
    if "start_file" in scalars or "fixup_file" in scalars:
        start = scalars.get("start_file", "start.elf")
        fixup = scalars.get("fixup_file", "fixup.dat")
        return start, fixup, "explicit start_file/fixup_file"

    # No explicit selection -> firmware auto-selects.
    if scalars.get("start_x") == "1":
        return "start_x.elf", "fixup_x.dat", "start_x=1"

    gpu_mem = scalars.get("gpu_mem")
    try:
        gpu_mem_val = int(gpu_mem) if gpu_mem is not None else None
    except ValueError:
        gpu_mem_val = None

    if gpu_mem_val is not None and gpu_mem_val <= 16:
        # THIS is the trap: with gpu_mem=16 and no explicit start_file, the
        # firmware loads the cut-down variant which Buildroot's VARIANT_PI does
        # not install.
        return "start_cd.elf", "fixup_cd.dat", "gpu_mem=%d (<=16) auto-selects cut-down firmware" % gpu_mem_val

    return "start.elf", "fixup.dat", "default"


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("boot_dir", help="directory with extracted boot-partition files")
    ap.add_argument("--expect-dtb", default="bcm2710-rpi-zero-2-w.dtb",
                    help="board DTB that must be present (default: Pi Zero 2 W)")
    args = ap.parse_args()

    boot = args.boot_dir
    if not os.path.isdir(boot):
        print("ERROR: boot dir %r does not exist" % boot, file=sys.stderr)
        return 2

    present = set()
    for root, _dirs, files in os.walk(boot):
        for f in files:
            rel = os.path.relpath(os.path.join(root, f), boot)
            present.add(rel.replace(os.sep, "/"))

    scalars, overlays = parse_config_txt(os.path.join(boot, "config.txt"))

    required = []  # (path, reason)
    required.append(("bootcode.bin", "first-stage GPU bootloader (Pi 0-3 load it from SD)"))

    start, fixup, why = required_firmware_files(scalars)
    required.append((start, "GPU firmware (%s)" % why))
    required.append((fixup, "GPU firmware linker (%s)" % why))

    kernel = scalars.get("kernel", "kernel.img")
    required.append((kernel, "kernel= in config.txt"))

    required.append((args.expect_dtb, "board device tree (auto-loaded by firmware)"))

    for ov in overlays:
        required.append(("overlays/%s.dtbo" % ov, "dtoverlay=%s in config.txt" % ov))
    if overlays:
        required.append(("overlays/overlay_map.dtb",
                         "overlay name map (needed to resolve dtoverlay=)"))

    # cmdline.txt should exist and name the kernel root device.
    cmdline_path = os.path.join(boot, "cmdline.txt")
    required.append(("cmdline.txt", "kernel command line"))

    print("== Boot partition: %s ==" % boot)
    print("Firmware selection: start_file=%s fixup_file=%s (%s)\n" % (start, fixup, why))

    missing = []
    for path, reason in required:
        ok = path in present
        print("  [%s] %-28s  %s" % ("OK" if ok else "MISSING", path, reason))
        if not ok:
            missing.append((path, reason))

    # Sanity-check the kernel cmdline.
    if os.path.isfile(cmdline_path):
        with open(cmdline_path, errors="replace") as fh:
            cmdline = fh.read().strip()
        print("\ncmdline.txt: %s" % cmdline)
        if "root=" not in cmdline:
            print("  WARNING: cmdline.txt has no root= -- kernel won't find rootfs")
        if "console=" not in cmdline:
            print("  WARNING: cmdline.txt has no console= -- boot failures will be silent")

    if missing:
        print("\nFAIL: %d required boot file(s) missing:" % len(missing))
        for path, reason in missing:
            print("  - %s  (%s)" % (path, reason))
        print("\nThis is exactly the failure mode that produces a Pi that does "
              "not boot with no output.")
        return 1

    print("\nPASS: all firmware/kernel files the boot chain needs are present.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
