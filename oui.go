package main

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// oui.txt is the IEEE's list of MAC address blocks and who they are assigned to, one
// "PREFIX<tab>Name" per line. The prefix is 6, 7 or 9 hex digits. gen_oui.go rebuilds it.
//
//go:embed oui.txt
var ouiText string

// A large block is sometimes handed back to the IEEE to be split into smaller ones. The
// large block then carries this name, which says nothing about who made the device.
const ouiSubdivided = "IEEE Registration Authority"

var ouiTable = sync.OnceValue(func() map[string]string {
	table := make(map[string]string, 60000)
	for _, line := range strings.Split(ouiText, "\n") {
		if prefix, name, ok := strings.Cut(line, "\t"); ok {
			table[prefix] = name
		}
	}
	return table
})

// manufacturer names who a MAC address (in AA:BB:CC:DD:EE:FF form) is registered to. The
// smallest block that holds the address wins. It returns "" when the address is in no
// registered block.
func manufacturer(mac string) string {
	hex := strings.ReplaceAll(mac, ":", "")
	if len(hex) != 12 {
		return ""
	}
	// The second-lowest bit of the first octet marks an address the device or its
	// software made up (phones and laptops with a private Wi-Fi address, virtual machines).
	if first, err := strconv.ParseUint(hex[:2], 16, 8); err == nil && first&2 != 0 {
		return "(private address)"
	}
	table := ouiTable()
	for _, digits := range []int{9, 7, 6} {
		if name, ok := table[hex[:digits]]; ok && name != ouiSubdivided {
			return name
		}
	}
	return ""
}
