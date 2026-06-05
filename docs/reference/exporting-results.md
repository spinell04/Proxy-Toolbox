# Exporting Results

There are two kinds of save in Proxy Toolbox:

1. **CSV results** — the full per-proxy run, saved to `results/`.
2. **Filtered proxies** — a clean `.txt` of the proxies that passed, saved to `proxyfiles/` and ready to reuse. See [Saving filtered proxies](#saving-filtered-proxies).

## CSV results

After IP Uniqueness Test, Ping Test, TM Request Tester, or Bayern Tester finishes, you'll see:

```
Save results to CSV? (Enter to skip, or type filename):
```

- Press **Enter** to skip — results stay only in the terminal.
- Type a filename to save. `.csv` is auto-appended if you leave it off.

## Where files are saved

All exports go to `results/` next to the binary:

```
Proxy-Toolbox/
├── proxytoolbox-mac-AppleSiliconCPU
├── proxyfiles/
└── results/
    ├── my-run.csv
    ├── uk-proxies.csv
    └── ...
```

The folder is created automatically the first time you export.

## File layout

All four tools use the same "summary on top" layout:

1. **Summary block** — one row per summary metric (proxies tested, errors, averages…)
2. **Blank row** — visual separator
3. **Header row** — column names for per-proxy data
4. **Data rows** — one per proxy

Example (Ping Test):

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

## Opening exports

Any spreadsheet app (Excel, Numbers, Google Sheets, LibreOffice Calc) handles the format. If the summary block confuses auto-parsing, you can delete the first few rows after opening.

For the **IP Uniqueness Test**, the export is slightly different — it contains the summary block plus a *Repeated IPs* section (no per-proxy rows, since the raw data isn't useful after the fact). See [IP Uniqueness Test](../tools/ip-uniqueness-test.md#csv-export) for the exact format.

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
| [IP Uniqueness Test](../tools/ip-uniqueness-test.md) | One proxy per unique exit IP (duplicates + errored dropped) | — |
| [Ping Test](../tools/ping-test.md) | Successful proxies under the latency threshold | `ping_max_latency_ms` |
| [TM Request Tester](../tools/tm-request-tester.md) | `200 OK` proxies under the latency threshold | `tm_max_latency_ms` |
| [Bayern Tester](../tools/bayern-tester.md) | `200 OK` proxies under the latency threshold | `bayern_max_latency_ms` |

The latency keys live in [`config.txt`](../getting-started/configuration.md#saving-filtered-proxies-latency-thresholds). A proxy is kept only if its latency is **strictly below** the threshold; leave the key blank to save all successful proxies. The prompt label shows the active limit (or `no latency limit`).

## The monitor log

The [Proxy Monitor](../tools/proxy-monitor.md) doesn't prompt for export — it streams failures to `results/monitor.log` continuously while it runs. The file is append-only across runs; delete it for a clean slate.
