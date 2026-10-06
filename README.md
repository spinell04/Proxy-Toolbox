# Proxy Toolbox

A CLI toolbox for testing, analyzing, and managing proxy lists. Interactive menus with arrow-key navigation.

## Features

Listed in menu order.

| Tool | Description |
|------|-------------|
| **IP Uniqueness Test** | Check exit IPs through each proxy and detect duplicates |
| **Site Request Test → Ticketmaster** | Full TLS request to Ticketmaster through each proxy |
| **Site Request Test → Bayern** | Full TLS request to fcbayern.com/de/tickets |
| **Monitor → Downtime monitor** | Continuous reachability monitoring with Discord webhook alerts |
| **Monitor → Session monitor** | Alerts when a proxy's exit IP changes |
| **Ping Test** | Ping a domain through proxies via TCP, HTTP, or HTTPS |
| **Randomize File** | Shuffle the proxy order in a file |
| **Proxy Parser** | Convert proxies between different formats |
| **Compare Results** | Local, read-only dashboard for comparing exported CSVs |

## Quick Start

Two routes, both from the [latest release](https://github.com/spinell04/Proxy-Toolbox/releases/latest):

- **Setup** — download one installer (`Setup-ProxyToolbox.exe`, `Setup-ProxyToolbox-mac-AppleSiliconCPU` or `Setup-ProxyToolbox-macOS-IntelCPU`), put it in the folder you want the toolbox in, and run it. It fetches the current toolbox binary, verifies its SHA-256 and places it there.
- **Direct download** — take the toolbox binary itself (`proxytoolbox.exe`, `proxytoolbox-mac-AppleSiliconCPU` or `proxytoolbox-macOS-IntelCPU`) and drop it in a folder.

Then:

1. Place your proxy files (`.txt`) in the `proxyfiles/` folder next to the binary
2. Run the binary and navigate the menu with arrow keys

Either way it is the only download you need: the binary checks for a newer release on each launch and installs it, leaving `config.txt`, `proxyfiles/` and `results/` alone. Full detail, including Gatekeeper and SmartScreen, is in [Installation](docs/getting-started/installation.md); the update mechanism is in [Auto-Update](docs/reference/auto-update.md).

## Configuration

Edit `config.txt` in the same directory as the binary:

```
# Number of parallel workers (applies to all tools)
workers=40

# Default domain for Ping Test (optional)
# Formats:
#   google.com          -> TCP connect only
#   http://google.com   -> full HTTP request
#   https://google.com  -> full HTTPS request
domain=google.com

# Prompt defaults for the two monitors, in ms. Different units.
#   monitor_interval_ms  Downtime monitor: the gap between individual
#                        checks, so N proxies means each one is revisited
#                        every N x interval.
#   session_interval_ms  Session monitor: the gap between cycles, and a
#                        cycle checks every proxy in parallel, so this is
#                        the sampling period directly.
monitor_interval_ms=1000
session_interval_ms=60000

# ─── Auto-update ─────────────────────────────────────
# On startup, check GitHub for a newer release, install it and restart.
# The download is verified against the release's SHA-256 before anything
# is replaced, and a failed check never stops the toolbox from starting.
#   on  = stay current automatically (recommended)
#   off = never check; pin whatever binary you have
auto_update=on

# ─── Latency measurement ─────────────────────────────
# Whether a DNS lookup counts toward the reported latency.
#   off  Resolve before timing, then connect to the address. This is what
#        `ping` reports.
#   on   Time the lookup too, as a client resolving on every request would.
# Site Request Test always includes it, whatever this says.
measure_dns=off
```

Full detail, including which tools the last key affects, is in
[Configuration](docs/getting-started/configuration.md#measure_dns).

## Proxy Formats

All tools auto-detect the input format. Supported formats:

| Format | Example |
|--------|---------|
| `host:port:user:pass` | `1.2.3.4:8080:admin:secret` |
| `user:pass:host:port` | `admin:secret:1.2.3.4:8080` |
| `user:pass@host:port` | `admin:secret@1.2.3.4:8080` |
| `http://user:pass@host:port` | `http://admin:secret@1.2.3.4:8080` |
| direct line (no proxy) | `direct`, `localhost`, `localhost:localhost:localhost:localhost` |

A **direct line** means "make this request with no proxy", measured under the same target, timeout and worker pool as the proxies around it. A line naming a port — `localhost:8080:user:pass` — is still a real proxy on loopback. Full details in [proxy formats → direct lines](docs/getting-started/proxy-formats.md#direct-lines-no-proxy).

The **Proxy Parser** tool can convert between any of these formats, plus strip auth to `host:port`.

## Proxy Files

Place `.txt` files in the `proxyfiles/` folder. One proxy per line. Lines starting with `#` are ignored.

```
proxyfiles/
  Mobile-static-mix.txt
  Private-static-mix.txt
  rotative.txt
  ...
```

## Building from Source

Requires Go 1.25+. No binaries are tracked in the repository — the release
workflow is the only thing that builds the ones people download — so building
is how you get one from a checkout.

```bash
# Current platform
go build -o proxytoolbox .

# Cross-compile
GOOS=darwin GOARCH=arm64 go build -o proxytoolbox-mac-AppleSiliconCPU .
GOOS=darwin GOARCH=amd64 go build -o proxytoolbox-macOS-IntelCPU .
GOOS=windows GOARCH=amd64 go build -o proxytoolbox.exe .
```

All of those output names are gitignored, so a local build never gets
committed by accident.

A binary built this way reports version **`dev`** in the menu title and
**never auto-updates** — only the release workflow stamps a real version. See
[Building from Source](docs/reference/building-from-source.md) for how to
reproduce a release build exactly.

## Export

After running IP Uniqueness Test, Ping Test, or either Site Request Test, you're prompted to save results to a CSV file. Exported files land in the `results/` folder.

The prompt suggests a name built from the tool, the proxy file you tested and the moment of the run — `<tool>_<proxyfile>_<date>_<time>.csv`. Type `.` to accept it:

```
Save results to CSV? (Enter to skip, "." for pinger_schroeder_2026-09-19_143207.csv, or type filename):
```

Every export opens with five metadata rows (`Tool`, `Run at`, `Proxy file`, `Target`, `Workers`) that describe the run, so **Compare Results** can tell a change in the proxies from a change in the test itself.

### Saving filtered proxies

After the IP Uniqueness, Ping, TM, and Bayern tests, you're also prompted to save the matching proxies to a `.txt` file in the `proxyfiles/` folder (you name it inline). Proxies are written in their original line format, ready to reuse.

- **IP Uniqueness Test** saves one proxy per unique exit IP — duplicates and errored proxies are dropped.
- **Ping / TM / Bayern** save successful proxies whose latency is below the per-tool threshold set in `config.txt` (`ping_max_latency_ms`, `tm_max_latency_ms`, `bayern_max_latency_ms`). A blank or invalid threshold means no filter, so all successful proxies are saved.

## Documentation

Published at **[proxy-toolbox.gitbook.io](https://proxy-toolbox.gitbook.io/docs/)**, and in [`docs/`](docs/) in this repo:

- **Getting Started** — [installation](docs/getting-started/installation.md), [configuration](docs/getting-started/configuration.md), [proxy formats](docs/getting-started/proxy-formats.md)
- **Tools**, in menu order — [IP Uniqueness Test](docs/tools/ip-uniqueness-test.md), Site Request Test ([Ticketmaster](docs/tools/tm-request-tester.md), [Bayern](docs/tools/bayern-tester.md)), Monitor ([Downtime](docs/tools/proxy-monitor.md), [Session](docs/tools/session-monitor.md)), [Ping Test](docs/tools/ping-test.md), [Randomize File](docs/tools/randomize-file.md), [Proxy Parser](docs/tools/proxy-parser.md), [Compare Results](docs/tools/compare-results.md)
- **Reference** — [exporting results](docs/reference/exporting-results.md), [auto-update](docs/reference/auto-update.md), [building from source](docs/reference/building-from-source.md)
- **[Troubleshooting](docs/troubleshooting.md)**
