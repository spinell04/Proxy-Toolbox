# Proxy Monitor

## What it does

Continuously pings every proxy in your file on a fixed interval, prints a live feed of each check, writes failures to a log, and — optionally — sends **Discord alerts** when your fleet goes down and recovers. It runs until you stop it with `Ctrl+C`, then prints a statistics summary.

Use it to watch a proxy pool over hours or days and get notified the moment it degrades.

## How it works

1. Pick a proxy file.
2. Choose a target domain (same three modes as the [Ping Test](ping-test.md) — raw TCP, HTTP, or HTTPS, auto-selected from the URL scheme). Pre-filled from `domain` in [`config.txt`](../getting-started/configuration.md) if set.
3. Choose an interval in milliseconds (default `1000`).
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
14:22:01      C1    #1    1.2.3.4                   210ms       HTTP 200
14:22:01      C1    #2    5.6.7.8                   -           FAIL  i/o timeout
14:22:02      C2    #1    1.2.3.4                   198ms       HTTP 200
...
```

- **Cyc** — which pass through the full list this check belongs to (`C1`, `C2`, …)
- **#** — proxy line number
- **Latency / Status** — `HTTP <code>` or `OK` on success; `FAIL` plus a short reason on failure
- Failures are also written to `results/monitor.log` with full timestamps.

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
  Host                      Checks  Fails   Avg        Longest fail streak
  1.2.3.4                   3600    12      205ms      4
  5.6.7.8                   3600    410     330ms      57
  ...

  Failure timeline (failures per minute):
  14:22  ████████  8
  14:23  ██  2
  14:24
  ...
===========================================================================
```

Use the longest-fail-streak column to spot proxies that drop for sustained periods, and the timeline to correlate outages with a point in time.

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
| `discord_webhook` | Discord webhook URL for alerts; blank = disabled | — |
| `discord_down_threshold` | Consecutive fleet failures before a DOWN alert | `3` |
| `discord_up_threshold` | Consecutive successes before a RECOVERED alert | `2` |

See [Configuration](../getting-started/configuration.md) for the full file.
