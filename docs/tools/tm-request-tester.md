# TM Request Tester

## What it does

Sends a full, browser-like TLS request to a Ticketmaster region through each proxy. Unlike the [Ping Test](ping-test.md), this uses a realistic Chrome TLS fingerprint — so if a proxy is going to get blocked by Ticketmaster's bot protection, you'll see it here.

## Why it's different from Ping Test

A generic HTTPS request is easy for Ticketmaster (and Cloudflare, Akamai, etc.) to fingerprint as "not a real browser." This tool uses [`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) to replicate Chrome's TLS handshake, along with a matching User-Agent and header order. That gets you much closer to real-browser behaviour, so the results reflect how a proxy will actually perform in practice.

## Supported regions

When you start the tool, you pick a region from the menu:

| Code | Region | URL |
|------|--------|-----|
| **US** | United States | `https://www.ticketmaster.com` |
| **UK** | United Kingdom | `https://www.ticketmaster.co.uk` |
| **ES** | Spain | `https://www.ticketmaster.es` |
| **DE** | Germany | `https://www.ticketmaster.de` |
| **NL** | Netherlands | `https://www.ticketmaster.nl` |
| **CA** | Canada | `https://www.ticketmaster.ca` |
| **MX** | Mexico | `https://www.ticketmaster.com.mx` |

Pick the region that matches where your proxies are geo-located — testing German proxies against `ticketmaster.com` (US) will give bad results.

## Reading the output

```
#      Host                      Speed       Status
-------------------------------------------------------
1      1.2.3.4                   520ms       200 OK
2      5.6.7.8                   480ms       200 OK
3      9.10.11.12                910ms       403 BLOCKED
4      13.14.15.16               —           ERROR  timeout
...

  Total proxies  : 1000
  Working        : 800
  Blocked (403)  : 150
  Failed         : 50

  ── Speed Stats (full TLS request) ──
  Average        : 520ms
  Median (p50)   : 480ms
  p95            : 1200ms
  Fastest        : 180ms
  Slowest        : 3500ms
```

### Status interpretation

| Status | Meaning |
|--------|---------|
| **`200 OK`** (green) | Proxy successfully completed a TLS request; Ticketmaster served the page. This is what you want. |
| **`403 BLOCKED`** (yellow) | The request went through, but Ticketmaster's bot protection flagged the proxy. Usable for some flows, but compromised for purchases/queue. |
| **`ERROR`** (red) | Network-level failure: timeout, connection refused, TLS handshake failure, etc. Either the proxy is dead or it doesn't support HTTPS properly. |

### Stats explained

- **Average** — arithmetic mean of all successful (200) latencies
- **Median (p50)** — middle value; less sensitive to outliers than average
- **p95** — 95% of successful requests were faster than this; useful for understanding the tail
- **Fastest / Slowest** — extremes; if the slowest is huge, you probably have a handful of bad proxies dragging stats

## CSV export

```
Summary,Value
Total proxies,1000
Working,800
Blocked (403),150
Failed,50
Average,520ms
Median (p50),480ms
p95,1200ms
Fastest,180ms
Slowest,3500ms
,
#,Host,Speed,Status,Error
1,1.2.3.4,320ms,200 OK,
...
```

See [Exporting Results](../reference/exporting-results.md).
