# Provider Head-to-Head — Design

**Status:** approved 2026-09-19; revised 2026-09-19 (p95/p99 dropped from every
display, the median/tail callout removed, the comparability banner replaced by
a narrower target-and-workers check, and the copy made non-advisory)

## Problem

The compare dashboard shipped with two multi-run views, and both encode an
unstated assumption: *the same proxies, measured at different times*.

- **Paired** joins on proxy ID and computes every figure on the intersection.
- **Timeline** places runs on a time axis and draws a trend through them.

The question this selection actually poses is different: *how do these two
providers' figures compare?* Two providers are **disjoint populations** — not
one population measured twice. Selecting `schro.csv` and `mobile.csv` therefore produced:

- Paired: "These two runs share no proxies", and nothing else.
- Timeline: a two-point trend line between two unrelated providers, which
  implies a temporal relationship that does not exist. Worse than the empty
  panel, because it renders something plausible.

Neither is a defect in those views. There is simply no view for the question.

## Grounding data

Four real TM Request Tester exports, 100 proxies each, same target, same
worker count, taken minutes apart:

| file | working | mean | p50 | p90 | std dev |
|---|---|---|---|---|---|
| schro | 100% | 1004ms | 728ms | 1329ms | 1029ms |
| premium | 100% | 874ms | 720ms | 1153ms | 734ms |
| private | 100% | 855ms | 754ms | 1102ms | 351ms |
| mobile | 93% | 1585ms | 1057ms | 2387ms | 1739ms |

Percentiles here are nearest-rank. The summary rows *inside* those four files
show slightly different figures, because they were exported before the tools
and the dashboard were unified on one definition — see
`2026-09-19-percentile-unification` in the plan. The detail rows, which every
figure above is recomputed from, are unaffected.

Two facts from this shape the design:

1. **Three providers tie at 100% working.** A scoreboard of success rates —
   the first thing asked for — separates none of them. The separation is
   entirely distributional.
2. **The centre and the spread do not order the providers the same way.**
   `premium` has the lowest median (720ms); `private` has the higher median
   of the two (754ms) and the lower p90 (1102ms against 1153ms) and about
   half the standard deviation (351ms against 734ms). No single row orders
   the three providers that tie at 100% working.

   The dashboard does not resolve that for the reader, and an earlier version
   that did — a callout naming the disagreement between the best p50 and the
   best p95 — has been removed. It editorialised, and it was built on a
   percentile the dashboard no longer displays.

So the percentile table and the distribution overlay carry equal weight with
the scoreboard. They are not supporting detail beneath it.

## The view

**Head-to-head**, matching *same tool, differing proxy files*, N providers.

1. **Scoreboard** — a column per provider: working / blocked / errored, each
   as a count *and* as a share of that provider's total, with the gap to the
   leader in percentage points. Counts alone mislead the moment two files have
   different lengths; 340/500 against 80/100 is the trap.
2. **Outcome composition** — stacked bars normalised to 100%, raw counts on
   the axis label.
3. **Latency table** — min, p25, p50, p75, p90, max, mean, stddev, IQR per
   provider, the lowest figure in each row marked. p95 and p99 are computed
   by `compare.Summary` and deliberately not displayed anywhere on the page.
4. **Distribution overlay** — the existing ECDF panel, relabelled by provider.
   It is distribution-based and needs no join, so it already works on disjoint
   sets unchanged.
5. **Error codes** — a row per code, a column per provider, count and share.
   Each row expands to the proxies carrying that code.

### Error codes, not error messages

`Status` cannot be the grouping key. For a failed row the four tools write the
literal string `ERROR`, so every failure lands in one bucket carrying no
information. The signal is in the message suffix; the prefix is boilerplate
repeated verbatim on every row:

```
Get "https://www.ticketmaster.de": unexpected EOF                 x3
Get "https://www.ticketmaster.de": context deadline exceeded ...  x3
Get "https://www.ticketmaster.de": read tcp ...: i/o timeout ...   x1
```

So a code is **the HTTP status where one exists** (403, 429 for blocked) and
**the taxonomy kind otherwise** (`timeout`, `eof`, `tls`, `auth`, `dns`,
`conn_refused`, `conn_reset`). `classifyError` already buckets these exact
strings; mobile becomes `timeout x4, eof x3`.

Expanded rows list the proxy alone. The raw message is suppressed — it is
noise, by the evidence above.

**One exception:** the `other` bucket keeps its raw messages. `other` means
"matched no known pattern", so `other x5` with the message suppressed is
unactionable by construction, and it is also the only signal that the taxonomy
needs a new entry.

### Is the gap real?

Sample sizes differ between providers, and a rate difference can be sampling
noise. Each rate carries a 95% Wilson score interval. Wilson rather than
normal-approximation: the latter degenerates to a zero-width interval at 100%,
which is exactly where three of the four reference providers sit.

**The pairwise claim is a separate statistic, and the first design got this
wrong.** The original rule was "mark a gap as not distinguishable when the two
intervals overlap". Comparing two confidence intervals for overlap is a known
fallacy — it is far too conservative — and it fails on the reference data:

| provider | rate | Wilson 95% |
|---|---|---|
| schro | 100/100 | [0.9630, 1.0000] |
| mobile | 93/100 | [0.8625, 0.9657] |

Those overlap in [0.9630, 0.9657], so the overlap rule reports "not
distinguishable". Fisher's exact test on the same table gives one-sided
p = 0.00701, two-sided = 0.014. The difference is larger than sampling noise,
and the overlap rule would have reported that it was not.

So: the interval is shown per provider, as a description of that provider's
own precision. The pairwise "is this gap real" claim uses **Fisher's exact
test** at the 5% level. Exact rather than a chi-square or two-proportion z:
the interesting cells here are small (7 failures) and one arm sits at exactly
100%, where normal approximations are least trustworthy.

### Comparability

Timeline compares **target**, **proxy file** and **worker count** across the
selection and warns when any of the three differs. Head-to-head does not use
that check: a differing proxy file is this view's premise, so the banner fired
on every selection and a second paragraph had to explain it away.

Head-to-head checks **target** and **worker count** only. If either differs it
says which, and what that does to the comparison. If both match it says
nothing — the view's own premise needs no defending. The only other line it
can show is that fewer than two of the runs recorded metadata, so the check
could not be made at all.

### Routing

Timeline and Paired narrow to selections whose runs share one proxy file.
Runs with no recorded proxy file (pre-metadata exports) keep today's
behaviour, since nothing distinguishes the two cases for them.

| selection | view |
|---|---|
| one run | Run detail |
| same tool, same proxy file, 2+ | Timeline, Paired |
| same tool, differing proxy files, 2+ | Head-to-head |
| more than one tool | Cross-tool |

## Out of scope

- Ranking providers by a single composite score. Which percentile matters is
  the reader's call, not the dashboard's.
- Any statement about which provider to choose. The dashboard reports
  arithmetic about the runs it was given and stops there.
- Significance testing on latency distributions.
- Any new measurement. This stays a post-hoc viewer over exported CSVs.
