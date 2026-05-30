package fsutil

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Each Tesla LUN is a partitioned disk: an MBR with a single exFAT partition
// starting at LBA 2048. The Tesla requires this layout (its own "Format USB
// Drive" produces exactly it); a bare exFAT filesystem written directly to the
// device is not recognized and the car offers to reformat. So the gadget binds
// the whole raw partition (the host sees [MBR + exFAT]), but the daemon reaches
// the inner exFAT through a single offset loop device — no kpartx, no host
// filesystem. See the validated design notes in CLAUDE.md.

// mbrSignatureOffset and friends index the classic MBR layout.
const (
	mbrSignatureOffset    = 510 // 0x1FE: 0x55 0xAA
	mbrPart1EntryOffset   = 446 // 0x1BE: first partition entry
	mbrPartStartLBAOffset = 8   // within an entry: 4-byte LE start LBA
)

// PartitionOffset returns the byte offset of the first MBR partition on device.
// It reads the device's first sector and parses the partition table rather than
// assuming a fixed alignment, so it stays correct regardless of how the disk
// was partitioned.
func PartitionOffset(device string) (int64, error) {
	f, err := os.Open(device)
	if err != nil {
		return 0, fmt.Errorf("partition offset: open %s: %w", device, err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	if _, err := io.ReadFull(f, buf); err != nil {
		return 0, fmt.Errorf("partition offset: read %s: %w", device, err)
	}
	if buf[mbrSignatureOffset] != 0x55 || buf[mbrSignatureOffset+1] != 0xAA {
		return 0, fmt.Errorf("partition offset: %s has no MBR signature", device)
	}

	start := binary.LittleEndian.Uint32(buf[mbrPart1EntryOffset+mbrPartStartLBAOffset : mbrPart1EntryOffset+mbrPartStartLBAOffset+4])
	if start == 0 {
		return 0, fmt.Errorf("partition offset: %s first partition entry is empty", device)
	}
	return int64(start) * 512, nil
}

// AttachLoop binds device at the given byte offset to a free loop device and
// returns the loop path (e.g. "/dev/loop3"). It uses a plain offset loop with
// no partition scan (-P), which is the loop configuration explicitly outside
// the udev partscan-rebind reliability bug class.
func AttachLoop(device string, offset int64, readOnly bool) (string, error) {
	args := []string{"--find", "--show", "--offset", strconv.FormatInt(offset, 10)}
	if readOnly {
		args = append(args, "--read-only")
	}
	args = append(args, device)

	out, err := exec.Command("losetup", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("losetup %s: %s: %w", device, strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// DetachLoop detaches a loop device. Detaching an already-detached or missing
// loop is treated as success so teardown is idempotent.
func DetachLoop(loop string) error {
	if loop == "" {
		return nil
	}
	if out, err := exec.Command("losetup", "-d", loop).CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "No such device") {
			return nil
		}
		return fmt.Errorf("losetup -d %s: %s: %w", loop, msg, err)
	}
	return nil
}

// DetachLoopsFor detaches any loop devices currently backed by device. Used at
// startup to reclaim loops leaked by a previous crashed run before we attach a
// fresh one (losetup -j lists associations for a backing device).
func DetachLoopsFor(device string) {
	out, err := exec.Command("losetup", "-j", device, "-O", "NAME", "--noheadings").CombinedOutput()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if loop := strings.TrimSpace(line); loop != "" {
			_ = DetachLoop(loop)
		}
	}
}
