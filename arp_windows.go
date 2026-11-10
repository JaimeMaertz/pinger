package main

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
)

// clearARPCache empties the machine's ARP cache. Windows only allows it from an elevated
// (Administrator) prompt.
func clearARPCache() error {
	out, err := exec.Command("netsh", "interface", "ip", "delete", "arpcache").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(out, err))
	}
	return nil
}

func readARPTable() (map[netip.Addr]string, error) {
	out, err := exec.Command("arp", "-a").Output()
	if err != nil {
		return nil, err
	}
	return parseARPTable(string(out)), nil
}

// rawSocketsAllowed reports whether pings can use a raw ICMP socket. On Windows that is
// the only kind the ping library supports.
func rawSocketsAllowed() bool { return true }

func firstLine(out []byte, err error) string {
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return err.Error()
}
