# IP Uniqueness Test

## What it does

Connects to an IP-reflection service through each proxy and records the exit IP returned. It then tells you how many of your proxies actually have distinct IPs — and flags any duplicates with the exact line numbers.

## Why it matters

A common proxy-provider scam is to sell a small pool of real IPs labelled as thousands of unique endpoints. You think you have 1,000 unique proxies; in reality it's 50 IPs reused 20 times each, which defeats the whole point of rotation.

This tool catches that.

## How it works

For each proxy, the tool makes a GET request to one of these endpoints (rotated to spread load):

- `https://api.ipify.org`
- `https://ifconfig.me/ip`
- `https://icanhazip.com`

If one fails, it falls back to the next. The returned body is the exit IP, which is then compared across all proxies.

Tests run in parallel using the `workers` value from [`config.txt`](../getting-started/configuration.md).

### Repeated lines are spaced one second apart

If the same proxy string appears more than once in the file, its copies are **not** checked in parallel. Each repeat waits one second after the previous one finishes, and the tool prints how many checks that affects before the run starts.

This is because a residential gateway keys its sticky session off the username, not the host. Repeating a line asks for the same session again, so firing the copies simultaneously measures a single instant and tells you nothing about whether the exit rotates. Spacing them turns a repeated line into a probe of one proxy over time, which is the only reason to repeat a line.

Two consequences worth knowing:

- **A file of N copies of one proxy takes at least N − 1 seconds.** The gap sits *between* checks, so the first copy waits for nothing. That is the point, not a regression.
- **A file with no duplicates is unaffected.** Repeats are handled outside the worker pool, so distinct proxies keep running at full width even when a repeated proxy is trickling alongside them.

A proxy is "the same" here when its whole `user:pass@host:port` matches — which is what identifies a session. Two lines sharing a host but carrying different session IDs are different proxies and run in parallel as usual.

## Reading the output

```
#      Proxy                                 Exit IP             Latency
--------------------------------------------------------------------------
1      admin:secret@1.2.3.4:8080             84.56.107.201         412ms
2      admin:secret@5.6.7.8:8080             91.38.203.163         488ms
3      admin:secret@9.10.11.12:8080          84.56.107.201         391ms  *** REPEATED x2 (lines: 1)
4      admin:secret@13.14.15.16:8080         ERROR  proxy connect: i/o timeout
...
```

- **#** — line number in the source file
- **Proxy** — the proxy, as `user:pass@host:port`, elided in the middle when it is too wide for the column
- **Exit IP** — what the world sees when traffic goes through that proxy
- **Latency** — round-trip time for the full request
- **ERROR** — a failed check drops the Exit IP and Latency columns and prints a shortened reason instead
- **REPEATED xN (lines: …)** — marker when the same exit IP has been seen before; shows which earlier lines matched

## Final summary

```
Proxies tested   : 1000
Errors           : 5
Unique IPs       : 941 / 995
Total time       : 56.337s

[!] Repeated IPs:
    84.56.107.201       3 times  ->  lines: 222, 700, 880
    91.38.203.163       2 times  ->  lines: 601, 701
    ...
```

- **Unique IPs** reads as `unique / successful` — i.e. out of the 995 proxies that responded, only 941 gave distinct exit IPs.
- The **Repeated IPs** section lists every duplicated IP and the source-file line numbers that share it, so you can delete or request replacements.

## CSV export

After the run, you're prompted to save results to CSV. The export has three sections: the metadata and summary block, the repeated IPs, then one row per proxy.

```
Summary,Value
Tool,iptester
Run at,2026-09-19T14:32:07Z
Proxy file,residential.txt
Target,
Workers,40
Proxies tested,1000
Errors,5
Unique IPs,941 / 995
Total time,56.337s
,
Repeated IP,Times,Lines
84.56.107.201,3,"222, 700, 880"
91.38.203.163,2,"601, 701"
,
#,Proxy,Exit IP,Latency,Error
1,admin:secret@1.2.3.4:8080,84.56.107.201,412ms,
2,admin:secret@5.6.7.8:8080,91.38.203.163,488ms,
3,admin:secret@9.10.11.12:8080,,,proxy connect: i/o timeout
...
```

`Target` is empty because this tool doesn't have one — it asks a reflection service what your exit IP is, rather than testing a destination you chose.

The per-proxy section is what lets [Compare Results](compare-results.md) attach an exit IP to a proxy that also appears in a ping or TM run — which is what makes its cross-tool matrix able to collapse rows by exit IP or exit /24.

Files are saved to `results/` next to the binary. See [Exporting Results](../reference/exporting-results.md) for details.

## Saving unique proxies

After the CSV prompt, IP Uniqueness Test offers to save a **deduplicated** proxy list to a `.txt` in `proxyfiles/`:

- One proxy is kept per distinct exit IP — the **first** one seen wins.
- Duplicate-IP proxies and any that errored are dropped.
- There's no latency threshold for this tool; the filter is purely "unique exit IP".

The result is a clean list with no redundant exits, written in the original line format and ready to reuse. See [Exporting Results → Saving filtered proxies](../reference/exporting-results.md#saving-filtered-proxies).
