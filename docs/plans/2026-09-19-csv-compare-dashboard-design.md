# CSV Compare Dashboard — Design

**Date:** 2026-09-19
**Status:** Approved, pending implementation plan

## Problem

Each tool exports a standalone CSV to `results/`. There is no way to compare runs
against each other, to track a proxy file across time, or to see how the same
proxies behave under different tools (ping vs TM vs Bayern). The only statistics
shown are counts and an average latency, which is the least useful summary of a
heavy-tailed latency distribution.

## Goal

A local, read-only dashboard that lets a user select any set of exported CSVs and
compare them — across runs of the same tool over time, and across different tools
over the same proxies — with real statistics and graphs.

Explicit non-goals: no API, no remote server, no new measurement, no change to how
tools test proxies.

## Constraints

- Localhost only. Single binary, no build step, works offline.
- Dashboard only. No terminal compare view.
- Reads the CSVs the tools already produce.

## Current State

### CSV shape

Every tool writes two tables into one file:

```
Summary,Value
Proxies tested,500
Successful,412
Errors,88
Total time,1m23s
Average latency,340ms
,
#,Host,Latency,Status,Error
1,1.2.3.4,340ms,HTTP 200,
```

Ragged, and the second header differs per tool (`Host` vs `Proxy`; iptester emits a
dedup table instead). A tool-aware parser is required.

### Identity is inconsistent

| Tool | CSV identifier | Example |
|---|---|---|
| pinger | `p.Host` | `1.2.3.4` — no port |
| iptester | `p.Host` | `1.2.3.4` |
| speedtester | `p.URL()` | `http://user:pass@1.2.3.4:8080` |
| bayerntester | `p.URL()` | `http://user:pass@1.2.3.4:8080` |

These cannot be joined. Separately, `p.Host` alone is a bug in its own right:
gateway-style residential pools share one host and port across hundreds of
credentials (`gate.provider.com:7000`), so pinger and iptester currently collapse
every proxy in such a pool into a single identifier.

### No self-identifying runs

`util.PromptExport` ignores its `defaultName` argument entirely
(`internal/util/export.go:18`); filenames are whatever the user types. A CSV
therefore records nothing about which tool produced it, when, against which target,
with how many workers, or over which proxy file.

## Decisions

### Canonical proxy ID: `user:pass@host:port`

`host:port` is not unique — gateway pools distinguish sessions by credentials only.
The full string is the identity. Added as `Proxy.ID()`, normalized from the parsed
`Proxy` struct so that the four accepted input formats all yield one ID. The
`http://` prefix on legacy speedtester/bayern rows is stripped at parse time so
those rows join as well. The host is lowercased, since DNS is case-insensitive
and a case difference between two files would otherwise split one proxy into
two join keys; credentials are case-sensitive and kept verbatim.

Credentials appear in exported CSVs. Accepted: the tool runs locally, and the user
already holds those credentials in `proxyfiles/`.

### Run metadata in the CSV

Five rows added to the existing summary block:

```
Tool,pinger
Run at,2026-09-19T14:32:07Z
Proxy file,residential_de.txt
Target,https://google.com
Workers,100
```

`Workers` means the size of the worker pool the tool actually used. speedtester
and bayerntester cap theirs at `min(cfg.Workers, len(proxies))`, so they record
the capped value; pinger and iptester spawn `cfg.Workers` unconditionally.
Recording the configured value instead would let a 12-proxy run claim 100
workers, and the comparability check would call it comparable to a 500-proxy run
at the same setting despite an 8x difference in real concurrency.

Rationale: a timeline needs a real time axis, and file mtime lies after any copy,
move, restore, or sync. More importantly, without `Target`, `Workers` and
`Proxy file`, the dashboard cannot tell a genuine degradation from a changed
target, a bumped worker count, or a different input file — it would draw
confident, false trends. Tool type is guessable from headers and timestamp
degrades to mtime, but those three fields are unrecoverable if not written at
export time.

### Metrics are not merged across tools

Ping latency is a raw CONNECT through the proxy; TM latency is a full
TLS-fingerprinted request to a hostile target. They are not the same quantity.
Cross-tool comparison joins at the proxy level only, never as a merged series.

`results/` is currently empty, so there is no legacy data to migrate.

## Architecture

### 1. Emit layer — changes to existing tools

- `proxy.Proxy.ID()` returns `user:pass@host:port`
- pinger, iptester: write `ID()` in place of `p.Host`
- speedtester, bayerntester: write `ID()` in place of `URL()`
- all tools: emit the five metadata rows
- `PromptExport` uses its `defaultName` argument, suggesting `pinger_2026-09-19_143207.csv`
  (seconds included: the dashboard keys runs by file name and `WriteCSV` truncates)

### 2. Parse layer — `internal/compare`

Tool-aware reader for the two-table CSV. Produces `Run{Meta, []ProxyResult}`.
Latency strings parse to milliseconds. Absent metadata becomes `unknown` rather
than an error. Pure functions, table-driven tests.

### 3. Stats layer — `internal/compare`

No external dependencies:

- percentiles (p50/p75/p90/p95/p99), min, max, stddev, IQR, MAD
- histogram and ECDF
- error taxonomy: `timeout | conn_refused | conn_reset | tls | dns | 407 | eof | other`
- status breakdown
- per-proxy joins across runs, set operations, correlation
- /24 subnet grouping

### 4. Serve layer

Menu item `Compare Results` parses `results/*.csv`, binds `127.0.0.1:<random>`,
opens the browser. Assets via `embed.FS`; uPlot vendored (~40KB). Read-only.

## Views

**Inventory (landing).** Every CSV as a row: tool, time, proxy file, target,
workers, n, success %, p50, p95, error rate. Sortable, checkboxes, free-form
selection of any count and any mix of tools.

**Same tool, N runs — timeline.** Trends for success %, p50, p95 and error rate;
stacked error-taxonomy area; ECDF overlay, one curve per run. A compatibility
banner warns when selected runs differ in target or proxy file.

**Same tool, exactly 2 — paired.** Scatter of A latency vs B latency, one dot per
proxy, with a diagonal reference line; delta histogram; top movers; status flips
(OK→403, ERROR→OK).

**Cross-tool.** Proxy matrix (rows = proxies, columns = tools, cells = latency and
status); funnel (`500 loaded → 412 ping OK → 380 TM 200 → 290 Bayern OK`); set
operations with export, so "OK in ping, failing TM" becomes a proxy file;
ping-vs-TM correlation scatter; exit-IP enrichment joined from iptester. An
overlap banner states join coverage — "312 of 500 proxies in all 3 runs".

**Single run — detail.** Histogram, percentile table, per-/24 box plots, slowest-N,
error breakdown.

## Testing

- Parser: table-driven over fixture CSVs per tool, including missing metadata and
  malformed rows.
- Stats: known inputs with hand-computed percentiles, taxonomy classification,
  join and set-operation cases.
- `Proxy.ID()`: all four input formats normalize to one ID.
- Serve layer: handler tests over a fixture `results/` directory.

## Out of Scope

- Terminal compare view
- Proxy Monitor, the fifth tool. It writes only `results/monitor.log` and never
  exports a CSV, so the dashboard's scan — which filters on `.csv` — ignores it.
  Its terminal `"Host"` column headers carry no contract.
- Re-running or re-measuring proxies
- Repeat sampling, timing breakdowns, significance testing
- Provider attribution
- Any remote or networked component
