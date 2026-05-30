#!/usr/bin/env python3
"""Runtime kernel-config completeness checker for the TeslaUSB Neo image.

A Buildroot build can succeed and still ship a kernel missing an option the
daemon needs at runtime — e.g. exFAT support, dm-snapshot, zram, or the USB
mass-storage gadget. The image boots, but the very first archive cycle (or
even presenting the drive to the car) fails on real hardware, with nothing in
CI to have caught it. This is the same class of "builds fine, broken on the
Pi" gap that scripts/check_boot_files.py guards for the firmware stage.

This script closes the kernel-config side: it parses the built kernel .config
and asserts every option TeslaUSB Neo depends on is enabled (built-in =y or
module =m). The required set is kept in lock-step with
buildroot/board/teslausb/linux.fragment.

It is dependency-free (stdlib only).

Usage:
    check_kernel_config.py <path-to-.config>
    check_kernel_config.py buildroot-src/output/build/linux-*/.config   # glob ok

Exit code 0 = all required options present, non-zero = something missing.
"""

import glob
import sys

# Option -> short reason, kept in sync with linux.fragment. Each must be
# present as "<SYM>=y" or "<SYM>=m"; "# <SYM> is not set" or absence fails.
REQUIRED = {
    "CONFIG_USB_DWC2": "USB OTG controller driver (gadget mode)",
    "CONFIG_USB_GADGET": "USB gadget framework",
    "CONFIG_USB_LIBCOMPOSITE": "configfs-based composite gadget",
    "CONFIG_USB_CONFIGFS": "configure the gadget via configfs",
    "CONFIG_USB_CONFIGFS_MASS_STORAGE": "mass-storage function (binds Tesla partitions)",
    "CONFIG_EXFAT_FS": "mount/format the Tesla exFAT partitions",
    "CONFIG_BLK_DEV_DM": "device-mapper (snapshot base)",
    "CONFIG_DM_SNAPSHOT": "dm-snapshot for the archive COW mechanism",
    "CONFIG_ZRAM": "RAM-backed COW store for the snapshot",
}


def parse_config(path):
    """Return the set of CONFIG_* symbols that are =y or =m in the file."""
    enabled = set()
    with open(path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if "=" not in line:
                continue
            sym, val = line.split("=", 1)
            if val in ("y", "m"):
                enabled.add(sym)
    return enabled


def main(argv):
    if len(argv) != 2:
        print(__doc__)
        return 2

    # Allow a glob so callers can pass the versioned linux-*/.config path.
    matches = sorted(glob.glob(argv[1]))
    if not matches:
        print(f"ERROR: no kernel .config matched {argv[1]!r}", file=sys.stderr)
        return 2
    path = matches[-1]

    enabled = parse_config(path)
    print(f"Checking {path} ({len(enabled)} options enabled)")

    missing = []
    for sym, reason in REQUIRED.items():
        ok = sym in enabled
        print(f"  [{'OK' if ok else 'MISSING'}] {sym} — {reason}")
        if not ok:
            missing.append(sym)

    if missing:
        print(
            f"\nFAIL: {len(missing)} required kernel option(s) not enabled: "
            + ", ".join(missing)
            + "\nAdd them to buildroot/board/teslausb/linux.fragment.",
            file=sys.stderr,
        )
        return 1

    print(f"\nOK: all {len(REQUIRED)} required kernel options are enabled.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
