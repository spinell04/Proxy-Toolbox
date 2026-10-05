# Proxy Formats

All tools auto-detect the input format, so you don't need to reformat files before using them. One proxy per line. Lines starting with `#` and blank lines are skipped.

## Supported formats

| Format | Example |
|--------|---------|
| `host:port:user:pass` | `1.2.3.4:8080:admin:secret` |
| `user:pass:host:port` | `admin:secret:1.2.3.4:8080` |
| `user:pass@host:port` | `admin:secret@1.2.3.4:8080` |
| `http://user:pass@host:port` | `http://admin:secret@1.2.3.4:8080` |

The **[Proxy Parser](../tools/proxy-parser.md)** tool can convert between any of these, plus strip auth to `host:port`.

## How auto-detection works

1. **If the line contains `@`** — it's parsed as a URL (with or without the `http://` prefix), using Go's standard `net/url` parser.
2. **Otherwise** — the line is split into 4 colon-separated parts. To decide between `host:port:user:pass` and `user:pass:host:port`, the parser checks the third part: if it contains a dot (like an IP or hostname) and the first part doesn't, it's treated as `user:pass:host:port`.

This means a proxy like `admin:secret:1.2.3.4:8080` is correctly read as user=`admin`, pass=`secret`, host=`1.2.3.4`, port=`8080`.

## Direct lines (no proxy)

A proxy file can also contain a line that means **make this request with no proxy**. The request goes out from this machine's own connection, under the same target, timeout and `workers` pool as every proxy in the file — so the result sits in the same table as the proxies, measured the same way.

### The three spellings

Matched case-insensitively against the whole trimmed line:

```
direct
localhost
localhost:localhost:localhost:localhost
```

Nothing else is a direct line. All three work because none of them is usable as an address: `direct` and `localhost` have too few colon-separated fields to parse at all, and the long form parses into a port of `localhost`, which no URL parser accepts.

### A line naming a port is still a proxy

This is the part worth reading before you edit a file. People run Squid, Charles and mitmproxy on loopback, and those lines have to keep meaning what they say:

| Line | What it means |
|---|---|
| `localhost:8080:user:pass` | the local proxy on 8080 |
| `127.0.0.1:8080:user:pass` | the local proxy on 8080 |
| `user:pass@localhost:8080` | the local proxy on 8080 |

If a line names a port, it is an address, and it is sent through that address. Loopback is not special.

Near-misses are not direct lines and are not proxies either, so they are rejected as invalid lines like any other unparseable text:

```
localhost:8080
localhost:localhost
directly
direct:direct:direct:direct
notlocalhost
```

### What a direct line measures in each tool

| Tool | A direct line measures |
|---|---|
| [Ping Test](../tools/ping-test.md) (raw TCP) | one TCP hop to the target instead of two |
| [Ping Test](../tools/ping-test.md) (HTTP/HTTPS) | the request from this machine's own connection |
| Site Request Test ([Ticketmaster](../tools/tm-request-tester.md) / [Bayern](../tools/bayern-tester.md)) | the page fetched from this machine's own IP |
| [IP Uniqueness Test](../tools/ip-uniqueness-test.md) | this machine's own public exit IP |
| [Session Monitor](../tools/session-monitor.md) | alerts when this machine's own IP changes |
| [Downtime Monitor](../tools/proxy-monitor.md) | reachability of the target with no proxy |

### How it appears

A direct entry shows **its own line, lowercased**, everywhere a proxy is shown: the live tables, the `Ctrl+C` summaries, the CSV `Proxy` column, the [Compare Results](../tools/compare-results.md) dashboard, Discord embeds and `results/monitor.log`.

```
#   Proxy                          Exit IPv4      Latency
---------------------------------------------------------
1   direct                         88.4.201.3     41ms
2   user1:pw@gw.example.com:9000   45.12.8.7      412ms
```

It is the spelling from the file rather than a fixed token, so a run shows the word the user wrote. Lowercasing is there so `Direct` and `direct` are one thing.

### Consequences, taken deliberately

- **The three spellings do not join each other in Compare Results.** A run written with `direct` and a run written with `localhost` are two rows, not one. The dashboard reports what the file said.
- **The long form is 39 characters.** The Session Monitor [never truncates its proxy column](../tools/session-monitor.md#the-proxy-column-is-never-truncated) and sizes it to the widest entry in the file, so one `localhost:localhost:localhost:localhost` line widens that whole table for the run. `direct` and `localhost` are 6 and 9 characters.
- **Direct requests leave from your own IP.** A file of direct lines with `workers=100` is 100 concurrent requests from one address to whatever the target is.
- **A direct line survives a format conversion.** The [Proxy Parser](../tools/proxy-parser.md) passes it through verbatim — it has no address to reformat, and running it through a formatter would emit `:::` from four empty fields and delete the baseline.
- **A proxy reporting the same exit IP as a direct line is not proxying.** The IP Uniqueness Test's duplicate detection already reports that, with line numbers, because a direct line is an ordinary row to it.
- **One spelling is genuinely ambiguous.** Someone running a local proxy might write bare `localhost` meaning "my local proxy" and expect a default port to be inferred. That line used to be rejected outright and is now a silent baseline measurement. There is no way to tell the two intents apart without a port; `localhost:3128` is still read as a proxy.
- **`HTTP_PROXY` and `HTTPS_PROXY` are ignored for direct lines.** The environment is not consulted. Routing a "direct" baseline through a corporate proxy would make the measurement untrue with nothing on screen to indicate it.

## Comments and blank lines

```
# This is a comment — ignored
1.2.3.4:8080:user:pass

# Blank lines above and below are fine
5.6.7.8:8080:user:pass
```

## Invalid lines

Lines that don't match any format are silently skipped. If you run a tool and see `No valid proxies found`, it means every line was rejected — double-check the format.
