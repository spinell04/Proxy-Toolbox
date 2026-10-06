# Configuration

All settings live in `config.txt` next to the binary. If the file is missing, defaults are used and a warning is printed.

## Where config.txt lives, and why it is not in git

`config.txt` sits **beside the executable** and is written on first run from a template compiled into the binary. It is listed in `.gitignore`.

That is not tidiness. `discord_webhook` is a **bearer credential**: anyone holding the URL can post to your channel. A credential in a commit is permanent — rewriting history does not un-leak it, because the commit may already have been cloned, forked or mirrored.

| | |
|---|---|
| `config.txt` | Your live settings. Gitignored. Written on first run, never overwritten — not by a later run, not by an upgrade. |
| `internal/bootstrap/config.default.txt` | The tracked template compiled into the binary. Credentials ship blank; every other key ships its built-in default. |

**Every change to the config must land in both.** A key added to the parser without a line in the template is invisible to new installs — and invisible in testing too, because your own `config.txt` already exists and is never rewritten. See [Changing a config key](../reference/building-from-source.md#changing-a-config-key); three tests enforce it.

Two guards keep it that way:

* **`scripts/pre-commit`** blocks a commit whose staged files contain a webhook-shaped URL. Install it with `ln -sf ../../scripts/pre-commit .git/hooks/pre-commit`.
* **`TestNoCredentialsInTrackedFiles`** scans every file git actually tracks, so it catches a secret committed *before* the hook was installed — the case the hook cannot see.

> **`.gitignore` has no effect on a file that is already tracked.** Adding a name to `.gitignore` after the fact does not stop it being committed; `git rm --cached <file>` does. This is why the test asks git for the tracked set rather than walking the directory.

If a credential does reach a commit, **rotate it**. Deleting the line is housekeeping; rotating is the only step that revokes access.

## Example

```
# ─── Proxy Tools Config ───────────────────────────────
# Number of parallel workers (applies to all tools)
workers=40

# Default domain for Ping Test / Downtime Monitor (optional)
# Formats:
#   google.com          -> TCP connect only
#   http://google.com   -> full HTTP request
#   https://google.com  -> full HTTPS request
domain=google.com

# ─── Exit IP lookups ──────────────────────────────────
# Which address families the IP Uniqueness Test and the Session
# Monitor ask for: ipv4 | ipv6 | both. Both tools also prompt.
# `both` is two lookups per check instead of one.
ip_mode=ipv4

# ─── Monitor intervals (ms) ───────────────────────────
# Gap between individual proxy checks (not between cycles).
# Blank / invalid / 0 = built-in default.
monitor_interval_ms=1000
session_interval_ms=60000

# ─── Monitor: Discord alerts ──────────────────────────
# Webhook URL for monitor alerts (leave empty to disable)
discord_webhook=
# Consecutive fleet failures before a DOWN alert (default 3)
discord_down_threshold=3
# Consecutive successes before a RECOVERED alert (default 2)
discord_up_threshold=2

# ─── Save-proxy latency filters (ms) ──────────────────
# When saving filtered proxies to proxyfiles/, keep only proxies
# under this latency. Blank / invalid / 0 = no filter (save all).
ping_max_latency_ms=
tm_max_latency_ms=
bayern_max_latency_ms=

# ─── Auto-update ──────────────────────────────────────
# On startup, check GitHub for a newer release, install it and
# restart. The download is verified against the release's SHA-256
# before anything is replaced, and a failed check never stops the
# toolbox from starting.
#   on  = stay current automatically (recommended)
#   off = never check; pin whatever binary you have
auto_update=on

# ─── Latency measurement ─────────────────────────────
# Whether a DNS lookup counts toward the reported latency.
#   off  Resolve before timing, then connect to the address. This is what
#        `ping` reports, and what makes two proxies comparable: the lookup is
#        paid once per host, so counting it charges one check for a cost every
#        later check avoided.
#   on   Time the lookup too, as a client resolving on every request would.
# Site Request Test always includes it; see docs/tools/ping-test.md.
measure_dns=off
```

## Options

### `workers`

How many proxies are tested in parallel. Higher = faster, but you'll hit diminishing returns and your own network/CPU limits.

| Value | When to use |
|-------|-------------|
| **10–20** | Slow connection or tiny proxy lists |
| **30–50** | Typical sweet spot for most lists |
| **80+** | Fast machine + fast connection + large lists |

Default if unset: `20`.

Applies to the parallel tools — IP Uniqueness Test, Ping Test, TM Request Tester, Bayern Tester, and the Session Monitor. (The Downtime Monitor checks proxies sequentially and ignores `workers`.)

### `domain`

Default domain for the **Ping Test** and **Downtime Monitor** tools. When you run either, the prompt pre-fills this value; press Enter to use it or type a different one.

The URL scheme controls the test mode:

| Value | Mode |
|-------|------|
| `google.com` | Raw TCP connect (fastest, just tests if the proxy can reach the host) |
| `http://google.com` | Full HTTP GET request through the proxy |
| `https://google.com` | Full HTTPS: CONNECT + TLS handshake + GET request |

If `domain` is empty or missing, the tool asks you to type one each time.

### `ip_mode`

Which **address families** the exit-IP lookups ask for. Applies to the [IP Uniqueness Test](../tools/ip-uniqueness-test.md) and the [Session Monitor](../tools/session-monitor.md); both prompt at the start of a run with this as the pre-filled default, exactly like `domain`.

| Value | Endpoints asked | Lookups per check |
|-------|-----------------|-------------------|
| `ipv4` | IPv4-only reflection endpoints | 1 |
| `ipv6` | IPv6-only reflection endpoints | 1 |
| `both` | both sets | **2** |

Blank, misspelled, or anything unrecognised falls back to `ipv4`.

**Why the default is `ipv4`.** Well-defined beats permissive, and IPv4 is what most target sites see. It also matters that the endpoint sets are single-family: the tools used to ask dual-stack services, and a dual-stack proxy answered with its IPv4 exit from one endpoint and its IPv6 exit from the next. The same proxy reported two different exits in the same second, which made the uniqueness count meaningless and the session monitor alert continuously.

**Why `both` costs double.** It is two HTTP round trips per check instead of one, against free third-party endpoints. For the IP Uniqueness Test that roughly doubles the run time; for the Session Monitor it doubles the sustained request rate, and the startup banner's `Load : ~N IP lookups per minute` line reflects that. Lengthen the interval to pay for it.

**A proxy that answers only one family is not broken.** In `both` mode a check succeeds if either family answered, and the family that did not is recorded as *unknown for that check* rather than as a change. See [Session Monitor](../tools/session-monitor.md#address-families-and-why-one-going-quiet-is-silent) for why that distinction is load-bearing.

## Monitor intervals

Each key is the **prompt default** for one monitor — you can still type a different value at runtime. Blank, non-numeric, or non-positive falls back to the built-in default.

| Key | Tool | Default |
|-----|------|---------|
| `monitor_interval_ms` | [Downtime Monitor](../tools/proxy-monitor.md) | `1000` |
| `session_interval_ms` | [Session Monitor](../tools/session-monitor.md) | `60000` |

The two are **not** the same unit.

`monitor_interval_ms` is the gap between individual checks — the Downtime Monitor walks the list one proxy at a time, so with N proxies each is revisited every `N × interval`.

`session_interval_ms` is the gap between **cycles**, and a Session Monitor cycle checks every proxy in parallel. The interval is therefore the sampling period directly, but the request volume scales with the list: `proxies × lookups per check ÷ interval` lookups per minute against free third-party IP endpoints. 100 proxies at `60000` is 100 lookups a minute in `ipv4` or `ipv6` mode, and 200 in `both`. The startup banner prints both figures.

The session default is far higher because each check is a full HTTP round trip to an IP-echo endpoint, while a downtime check is a single reachability probe.

## Monitor: Discord alerts

`discord_webhook` is shared by both monitors. The threshold keys below apply to the [Downtime Monitor](../tools/proxy-monitor.md) only — the [Session Monitor](../tools/session-monitor.md) uses a fixed per-proxy 5-and-30 failure ladder that is not configurable.

### `discord_webhook`

Discord webhook URL. When set, the Downtime Monitor posts **DOWN** and **RECOVERED** embeds on fleet-level state changes, and the Session Monitor posts per-proxy **ROTATED**, **FAILING** and **RECOVERED** embeds. Leave it blank to disable alerts and run fully local.

### `discord_down_threshold`

How many consecutive fleet check-failures must occur before a single DOWN alert is sent. Default `3`. Raising it makes alerts less twitchy on flaky pools.

### `discord_up_threshold`

After a DOWN, how many consecutive successes are needed before a RECOVERED alert is sent. Default `2`.

## Saving filtered proxies: latency thresholds

These keys gate which proxies get written when you save a filtered list to `proxyfiles/` (see [Exporting Results → Saving filtered proxies](../reference/exporting-results.md#saving-filtered-proxies)).

| Key | Tool | Effect |
|-----|------|--------|
| `ping_max_latency_ms` | [Ping Test](../tools/ping-test.md) | Save successful proxies faster than this (ms) |
| `tm_max_latency_ms` | [TM Request Tester](../tools/tm-request-tester.md) | Save `200 OK` proxies faster than this (ms) |
| `bayern_max_latency_ms` | [Bayern Tester](../tools/bayern-tester.md) | Save `200 OK` proxies faster than this (ms) |

A proxy is kept only if its latency is **strictly below** the threshold. Leave a key **blank** (or set `0` / a non-number) to disable that filter — every successful proxy is then offered for saving. The IP Uniqueness Test saves by unique exit IP instead and has no latency key.

## Auto-update

### `auto_update`

Whether the binary checks GitHub for a newer release on startup, installs it and restarts. Default `on`.

| Value | Effect |
|-------|--------|
| `on`, `true`, `yes`, `1` | Check on every launch and install anything newer |
| `off`, `false`, `no`, `0` | Never check; stay on the binary you have |

Case does not matter. **Anything unrecognised takes the default, which is `on`** — unlike `ip_mode`, where a misspelling falls back to a safe value, a misspelling here must not silently disable updates. A binary that stopped updating because of a typo looks exactly like one that is up to date.

Turn it **off** for two reasons:

- **A release broken on your machine.** Without the switch, every launch reinstalls it.
- **A long-running monitor.** An update restarts the process, and a restart loses the in-memory history the `Ctrl+C` summary is built from. A box running the [Session Monitor](../tools/session-monitor.md) for days should not restart underneath you.

This is the one key read **before** `config.txt` is created on a first ever run — the update check happens before anything else in `main`, because a successful update re-execs and anything done first would be done twice. An absent or unreadable file yields the default rather than a warning, so whether the toolbox starts never hinges on reading an optional setting.

`config.txt` itself is never touched by an update, and neither are `proxyfiles/` or `results/`. See [Auto-Update](../reference/auto-update.md) for the full launch sequence, the `.old` rollback file, and why the checksum is verified before the running binary is renamed.

## Latency measurement

### `measure_dns`

Whether a **DNS lookup** counts toward the latency a tool reports. Default `off`.

| Value | Effect |
|-------|--------|
| `off`, `false`, `no`, `0` | Resolve before the clock starts, then time the connect alone |
| `on`, `true`, `yes`, `1` | Time the lookup too, as a client resolving on every request would |

Case does not matter, and anything unrecognised takes the default, like every other boolean key.

With `off`, a run resolves **every unique address it is about to dial before it measures anything**, and each check then dials an address out of that cache:

```
before the run   resolve every unique address the run will dial   (untimed)
during the run   every check dials an IP from the cache           (no lookup)
```

Four details follow from that:

- The lookup is **one per host, not one per proxy.** 100 proxies behind a single gateway cost one resolution.
- The cache lives for **one run**. A monitor left running for days keeps the addresses it started with; restarting it picks up a rotated DNS record.
- An **IP literal** in the proxy file is never looked up at all.
- A name that **will not resolve is not fatal.** The address stays a hostname, the check dials it by name, and that one check includes the lookup. A broken name then fails at dial time with the error it always had, rather than failing the whole run before any results exist.

Which address gets resolved depends on the line. For a **proxied** line it is the **proxy's gateway** — the target is resolved by the proxy, on its own time. For a **direct** line it is the target itself.

The difference is not cosmetic. Against a local listener:

```
measure_dns=on    reported latency : 41.04ms
measure_dns=off   reported latency : 256µs
```

**Why the default is `off`.** The lookup is paid **once per host**, but latency is reported **per proxy**. Counting it charges one check for a cost the other ninety-nine avoided, so one row in the table carries a number the rest of the file does not. `off` also lines the reported figure up with what `ping` prints, since `ping` resolves once before printing anything and times only the echo — see [Ping Test](../tools/ping-test.md#what-the-reported-latency-covers).

### Which tools it applies to

| Tool | DNS in the reported latency |
|---|---|
| [Ping Test](../tools/ping-test.md) (TCP / HTTP / HTTPS) | Excluded by default; `measure_dns` controls it |
| [IP Uniqueness Test](../tools/ip-uniqueness-test.md) | Excluded by default; `measure_dns` controls it |
| [Downtime Monitor](../tools/proxy-monitor.md) | Excluded by default; `measure_dns` controls it |
| [Session Monitor](../tools/session-monitor.md) | Excluded by default; `measure_dns` controls it |
| [TM Request Test](../tools/tm-request-tester.md) | **Always included — `measure_dns` has no effect** |
| [Bayern Tester](../tools/bayern-tester.md) | **Always included — `measure_dns` has no effect** |

The two Site Request Test tools go through the [`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) library, whose dialer option controls *how* to dial, not *what address*. Excluding the lookup there would mean putting the IP in the URL plus an SNI override and a hand-built `Host` header — which breaks the moment a redirect crosses hosts, in the two tools whose whole purpose is looking like a real browser.

It is also the more defensible half of the split. Those two measure a **page fetch**, where a browser pays for resolution too. The other four measure **reach**, where the lookup is overhead.

> **Latency numbers in those four tools are lower than they were before this key existed.** A CSV exported before the change and one exported after are not comparable on absolute latency. See [Compare Results](../tools/compare-results.md#runs-from-before-and-after-dns-was-taken-out-of-the-latency).

## Comments and blank lines

Lines starting with `#` and blank lines are ignored, so feel free to leave notes for yourself.
