# Session Monitor

## What it does

Watches each proxy's **exit IP** and alerts the moment it changes. Where the [Downtime Monitor](proxy-monitor.md) answers "is the proxy up?", this answers "is the proxy still the same IP it was?" — the question that matters for sticky-session and static residential pools, where an unannounced rotation breaks whatever was bound to the old IP.

It runs until you stop it with `Ctrl+C`, then prints a per-proxy summary.

Reach it from **Monitor → Session monitor** in the main menu.

## How it works

1. Pick a proxy file.
2. Choose an **IP mode** from the arrow-key menu — IPv4 only, IPv6 only, or both. The `ip_mode` value from `config.txt` is pre-selected (built-in `ipv4`), so Enter accepts it.
3. Choose an interval in milliseconds (default from `session_interval_ms`, built-in `60000`).
4. Every interval the monitor checks **all** proxies in parallel, fetching each exit IP via the same single-family endpoint sets the [IP Uniqueness Test](ip-uniqueness-test.md) uses. Concurrency comes from `workers`.

The **first** address a family answers with is a silent baseline. Every later address that differs from that family's previous one is a rotation, and every rotation is alerted — there is no threshold and no "expected rotation" suppression.

### Address families, and why one going quiet is silent

The two families are tracked **independently**. In `both` mode a single check can rotate the IPv4 exit, the IPv6 exit, both, or neither, and every alert and log line names which family moved.

One rule in here looks like a bug until you know why it is there:

> **A family that stops answering is silent, and its last known address is kept.**

A proxy with no IPv6 route and an IPv6 endpoint that happened to time out are indistinguishable from this side of the connection. If "no answer" were treated as "the exit disappeared", every v4-only proxy in the file would alert on every single cycle, forever, about nothing. So no answer means *unknown for this check* — not absent. Nothing is alerted, nothing is cleared, and the next answer is compared against the address the family actually had.

The mirror of that rule: a family answering for the **first** time, however late in the run, is a silent baseline, exactly like the first observation of a proxy. A v6 route that only appears on the fortieth cycle has not rotated; it has been seen once.

This is the rule that makes the whole thing usable. Before single-family endpoints existed the monitor asked dual-stack services, so a dual-stack proxy answered with its v4 exit on one check and its v6 exit on the next — and alerted on roughly half of all consecutive check pairs, continuously, about a proxy that had not changed at all.

## The interval, and what it costs (read this)

Unlike the [Downtime Monitor](proxy-monitor.md), the interval here is the gap between **cycles**, and a cycle checks every proxy at once. So the interval *is* the sampling period: a longer list does not push a proxy's next check further out, it just makes each cycle wider.

```
every proxy checked every <interval>
IP lookups per minute = proxies ÷ interval, in minutes
```

That second line is the one to watch. The exit IP comes from free third-party endpoints, and **100 proxies on a 60-second cycle is 100 lookups a minute, 6 000 an hour, indefinitely.** Those services will start refusing you, and a refusal arrives as a failed check. The banner puts the figure in front of you:

```
File     : proxyfiles/sticky.txt
Proxies  : 100
Workers  : 100
Interval : 60000ms per cycle
IP mode  : IPv4 only
Coverage : every proxy checked every ~1m
Load     : ~100 IP lookups per minute
Webhook  : enabled
Log file : results/session-monitor.log
```

**In `both` mode that figure doubles**, because each check asks two endpoint sets instead of one:

```
IP mode  : IPv4 and IPv6
Load     : ~200 IP lookups per minute
```

The banner reports it correctly, which is the point — `both` is twice the request volume against services that are doing you a favour. Lengthen the interval, or shorten the list, to pay for it.

This is a report, not a limit. Nothing is clamped. Size the interval against your list: sticky sessions last an hour, so a one- or five-minute cycle sees every rotation with room to spare, and there is nothing to gain from checking every second.

**If a cycle takes longer than the interval** — slow proxies, a list far larger than `workers` — the monitor says so and starts the next cycle immediately rather than queueing up behind itself:

```
09:16:31  cycle 4 took 71.2s, longer than the 1m0s interval — running back to back
```

Cadence is measured from the **start** of each cycle, so the sampling period stays the interval instead of drifting by however long the checks took.

## Reading the live feed

Rows appear as each check finishes, so within a cycle they are in **completion order, not file order** — the `#` column is the proxy's line in the file.

```
Time      Cyc   #     Proxy                                 Exit IPv4           Latency   Status
------------------------------------------------------------------------------------------------
09:14:02  C1    #1    user-session1:pw@gw.example.com:9000  45.12.8.7           412ms     OK
09:14:02  C1    #2    user-session2:pw@gw.example.com:9001  45.12.8.9           388ms     OK
09:15:02  C2    #2    user-session2:pw@gw.example.com:9001  45.12.8.9           401ms     OK
09:15:03  C2    #1    user-session1:pw@gw.example.com:9000  88.4.201.3          409ms     *** ROTATED  IPv4 45.12.8.7 -> 88.4.201.3
09:16:04  C3    #2    user-session2:pw@gw.example.com:9001  -                   -         FAIL  i/o timeout
```

In `both` mode there is a column per family:

```
Time      Cyc   #     Proxy                                 Exit IPv4           Exit IPv6                                Latency   Status
-----------------------------------------------------------------------------------------------------------------------------------------
09:14:02  C1    #1    user-session1:pw@gw.example.com:9000  45.12.8.7           2a02:908:1c6::7                          412ms     OK
09:14:02  C1    #2    user-session2:pw@gw.example.com:9001  45.12.8.9           2a02:908:1c6::9                          388ms     OK
09:15:02  C2    #2    user-session2:pw@gw.example.com:9001  45.12.8.9           -                                        401ms     OK
09:15:03  C2    #1    user-session1:pw@gw.example.com:9000  88.4.201.3          2a02:908:1c6::7                          409ms     *** ROTATED  IPv4 45.12.8.7 -> 88.4.201.3
09:16:04  C3    #2    user-session2:pw@gw.example.com:9001  -                   -                                        -         FAIL  i/o timeout
```

