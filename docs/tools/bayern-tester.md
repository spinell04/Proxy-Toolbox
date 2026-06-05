# Bayern Tester

## What it does

Sends a full, browser-like TLS request to the FC Bayern ticket shop (`https://fcbayern.com/de/tickets`) through each proxy. It's the same engine as the [TM Request Tester](tm-request-tester.md), but pointed at a single fixed target instead of a region menu — useful when you specifically need proxies that can reach the Bayern queue.

## Why a dedicated tool

FC Bayern's ticket shop sits behind the same kind of bot protection as Ticketmaster. A generic HTTPS request is trivially fingerprinted as "not a real browser." This tool uses [`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) to replicate a **Chrome 133** TLS handshake (matching User-Agent and header order), so the results reflect how a proxy will actually behave against the live shop.

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

#      Host                      Speed       Status
───────────────────────────────────────────────────────
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
| **`200 OK`** (green) | Proxy completed the TLS request and the shop served the page. This is what you want. |
| **`403 BLOCKED`** (yellow) | The request went through, but bot protection flagged the proxy. Compromised for the queue/purchase flow. |
| **`ERROR`** (red) | Network-level failure: timeout, connection refused, TLS handshake failure. The proxy is dead or doesn't tunnel HTTPS properly. |

### Stats explained

- **Average** — mean of all successful (200) latencies
- **Median (p50)** — middle value; less skewed by outliers than the average
- **p95** — 95% of successful requests were faster than this; shows the slow tail
- **Fastest / Slowest** — extremes across successful requests

## CSV export

Same "summary on top" layout as the other tools:

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

## Saving fast proxies

After the CSV prompt, Bayern Tester also offers to save the **working proxies** to a `.txt` in `proxyfiles/`, filtered by latency. The threshold comes from `bayern_max_latency_ms` in [`config.txt`](../getting-started/configuration.md):

- Only `200 OK` proxies are considered (errors and `403 BLOCKED` are dropped).
- A proxy is saved if its latency is **below** `bayern_max_latency_ms`.
- Leave the key blank (or `0`/invalid) to disable the filter — every working proxy is saved.

Proxies are written in their original line format, ready to feed straight back into the toolbox. See [Exporting Results → Saving filtered proxies](../reference/exporting-results.md#saving-filtered-proxies).
