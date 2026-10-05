# Bayern Tester

The main menu's **Site Request Test** entry opens a submenu with two testers:

* **Ticketmaster** — [full request to a Ticketmaster region](tm-request-tester.md)
* **Bayern** — full request to the FC Bayern ticket shop (this page)
* **Back** — return to the main menu

Both send a full page request through every proxy, which is what separates them from the [Ping Test](ping-test.md): they measure a real fetch against real bot protection, not a connect.

## What it does

Sends a full, browser-like TLS request to the FC Bayern ticket shop (`https://fcbayern.com/de/tickets`) through each proxy. It's the same engine as the [TM Request Tester](tm-request-tester.md), pointed at a single fixed target instead of a region menu.

## Why a dedicated tool

FC Bayern's ticket shop sits behind the same kind of bot protection as Ticketmaster. A generic HTTPS request is trivially fingerprinted as "not a real browser." This tool uses [`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) to replicate a **Chrome 133** TLS handshake, with a matching User-Agent and header order. The status it records is the one the live shop returned to that request.

The target is hard-coded:

| Target | TLS profile |
|--------|-------------|
| `https://fcbayern.com/de/tickets` | Chrome 133 (tlsclient) |

There's no region prompt — just pick your proxy file and go.

## Reading the output

```
─────────────────────────────────────────────────────────────
  Target  : https://fcbayern.com/de/tickets
  Proxies : 1000
  Workers : 40
  TLS     : Chrome 133 (tlsclient)
─────────────────────────────────────────────────────────────

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
| **`200 OK`** (green) | The proxy completed the TLS request and the shop served the page. |
| **`403 BLOCKED`** (yellow) | The connection and TLS handshake succeeded, but the shop returned 403 rather than the page. |
| **`ERROR`** (red) | No response was recorded — timeout, connection refused, TLS handshake failure. The error text is reported as received; a single failed request does not separate an unreachable proxy from one that does not tunnel HTTPS, or from a transient fault. |

### Stats explained

- **Average** — mean of all successful (200) latencies
- **Median (p50)** — middle value; less skewed by outliers than the average
- **p95** — 95% of successful requests came back in this time or less; shows the slow tail
- **Fastest / Slowest** — extremes across successful requests

Percentiles use the **nearest-rank** method: the successful latencies are sorted and the value at rank `ceil(p × n)` is reported. Nothing is interpolated, so a reported p50 or p95 is always a latency some real proxy recorded. The [compare dashboard](compare-results.md) uses the same method, so its figures match these.

## CSV export

Same layout as the other tools — five metadata rows describing the run, then the summary, then the per-proxy rows:

```
Summary,Value
Tool,bayerntester
Run at,2026-09-19T15:11:40Z
Proxy file,residential.txt
Target,https://fcbayern.com/de/tickets
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

After the CSV prompt, Bayern Tester also offers to save the **working proxies** to a `.txt` in `proxyfiles/`, filtered by latency. The threshold comes from `bayern_max_latency_ms` in [`config.txt`](../getting-started/configuration.md):

- Only `200 OK` proxies are considered (errors and `403 BLOCKED` are dropped).
- A proxy is saved if its latency is **below** `bayern_max_latency_ms`.
- Leave the key blank (or `0`/invalid) to disable the filter — every working proxy is saved.

Proxies are written in their original line format, ready to feed straight back into the toolbox. See [Exporting Results → Saving filtered proxies](../reference/exporting-results.md#saving-filtered-proxies).
