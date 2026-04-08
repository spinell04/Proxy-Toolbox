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

## Setting a default domain

You can set a `domain` in [`config.txt`](../getting-started/configuration.md) to pre-fill the prompt:

```
domain=https://google.com
```

When you run Ping Test, press **Enter** to use it, or type something different.

## Reading the output

```
#      Host                      Latency     Status
-------------------------------------------------------
1      1.2.3.4                   320ms       HTTP 200
2      5.6.7.8                   410ms       HTTP 200
3      9.10.11.12                -           ERROR  proxy connect: i/o timeout
...

Proxies tested   : 1000
Successful       : 985
Errors           : 15
Total time       : 42.1s
Average latency  : 384ms
```

- **Status** shows `OK` for raw TCP success, or `HTTP <code>` for HTTP/HTTPS modes.
- **ERROR** lines include a shortened reason (timeout, CONNECT rejected, DNS failure, etc.).
- **Average latency** is the mean across successful requests only.

## CSV export

The export has the summary on top followed by per-proxy rows:

```
Summary,Value
Proxies tested,1000
Successful,985
Errors,15
Total time,42.1s
Average latency,384ms
,
#,Host,Latency,Status,Error
1,1.2.3.4,320ms,HTTP 200,
2,5.6.7.8,410ms,HTTP 200,
3,9.10.11.12,,ERROR,proxy connect: i/o timeout
...
```

See [Exporting Results](../reference/exporting-results.md) for details.
