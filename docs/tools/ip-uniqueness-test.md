# IP Uniqueness Test

## What it does

Connects to an IP-reflection service through each proxy and records the exit IP returned. It then tells you how many of your proxies actually have distinct IPs — and flags any duplicates with the exact line numbers.

## What the result tells you

A list's line count and its distinct-exit-IP count are two different numbers. They are equal only when every proxy exits from its own address; where addresses are shared, the distinct count is lower.

The test reports both, and names the lines that share an address. What that means for a given list is yours to judge — the tool measures, it does not interpret.

## How it works

For each proxy, the tool makes a GET request to one of a set of IP-reflection
endpoints (rotated to spread load). If one fails, it falls back to the next.

Which set depends on the **IP mode**, chosen from an arrow-key menu at the
start of every run. The `ip_mode` value in
[`config.txt`](../getting-started/configuration.md) is pre-selected, so Enter
accepts it and you only move if you want something else:

```
  IP mode
  From config.txt: ipv4

  > IPv4 only     — Exit IPv4 address, the family most sites see
    IPv6 only     — Exit IPv6 address
    IPv4 and IPv6 — Both, tracked separately (two lookups per check)
```

| mode | endpoints | requests per proxy |
|---|---|---|
| `ipv4` | `api.ipify.org`, `ipv4.icanhazip.com`, `v4.ident.me` | 1 |
| `ipv6` | `api6.ipify.org`, `ipv6.icanhazip.com`, `v6.ident.me` | 1 |
| `both` | both sets | 2 |

Every host in those sets is single-family. That matters more than it looks: the
tool used to ask `ifconfig.me` and `icanhazip.com`, both of which are
dual-stack, so a dual-stack proxy reported its IPv4 exit on roughly one check in
three and its IPv6 exit on the other two. Same proxy, same second, different
answer — and "Unique IPs: 87 / 100" was counted across two address spaces.

`both` **doubles the request volume**: two lookups per proxy instead of one,
against free third-party services. Use it when you need to know both exits;
`ipv4` is the default because it is what most target sites see.

Every answer is parsed as an IP address and checked against the family that was
asked for. An HTTP status other than 200, a body that is not an address (a
throttle page, say) or an address of the wrong family is discarded and the next
endpoint is tried — none of them is recorded as an exit IP.

Tests run in parallel using the `workers` value from
[`config.txt`](../getting-started/configuration.md).

### Direct lines

