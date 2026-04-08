# Proxy Toolbox

A CLI toolbox for testing, analyzing, and managing proxy lists. Interactive menus with arrow-key navigation.

## Features

| Tool | Description |
|------|-------------|
| **IP Uniqueness Test** | Check exit IPs through each proxy and detect duplicates |
| **Ping Test** | Ping a domain through proxies via TCP, HTTP, or HTTPS |
| **TM Request Tester** | Test proxy speed with a full TLS request to Ticketmaster |
| **Randomize File** | Shuffle the proxy order in a file |
| **Proxy Parser** | Convert proxies between different formats |

## Quick Start

1. Download the binary for your platform from the project root:
   - `proxytoolbox-mac-AppleSiliconCPU` — Apple Silicon (M1/M2/M3/M4)
   - `proxytoolbox-macOS-IntelCPU` — Intel Macs
   - `proxytoolbox.exe` — Windows
2. Place your proxy files (`.txt`) in the `proxyfiles/` folder next to the binary
3. Run the binary and navigate the menu with arrow keys

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
```

## Proxy Formats

All tools auto-detect the input format. Supported formats:

| Format | Example |
|--------|---------|
| `host:port:user:pass` | `1.2.3.4:8080:admin:secret` |
| `user:pass:host:port` | `admin:secret:1.2.3.4:8080` |
| `user:pass@host:port` | `admin:secret@1.2.3.4:8080` |
| `http://user:pass@host:port` | `http://admin:secret@1.2.3.4:8080` |

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

Requires Go 1.25+.

```bash
# Current platform
go build -o proxytoolbox .

# Cross-compile
GOOS=darwin GOARCH=arm64 go build -o proxytoolbox-mac-AppleSiliconCPU .
GOOS=darwin GOARCH=amd64 go build -o proxytoolbox-macOS-IntelCPU .
GOOS=windows GOARCH=amd64 go build -o proxytoolbox.exe .
```

## Export

After running IP Uniqueness Test, Ping Test, or TM Request Tester, you'll be prompted to save results to a CSV file. Exported files are saved to the `results/` folder.
