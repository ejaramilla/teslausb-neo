package fsutil

import "testing"

func TestMountedIn(t *testing.T) {
	proc := []byte(`proc /proc proc rw,relatime 0 0
/dev/mmcblk0p3 /data ext4 rw,noatime 0 0
/dev/mapper/teslausb-snap /mnt/snap exfat ro,relatime 0 0
tmpfs /tmp tmpfs rw 0 0`)

	cases := map[string]bool{
		"/data":     true,
		"/mnt/snap": true,
		"/proc":     true,
		"/mnt":      false, // prefix of /mnt/snap but not itself a mount
		"/dev":      false,
		"":          false,
	}
	for path, want := range cases {
		if got := mountedIn(proc, path); got != want {
			t.Errorf("mountedIn(%q) = %v, want %v", path, got, want)
		}
	}
}
