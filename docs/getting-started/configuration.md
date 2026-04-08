# Configuration

All settings live in `config.txt` next to the binary. If the file is missing, defaults are used and a warning is printed.

## Example

```
# ─── Proxy Tools Config ───────────────────────────────
# Number of parallel workers (applies to all tools)
workers=40

# Default domain for Ping Test (optional)
# Formats:
#   google.com          -> TCP connect only
#   http://google.com   -> full HTTP request
#   https://google.com  -> full HTTPS request
domain=google.com
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

Applies to **all tools** — IP Uniqueness Test, Ping Test, and TM Request Tester.

### `domain`

Default domain for the **Ping Test** tool. When you run Ping Test, the prompt will pre-fill this value; press Enter to use it or type a different one.

The URL scheme controls the test mode:

| Value | Mode |
|-------|------|
| `google.com` | Raw TCP connect (fastest, just tests if the proxy can reach the host) |
| `http://google.com` | Full HTTP GET request through the proxy |
| `https://google.com` | Full HTTPS: CONNECT + TLS handshake + GET request |

If `domain` is empty or missing, Ping Test will ask you to type one each time.

## Comments and blank lines

Lines starting with `#` and blank lines are ignored, so feel free to leave notes for yourself.
