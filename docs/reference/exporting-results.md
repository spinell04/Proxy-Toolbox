# Exporting Results

There are two kinds of save in Proxy Toolbox:

1. **CSV results** — the full per-proxy run, saved to `results/`.
2. **Filtered proxies** — a clean `.txt` of the proxies that passed, saved to `proxyfiles/` and ready to reuse. See [Saving filtered proxies](#saving-filtered-proxies).

## CSV results

After IP Uniqueness Test, Ping Test, TM Request Tester, or Bayern Tester finishes, you'll see:

```
Save results to CSV? (Enter to skip, "." for pinger_schroeder_2026-09-19_143207.csv, or type filename):
```

- Press **Enter** to skip — results stay only in the terminal.
- Type **`.`** to accept the suggested name. It is built from the tool, the proxy file you tested and the moment the run started: `<tool>_<proxyfile>_<date>_<time>.csv`, e.g. `iptester_schroeder_2026-09-19_143207.csv`. The proxy file is there because comparing providers is the common case, and the file is what names the provider. Its extension is dropped and anything that is not a letter, digit, dash or underscore becomes a dash. Names sort chronologically per tool and provider, which is convenient in [Compare Results](../tools/compare-results.md).
- Or type a filename. `.csv` is auto-appended if you leave it off, and any directory component is stripped — exports always land in `results/`.

The tool names used in the suggestion are `pinger`, `iptester`, `speedtester` and `bayerntester`.

## Where files are saved

All exports go to `results/` next to the binary:

```
Proxy-Toolbox/
├── proxytoolbox-mac-AppleSiliconCPU
├── proxyfiles/
└── results/
    ├── pinger_schroeder_2026-09-19_143207.csv
    ├── speedtester_schroeder_2026-09-19_151140.csv
    └── ...
```

The folder is created automatically the first time you export.

## File layout

All four tools use the same "metadata, then summary, then detail" layout:

1. **Metadata block** — five rows describing the run
2. **Summary block** — one row per summary metric (proxies tested, errors, averages…)
3. **Blank row** — section separator
4. **Header row** — column names for the per-proxy table
5. **Data rows** — one per proxy

Where a summary block carries percentiles (`Median (p50)` and `p95`, in the TM Request Tester and Bayern Tester exports), they are computed by the **nearest-rank** method: the successful latencies are sorted and the value at rank `ceil(p × n)` is written out. Nothing is interpolated, so the exported figure is always a latency some real proxy recorded, and it matches what the [compare dashboard](../tools/compare-results.md) recomputes from the detail rows.

Example (Ping Test):

```
Summary,Value
Tool,pinger
Run at,2026-09-19T14:32:07Z
Proxy file,residential.txt
Target,https://google.com
Workers,40
IP mode,
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

### The metadata block

The six rows at the top describe the run that produced the file:

| Row | Contents |
|-----|----------|
| `Tool` | Which tool ran — `pinger`, `iptester`, `speedtester` or `bayerntester` |
| `Run at` | When the run started, UTC, in RFC 3339 (`2026-09-19T14:32:07Z`) |
| `Proxy file` | The base name of the proxy file tested. Only the base name — the path to it on your machine never leaves your machine |
| `Target` | The domain or URL tested. Empty for the IP Uniqueness Test, which has no target |
| `Workers` | The concurrency the run used |
| `IP mode` | Which address families the exit-IP lookups asked for: `ipv4`, `ipv6` or `both`. Empty for every tool but the IP Uniqueness Test, which is the only one that looks up an exit IP |

These rows are what make an export self-describing, and they are what [Compare Results](../tools/compare-results.md) uses to tell a change in the proxies from a changed target, a different input file, a bumped worker count, or a run that measured a different address family.

CSVs exported before these rows existed still open and still parse — they just can't be placed on a timeline, and show `unknown` where the metadata would be. The same holds one row at a time: an export made before `ip_mode` existed has no `IP mode` row, and that is read as **unknown**, never as "the same as the others".

### The proxy column

The per-proxy table identifies each proxy by its canonical form, `user:pass@host:port`, in every tool. That identifier is the same string in every export, which is what lets Compare Results join one run against another — including across tools.

**These files contain your proxy credentials in plaintext.** Treat `results/` accordingly: don't commit it, and don't pass exports around casually.

### Per-tool columns

| Tool | Per-proxy columns |
|------|-------------------|
| Ping Test | `#`, `Proxy`, `Latency`, `Status`, `Error` |
| TM Request Tester | `#`, `Proxy`, `Latency`, `Status`, `Error` |
| Bayern Tester | `#`, `Proxy`, `Latency`, `Status`, `Error` |
| IP Uniqueness Test | `#`, `Proxy`, then one column per address family the run asked for — `Exit IPv4`, `Exit IPv6`, or both — then `Latency`, `Error` |

The IP Uniqueness Test's address columns are **named per family**, and only the families the run asked for appear: an `ipv4` run writes `Exit IPv4` alone, a `both` run writes `Exit IPv4` and `Exit IPv6`.

Exports made before address families were selectable carry one unlabelled `Exit IP` column instead. Both spellings load: [Compare Results](../tools/compare-results.md) reads the legacy column and sorts its address into the right family by parsing it, so older exports keep working and show exit IPs exactly as they always did.

The **IP Uniqueness Test** export carries one extra section: a *Repeated IPs* table, between the summary and the per-proxy table, listing every duplicated exit address, the family it belongs to, and the source lines that share it. See [IP Uniqueness Test](../tools/ip-uniqueness-test.md#csv-export) for the exact format.

One more thing to know about the `Error` column there: in `both` mode a check that got one address but not the other is a **success**, and the family that gave no answer is reported in `Error` while `Latency` is still filled in. An error alongside an address is a partial check, not a failed one.

## Opening exports

Any spreadsheet app (Excel, Numbers, Google Sheets, LibreOffice Calc) handles the format. If the metadata and summary blocks confuse auto-parsing, delete the rows above the header row after opening.

## Comparing exports

Exports are meant to be read together, not one at a time. [Compare Results](../tools/compare-results.md) opens a local, read-only dashboard over everything in `results/` — timelines for one tool, before/after diffs for a pair of runs, and per-proxy joins across tools.

## Saving filtered proxies

After the CSV prompt, four tools offer a **second** save — a plain `.txt` of just the proxies that matter, written in their original line format so you can drop the file straight back into `proxyfiles/`.

```
Save filtered proxies (latency < 800ms) to proxyfiles/? (Enter to skip, or type filename):
```

- Press **Enter** to skip.
- Type a filename to save. `.txt` is auto-appended if you leave it off.
- Files land in `proxyfiles/` next to the binary (created automatically).

### What gets saved

| Tool | Filter | Config key |
|------|--------|------------|
| [IP Uniqueness Test](../tools/ip-uniqueness-test.md) | One proxy per unique exit identity — one address in single-family mode, the **pair** in `both` mode (duplicates + errored dropped) | — |
| [Ping Test](../tools/ping-test.md) | Successful proxies under the latency threshold | `ping_max_latency_ms` |
| [TM Request Tester](../tools/tm-request-tester.md) | `200 OK` proxies under the latency threshold | `tm_max_latency_ms` |
| [Bayern Tester](../tools/bayern-tester.md) | `200 OK` proxies under the latency threshold | `bayern_max_latency_ms` |

The latency keys live in [`config.txt`](../getting-started/configuration.md#saving-filtered-proxies-latency-thresholds). A proxy is kept only if its latency is **strictly below** the threshold; leave the key blank to save all successful proxies. The prompt label shows the active limit (or `all working proxies` when no limit is set).

## The monitor logs

Neither monitor prompts for export. Each streams to its own log in `results/` while it runs, and both files are append-only across runs — delete one for a clean slate.

| Tool | Log file | What it records |
|------|----------|-----------------|
| [Downtime Monitor](../tools/proxy-monitor.md) | `results/monitor.log` | Every failed check, with the target and the full error |
| [Session Monitor](../tools/session-monitor.md) | `results/session-monitor.log` | Every failed check; every exit-IP rotation with the address family, the old address, the new address and how long the old one was held; and a `PARTIAL` line for a family that gave no answer on a check that otherwise succeeded |
