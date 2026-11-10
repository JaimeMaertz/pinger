package main

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTargets(t *testing.T) {
	cases := []struct {
		arg         string
		want        int
		first, last string
	}{
		{"172.22.0.15", 1, "172.22.0.15", "172.22.0.15"},
		{"172.22.0.1/24", 254, "172.22.0.1", "172.22.0.254"},
		{"172.22.0.77/24", 254, "172.22.0.1", "172.22.0.254"},
		{"10.0.0.0/30", 2, "10.0.0.1", "10.0.0.2"},
		{"10.0.0.0/31", 2, "10.0.0.0", "10.0.0.1"},
		{"10.0.0.9/32", 1, "10.0.0.9", "10.0.0.9"},
		{"10.1.0.0/16", 65534, "10.1.0.1", "10.1.255.254"},
		{"255.255.255.0/24", 254, "255.255.255.1", "255.255.255.254"},
	}
	for _, c := range cases {
		got, err := expandTargets([]string{c.arg})
		if err != nil {
			t.Errorf("%s: %v", c.arg, err)
			continue
		}
		if len(got) != c.want || got[0].String() != c.first || got[len(got)-1].String() != c.last {
			t.Errorf("%s: got %d hosts %v..%v, want %d hosts %s..%s",
				c.arg, len(got), got[0], got[len(got)-1], c.want, c.first, c.last)
		}
	}

	for _, bad := range []string{"", "host.local", "172.22.0.1/33", "10.0.0.0/8", "fe80::1", "fe80::/64"} {
		if _, err := expandTargets([]string{bad}); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}

	got, _ := expandTargets([]string{"10.0.0.1", "10.0.0.0/30", "10.0.0.1"})
	if len(got) != 2 {
		t.Errorf("duplicates: got %v, want two addresses", got)
	}
}

func TestNormalizeMAC(t *testing.T) {
	const want = "00:10:7F:AA:0B:CC"
	for _, in := range []string{
		"00:10:7f:aa:0b:cc", "00-10-7F-AA-0B-CC", "0010.7faa.0bcc", "00.10.7F.AA.0B.CC", "00107FAA0BCC", " 0:10:7f:aa:b:cc ",
	} {
		if got, ok := normalizeMAC(in); !ok || got != want {
			t.Errorf("normalizeMAC(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "dynamic", "0x2", "eth0", "192.168.100.200", "00:10:7f:aa:bb", "00:10:7f:aa:bb:cc:dd", "0.10.7f.aa.b.cc"} {
		if got, ok := normalizeMAC(in); ok {
			t.Errorf("normalizeMAC(%q) = %q, want not a MAC", in, got)
		}
	}
}

func TestParseARPTable(t *testing.T) {
	windows := "\r\nInterface: 172.22.0.50 --- 0x5\r\n" +
		"  Internet Address      Physical Address      Type\r\n" +
		"  172.22.0.1            00-10-7f-aa-bb-cc     dynamic   \r\n" +
		"  172.22.0.71           d4-e0-8e-01-02-03     dynamic   \r\n" +
		"  172.22.0.255          ff-ff-ff-ff-ff-ff     static    \r\n" +
		"  224.0.0.22            01-00-5e-00-00-16     static    \r\n"
	linux := "IP address       HW type     Flags       HW address            Mask     Device\n" +
		"172.22.0.1       0x1         0x2         00:10:7f:aa:bb:cc     *        eth0\n" +
		"172.22.0.71      0x1         0x2         d4:e0:8e:01:02:03     *        eth0\n" +
		"172.22.0.9       0x1         0x0         00:00:00:00:00:00     *        eth0\n"

	for name, text := range map[string]string{"windows": windows, "linux": linux} {
		got := parseARPTable(text)
		if len(got) != 2 ||
			got[netip.MustParseAddr("172.22.0.1")] != "00:10:7F:AA:BB:CC" ||
			got[netip.MustParseAddr("172.22.0.71")] != "D4:E0:8E:01:02:03" {
			t.Errorf("%s: got %v", name, got)
		}
	}
}

func TestLoadDevices(t *testing.T) {
	dir := t.TempDir()

	missing, err := loadDevices(filepath.Join(dir, "none.json"))
	if err != nil || len(missing) != 0 {
		t.Errorf("missing file: got %v, %v; want empty and no error", missing, err)
	}

	path := filepath.Join(dir, "devices.json")
	os.WriteFile(path, []byte(`[
		{"mac": "00-10-7f-aa-bb-cc", "name": "ENC-001", "model": "DM-NVX-E30", "jobsite": "Example Job"},
		{"mac": "D4E08E010203", "name": "Camera 1"},
		{"mac": "00:05:A6:00:00:01", "jobsite": "Example Job"}
	]`), 0o644)
	devices, err := loadDevices(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := devices["00:10:7F:AA:BB:CC"]; got.Name != "ENC-001" || got.Model != "DM-NVX-E30" || got.Jobsite != "Example Job" {
		t.Errorf("got %+v", got)
	}
	if got := devices["D4:E0:8E:01:02:03"]; got.Name != "Camera 1" || got.Model != "" || got.Jobsite != "" {
		t.Errorf("got %+v", got)
	}
	if got := devices["00:05:A6:00:00:01"]; got.Name != "" || got.Jobsite != "Example Job" {
		t.Errorf("got %+v", got)
	}
	if got := devices["00:00:00:00:00:01"]; got != (device{}) {
		t.Errorf("unknown device = %+v, want empty", got)
	}

	os.WriteFile(path, []byte(`[{"mac": "not-a-mac", "name": "Bad"}]`), 0o644)
	if _, err := loadDevices(path); err == nil {
		t.Error("bad MAC: expected an error")
	}
}

func TestManufacturer(t *testing.T) {
	cases := map[string]string{
		"00:05:A6:01:02:03": "Extron Electronics",
		"00:10:7F:AA:BB:CC": "CRESTRON ELECTRONICS, INC.",
		"34:E1:D1:90:00:01": "Biamp",             // a 28-bit block inside a subdivided 24-bit one
		"02:00:00:00:00:01": "(private address)", // locally administered
		"00:00:00:00:00:00": "XEROX CORPORATION",
		"not a mac":         "",
	}
	for mac, want := range cases {
		if got := manufacturer(mac); got != want {
			t.Errorf("manufacturer(%q) = %q, want %q", mac, got, want)
		}
	}
	if got := manufacturer("34:E1:D1:F0:00:01"); got == ouiSubdivided {
		t.Errorf("a subdivided block reported %q", got)
	}
}

func TestWithoutLocal(t *testing.T) {
	a, b, c := netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"), netip.MustParseAddr("10.0.0.3")
	remote, skipped := withoutLocal([]netip.Addr{a, b, c}, map[netip.Addr]bool{b: true})
	if len(remote) != 2 || remote[0] != a || remote[1] != c || len(skipped) != 1 || skipped[0] != b {
		t.Errorf("remote %v, skipped %v", remote, skipped)
	}
}