- **Cyc** — which pass through the full list this check belongs to
- **Proxy** — the full `user:pass@host:port` ID, never shortened
- **Exit IPv4 / Exit IPv6** — the addresses this check got. A `-` is a family that gave no answer: cycle C2 above is the "unknown, not absent" case, and note that it produces an `OK`, not a `FAIL`, and no rotation.
- **Status** — `OK`, a yellow `*** ROTATED IPvN old -> new` (both families when both moved, separated by `|`), or a red `FAIL` plus a short reason

### The proxy column is never truncated

Every other tool elides a wide proxy ID in the middle to keep its table inside a standard terminal. This one does not, in the live feed, the `Ctrl+C` table, the log file or the webhook. Gateway proxies from one pool differ only by the session ID buried in the middle of the username, which is exactly the part middle-elision removes — two proxies would render identically in the one tool whose job is telling you *which* proxy rotated.

The column is instead sized once per run, to the widest ID in the file, so a file of long gateway credentials produces a wide table. That is the trade deliberately taken.

## Alerts

Both monitors share the `discord_webhook` key. Leave it blank to run fully local.

| Alert | Trigger | Colour |
|-------|---------|--------|
| **Proxy IPv4 ROTATED** / **Proxy IPv6 ROTATED** | That family's exit address differs from the last one it answered with | Yellow |
| **Proxy CHECK FAILING** | The 5th consecutive failed check for that proxy, and again on the 30th | Red |
| **Proxy CHECK RECOVERED** | The first success after a **FAILING** alert was sent | Green |

Everything is **per proxy**. Nothing is batched or aggregated across the fleet.

Every embed carries the proxy's full ID, its **`#`** — the proxy's line in the file, the same number the live feed prints — and the cycle it happened on. The `#` is there because a webhook read on a phone is where you least want to be matching a 100-character credential string by eye against a file.

### Rotation alerts

The embed names the proxy, its `#`, the **address family** that rotated, the old address, the new address and how long the old one had been held. The family is in the title too, so a webhook read on a phone says which exit moved without being opened.

The first observation of a family never alerts — there is nothing to compare it to. Neither does a family falling silent. In `both` mode a check where both families rotated sends **two** embeds, one per family: they are two separate facts about the proxy.

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

A failed check is **not** a rotation. It does not alert as one and it does not overwrite either family's last known address, so the next success is compared against the addresses the proxy actually had.

In `both` mode a check fails only when **neither** family answered. One family answering is a successful check, so a v4-only proxy never climbs the ladder — the family that did not answer is reported in the log as a `PARTIAL` line instead.

## Statistics summary (on `Ctrl+C`)

```
=======================================================================================================

  Session monitoring ran for: 1h0m0s  |  Cycles: 60  |  Total checks: 120

  #     Proxy                                 Checks   Fails   Rotations   IPs     Current IPv4
  -----------------------------------------------------------------------------------------------------
  1     user-session1:pw@gw.example.com:9000  60       0       1           2       88.4.201.3
  2     user-session2:pw@gw.example.com:9001  60       3       0           1       45.12.8.9

  Fleet totals: 1 rotations  |  4 distinct exit IPs  |  3 failed checks
=======================================================================================================
```

In `both` mode the current-address column becomes two:

```
================================================================================================================================================

  Session monitoring ran for: 1h0m0s  |  Cycles: 60  |  Total checks: 120

  #     Proxy                                 Checks   Fails   Rotations   IPs     Current IPv4        Current IPv6
  ----------------------------------------------------------------------------------------------------------------------------------------------
  1     user-session1:pw@gw.example.com:9000  60       0       1           3       88.4.201.3          2a02:908:1c6::7
  2     user-session2:pw@gw.example.com:9001  60       3       0           2       45.12.8.9           2a02:908:1c6::9

  Fleet totals: 1 rotations  |  4 distinct exit IPs  |  3 failed checks
================================================================================================================================================
```

**Rotations** counts **per family**, so a check that rotated both exits counts two. A proxy that flips between two addresses shows a high rotation count against a low distinct-address count — that pattern is a pool cycling a small block, not a pool of many addresses.

**IPs** is the distinct addresses that proxy showed across every tracked family, summed: in `both` mode a proxy with one v4 exit and two v6 exits shows `3`.

## The log file

Failures and rotations are appended to `results/session-monitor.log`:

```
2026-09-29 09:15:03  ROTATED  user-session1:pw@gw.example.com:9000  IPv4  45.12.8.7 -> 88.4.201.3  held 1m1s
2026-09-29 09:16:04  FAIL  user-session2:pw@gw.example.com:9001  i/o timeout
2026-09-29 09:17:02  PARTIAL  user-session2:pw@gw.example.com:9001  IPv6: [https://v6.ident.me] dial tcp: no route to host
```

A `ROTATED` line names the family. A `PARTIAL` line records a family that gave no answer on a check that otherwise succeeded — the reason is logged so you can tell a proxy with no route for that family from an endpoint having a bad day, but it is not an alert and not a rotation.

Append-only across runs, like the downtime monitor's log.

## Configuration summary

| Key | Purpose | Default |
|-----|---------|---------|
| `session_interval_ms` | Prompt default for the gap between cycles | `60000` |
| `ip_mode` | Prompt default for which address families to look up | `ipv4` |
| `discord_webhook` | Discord webhook URL; blank = disabled | — |

See [Configuration](../getting-started/configuration.md) for the full file.
