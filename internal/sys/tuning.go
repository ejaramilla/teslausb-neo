package sys

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TuningConfig mirrors the tuning section from the application configuration.
type TuningConfig struct {
	DirtyRatio              int
	DirtyBackgroundBytes    int64
	DirtyWritebackCentisecs int
	CPUGovernor             string
	CPUGovernorArchiving    string
}

const (
	vmPath          = "/proc/sys/vm"
	cpuGovernorPath = "/sys/devices/system/cpu/cpufreq/policy0/scaling_governor"
	zramModulePath  = "/sys/block/zram0"
)

// ApplyVMTuning sets kernel virtual memory parameters for optimal USB gadget
// write performance.
func ApplyVMTuning(cfg TuningConfig) error {
	params := map[string]string{
		"dirty_ratio":              fmt.Sprintf("%d", cfg.DirtyRatio),
		"dirty_background_bytes":   fmt.Sprintf("%d", cfg.DirtyBackgroundBytes),
		"dirty_writeback_centisecs": fmt.Sprintf("%d", cfg.DirtyWritebackCentisecs),
	}

	for name, value := range params {
		path := filepath.Join(vmPath, name)
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			return fmt.Errorf("set %s to %s: %w", name, value, err)
		}
	}

	return nil
}

// SetIOScheduler sets the I/O scheduler for the given block device. Typically
// used to set BFQ on mmcblk devices for better mixed-workload performance.
func SetIOScheduler(device, scheduler string) error {
	// device should be like "mmcblk0" (without /dev/ prefix)
	devName := strings.TrimPrefix(device, "/dev/")
	schedPath := filepath.Join("/sys/block", devName, "queue", "scheduler")

	if err := os.WriteFile(schedPath, []byte(scheduler), 0644); err != nil {
		return fmt.Errorf("set scheduler %s on %s: %w", scheduler, device, err)
	}
	return nil
}

// SetCPUGovernor sets the CPU frequency scaling governor.
func SetCPUGovernor(governor string) error {
	if err := os.WriteFile(cpuGovernorPath, []byte(governor), 0644); err != nil {
		return fmt.Errorf("set cpu governor to %s: %w", governor, err)
	}
	return nil
}

// SetupZRAM creates and enables a zram swap device with the given size in
// megabytes. This provides compressed in-memory swap which is useful on
// memory-constrained Raspberry Pi systems.
func SetupZRAM(sizeMB int) error {
	// Load the zram module if not already loaded
	if _, err := os.Stat(zramModulePath); os.IsNotExist(err) {
		cmd := exec.Command("modprobe", "zram")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("modprobe zram: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	// Set the disk size
	sizeBytes := fmt.Sprintf("%d", sizeMB*1024*1024)
	disksizePath := filepath.Join(zramModulePath, "disksize")
	if err := os.WriteFile(disksizePath, []byte(sizeBytes), 0644); err != nil {
		return fmt.Errorf("set zram disksize: %w", err)
	}

	// Format as swap
	cmd := exec.Command("mkswap", "/dev/zram0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mkswap /dev/zram0: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Enable swap with high priority
	cmd = exec.Command("swapon", "-p", "5", "/dev/zram0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("swapon /dev/zram0: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// ApplyAll applies all system tuning settings from the given configuration.
func ApplyAll(cfg TuningConfig) error {
	if err := ApplyVMTuning(cfg); err != nil {
		return fmt.Errorf("vm tuning: %w", err)
	}

	if err := SetIOScheduler("mmcblk0", "bfq"); err != nil {
		// Non-fatal: BFQ may not be available on all kernels
		fmt.Fprintf(os.Stderr, "warning: could not set IO scheduler: %v\n", err)
	}

	if cfg.CPUGovernor != "" {
		if err := SetCPUGovernor(cfg.CPUGovernor); err != nil {
			return fmt.Errorf("cpu governor: %w", err)
		}
	}

	if err := SetupZRAM(256); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not setup zram: %v\n", err)
	}

	return nil
}
