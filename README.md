# pinger

A command-line tool that pings one address or a whole subnet at once, then lists what
answered: IP, ping time, MAC address, manufacturer, and (when you keep a device database)
the device's name, model and job site. Written in Go, for Windows and Linux.

I wrote it for indentifying devices on AV networks, either on site during
commissioning or on the bench, but it works on any IPv4 LAN. 
It was built specifically for speed on large networks, and outperforms Nmap and Advanced IP Scanner in scan speed.

```
pinger 172.22.0.15        ping one address
pinger 172.22.0.1/24      ping every host in 172.22.0.0/24

IP           TIME      MAC                MANUFACTURER                DEVICE          MODEL  JOBSITE
172.22.0.1   0.4 ms    00:10:7F:AA:BB:CC  CRESTRON ELECTRONICS, INC.  Rack Processor  CP4N   Example Job
172.22.0.9   no reply  00:05:A6:01:02:03  Extron Electronics
172.22.0.71  1.2 ms    10:25:00:01:02:03

2 of 253 hosts answered in 1.1s
left out this machine: 172.22.0.50
```

## Install

Download from the [Releases](https://github.com/JaimeMaertz/pinger/releases) page:
`pinger-<version>-windows-amd64.zip` or `pinger-<version>-linux-amd64.tar.gz`. Each holds the
executable and `oui.txt`. Put the folder on your PATH and keep `oui.txt` beside the executable:
pinger has the list built in, but [netdiagram](https://github.com/JaimeMaertz/net-diagram) and
[ip-table-database-editor](https://github.com/JaimeMaertz/ip-table-database-editor) read that
file from beside pinger.

Or build it with Go 1.25 or later:

```
go install github.com/JaimeMaertz/pinger@latest
```

`go install` gives only the executable. If you use the other tools, copy `oui.txt` from this
repository (or the release zip) beside it.

## How a sweep runs

1. **The ARP cache is cleared** so that every MAC address read afterwards comes from this
   sweep, not from a device that has since left. Clearing needs an Administrator prompt on
   Windows and `sudo` on Linux. Without it, the sweep prints a warning and carries on.
2. **Every host is pinged at the same time**, starting 2 ms apart. Each host gets up to 2
   echo requests a second apart, inside a 3 second wait, and is done at its first reply.
3. **The ARP table is read** and matched to the hosts by IP.
4. **Each MAC is matched to its manufacturer** using the IEEE registry, which is built into
   the program, so no network access is needed.
5. **Each MAC is looked up in the device database**, if there is one.

The machine running the sweep is left out. Its own addresses, on every adapter, are
neither pinged nor listed, and the last line says which ones were skipped.

The manufacturer column is blank for a MAC that isn't in any registered block. It reads
`(private address)` for an address the device made up, such as a phone or laptop using a
private Wi-Fi address, or a virtual machine.

A row marked `no reply` is a host that answered ARP during the sweep but not the ping,
because it blocks ICMP. These rows only appear when the ARP cache was cleared; otherwise
the entry could be stale. MAC addresses can only be read for hosts on the machine's own
subnet, so a routed host shows a time and no MAC.

### Why the retries and the spacing

With an empty ARP cache, a host can't answer until the machine has resolved its MAC, and
that can take seconds (Windows repeats an unanswered ARP request once a second). Sending
one request with a 1 second wait missed 6 or 7 of 79 hosts on an office /24 when their
entries were cold. Once they were cached, it missed none. Because of this, a sweep of a
quiet subnet takes the full `-timeout`, about 3.6 s for a /24.

On large sweeps, the 2 ms gap between starts sets the time, not `-workers`. A /16 has
65,534 hosts, so starting them all takes about 2 min 45 s, plus the timeout.

## Options

| Option      | Default                              | Meaning                                                               |
| ----------- | ------------------------------------ | --------------------------------------------------------------------- |
| `-db`       | `devices.json` beside the executable | device database to read                                               |
| `-timeout`  | `3s`                                 | how long to wait for each host                                        |
| `-count`    | `2`                                  | echo requests per host, one second apart, stopping at the first reply |
| `-workers`  | `2560`                               | hosts pinged at the same time                                         |
| `-no-clear` | off                                  | leave the ARP cache alone                                             |
| `-json`     | off                                  | print JSON instead of the table, for other tools to read              |
| `-version`  |                                      | print the version and exit                                            |

A subnet can be as large as a /16. Options go before the addresses. You can give several
addresses and subnets in one run.

## Device database

`devices.json` is a list of devices keyed by MAC address. Only `mac` is required. `name`
fills the DEVICE column, `model` the MODEL column and `jobsite` the JOBSITE column. MACs
can be written with colons, dashes, dots (`C4.42.68.12.34.56`, or Cisco-style
`c442.6812.3456`) or no separators, in either case. See `devices.example.json`:

```json
[
  { "mac": "00:10:7F:AA:BB:CC", "name": "Rack Processor", "model": "CP4N", "jobsite": "Example Job" },
  { "mac": "D4-E0-8E-01-02-03", "name": "Conference Room Camera" }
]
```

If the database is missing, that's not an error: the sweep runs without names. If it won't
parse, or holds something that isn't a MAC address, the run stops and names the bad entry.

[ip-table-database-editor](https://github.com/JaimeMaertz/ip-table-database-editor)
builds and edits this file from spreadsheets and from pinger's own sweeps.

## Platform notes

- **Windows** pings through a raw ICMP socket, which works from a normal prompt. Only
  clearing the ARP cache needs elevation.
- **Linux** uses a raw socket when allowed (root or `cap_net_raw`). Otherwise it uses
  unprivileged ICMP, which works when `net.ipv4.ping_group_range` includes your group.
- **WSL** sees its own virtual network, not the building's. Run `pinger.exe` from Windows
  to sweep the real one.

## Build and test

```
go test ./...
go build -o pinger .
GOOS=windows GOARCH=amd64 go build -o pinger.exe .
```

To refresh the manufacturer list, run `go run gen_oui.go` and rebuild. New address blocks
appear in the IEEE registry all the time, so a blank manufacturer on a new product is your
cue to do this.

## Source layout

| File                             | What it holds                                                    |
| -------------------------------- | ---------------------------------------------------------------- |
| `main.go`                        | options, the order of a sweep, the results table and JSON output |
| `targets.go`                     | turns `ip` and `ip/prefix` arguments into the list of hosts      |
| `scan.go`                        | the concurrent pings, on `prometheus-community/pro-bing`         |
| `arp.go`                         | MAC formatting and the ARP table parser shared by both systems   |
| `arp_windows.go`, `arp_linux.go` | what differs per system (`netsh` / `arp -a`, `ip neigh`)         |
| `oui.go`, `oui.txt`              | the manufacturer lookup and the built-in IEEE block list         |
| `gen_oui.go`                     | downloads a fresh `oui.txt` from the IEEE                        |
| `devices.go`                     | the device database                                              |
| `pinger_test.go`                 | tests                                                            |

## Related tools

- [ip-table-database-editor](https://github.com/JaimeMaertz/ip-table-database-editor): fills `devices.json` from IP-table spreadsheets and sweeps
- [netdiagram](https://github.com/JaimeMaertz/net-diagram): uses pinger's sweep to find switches, then draws how devices connect
- [ip-switcher](https://github.com/JaimeMaertz/ip-switcher): a tray app for switching your adapter between DHCP and static presets

## License

Source-available: an MIT-based license with a no-resale condition (see [LICENSE](LICENSE)),
not an OSI open-source license. You're free to use it and modify it,
including at work and on paid jobs, to share it free of charge, and to charge for installing,
training on or supporting it. Keep the copyright notice and the license with any copy.

What needs permission is selling it, or selling a product or hosted service built on it. For
that, contact JaimeMaertz@gmail.com.

Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md).
