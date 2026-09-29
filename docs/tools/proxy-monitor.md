# Downtime Monitor

The main menu's **Monitor** entry opens a submenu with two long-running tools:

* **Downtime monitor** — continuous reachability checks with alerts (this page)
* **Session monitor** — [alerts when a proxy's exit IP changes](session-monitor.md)
* **Back** — return to the main menu

Both share the `discord_webhook` key and both run until `Ctrl+C`.

## What it does

Continuously pings every proxy in your file on a fixed interval, prints a live feed of each check, writes failures to a log, and — optionally — sends **Discord alerts** when your fleet goes down and recovers. It runs until you stop it with `Ctrl+C`, then prints a statistics summary.

Use it to watch a proxy pool over hours or days and get notified the moment it degrades.

## How it works

1. Pick a proxy file.
2. Choose a target domain (same three modes as the [Ping Test](ping-test.md) — raw TCP, HTTP, or HTTPS, auto-selected from the URL scheme). Pre-filled from `domain` in [`config.txt`](../getting-started/configuration.md) if set.
3. Choose an interval in milliseconds (default from `monitor_interval_ms`, built-in `1000`).
4. The monitor loops in **cycles** — each cycle pings every proxy once, then waits the interval before the next cycle.

Every check is printed live and every failure is appended to `results/monitor.log`.

## Reading the live feed

```
File     : proxyfiles/residential.txt
Proxies  : 200
Target   : https://google.com
Interval : 1000ms
Webhook  : enabled
Log file : results/monitor.log

Press Ctrl+C to stop and show statistics.

Time          Cyc   #     Host                      Latency     Status
----------------------------------------------------------------------
14:22:01  C1    #1    1.2.3.4                   210ms       HTTP 200
14:22:01  C1    #2    5.6.7.8                   -           FAIL  i/o timeout
14:22:02  C2    #1    1.2.3.4                   198ms       HTTP 200
...
```

- **Cyc** — which pass through the full list this check belongs to (`C1`, `C2`, …)
- **#** — proxy line number
- **Latency / Status** — `HTTP <code>` or `OK` on success; `FAIL` plus a short reason on failure
- Failures are also written to `results/monitor.log` with full timestamps.

The header pads **Time** to 12 columns and the rows print an 8-character timestamp, so the data rows sit four columns to the left of their header. Cosmetic only.

## Discord alerts

If `discord_webhook` is set in [`config.txt`](../getting-started/configuration.md), the monitor sends rich embed alerts on **fleet-level** state transitions — not one message per failed proxy, so you don't get spammed.

The logic is a streak-based state machine across the whole fleet:

| Alert | Trigger | Embed colour |
|-------|---------|--------------|
| **DOWN** | Consecutive fleet check-failures reach `discord_down_threshold` (default `3`) | Red |
| **RECOVERED** | After a DOWN, consecutive successes reach `discord_up_threshold` (default `2`) | Green |

Only one DOWN alert fires per outage; the next alert you get is the matching RECOVERED (which includes how long the fleet was down). Webhook sends are asynchronous, so alerting never blocks the monitor loop.

Leave `discord_webhook` blank to run fully local with no alerts.

## Statistics summary (on `Ctrl+C`)

Stopping the monitor prints per-proxy totals and a failure timeline, e.g.:

```
===========================================================================

  Monitoring ran for: 1h0m2s  |  Cycles: 1800  |  Total checks: 3600

  #     Host                      Checks   Fails   Success%   Avg Latency   Max Streak
  -----------------------------------------------------------------------
  1     1.2.3.4                   1800     12      99.3%      205ms         4
  2     5.6.7.8                   1800     410     77.2%      330ms         57
  ...

  Failure Timeline (1-min buckets)
  14:22  ████████  8
  14:23  ██  2
  14:24
  ...
===========================================================================
```

- **Success%** is tinted green at 100%, yellow from 90%, red below.
- **Max Streak** is the longest run of consecutive failures — use it to spot proxies that drop for sustained periods, and the timeline to correlate outages with a point in time.
- The timeline buckets failures by the minute and caps the display at 60 rows. A run with no failures prints `No failures recorded.` instead.

## The log file

Failures are appended to `results/monitor.log` (created automatically):

```
2026-06-04 14:22:01  FAIL  5.6.7.8:8080  -> https://google.com  i/o timeout
```

It's append-only across runs, so you keep a running history. Delete it whenever you want a clean slate.

## Configuration summary

| Key | Purpose | Default |
|-----|---------|---------|
| `domain` | Default ping target (shared with Ping Test) | — |
| `monitor_interval_ms` | Prompt default for the gap between checks | `1000` |
| `discord_webhook` | Discord webhook URL for alerts; blank = disabled | — |
| `discord_down_threshold` | Consecutive fleet failures before a DOWN alert | `3` |
| `discord_up_threshold` | Consecutive successes before a RECOVERED alert | `2` |

See [Configuration](../getting-started/configuration.md) for the full file.
