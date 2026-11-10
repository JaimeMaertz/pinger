package main

import (
	"net"
	"net/netip"
	"regexp"
	"strings"
)

var macShapes = []*regexp.Regexp{
	regexp.MustCompile(`^[0-9a-f]{1,2}([:-][0-9a-f]{1,2}){5}$`), // 00:10:7f:aa:bb:cc, 00-10-7F-AA-BB-CC
	regexp.MustCompile(`^[0-9a-f]{4}(\.[0-9a-f]{4}){2}$`),       // 0010.7faa.bbcc
	regexp.MustCompile(`^[0-9a-f]{2}(\.[0-9a-f]{2}){5}$`),       // 00.10.7F.AA.BB.CC
	regexp.MustCompile(`^[0-9a-f]{12}$`),                        // 00107faabbcc
}

// normalizeMAC returns a MAC address as AA:BB:CC:DD:EE:FF, whatever separators and case it
// was written with. The second result is false when the text is not a MAC address.
func normalizeMAC(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	shape := -1
	for i, re := range macShapes {
		if re.MatchString(s) {
			shape = i
			break
		}
	}
	if shape < 0 {
		return "", false
	}

	var hex string
	if shape == 0 {
		// Linux drops no digits, but some tools print single-digit octets (0:10:7f:...).
		for _, octet := range strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == '-' }) {
			if len(octet) == 1 {
				octet = "0" + octet
			}
			hex += octet
		}
	} else {
		hex = strings.ReplaceAll(s, ".", "")
	}

	hex = strings.ToUpper(hex)
	octets := make([]string, 6)
	for i := range octets {
		octets[i] = hex[i*2 : i*2+2]
	}
	return strings.Join(octets, ":"), true
}

// parseARPTable reads the text of an ARP table (`arp -a` on Windows, /proc/net/arp on
// Linux) into address-to-MAC pairs. A row counts when its first column is an IPv4 address
// and a later column is a MAC address. Unresolved, broadcast and multicast entries are
// left out.
func parseARPTable(text string) map[netip.Addr]string {
	table := make(map[netip.Addr]string)
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip, err := netip.ParseAddr(fields[0])
		if err != nil || !ip.Is4() {
			continue
		}
		for _, field := range fields[1:] {
			mac, ok := normalizeMAC(field)
			if !ok {
				continue
			}
			if isUnicastMAC(mac) {
				table[ip] = mac
			}
			break
		}
	}
	return table
}

func isUnicastMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return false
	}
	if hw[0]&1 == 1 { // multicast and broadcast
		return false
	}
	for _, b := range hw {
		if b != 0 {
			return true
		}
	}
	return false // all zeros: an unresolved entry
}

// localAddrs returns this machine's own IPv4 addresses, one or more per network adapter.
// A sweep leaves them out: the machine doing the scan is not something it is looking for.
func localAddrs() map[netip.Addr]bool {
	local := make(map[netip.Addr]bool)
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return local
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip, ok := netip.AddrFromSlice(ipnet.IP.To4()); ok {
			local[ip] = true
		}
	}
	return local
}

// withoutLocal splits the targets into the ones to ping and this machine's own addresses.
func withoutLocal(targets []netip.Addr, local map[netip.Addr]bool) (remote, skipped []netip.Addr) {
	for _, ip := range targets {
		if local[ip] {
			skipped = append(skipped, ip)
		} else {
			remote = append(remote, ip)
		}
	}
	return remote, skipped
}
