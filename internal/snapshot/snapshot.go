// Package snapshot provides dm-snapshot management with a zram copy-on-write
// backing device. This allows the USB gadget image to be mounted read-only
// while the car writes to the snapshot overlay, which can later be merged
// back or discarded.
package snapshot

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Snapshot manages a device-mapper snapshot backed by a zram COW device.
type Snapshot struct {
	originDevice string
	snapshotName string
	zramDevice   string
	zramSizeMB   int
	mountpoint   string
}

// Create sets up a new dm-snapshot on originDevice.
// It allocates a zram device for the COW layer, then creates both the
// snapshot-origin and snapshot device-mapper targets.
func Create(originDevice string) (*Snapshot, error) {
	s := &Snapshot{
		originDevice: originDevice,
		snapshotName: "teslausb-snap",
		zramSizeMB:   512,
	}

	// Allocate a zram device.
	zramID, err := s.runCmd("cat", "/sys/class/zram-control/hot_add")
	if err != nil {
		return nil, fmt.Errorf("snapshot: zram hot_add: %w", err)
	}
	zramID = strings.TrimSpace(zramID)
	s.zramDevice = "/dev/zram" + zramID

	sizeBytes := int64(s.zramSizeMB) * 1024 * 1024
	if err := s.writeFile(fmt.Sprintf("/sys/block/zram%s/disksize", zramID), strconv.FormatInt(sizeBytes, 10)); err != nil {
		return nil, fmt.Errorf("snapshot: set zram size: %w", err)
	}

	// Get the size of the origin device in 512-byte sectors.
	sectorStr, err := s.runCmd("blockdev", "--getsz", originDevice)
	if err != nil {
		return nil, fmt.Errorf("snapshot: blockdev: %w", err)
	}
	sectors := strings.TrimSpace(sectorStr)

	// Create the snapshot-origin target.
	originTable := fmt.Sprintf("0 %s snapshot-origin %s", sectors, originDevice)
	if _, err := s.runCmd("dmsetup", "create", s.snapshotName+"-origin", "--table", originTable); err != nil {
		return nil, fmt.Errorf("snapshot: create origin: %w", err)
	}

	// Create the snapshot target.
	// Format: 0 <sectors> snapshot <origin> <cow> P 8
	snapTable := fmt.Sprintf("0 %s snapshot /dev/mapper/%s-origin %s P 8", sectors, s.snapshotName, s.zramDevice)
	if _, err := s.runCmd("dmsetup", "create", s.snapshotName, "--table", snapTable); err != nil {
		return nil, fmt.Errorf("snapshot: create snapshot: %w", err)
	}

	return s, nil
}

// Mount mounts the snapshot device read-only at the given mountpoint.
func (s *Snapshot) Mount(mountpoint string) error {
	s.mountpoint = mountpoint
	if _, err := s.runCmd("mkdir", "-p", mountpoint); err != nil {
		return fmt.Errorf("snapshot: mkdir %s: %w", mountpoint, err)
	}
	if _, err := s.runCmd("mount", "-o", "ro", "/dev/mapper/"+s.snapshotName, mountpoint); err != nil {
		return fmt.Errorf("snapshot: mount: %w", err)
	}
	return nil
}

// Release unmounts the snapshot, removes the device-mapper targets, and
// frees the zram device.
func (s *Snapshot) Release() error {
	var firstErr error

	if s.mountpoint != "" {
		if _, err := s.runCmd("umount", s.mountpoint); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("snapshot: umount: %w", err)
		}
		s.mountpoint = ""
	}

	// Remove snapshot before origin (order matters).
	if _, err := s.runCmd("dmsetup", "remove", s.snapshotName); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("snapshot: remove snapshot dm: %w", err)
	}

	if _, err := s.runCmd("dmsetup", "remove", s.snapshotName+"-origin"); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("snapshot: remove origin dm: %w", err)
	}

	// Free the zram device.
	if s.zramDevice != "" {
		zramID := strings.TrimPrefix(s.zramDevice, "/dev/zram")
		if _, err := s.runCmd("zramctl", "--reset", s.zramDevice); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("snapshot: zramctl reset: %w", err)
		}
		_ = s.writeFile("/sys/class/zram-control/hot_remove", zramID)
		s.zramDevice = ""
	}

	return firstErr
}

// IsValid checks whether the dm snapshot is healthy by querying its status.
// A valid snapshot reports "Invalid" == false.
func (s *Snapshot) IsValid() bool {
	out, err := s.runCmd("dmsetup", "status", s.snapshotName)
	if err != nil {
		return false
	}
	return !strings.Contains(out, "Invalid")
}

// runCmd executes a command and returns its combined stdout/stderr output.
func (s *Snapshot) runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// writeFile writes value to a sysfs/procfs path via shell echo to handle
// permission requirements consistently.
func (s *Snapshot) writeFile(path, value string) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo %s > %s", value, path))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write %s: %s", path, strings.TrimSpace(string(out)))
	}
	return nil
}
