# Ping Test

## What it does

Measures reachability and latency from each proxy to a target domain you specify. Works in three different modes depending on the URL scheme — from a raw TCP connect up to a full HTTPS request.

## Three modes, one tool

The mode is auto-selected based on what you type as the target:

| Input | Mode | What it measures |
|-------|------|------------------|
| `google.com` | **Raw TCP** | Can the proxy open a TCP socket to port 80 of the target? Fastest, no HTTP. |
| `http://google.com` | **HTTP** | Full HTTP GET request through the proxy. Includes response status. |
| `https://google.com` | **HTTPS** | CONNECT + TLS handshake + HTTPS GET. Slowest but most realistic. |

### When to use each

- **Raw TCP** — fastest smoke test. Confirms the proxy is alive and can route to a port. No HTTP overhead, so latency numbers are "pure" network RTT.
- **HTTP** — the proxy actually forwards a real HTTP request. You get back a status code and the response time reflects the full round-trip.
- **HTTPS** — closest to real browsing. Tests that the proxy can tunnel TLS (via CONNECT), that the handshake completes, and that the target responds.

## Direct lines

A [direct line](../getting-started/proxy-formats.md#direct-lines-no-proxy) in the file is measured against the same target, timeout and worker pool as the proxies, with no proxy in the path:

- In **raw TCP** mode it is one TCP connect to the target, where a proxied row is two hops — the proxy connect, then the `CONNECT` to the target. A failure on a direct row is reported bare rather than as `proxy connect:`, since there is no proxy to attribute it to.
- In **HTTP** and **HTTPS** modes it is the request made from this machine's own connection.

`HTTP_PROXY` and `HTTPS_PROXY` are not consulted for those rows, so a baseline is never silently routed through a proxy the environment set.

## What the reported latency covers

The latency in the table is a **connect time**, not a connect plus a DNS lookup. Before the first measurement the run resolves every unique address it is about to dial — untimed — and each check then dials an address out of that cache. The lookup is paid once per host, outside the clock.

It did not used to be. The timer wrapped a dial by hostname, and `net.DialTimeout("tcp", "ticketmaster.com:80", …)` resolves *and then* connects, so both landed in the number. That is why the toolbox and `ping` disagreed, and the gap was widest exactly where the network was fastest — on a sub-millisecond path the lookup was the whole figure:

```
ping ticketmaster.com     time<1ms
toolbox, run 1            8ms      <- cold DNS
toolbox, run 2            4ms      <- OS cache warm
```

`ping` resolves once before it prints anything and times only the echo. Excluding the lookup is what puts the two on the same footing, and it is also what makes two rows in the same table comparable: the lookup is paid once per host but latency is reported per proxy, so counting it charges one check for a cost the other ninety-nine avoided.

What gets resolved depends on the line. A **proxied** row resolves the **proxy's gateway** — the target is resolved by the proxy, on its own time. A **direct** row resolves the target itself.

Three things worth knowing:

- **An IP literal is never looked up**, so a file of raw addresses never touches a resolver.
- **A hostname that will not resolve is not fatal.** The address stays a name, that one check dials it by name and therefore includes the lookup, and it fails at dial time with the error it always had.
- **The cache lives for one run.** A long-running monitor keeps the addresses it started with; restarting it picks up a rotated record.

Set `measure_dns=on` in [`config.txt`](../getting-started/configuration.md#measure_dns) to time the lookup as well, the way a client resolving on every request would. Two things follow from the default being `off`:

- The [TM Request Tester](tm-request-tester.md) and [Bayern Tester](bayern-tester.md) **always** include the lookup and `measure_dns` does not change that, so a TM latency and a ping latency are not the same measurement.
- Latencies here are **lower than they were before this existed**. An old CSV and a new one are not comparable on absolute latency — see [Compare Results](compare-results.md#runs-from-before-and-after-dns-was-taken-out-of-the-latency).

## Setting a default domain

You can set a `domain` in [`config.txt`](../getting-started/configuration.md) to pre-fill the prompt:

```
domain=https://google.com
```

When you run Ping Test, press **Enter** to use it, or type something different.

## Reading the output

```
#      Proxy                                         Latency     Status
-------------------------------------------------------------------------
1      admin:secret@1.2.3.4:8080                     320ms       HTTP 200
2      admin:secret@5.6.7.8:8080                     410ms       HTTP 200
3      admin:secret@9.10.11.12:8080                  -           ERROR  proxy connect: i/o timeout
...

Proxies tested   : 1000
Successful       : 985
Errors           : 15
Total time       : 42.1s
Average latency  : 384ms
```

- **Proxy** is the full `user:pass@host:port`, elided in the middle when it is too wide for the column.
- **Status** shows `OK` for raw TCP success, or `HTTP <code>` for HTTP/HTTPS modes.
- **ERROR** lines include a shortened reason (timeout, CONNECT rejected, DNS failure, etc.).
- **Average latency** is the mean across successful requests only.

## CSV export

The export opens with five metadata rows describing the run, then the summary, then the per-proxy rows:

```
Summary,Value
Tool,pinger
Run at,2026-09-19T14:32:07Z
Proxy file,residential.txt
Target,https://google.com
Workers,40
Proxies tested,1000
Successful,985
Errors,15
Total time,42.1s
Average latency,384ms
,
#,Proxy,Latency,Status,Error
1,admin:secret@1.2.3.4:8080,320ms,HTTP 200,
2,admin:secret@5.6.7.8:8080,410ms,HTTP 200,
3,admin:secret@9.10.11.12:8080,,ERROR,proxy connect: i/o timeout
...
```

See [Exporting Results](../reference/exporting-results.md) for details.

## Saving fast proxies

After the CSV prompt, Ping Test offers to save the **successful** proxies to a `.txt` in `proxyfiles/`, filtered by latency. The threshold comes from `ping_max_latency_ms` in [`config.txt`](../getting-started/configuration.md):

- Only proxies that responded (no error) are considered.
- A proxy is saved if its latency is **below** `ping_max_latency_ms`.
- Leave the key blank (or `0`/invalid) to disable the filter — every successful proxy is saved.

Proxies are written in their original line format. See [Exporting Results → Saving filtered proxies](../reference/exporting-results.md#saving-filtered-proxies).
