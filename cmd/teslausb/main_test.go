package main

import (
	"os"
	"regexp"
	"testing"
)

// TestPartitionConstants guards the SD-card partition layout against drift.
// An earlier version bound cam=p3 — but p3 is the ext4 /data partition and p4
// is the MBR extended container, so the daemon exposed /data to the car and
// ran fsck.exfat on ext4. The Tesla exFAT partitions MUST start at p5.
func TestPartitionConstants(t *testing.T) {
	cases := map[string]string{
		"cam":       camPartition,
		"music":     musicPartition,
		"lightshow": lightshowPartition,
		"data":      dataPartition,
	}
	want := map[string]string{
		"cam":       "/dev/mmcblk0p5",
		"music":     "/dev/mmcblk0p6",
		"lightshow": "/dev/mmcblk0p7",
		"data":      "/dev/mmcblk0p3",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s partition = %s, want %s", name, got, want[name])
		}
	}

	// The cam (and every Tesla exFAT) partition must never be the ext4 data
	// partition — snapshotting + fsck.exfat on it would corrupt the DB/config.
	for _, p := range []string{camPartition, musicPartition, lightshowPartition} {
		if p == dataPartition {
			t.Errorf("Tesla partition %s collides with the ext4 data partition", p)
		}
	}
}

// TestSetupScriptMatchesPartitionConstants ties setup.sh (the Pi OS installer)
// to the daemon's partition constants so the two can't drift apart again. It
// asserts setup.sh formats cam→p5, music→p6, lightshow→p7.
func TestSetupScriptMatchesPartitionConstants(t *testing.T) {
	data, err := os.ReadFile("../../setup.sh")
	if err != nil {
		t.Fatalf("read setup.sh: %v", err)
	}
	script := string(data)

	mapping := map[string]string{
		"cam":       partNum(t, camPartition),
		"music":     partNum(t, musicPartition),
		"lightshow": partNum(t, lightshowPartition),
	}
	for label, num := range mapping {
		// e.g. format_if_empty "${BOOT_DISK}p5" exfat cam
		re := regexp.MustCompile(`format_if_empty\s+"\$\{BOOT_DISK\}p` + num + `"\s+exfat\s+` + label)
		if !re.MatchString(script) {
			t.Errorf("setup.sh does not format %s on p%s (constant is %s)", label, num, mapping[label])
		}
	}
}

// partNum extracts the trailing partition number from a /dev/mmcblk0pN path.
func partNum(t *testing.T, dev string) string {
	t.Helper()
	m := regexp.MustCompile(`p(\d+)$`).FindStringSubmatch(dev)
	if m == nil {
		t.Fatalf("cannot parse partition number from %q", dev)
	}
	return m[1]
}
