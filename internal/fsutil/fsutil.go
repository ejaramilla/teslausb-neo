// Package fsutil provides filesystem utility functions (mount, unmount,
// fsck, fstrim, disk usage).
package fsutil

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// RunFsck runs fsck.exfat in auto-repair mode on the given device.
func RunFsck(device string) error {
	cmd := exec.Command("fsck.exfat", "-p", device)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fsck %s: %s: %w", device, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Fstrim runs fstrim on the given mountpoint to discard unused blocks.
func Fstrim(mountpoint string) error {
	cmd := exec.Command("fstrim", mountpoint)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fstrim %s: %s: %w", mountpoint, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Mount mounts a device at the given mountpoint.
func Mount(device, mountpoint, fstype string, readOnly bool) error {
	args := []string{"-t", fstype}
	if readOnly {
		args = append(args, "-o", "ro")
	}
	args = append(args, device, mountpoint)

	cmd := exec.Command("mount", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount %s on %s: %s: %w", device, mountpoint, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Unmount unmounts the given mountpoint.
func Unmount(mountpoint string) error {
	cmd := exec.Command("umount", mountpoint)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount %s: %s: %w", mountpoint, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// GetDiskUsage returns the used and free bytes for the filesystem at the
// given mountpoint.
func GetDiskUsage(mountpoint string) (used, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountpoint, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", mountpoint, err)
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free = stat.Bfree * uint64(stat.Bsize)
	used = total - free

	return used, free, nil
}
