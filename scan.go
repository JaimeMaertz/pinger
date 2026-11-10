package main

import (
	"net/netip"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

// pingInterval is the gap between echo requests to one host when -count is above 1. A
// host with no ARP entry yet cannot be reached until the machine has resolved its MAC,
// and Windows repeats an unanswered ARP request once a second, so the requests are spread
// over seconds, not sent in one burst.
const pingInterval = 1000 * time.Millisecond

// startStagger is the gap between starting one host and the next. With an empty ARP
// cache every first ping also sends an ARP broadcast; starting a whole subnet in the same
// millisecond sends hundreds of broadcasts at once, and some of them go unanswered.
const startStagger = 2 * time.Millisecond

type scanOptions struct {
	count      int
	timeout    time.Duration
	workers    int
	privileged bool
}

type pingResult struct {
	ip    netip.Addr
	alive bool
	rtt   time.Duration
	err   error
}

// sweep pings every target, at most opt.workers at a time, and returns one result per
// target in no particular order.
func sweep(targets []netip.Addr, opt scanOptions) []pingResult {
	jobs := make(chan netip.Addr)
	results := make([]pingResult, 0, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup

	workers := min(opt.workers, len(targets))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				r := pingOne(ip, opt)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}
		}()
	}
	for _, ip := range targets {
		jobs <- ip
		time.Sleep(startStagger)
	}
	close(jobs)
	wg.Wait()
	return results
}

// pingOne sends up to opt.count echo requests to one address, pingInterval apart, and
// waits up to opt.timeout in total. It returns at the first reply, so the later requests
// are retries: they only go out to a host that has not answered yet.
func pingOne(ip netip.Addr, opt scanOptions) pingResult {
	res := pingResult{ip: ip}
	pinger, err := probing.NewPinger(ip.String())
	if err != nil {
		res.err = err
		return res
	}
	pinger.Count = opt.count
	pinger.Timeout = opt.timeout
	pinger.Interval = pingInterval
	pinger.SetPrivileged(opt.privileged)
	pinger.OnRecv = func(*probing.Packet) { pinger.Stop() }
	if err := pinger.Run(); err != nil {
		res.err = err
		return res
	}
	stats := pinger.Statistics()
	if stats.PacketsRecv > 0 {
		res.alive = true
		res.rtt = stats.MinRtt
	}
	return res
}
