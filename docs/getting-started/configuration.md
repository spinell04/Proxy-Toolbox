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

## Monitor intervals

Each key is the **prompt default** for one monitor — you can still type a different value at runtime. Blank, non-numeric, or non-positive falls back to the built-in default.

| Key | Tool | Default |
|-----|------|---------|
| `monitor_interval_ms` | [Downtime Monitor](../tools/proxy-monitor.md) | `1000` |
| `session_interval_ms` | [Session Monitor](../tools/session-monitor.md) | `60000` |

The two are **not** the same unit.

`monitor_interval_ms` is the gap between individual checks — the Downtime Monitor walks the list one proxy at a time, so with N proxies each is revisited every `N × interval`.

`session_interval_ms` is the gap between **cycles**, and a Session Monitor cycle checks every proxy in parallel. The interval is therefore the sampling period directly, but the request volume scales with the list: `proxies ÷ interval` lookups per minute against free third-party IP endpoints. 100 proxies at `60000` is 100 lookups a minute. The startup banner prints both figures.

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

## Comments and blank lines

Lines starting with `#` and blank lines are ignored, so feel free to leave notes for yourself.
