package main

import (
	"fmt"
	"net/netip"
	"strings"
)

// minPrefixBits is the largest subnet a single argument may name (a /16 is 65,534 hosts).
const minPrefixBits = 16

// expandTargets turns the command-line arguments into the list of addresses to ping.
// A bare address is one target. An address with a prefix length (172.22.0.1/24) is every
// host in that subnet; the network and broadcast addresses are left out, except in a /31
// or /32, which have neither.
func expandTargets(args []string) ([]netip.Addr, error) {
	seen := make(map[netip.Addr]bool)
	var targets []netip.Addr
	add := func(a netip.Addr) {
		if !seen[a] {
			seen[a] = true
			targets = append(targets, a)
		}
	}

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if !strings.Contains(arg, "/") {
			addr, err := netip.ParseAddr(arg)
			if err != nil || !addr.Is4() {
				return nil, fmt.Errorf("%q is not an IPv4 address", arg)
			}
			add(addr)
			continue
		}

		prefix, err := netip.ParsePrefix(arg)
		if err != nil || !prefix.Addr().Is4() {
			return nil, fmt.Errorf("%q is not an IPv4 address with a prefix length, like 172.22.0.1/24", arg)
		}
		if prefix.Bits() < minPrefixBits {
			return nil, fmt.Errorf("%q is larger than a /%d", arg, minPrefixBits)
		}
		prefix = prefix.Masked()

		var hosts []netip.Addr
		for a := prefix.Addr(); a.IsValid() && prefix.Contains(a); a = a.Next() {
			hosts = append(hosts, a)
		}
		if prefix.Bits() <= 30 {
			hosts = hosts[1 : len(hosts)-1]
		}
		for _, a := range hosts {
			add(a)
		}
	}
	return targets, nil
}
