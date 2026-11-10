package main

import (
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/net/icmp"
)

// clearARPCache empties the machine's ARP cache. Linux only allows it as root.
func clearARPCache() error {
	out, err := exec.Command("ip", "neigh", "flush", "all").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (run with sudo)", firstLine(out, err))
	}
	return nil
}

func readARPTable() (map[netip.Addr]string, error) {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil, err
	}
	return parseARPTable(string(data)), nil
}

// rawSocketsAllowed reports whether pings can use a raw ICMP socket, which needs root or
// the cap_net_raw capability. Without it the sweep falls back to unprivileged ICMP, which
// the kernel allows for the groups in net.ipv4.ping_group_range.
func rawSocketsAllowed() bool {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func firstLine(out []byte, err error) string {
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return err.Error()
}
