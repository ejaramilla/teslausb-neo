package snapshot

import (
	"testing"
)

func TestSnapshotDevicePaths_DifferentNames(t *testing.T) {
	// The snapshot and origin device-mapper paths must incorporate the
	// snapshot name. This matters because multiple snapshots could
	// coexist (e.g., cam + music).
	tests := []struct {
		snapName     string
		wantOriginDM string
		wantSnapDM   string
	}{
		{
			snapName:     "teslausb-snap",
			wantOriginDM: "/dev/mapper/teslausb-snap-origin",
			wantSnapDM:   "/dev/mapper/teslausb-snap",
		},
		{
			snapName:     "cam-snapshot",
			wantOriginDM: "/dev/mapper/cam-snapshot-origin",
			wantSnapDM:   "/dev/mapper/cam-snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.snapName, func(t *testing.T) {
			s := &Snapshot{snapshotName: tt.snapName}

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
