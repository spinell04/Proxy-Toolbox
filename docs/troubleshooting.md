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

## All proxies return `ERROR` in Ping Test or TM Request Tester

Usually one of:

- **Dead proxies** — test a few manually with curl:

  ```bash
  curl -x http://user:pass@1.2.3.4:8080 https://api.ipify.org
  ```

  If curl times out too, the proxies themselves are the problem.
- **Wrong credentials** — if the provider rotated passwords, refresh your list.
- **IP whitelist** — some providers require your public IP to be allowlisted before the proxies work. Check the provider dashboard.
- **Firewall / antivirus** — corporate firewalls sometimes block outbound proxy connections. Try from a different network.

## `403 BLOCKED` on everything in TM Request Tester

Ticketmaster flagged the proxies. Possible causes:

- The proxies are public/shared and already on Ticketmaster's blacklist
- The region mismatch — using US proxies against `ticketmaster.de` triggers geoblocks
- Datacenter IPs — Ticketmaster aggressively blocks datacenter ranges; residential or mobile proxies perform better

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

## Tests are slow

Increase `workers` in `config.txt`. 40 is a good starting point; try 60–80 if your machine and network can handle it. See [Configuration](getting-started/configuration.md).
