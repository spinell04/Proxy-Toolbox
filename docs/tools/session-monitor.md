# Session Monitor

## What it does

Watches each proxy's **exit IP** and alerts the moment it changes. Where the [Downtime Monitor](proxy-monitor.md) answers "is the proxy up?", this answers "is the proxy still the same IP it was?" — the question that matters for sticky-session and static residential pools, where an unannounced rotation breaks whatever was bound to the old IP.

It runs until you stop it with `Ctrl+C`, then prints a per-proxy summary.

Reach it from **Monitor → Session monitor** in the main menu.

## How it works

1. Pick a proxy file.
2. Choose an interval in milliseconds (default from `session_interval_ms`, built-in `60000`).
3. Every interval the monitor checks **all** proxies in parallel, fetching each exit IP via the same rotating endpoint set the [IP Uniqueness Test](ip-uniqueness-test.md) uses. Concurrency comes from `workers`.

The **first** exit IP seen for a proxy is a silent baseline. Every later IP that differs from the one before it is a rotation, and every rotation is alerted — there is no threshold and no "expected rotation" suppression.

## The interval, and what it costs (read this)

Unlike the [Downtime Monitor](proxy-monitor.md), the interval here is the gap between **cycles**, and a cycle checks every proxy at once. So the interval *is* the sampling period: a longer list does not push a proxy's next check further out, it just makes each cycle wider.

```
every proxy checked every <interval>
IP lookups per minute = proxies ÷ interval, in minutes
```

That second line is the one to watch. The exit IP comes from free third-party endpoints (`api.ipify.org`, `ifconfig.me`, `icanhazip.com`), and **100 proxies on a 60-second cycle is 100 lookups a minute, 6 000 an hour, indefinitely.** Those services will start refusing you, and a refusal arrives as a failed check. The banner puts the figure in front of you:

```
File     : proxyfiles/sticky.txt
Proxies  : 100
Workers  : 100
Interval : 60000ms per cycle
Coverage : every proxy checked every ~1m
Load     : ~100 IP lookups per minute
Webhook  : enabled
Log file : results/session-monitor.log
```

This is a report, not a limit. Nothing is clamped. Size the interval against your list: sticky sessions last an hour, so a one- or five-minute cycle sees every rotation with room to spare, and there is nothing to gain from checking every second.

**If a cycle takes longer than the interval** — slow proxies, a list far larger than `workers` — the monitor says so and starts the next cycle immediately rather than queueing up behind itself:

```
09:16:31  cycle 4 took 71.2s, longer than the 1m0s interval — running back to back
```

Cadence is measured from the **start** of each cycle, so the sampling period stays the interval instead of drifting by however long the checks took.

## Reading the live feed

Rows appear as each check finishes, so within a cycle they are in **completion order, not file order** — the `#` column is the proxy's line in the file.

```
Time      Cyc   #     Proxy                                 Exit IP           Latency   Status
----------------------------------------------------------------------------------------------
09:14:02  C1    #1    user-session1:pw@gw.example.com:9000  45.12.8.7         412ms     OK
09:14:02  C1    #2    user-session2:pw@gw.example.com:9001  45.12.8.9         388ms     OK
09:15:02  C2    #2    user-session2:pw@gw.example.com:9001  45.12.8.9         401ms     OK
09:15:03  C2    #1    user-session1:pw@gw.example.com:9000  88.4.201.3        409ms     *** ROTATED  45.12.8.7 -> 88.4.201.3
09:16:04  C3    #2    user-session2:pw@gw.example.com:9001  -                 -         FAIL  i/o timeout
```

- **Cyc** — which pass through the full list this check belongs to
- **Proxy** — the full `user:pass@host:port` ID, never shortened
- **Status** — `OK`, a yellow `*** ROTATED old -> new`, or a red `FAIL` plus a short reason

### The proxy column is never truncated

