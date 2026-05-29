// Package fsutil provides filesystem utility functions (mount, unmount,
// fsck, fstrim, disk usage).
package fsutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// IsMounted reports whether the given path is currently a mount point by
// scanning /proc/mounts (the mount target is field 2 of each line).
func IsMounted(path string) bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	return mountedIn(data, path)
}

// mountedIn reports whether procMounts (the contents of /proc/mounts) lists
// path as a mount target. Split out so it can be unit-tested without /proc.
func mountedIn(procMounts []byte, path string) bool {
	for _, line := range strings.Split(string(procMounts), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == path {
			return true
		}
	}
	return false
}

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

// Mount mounts a device at the given mountpoint. It is idempotent: if the
// mountpoint already has something mounted (e.g. left over from a crashed
// run), it is unmounted first so we never stack mounts or mount the wrong
// device under an existing one.
func Mount(device, mountpoint, fstype string, readOnly bool) error {
	if IsMounted(mountpoint) {
		if err := Unmount(mountpoint); err != nil {
			return fmt.Errorf("mount %s: stale mount at %s could not be cleared: %w", device, mountpoint, err)
		}
	}

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

// Unmount unmounts the given mountpoint. It is idempotent (a path that is not
// mounted is treated as success) and falls back to a lazy unmount if the
// mountpoint is briefly busy.
func Unmount(mountpoint string) error {
	if !IsMounted(mountpoint) {
		return nil
	}
	cmd := exec.Command("umount", mountpoint)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// Retry with a lazy unmount before giving up.
	if lout, lerr := exec.Command("umount", "-l", mountpoint).CombinedOutput(); lerr != nil {
		return fmt.Errorf("umount %s: %s: %w", mountpoint, strings.TrimSpace(string(out)+" "+string(lout)), err)
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
	// Bavail = blocks available to unprivileged users (excludes the
	// root-reserved blocks counted by Bfree); this matches what can actually
	// be written, which is what the free-space-reserve check needs.
	free = stat.Bavail * uint64(stat.Bsize)
	used = total - stat.Bfree*uint64(stat.Bsize)

	return used, free, nil
}
