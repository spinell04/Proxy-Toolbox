# Exporting Results

After any of the three testing tools finishes (IP Uniqueness Test, Ping Test, TM Request Tester), you'll see:

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

All three tools use the same "summary on top" layout:

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
