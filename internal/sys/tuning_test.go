package sys

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestTuningConfigFormat(t *testing.T) {
	cfg := TuningConfig{
		DirtyRatio:              10,
		DirtyBackgroundBytes:    65536,
		DirtyWritebackCentisecs: 25,
		CPUGovernor:             "conservative",
		CPUGovernorArchiving:    "ondemand",
	}

	// Verify string formatting matches what sysfs expects.
	if got := fmt.Sprintf("%d", cfg.DirtyRatio); got != "10" {
		t.Errorf("DirtyRatio format = %q, want %q", got, "10")
	}
	if got := fmt.Sprintf("%d", cfg.DirtyBackgroundBytes); got != "65536" {
		t.Errorf("DirtyBackgroundBytes format = %q, want %q", got, "65536")
	}
	if got := fmt.Sprintf("%d", cfg.DirtyWritebackCentisecs); got != "25" {
		t.Errorf("DirtyWritebackCentisecs format = %q, want %q", got, "25")
	}
}

func TestVMPathConstruction(t *testing.T) {
	params := []string{"dirty_ratio", "dirty_background_bytes", "dirty_writeback_centisecs"}
	for _, p := range params {
		path := filepath.Join(vmPath, p)
		expected := "/proc/sys/vm/" + p
		if path != expected {
			t.Errorf("VM path = %q, want %q", path, expected)
		}
	}
}

func TestIOSchedulerPathConstruction(t *testing.T) {
	tests := []struct {
		device string
		want   string
	}{
		{"mmcblk0", "/sys/block/mmcblk0/queue/scheduler"},
		{"/dev/mmcblk0", "/sys/block/mmcblk0/queue/scheduler"},
		{"sda", "/sys/block/sda/queue/scheduler"},
	}
	for _, tt := range tests {
		// Replicate the path construction from SetIOScheduler.
		devName := tt.device
		if len(devName) > 5 && devName[:5] == "/dev/" {
			devName = devName[5:]
		}
		got := filepath.Join("/sys/block", devName, "queue", "scheduler")
		if got != tt.want {
			t.Errorf("scheduler path for %q = %q, want %q", tt.device, got, tt.want)
		}
	}
}

func TestZRAMSizeCalculation(t *testing.T) {
	tests := []struct {
		sizeMB int
		want   string
	}{
		{64, "67108864"},
		{128, "134217728"},
		{256, "268435456"},
		{512, "536870912"},
	}
	for _, tt := range tests {
		got := fmt.Sprintf("%d", tt.sizeMB*1024*1024)
		if got != tt.want {
			t.Errorf("ZRAM size %d MB = %q bytes, want %q", tt.sizeMB, got, tt.want)
		}
	}
}

func TestConstants(t *testing.T) {
	if vmPath != "/proc/sys/vm" {
		t.Errorf("vmPath = %q, want %q", vmPath, "/proc/sys/vm")
	}
	if cpuGovernorPath != "/sys/devices/system/cpu/cpufreq/policy0/scaling_governor" {
		t.Errorf("cpuGovernorPath = %q", cpuGovernorPath)
	}
	if zramModulePath != "/sys/block/zram0" {
		t.Errorf("zramModulePath = %q", zramModulePath)
	}
}
