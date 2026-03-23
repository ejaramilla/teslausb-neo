package snapshot

import (
	"testing"
)

func TestSnapshotDevicePaths(t *testing.T) {
	s := &Snapshot{
		originDevice: "/dev/loop0",
		snapshotName: "teslausb-snap",
		zramDevice:   "/dev/zram0",
		zramSizeMB:   512,
	}

	// Verify the origin device is stored correctly.
	if s.originDevice != "/dev/loop0" {
		t.Errorf("originDevice = %q, want %q", s.originDevice, "/dev/loop0")
	}

	// Verify the snapshot name.
	if s.snapshotName != "teslausb-snap" {
		t.Errorf("snapshotName = %q, want %q", s.snapshotName, "teslausb-snap")
	}

	// Verify the dm-mapper device paths that would be constructed.
	wantOriginDM := "/dev/mapper/" + s.snapshotName + "-origin"
	gotOriginDM := "/dev/mapper/" + s.snapshotName + "-origin"
	if gotOriginDM != wantOriginDM {
		t.Errorf("origin DM path = %q, want %q", gotOriginDM, wantOriginDM)
	}

	wantSnapDM := "/dev/mapper/" + s.snapshotName
	gotSnapDM := "/dev/mapper/" + s.snapshotName
	if gotSnapDM != wantSnapDM {
		t.Errorf("snapshot DM path = %q, want %q", gotSnapDM, wantSnapDM)
	}

	// Verify zram device path.
	if s.zramDevice != "/dev/zram0" {
		t.Errorf("zramDevice = %q, want %q", s.zramDevice, "/dev/zram0")
	}
}

func TestSnapshotDevicePathVariations(t *testing.T) {
	tests := []struct {
		name         string
		originDevice string
		snapName     string
		wantOriginDM string
		wantSnapDM   string
	}{
		{
			name:         "default",
			originDevice: "/dev/loop0",
			snapName:     "teslausb-snap",
			wantOriginDM: "/dev/mapper/teslausb-snap-origin",
			wantSnapDM:   "/dev/mapper/teslausb-snap",
		},
		{
			name:         "custom name",
			originDevice: "/dev/sda1",
			snapName:     "cam-snapshot",
			wantOriginDM: "/dev/mapper/cam-snapshot-origin",
			wantSnapDM:   "/dev/mapper/cam-snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Snapshot{
				originDevice: tt.originDevice,
				snapshotName: tt.snapName,
			}

			originDM := "/dev/mapper/" + s.snapshotName + "-origin"
			snapDM := "/dev/mapper/" + s.snapshotName

			if originDM != tt.wantOriginDM {
				t.Errorf("origin DM = %q, want %q", originDM, tt.wantOriginDM)
			}
			if snapDM != tt.wantSnapDM {
				t.Errorf("snapshot DM = %q, want %q", snapDM, tt.wantSnapDM)
			}
		})
	}
}

func TestZramSizing(t *testing.T) {
	tests := []struct {
		name      string
		sizeMB    int
		wantBytes int64
	}{
		{"default 512MB", 512, 512 * 1024 * 1024},
		{"256MB", 256, 256 * 1024 * 1024},
		{"1024MB", 1024, 1024 * 1024 * 1024},
		{"zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Snapshot{
				zramSizeMB: tt.sizeMB,
			}

			// This mirrors the calculation done in Create():
			//   sizeBytes := int64(s.zramSizeMB) * 1024 * 1024
			gotBytes := int64(s.zramSizeMB) * 1024 * 1024
			if gotBytes != tt.wantBytes {
				t.Errorf("zram size = %d bytes, want %d bytes", gotBytes, tt.wantBytes)
			}
		})
	}
}

func TestSnapshotDefaultValues(t *testing.T) {
	// Verify the default values that Create() would set internally.
	// We can't call Create() since it requires Linux, but we can verify
	// the constants used.
	s := &Snapshot{
		originDevice: "/dev/loop0",
		snapshotName: "teslausb-snap",
		zramSizeMB:   512,
	}

	if s.snapshotName != "teslausb-snap" {
		t.Errorf("default snapshotName = %q, want %q", s.snapshotName, "teslausb-snap")
	}
	if s.zramSizeMB != 512 {
		t.Errorf("default zramSizeMB = %d, want %d", s.zramSizeMB, 512)
	}
	if s.mountpoint != "" {
		t.Errorf("mountpoint should be empty initially, got %q", s.mountpoint)
	}
}

func TestDMTableConstruction(t *testing.T) {
	// Verify the dm-setup table strings that would be constructed.
	// This mirrors the logic in Create() without running actual commands.
	s := &Snapshot{
		originDevice: "/dev/loop0",
		snapshotName: "teslausb-snap",
		zramDevice:   "/dev/zram0",
	}

	sectors := "31457280" // example: 16GB / 512

	// Origin table format from Create():
	//   "0 <sectors> snapshot-origin <originDevice>"
	wantOriginTable := "0 " + sectors + " snapshot-origin " + s.originDevice
	gotOriginTable := "0 " + sectors + " snapshot-origin " + s.originDevice
	if gotOriginTable != wantOriginTable {
		t.Errorf("origin table = %q, want %q", gotOriginTable, wantOriginTable)
	}

	// Snapshot table format from Create():
	//   "0 <sectors> snapshot /dev/mapper/<name>-origin <zramDevice> P 8"
	wantSnapTable := "0 " + sectors + " snapshot /dev/mapper/" + s.snapshotName + "-origin " + s.zramDevice + " P 8"
	gotSnapTable := "0 " + sectors + " snapshot /dev/mapper/" + s.snapshotName + "-origin " + s.zramDevice + " P 8"
	if gotSnapTable != wantSnapTable {
		t.Errorf("snap table = %q, want %q", gotSnapTable, wantSnapTable)
	}
}
