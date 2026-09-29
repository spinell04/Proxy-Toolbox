# Installation

## 1. Download the binary

Pick the binary for your platform from the project root:

| Platform | Binary |
|----------|--------|
| Apple Silicon (M1/M2/M3/M4) | `proxytoolbox-mac-AppleSiliconCPU` |
| Intel Macs | `proxytoolbox-macOS-IntelCPU` |
| Windows | `proxytoolbox.exe` |

> On macOS, you may need to run `chmod +x proxytoolbox-mac-AppleSiliconCPU` the first time, and approve it in **System Settings → Privacy & Security** if Gatekeeper blocks it.

## 2. Folder layout

Place the binary in a folder alongside these items:

```
Proxy-Toolbox/
├── proxytoolbox-mac-AppleSiliconCPU   (or whichever binary)
├── config.txt
├── proxyfiles/
│   ├── my-list.txt
│   └── another-list.txt
└── results/            (auto-created when you export)
```

The binary looks for `proxyfiles/` and `config.txt` **relative to its own location** — not the current working directory. So as long as they sit next to each other, it works from anywhere.

## 3. Prepare a proxy file

Drop any `.txt` file into `proxyfiles/`. One proxy per line. See [Proxy Formats](proxy-formats.md) for what's supported.

```
# proxyfiles/my-list.txt
1.2.3.4:8080:user:pass
5.6.7.8:8080:user:pass
```

## 4. Run it

From a terminal:

```bash
./proxytoolbox-mac-AppleSiliconCPU
```

Or on Windows, double-click `proxytoolbox.exe`.

On first run the toolbox creates what it needs and names the directory it used:

```
Created proxyfiles/, results/, config.txt in /Users/you/proxy-toolbox
```

| Created | Purpose |
|---|---|
| `proxyfiles/` | Your proxy lists. Put `.txt` files here — every tool reads its input from this folder. |
| `results/` | Exported CSVs and monitor logs. |
| `config.txt` | Settings, fully commented. See [Configuration](configuration.md). |

Anything already present is left alone: **`config.txt` is never overwritten**, so your settings survive every later run and every upgrade. Later runs print nothing.

Paths resolve against the **executable's own folder**, not the shell's working directory, so a binary behaves the same double-clicked or run from a terminal.

Building from source is the one exception: under `go run .` the binary lives in Go's build cache, so the toolbox falls back to the **working directory** instead. The startup line names whichever it used, so there is no guessing.

You'll land on the main menu:

```
Proxy Toolbox
> IP Uniqueness Test — Check exit IPs, detect duplicates
  Site Request Test  — Full page requests to Ticketmaster or Bayern
  Monitor            — Downtime and session monitoring
  Ping Test          — Ping a domain through proxies
  Randomize File     — Shuffle proxy order in a file
  Proxy Parser       — Convert proxy format in a file
  Compare Results    — Local dashboard for comparing exported CSVs
  Exit
```

Use **↑ / ↓** to navigate and **Enter** to select.

**Site Request Test** and **Monitor** open submenus rather than running straight away:

```
Site Request Tester          Monitor
> Ticketmaster               > Downtime monitor
  Bayern                       Session monitor
  Back                         Back
```
