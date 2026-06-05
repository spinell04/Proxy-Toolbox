# Configuration

All settings live in `config.txt` next to the binary. If the file is missing, defaults are used and a warning is printed.

## Example

```
# ─── Proxy Tools Config ───────────────────────────────
# Number of parallel workers (applies to all tools)
workers=40

# Default domain for Ping Test / Proxy Monitor (optional)
# Formats:
#   google.com          -> TCP connect only
#   http://google.com   -> full HTTP request
#   https://google.com  -> full HTTPS request
domain=google.com

# ─── Proxy Monitor: Discord alerts ────────────────────
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

Applies to the parallel tools — IP Uniqueness Test, Ping Test, TM Request Tester, and Bayern Tester. (Proxy Monitor checks proxies sequentially per cycle and ignores `workers`.)

### `domain`

Default domain for the **Ping Test** and **Proxy Monitor** tools. When you run either, the prompt pre-fills this value; press Enter to use it or type a different one.

The URL scheme controls the test mode:

| Value | Mode |
|-------|------|
| `google.com` | Raw TCP connect (fastest, just tests if the proxy can reach the host) |
| `http://google.com` | Full HTTP GET request through the proxy |
| `https://google.com` | Full HTTPS: CONNECT + TLS handshake + GET request |

If `domain` is empty or missing, the tool asks you to type one each time.

## Proxy Monitor: Discord alerts

These keys configure the [Proxy Monitor](../tools/proxy-monitor.md) tool only.

### `discord_webhook`

Discord webhook URL. When set, the monitor posts **DOWN** and **RECOVERED** embeds on fleet-level state changes. Leave it blank to disable alerts and run fully local.

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
