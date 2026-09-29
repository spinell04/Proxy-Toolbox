# TM Request Tester

The main menu's **Site Request Test** entry opens a submenu with two testers:

* **Ticketmaster** — full request to a Ticketmaster region (this page)
* **Bayern** — [full request to the FC Bayern ticket shop](bayern-tester.md)
* **Back** — return to the main menu

Both send a full page request through every proxy, which is what separates them from the [Ping Test](ping-test.md): they measure a real fetch against real bot protection, not a connect.

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
#      Proxy                                         Speed       Status
─────────────────────────────────────────────────────────────────────────
1      admin:secret@1.2.3.4:8080                     520ms       200 OK
2      admin:secret@5.6.7.8:8080                     480ms       200 OK
3      admin:secret@9.10.11.12:8080                  910ms       403 BLOCKED
4      admin:secret@13.14.15.16:8080                 —           ERROR  timeout
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
- **p95** — 95% of successful requests came back in this time or less; useful for understanding the tail
- **Fastest / Slowest** — extremes; if the slowest is huge, you probably have a handful of bad proxies dragging stats

Percentiles use the **nearest-rank** method: the successful latencies are sorted and the value at rank `ceil(p × n)` is reported. Nothing is interpolated, so a reported p50 or p95 is always a latency some real proxy recorded. The [compare dashboard](compare-results.md) uses the same method, so its figures match these.

## CSV export

Five metadata rows describing the run, then the summary, then the per-proxy rows:

```
Summary,Value
Tool,speedtester
Run at,2026-09-19T15:11:40Z
Proxy file,residential.txt
Target,https://www.ticketmaster.com
Workers,40
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
#,Proxy,Latency,Status,Error
1,admin:secret@1.2.3.4:8080,320ms,200 OK,
2,admin:secret@5.6.7.8:8080,910ms,403 BLOCKED,
3,admin:secret@9.10.11.12:8080,,ERROR,timeout
...
```

The on-screen column is headed **Speed**; in the CSV the same figure is headed **Latency**.

See [Exporting Results](../reference/exporting-results.md).

## Saving fast proxies

After the CSV prompt, TM Request Tester offers to save the **working** proxies to a `.txt` in `proxyfiles/`, filtered by latency. The threshold comes from `tm_max_latency_ms` in [`config.txt`](../getting-started/configuration.md):

- Only `200 OK` proxies are considered (errors and `403 BLOCKED` are dropped).
- A proxy is saved if its latency is **below** `tm_max_latency_ms`.
- Leave the key blank (or `0`/invalid) to disable the filter — every working proxy is saved.

Proxies are written in their original line format. See [Exporting Results → Saving filtered proxies](../reference/exporting-results.md#saving-filtered-proxies).
