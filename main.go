// pinger pings a single address or every host in a subnet at once, then lists the
// hosts that answered with their ping time, MAC address, manufacturer and, when the MAC is
// in the device database, the device's name.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// version is set at release build time: go build -ldflags "-X main.version=v1.0.0".
var version = "dev"

func main() {
	var (
		dbPath  = flag.String("db", defaultDBPath(), "device database (JSON)")
		timeout = flag.Duration("timeout", time.Second*3, "how long to wait for each host")
		count   = flag.Int("count", 2, "echo requests per host")
		workers = flag.Int("workers", 2560, "hosts pinged at the same time")
		noClear = flag.Bool("no-clear", false, "leave the ARP cache alone before the sweep")
		asJSON  = flag.Bool("json", false, "write the results as JSON instead of a table")
		showVer = flag.Bool("version", false, "print the version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <ip | ip/prefix> ...\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s 172.22.0.15       ping one address\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s 172.22.0.1/24     ping every host in the subnet\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Println("pinger", version)
		return
	}
	if flag.NArg() == 0 || *count < 1 || *workers < 1 {
		flag.Usage()
		os.Exit(2)
	}
	targets, err := expandTargets(flag.Args())
	if err != nil {
		fatal(err)
	}
	targets, skipped := withoutLocal(targets, localAddrs())
	if len(targets) == 0 {
		fatal(fmt.Errorf("nothing to ping: %s is this machine", joinAddrs(skipped)))
	}
	devices, err := loadDevices(*dbPath)
	if err != nil {
		fatal(err)
	}

	// A stale ARP entry would put a MAC address beside a host that is no longer there, so
	// the cache is emptied first and every entry read afterwards comes from this sweep.
	cleared := false
	if !*noClear {
		if err := clearARPCache(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: ARP cache not cleared: %v\n", err)
		} else {
			cleared = true
		}
	}

	start := time.Now()
	results := sweep(targets, scanOptions{
		count:      *count,
		timeout:    *timeout,
		workers:    *workers,
		privileged: rawSocketsAllowed(),
	})
	elapsed := time.Since(start)

	macs, err := readARPTable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: ARP table not read: %v\n", err)
		macs = make(map[netip.Addr]string)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ip.Less(results[j].ip) })

	if *asJSON {
		writeJSON(results, macs, devices, cleared, skipped, len(targets), elapsed)
		return
	}

	out := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(out, "IP\tTIME\tMAC\tMANUFACTURER\tDEVICE\tMODEL\tJOBSITE")
	row := func(ip netip.Addr, time, mac string) {
		d := devices[mac]
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", ip, time, mac, manufacturer(mac), d.Name, d.Model, d.Jobsite)
	}
	alive, failed := 0, 0
	var firstErr error
	for _, r := range results {
		if r.err != nil {
			failed++
			if firstErr == nil {
				firstErr = r.err
			}
		}
		mac := macs[r.ip]
		switch {
		case r.alive:
			alive++
			row(r.ip, formatRTT(r.rtt), mac)
		case mac != "" && cleared:
			// The host answered ARP during this sweep but not the ping (ICMP blocked).
			// Without a cleared cache the entry could be stale, so it is not listed.
			row(r.ip, "no reply", mac)
		}
	}
	out.Flush()

	fmt.Printf("\n%d of %d hosts answered in %s\n", alive, len(targets), elapsed.Round(10*time.Millisecond))
	if len(skipped) > 0 {
		fmt.Printf("left out this machine: %s\n", joinAddrs(skipped))
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d pings could not be sent: %v\n", failed, firstErr)
		if alive == 0 {
			os.Exit(1)
		}
	}
}

// jsonHost is one row of -json output: the same hosts the table lists.
type jsonHost struct {
	IP           string  `json:"ip"`
	Replied      bool    `json:"replied"` // false: answered ARP but not the ping
	RTTms        float64 `json:"rtt_ms,omitempty"`
	MAC          string  `json:"mac,omitempty"`
	Manufacturer string  `json:"manufacturer,omitempty"`
	Name         string  `json:"name,omitempty"`
	Model        string  `json:"model,omitempty"`
	Jobsite      string  `json:"jobsite,omitempty"`
}

// writeJSON prints the sweep as one JSON object, for other tools (netdiagram) to read.
func writeJSON(results []pingResult, macs map[netip.Addr]string, devices map[string]device,
	cleared bool, skipped []netip.Addr, targets int, elapsed time.Duration) {
	out := struct {
		Hosts      []jsonHost `json:"hosts"`
		Targets    int        `json:"targets"`
		Seconds    float64    `json:"seconds"`
		ARPCleared bool       `json:"arp_cleared"`
		Skipped    []string   `json:"skipped,omitempty"`
		Errors     int        `json:"errors,omitempty"`
	}{Hosts: []jsonHost{}, Targets: targets, Seconds: elapsed.Seconds(), ARPCleared: cleared}
	for _, r := range results {
		if r.err != nil {
			out.Errors++
		}
		mac := macs[r.ip]
		if !r.alive && (mac == "" || !cleared) {
			continue
		}
		d := devices[mac]
		h := jsonHost{IP: r.ip.String(), Replied: r.alive, MAC: mac, Manufacturer: manufacturer(mac),
			Name: d.Name, Model: d.Model, Jobsite: d.Jobsite}
		if r.alive {
			h.RTTms = float64(r.rtt.Microseconds()) / 1000
		}
		out.Hosts = append(out.Hosts, h)
	}
	for _, s := range skipped {
		out.Skipped = append(out.Skipped, s.String())
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

func formatRTT(d time.Duration) string {
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

func joinAddrs(addrs []netip.Addr) string {
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
	os.Exit(2)
}