Every other tool elides a wide proxy ID in the middle to keep its table inside a standard terminal. This one does not, in the live feed, the `Ctrl+C` table, the log file or the webhook. Gateway proxies from one pool differ only by the session ID buried in the middle of the username, which is exactly the part middle-elision removes — two proxies would render identically in the one tool whose job is telling you *which* proxy rotated.

The column is instead sized once per run, to the widest ID in the file, so a file of long gateway credentials produces a wide table. That is the trade deliberately taken.

## Alerts

Both monitors share the `discord_webhook` key. Leave it blank to run fully local.

| Alert | Trigger | Colour |
|-------|---------|--------|
| **Proxy IP ROTATED** | The exit IP differs from the last one observed for that proxy | Yellow |
| **Proxy CHECK FAILING** | The 5th consecutive failed check for that proxy, and again on the 30th | Red |
| **Proxy CHECK RECOVERED** | The first success after a **FAILING** alert was sent | Green |

Everything is **per proxy**. Nothing is batched or aggregated across the fleet.

Every embed carries the proxy's full ID, its **`#`** — the proxy's line in the file, the same number the live feed prints — and the cycle it happened on. The `#` is there because a webhook read on a phone is where you least want to be matching a 100-character credential string by eye against a file.

### Rotation alerts

The embed names the proxy, its `#`, the old IP, the new IP and how long the old IP had been held. The first observation of a proxy never alerts — there is nothing to compare it to.

### Recovery alerts

The first success after a **FAILING** alert sends a **Proxy CHECK RECOVERED** embed naming the proxy, its `#`, how long it had been failing, the last error seen and the cycle. It fires only where a **FAILING** alert had already gone out — a proxy that fails three times and comes back is silent in both directions — and it is independent of the IP check, so a proxy that recovers on a *different* exit IP sends a rotation embed alongside it.

### The 5-and-30 failure ladder

A single failed check means nothing: gateways drop requests, and one timeout is not an outage. So failures alert on a ladder:

| Consecutive failures | Behaviour |
|----------------------|-----------|
| 1–4 | Silent |
| **5** | **FAILING** alert, naming the exact error text |
| 6–29 | Silent |
| **30** | **FAILING** alert again — the streak is entrenched |
| 31+ | Silent; the proxy is known-dead and more alerts are noise |

Any successful check resets the counter to zero, re-arming the ladder for next time.

A failed check is **not** a rotation. It does not alert as one and it does not overwrite the last known IP, so the next success is compared against the IP the proxy actually had.

## Statistics summary (on `Ctrl+C`)

```
=========================================================================

  Session monitoring ran for: 1h0m0s  |  Cycles: 60  |  Total checks: 120

  #     Proxy                                 Checks   Fails   Rotations   IPs     Current IP
  ---------------------------------------------------------------------------------------------
  1     user-session1:pw@gw.example.com:9000  60       0       1           2       88.4.201.3
  2     user-session2:pw@gw.example.com:9001  60       3       0           1       45.12.8.9

  Fleet totals: 1 rotations  |  3 distinct exit IPs  |  3 failed checks
===============================================================================================
```

The opening rule is a fixed 73 columns while the closing one is sized to the table, so the two differ whenever the widest proxy ID in the file is not 14 characters.

**Rotations** counts every change, so a proxy that flips between two IPs shows a high rotation count against a low distinct-IP count — that pattern is a pool cycling a small block, not a pool of many addresses.

## The log file

Failures and rotations are appended to `results/session-monitor.log`:

```
2026-09-29 09:15:03  ROTATED  user-session1:pw@gw.example.com:9000  45.12.8.7 -> 88.4.201.3  held 1m1s
2026-09-29 09:16:04  FAIL  user-session2:pw@gw.example.com:9001  i/o timeout
```

Append-only across runs, like the downtime monitor's log.

## Configuration summary

| Key | Purpose | Default |
|-----|---------|---------|
| `session_interval_ms` | Prompt default for the gap between cycles | `60000` |
| `discord_webhook` | Discord webhook URL; blank = disabled | — |

See [Configuration](../getting-started/configuration.md) for the full file.
