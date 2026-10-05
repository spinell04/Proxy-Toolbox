# Troubleshooting

Common errors and what they mean.

## `No valid proxies found.`

Every line in the file failed to parse. Check:

- **Line format** — see [Proxy Formats](getting-started/proxy-formats.md). The toolbox auto-detects between 4 formats, but anything else is rejected.
- **Extra whitespace or weird characters** — sometimes exports from provider dashboards include invisible characters, trailing commas, or quotes. Open the file in a plain-text editor and clean it up.
- **Wrong number of colons** — `host:port` alone (with no auth) isn't accepted by `ParseLine`. Add dummy credentials or use a format with `@`.

## `cannot read proxyfiles/ directory`

The binary can't find a `proxyfiles/` folder. Make sure:

- The folder exists next to the binary
- At least one `.txt` file is inside
- You're running the binary from a location where it can resolve its own path (normally fine for compiled binaries; `go run` falls back to the current working directory)

## All proxies return `ERROR` in Ping Test or Site Request Test

Usually one of:

- **Dead proxies** — test a few manually with curl:

  ```bash
  curl -x http://user:pass@1.2.3.4:8080 https://api.ipify.org
  ```

  If curl times out too, the proxies themselves are the problem.
- **Wrong credentials** — if the provider rotated passwords, refresh your list.
- **IP whitelist** — some providers require your public IP to be allowlisted before the proxies work. Check the provider dashboard.
- **Firewall / antivirus** — corporate firewalls sometimes block outbound proxy connections. Try from a different network.

## `403 BLOCKED` on everything in Site Request Test

The target's bot protection (Ticketmaster, or FC Bayern's shop) flagged the proxies. Possible causes:

- The proxies are public/shared and already on the target's blacklist
- Region mismatch — e.g. using US proxies against `ticketmaster.de` triggers geoblocks (the Bayern Tester always hits the German shop, so non-EU proxies fare worse)
- Datacenter IPs — these targets aggressively block datacenter ranges; residential or mobile proxies perform better

## Direct lines