A [direct line](../getting-started/proxy-formats.md#direct-lines-no-proxy) looks up this machine's own public exit IP, through the same endpoint sets and the same IP mode as every proxy in the file. `HTTP_PROXY` and `HTTPS_PROXY` are not consulted, so the address reported for that row is the one this machine exits from.

It is an ordinary row to the duplicate detection, which is the second thing a direct line is for here: **a proxy whose exit IP matches the direct line's is not proxying.** It appears in the `REPEATED` marker and the **Repeated IPs** section with the direct line's number alongside it, the same as any two proxies sharing an address.

### Repeated lines are spaced one second apart

If the same proxy string appears more than once in the file, its copies are **not** checked in parallel. Each repeat waits one second after the previous one finishes, and the tool prints how many checks that affects before the run starts.

This is because a residential gateway keys its sticky session off the username, not the host. Repeating a line asks for the same session again, so firing the copies simultaneously measures a single instant and tells you nothing about whether the exit rotates. Spacing them turns a repeated line into a probe of one proxy over time, which is the only reason to repeat a line.

Two consequences worth knowing:

- **A file of N copies of one proxy takes at least N − 1 seconds.** The gap sits *between* checks, so the first copy waits for nothing. That is the point, not a regression.
- **A file with no duplicates is unaffected.** Repeats are handled outside the worker pool, so distinct proxies keep running at full width even when a repeated proxy is trickling alongside them.

A proxy is "the same" here when its whole `user:pass@host:port` matches — which is what identifies a session. Two lines sharing a host but carrying different session IDs are different proxies and run in parallel as usual.

## Reading the output

In `ipv4` mode (the default):

```
File    : proxyfiles/residential.txt
Proxies : 1000
Workers : 40
IP mode : IPv4 only

#      Proxy                                 Exit IPv4           Latency
--------------------------------------------------------------------------
1      admin:secret@1.2.3.4:8080             84.56.107.201         412ms
2      admin:secret@5.6.7.8:8080             91.38.203.163         488ms
3      admin:secret@9.10.11.12:8080          84.56.107.201         391ms  *** REPEATED IPv4 x2 (lines: 1)
4      admin:secret@13.14.15.16:8080         77.21.9.18            355ms
5      admin:secret@17.18.19.20:8080         ERROR  proxy connect: i/o timeout
```

In `both` mode the table gains a second address column:

```
#      Proxy                                 Exit IPv4           Exit IPv6                                Latency
-------------------------------------------------------------------------------------------------------------------
1      admin:secret@1.2.3.4:8080             84.56.107.201       2a02:908:1c6:a2c0::1                       412ms
2      admin:secret@5.6.7.8:8080             91.38.203.163       2a02:908:1c6:a2c0::2                       488ms
3      admin:secret@9.10.11.12:8080          84.56.107.201       2a02:908:1c6:a2c0::9                       391ms  *** REPEATED IPv4 x2 (lines: 1)
4      admin:secret@13.14.15.16:8080         77.21.9.18          -                                          355ms
5      admin:secret@17.18.19.20:8080         ERROR  proxy connect: i/o timeout
```

- **#** — line number in the source file
- **Proxy** — the proxy, as `user:pass@host:port`, elided in the middle when it is too wide for the column
- **Exit IPv4 / Exit IPv6** — what the world sees when traffic goes through that proxy, per address family
- **`-`** — that family gave no answer on this check. It is *unknown*, not known to be absent: line 4 may be a proxy with no IPv6 route, or an IPv6 endpoint that happened to time out. A check with at least one address is a success, so a v4-only proxy is not an error.
- **Latency** — round-trip time for the full check, both lookups included in `both` mode
- **ERROR** — no family answered at all. The address and latency columns are dropped and a shortened reason is printed instead.
- **REPEATED IPvN xN (lines: …)** — marker when the same exit address has been seen before *within that family*; shows which earlier lines matched

## Final summary

In `both` mode there are two figures, one per family:

```
Proxies tested   : 1000
Errors           : 5
Unique IPv4      : 941 / 995
Unique IPv6      : 688 / 995
Total time       : 56.337s

[!] Repeated IPs:
    IPv4  84.56.107.201                            3 times  ->  lines: 222, 700, 880
    IPv6  2a02:908:1c6:a2c0::1                     4 times  ->  lines: 12, 88, 341, 902
```

In single-family mode there is one figure, for the family you asked for.

- **Unique IPvN** reads as `unique / answered` — out of the 995 proxies that gave an IPv4 address, only 941 gave distinct ones. The denominator is per family: a proxy that answered v4 and not v6 counts towards the IPv4 figure only, so the IPv6 line is never flattered by proxies that have no IPv6 route.
- Two figures rather than one is the point of `both`. A v6 pool is very often far less diverse than the v4 pool behind the same gateway, and a single number across both address spaces hides exactly that.
- The **Repeated IPs** section lists every duplicated address, the family it belongs to, and the source-file line numbers that share it, so you can delete or request replacements.

## CSV export

After the run, you're prompted to save results to CSV. The export has three sections: the metadata and summary block, the repeated IPs, then one row per proxy.

```
Summary,Value
Tool,iptester
Run at,2026-09-19T14:32:07Z
Proxy file,residential.txt
Target,
Workers,40
IP mode,both
Proxies tested,1000
Errors,5
Unique IPv4,941 / 995
Unique IPv6,688 / 995
Total time,56.337s
,
Repeated IP,Family,Times,Lines
84.56.107.201,IPv4,3,"222, 700, 880"
2a02:908:1c6:a2c0::1,IPv6,4,"12, 88, 341, 902"
,
#,Proxy,Exit IPv4,Exit IPv6,Latency,Error
1,admin:secret@1.2.3.4:8080,84.56.107.201,2a02:908:1c6:a2c0::1,412ms,
2,admin:secret@5.6.7.8:8080,91.38.203.163,,488ms,IPv6: [https://v6.ident.me] dial tcp: no route to host
3,admin:secret@9.10.11.12:8080,,,,proxy connect: i/o timeout
...
```

The address columns are named per family, and only the families the run asked
for appear: an `ipv4` run writes `Exit IPv4` alone. `IP mode` records which mode
produced the file.

Row 2 shows a partial check: IPv4 answered, IPv6 did not, and the reason is
recorded in the `Error` column even though the check itself succeeded. An empty
`Error` with an address present is a clean check.

`Target` is empty because this tool doesn't have one — it asks a reflection service what your exit IP is, rather than testing a destination you chose.

The per-proxy section is what lets [Compare Results](compare-results.md) attach an exit IP to a proxy that also appears in a ping or TM run — which is what makes its cross-tool matrix able to collapse rows by exit IP or exit /24.

Files are saved to `results/` next to the binary. See [Exporting Results](../reference/exporting-results.md) for details.

## Saving unique proxies

After the CSV prompt, IP Uniqueness Test offers to save a **deduplicated** proxy list to a `.txt` in `proxyfiles/`:

- One proxy is kept per distinct exit **identity** — the **first** one seen wins.
- In single-family mode the identity is that one address. In `both` mode it is the **pair**: two proxies are duplicates only when their IPv4 exits *and* their IPv6 exits match.
- That is deliberately conservative. This file is reused as input, and dropping a proxy because it shares one of two exits would throw away something you paid for. A proxy sharing a v4 exit with another but reaching a different v6 exit is kept.
- Proxies where no family answered are dropped, along with any that errored.
- There's no latency threshold for this tool; the filter is purely "unique exit identity".

The result is a clean list with no redundant exits, written in the original line format and ready to reuse. See [Exporting Results → Saving filtered proxies](../reference/exporting-results.md#saving-filtered-proxies).
