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
		// e.g. format_tesla_drive "${BOOT_DISK}p5" cam 1
		re := regexp.MustCompile(`format_tesla_drive\s+"\$\{BOOT_DISK\}p` + num + `"\s+` + label)
		if !re.MatchString(script) {
			t.Errorf("setup.sh does not format %s on p%s (constant is %s)", label, num, mapping[label])
		}
	}
}

// TestInstallScriptsUsePartitionedExfat is the regression guard for the
// "car reformats to 1 GB" bug: the Tesla rejects a bare exFAT written directly
// to a LUN and requires an MBR + exFAT partition. Both install paths must
// partition each Tesla drive with sfdisk (type 07) and must NOT mkfs.exfat
// directly on the raw cam/music/lightshow device.
func TestInstallScriptsUsePartitionedExfat(t *testing.T) {
	scripts := map[string]string{
		"setup.sh":              "../../setup.sh",
		"teslausb-provision.sh": "../../buildroot/board/teslausb/rootfs_overlay/usr/bin/teslausb-provision.sh",
	}
	// Bare exFAT straight onto a whole partition device — the broken pattern.
	bareExfat := regexp.MustCompile(`mkfs\.exfat[^\n]*\$\{[A-Z_]+\}p[567]"`)

	for name, path := range scripts {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		script := string(data)

		if !regexp.MustCompile(`sfdisk`).MatchString(script) {
			t.Errorf("%s does not use sfdisk to write a partition table (Tesla requires MBR+exFAT)", name)
		}
		if !regexp.MustCompile(`type=0?7`).MatchString(script) {
			t.Errorf("%s does not create an exFAT-type (07) partition", name)
		}
		if bareExfat.MatchString(script) {
			t.Errorf("%s formats bare exFAT directly on a raw partition device; the Tesla will reject it and reformat", name)
		}
		// mkfs.exfat must target a loop device (the inner partition), not pN.
		if !regexp.MustCompile(`mkfs\.exfat[^\n]*\$loop`).MatchString(script) {
			t.Errorf("%s should mkfs.exfat on the offset loop device, not the raw partition", name)
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