A direct line means "make this request with no proxy". The three spellings and what each tool measures for one are in [Proxy Formats → direct lines](getting-started/proxy-formats.md#direct-lines-no-proxy).

| Symptom | Cause | What to do |
|---|---|---|
| A bare `localhost` line was measured as a baseline, but you meant the local proxy you have running | `localhost` with no port is one of the three direct spellings. With no port there is nothing to distinguish "my local proxy" from "no proxy", so the line is taken as a baseline | Give it the port: `localhost:3128:user:pass`, or `localhost:3128` in a format with `@`. Any line naming a port is read as a real proxy on loopback |
| A direct row shows a blank, a `:` or some other odd value in a table, CSV, embed or log | Should not happen. A direct line labels itself with its own line, lowercased, in every surface that shows a proxy | [Open an issue](https://github.com/spinell04/Proxy-Toolbox/issues) naming the tool, the surface and the exact line you used |
| One line widened the whole Session Monitor proxy column | `localhost:localhost:localhost:localhost` is 39 characters, and the Session Monitor [never truncates that column](tools/session-monitor.md#the-proxy-column-is-never-truncated) — it sizes it to the widest entry in the file | Nothing is wrong. `direct` and `localhost` are 6 and 9 characters and mean the same thing |

## Monitor → Downtime monitor never sends Discord alerts

If the live feed shows failures but no Discord message arrives:

- **`discord_webhook` is blank** — alerts are disabled. Set the webhook URL in [`config.txt`](getting-started/configuration.md).
- **Threshold not reached** — a DOWN alert only fires after `discord_down_threshold` *consecutive* fleet failures (default 3). Intermittent single failures won't trigger it. The [Session Monitor](tools/session-monitor.md) ignores these keys: its ladder is a fixed per-proxy 5 and 30, and it alerts on every rotation.
- **Bad webhook URL** — test it with `curl`:

  ```bash
  curl -H "Content-Type: application/json" -d '{"content":"test"}' "<your-webhook-url>"
  ```

  A `204` response means the webhook is valid.

## Colors show as `\033[31m...` garbage on Windows

The toolbox detects whether stdout is a terminal and disables ANSI color codes when it isn't. If you're seeing raw codes:

- You might be redirecting output to a file (`> out.txt`) — that's expected and harmless; the file will just contain the codes.
- On classic `cmd.exe`, ANSI support is off by default. Use **Windows Terminal** or **PowerShell** instead.

## The binary doesn't launch on macOS

macOS Gatekeeper may block unsigned binaries. Fix:

```bash
chmod +x proxytoolbox-mac-AppleSiliconCPU
xattr -d com.apple.quarantine proxytoolbox-mac-AppleSiliconCPU
```

Or open **System Settings → Privacy & Security**, scroll down to the blocked binary notice, and click **Allow Anyway**.

## Setup (the installer)

Every outcome ends with `Press Enter to close...`, so the message stays on screen whether the installer succeeded or not. What Setup does on each path is in [Installation](getting-started/installation.md).

| Symptom | Cause | What to do |
|---|---|---|
| `The toolbox is already installed in this folder.` but you wanted the newest version | The installer places a first copy and declines to touch an existing one. It checks for the file before it contacts GitHub | Open the toolbox; it updates itself on launch. Or delete the binary and run Setup again |
| `Could not reach the release: GitHub API returned 404` | No release is published yet, or the request did not reach GitHub | Check the [releases page](https://github.com/spinell04/Proxy-Toolbox/releases/latest) and the network, then run Setup again |
| `Install failed: checksum mismatch …` | The download did not match the digest the release's `SHA256SUMS` publishes for that file | Nothing to clean up — verification happens before placement, so nothing was written. Run Setup again |
| macOS blocks Setup from opening | The browser tagged the download with `com.apple.quarantine` | Right-click the installer → **Open**, or `xattr -d com.apple.quarantine Setup-ProxyToolbox-mac-AppleSiliconCPU` |
| Windows SmartScreen blocks `Setup-ProxyToolbox.exe` | It is unsigned, like every binary in these releases | **More info → Run anyway** |
| `No Proxy Toolbox build is published for …` | Linux and Windows-on-ARM have no published asset, and the installer stops before contacting GitHub rather than installing something that cannot run | [Build from source](reference/building-from-source.md) |

## An `[update]` line at startup

Every auto-update outcome is one line, and then the menu. Nothing here needs fixing — the full mechanism is in [Auto-Update](reference/auto-update.md).

| Symptom | Cause | What to do |
|---|---|---|
| `[update] check failed: GitHub API rate limit reached` | Unauthenticated GitHub allows 60 requests an hour per IP, and a shared or NATed address reaches that without you doing anything | Nothing; it checks again next launch |
| `[update] check failed: …` with no network | Offline, or a captive portal swallowing the connection | Nothing; the 3-second timeout expires and the toolbox starts normally |
| `[update] failed, staying on vX: checksum mismatch` | The download was interrupted or tampered with | Nothing; the running binary was never touched, and it retries next launch |
| A `.old` file beside the binary | The previous version. Normally deleted milliseconds after an update, by the new binary's own startup — so finding one means the new version never started | Rename it back over the new binary, and set `auto_update=off` before relaunching. See [Auto-Update](reference/auto-update.md) |
| The version never changes | `auto_update=off`, or this is a `dev` build | Check `auto_update` in [`config.txt`](getting-started/configuration.md), then check the version in the menu title |
| Menu title says `Proxy Toolbox dev` | Locally built, not a release download | Expected — `dev` builds never auto-update. Download a [release](https://github.com/spinell04/Proxy-Toolbox/releases/latest) if you want one that does |

The success case is two lines and a restart:

```
[update] v1.0.1 -> v1.0.2, downloading...
[update] installed v1.0.2, restarting...
```

If you rolled back by renaming `.old`, set `auto_update=off` **before** relaunching — otherwise the next launch reinstalls the version you just removed. In the ordinary case `.old` is already gone; roll back by downloading an older release instead.

## Tests are slow

Increase `workers` in `config.txt`. 40 is a good starting point; try 60–80 if your machine and network can handle it. See [Configuration](getting-started/configuration.md).
