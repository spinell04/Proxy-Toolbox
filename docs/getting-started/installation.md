# Installation

## 1. Get the binary

Two routes, both ending in the same place: this platform's toolbox binary sitting in a folder, keeping itself current from then on.

### Route A — Setup

Download one file from the [latest release](https://github.com/spinell04/Proxy-Toolbox/releases/latest):

| Platform | Installer |
|----------|-----------|
| Apple Silicon (M1/M2/M3/M4) | `Setup-ProxyToolbox-mac-AppleSiliconCPU` |
| Intel Macs | `Setup-ProxyToolbox-macOS-IntelCPU` |
| Windows | `Setup-ProxyToolbox.exe` |

Put it in the folder you want the toolbox to live in, and run it. It is ~5.6 MB against the toolbox's ~12.7 MB, because it carries no toolbox inside it — it fetches one. In order:

1. If this platform's toolbox binary is **already in the installer's own folder**, it says so and exits, without contacting GitHub.
2. Otherwise it asks GitHub for `/releases/latest`, downloads this platform's asset, verifies its SHA-256 against the release's `SHA256SUMS` **before anything is placed**, sets the executable bit, and renames it into the folder.
3. It prints the version, the asset and the directory.

**Every path then waits for Enter**, failures included. A double-clicked `.exe` opens a console that closes the instant the process exits, so without that wait the window flashes and the message is gone before anyone reads it.

A run with no toolbox present, against a repository with no release to install from:

```
Proxy Toolbox installer v9.9.9

Could not reach the release: GitHub API returned 404

Press Enter to close...
```

A run beside an existing install:

```
Proxy Toolbox installer v9.9.9

The toolbox is already installed in this folder.
If you want to update it, just open it and it will auto-update.

Press Enter to close...
```

That second message is the installer handing the job to the updater: the toolbox in that folder checks GitHub on every launch, so opening it is what moves it to the newest release. See [Auto-Update](../reference/auto-update.md).

Four properties worth knowing before you keep or discard the installer:

- **It never needs re-downloading.** It resolves `/releases/latest` at run time, so a copy saved a year ago installs whatever is current then — the same property that keeps the updater working across releases. The release it was built in is stamped in its header, for naming it in a support question; it drives nothing.
- **Running it twice is a no-op, not a repair.** The already-installed check looks for the file, not for a working file. If the binary in the folder is corrupt, delete it and run Setup again.
- **A failed install leaves the folder exactly as it was.** Verification precedes placement and the staged `.new` file is removed on every failure path, so there is no partial binary to find or clean up. A release with no `SHA256SUMS` is refused rather than installed unverified.
- **It stops at the binary.** It does not create `proxyfiles/`, `results/` or `config.txt` — the toolbox does that on its own first run — and it does not launch the toolbox.

### Route B — Direct download

Pick the toolbox binary itself from the [latest release](https://github.com/spinell04/Proxy-Toolbox/releases/latest), and drop it in a folder:

| Platform | Binary |
|----------|--------|
| Apple Silicon (M1/M2/M3/M4) | `proxytoolbox-mac-AppleSiliconCPU` |
| Intel Macs | `proxytoolbox-macOS-IntelCPU` |
| Windows | `proxytoolbox.exe` |

On macOS, `chmod +x <binary>` may be needed the first time — the executable bit survives the release archive but not every browser. Setup sets it itself.

### Either route

Each release carries a `SHA256SUMS` asset listing the digest of all six published files — three toolbox binaries and three installers — so anything you downloaded can be checked by hand with `shasum -a 256 <file>`.

**This is a one-time download.** On every launch the toolbox checks GitHub for a newer release and installs it, verifying the download against that `SHA256SUMS` before replacing anything. `config.txt`, `proxyfiles/` and `results/` are never touched. Set `auto_update=off` in `config.txt` to pin the version you have — see [Auto-Update](../reference/auto-update.md).

Linux and Windows-on-ARM have no published binary and no published installer. Setup on those platforms says so and exits without contacting GitHub; on those, [build from source](../reference/building-from-source.md).

### Gatekeeper and SmartScreen

On macOS, a file a browser downloads carries `com.apple.quarantine`, and Gatekeeper blocks it until you right-click → **Open** (or run `xattr -d com.apple.quarantine <file>`). You can also approve it afterwards in **System Settings → Privacy & Security**. This applies to whichever file the browser wrote: the installer on Route A, the toolbox binary on Route B.

A file written by the installer is created by an ordinary process rather than by LaunchServices, so it is **expected** to carry no quarantine attribute and to launch without a prompt — as are the binaries the auto-updater writes afterwards. On that reasoning, Route A meets Gatekeeper once, at Setup, rather than on each toolbox download. **This is reasoned from how the quarantine attribute is applied and has not been tested with a real browser download on a real Mac. Treat it as expected, not verified.**

On Windows, SmartScreen warns about the unsigned `Setup-ProxyToolbox.exe` the same way it warns about the unsigned `proxytoolbox.exe`, and about any unsigned executable: **More info → Run anyway**. Neither route changes that.

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
Proxy Toolbox v1.0.2
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

The title carries the version, which is how you tell what you are running. A locally built binary says `Proxy Toolbox dev` instead, and a `dev` build never auto-updates.

**Site Request Test** and **Monitor** open submenus rather than running straight away:

```
Site Request Tester          Monitor
> Ticketmaster               > Downtime monitor
  Bayern                       Session monitor
  Back                         Back
```
