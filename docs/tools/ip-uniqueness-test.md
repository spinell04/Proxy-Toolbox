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

## Reading the output

```
#      Host                      Exit IP             Latency
-----------------------------------------------------------------
1      84.56.107.201             84.56.107.201         412ms
2      91.38.203.163             91.38.203.163         488ms
3      84.56.107.201             84.56.107.201         391ms  *** REPEATED x2 (lines: 1)
...
```

- **#** — line number in the source file
- **Host** — the proxy host
- **Exit IP** — what the world sees when traffic goes through that proxy
- **Latency** — round-trip time for the full request
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

After the run, you're prompted to save results to CSV. The export contains the summary block on top, then the repeated IPs section:

```
Summary,Value
Proxies tested,1000
Errors,5
Unique IPs,941 / 995
Total time,56.337s
,
Repeated IP,Times,Lines
84.56.107.201,3,"222, 700, 880"
91.38.203.163,2,"601, 701"
...
```

Files are saved to `results/` next to the binary. See [Exporting Results](../reference/exporting-results.md) for details.
