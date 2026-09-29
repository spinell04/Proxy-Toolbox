# Provider Head-to-Head Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development.

**Goal:** Add a Head-to-head view that compares providers as independent
populations, and route provider selections to it instead of Timeline/Paired.

**Design:** `docs/plans/2026-09-19-provider-head-to-head-design.md` — read it
first. It carries the evidence behind every decision here.

**Constraints, inherited from the existing work and not negotiable:**
- Go stdlib only. `go.mod` is not touched.
- No new frontend dependencies. Plain ES modules, no bundler. uPlot is already
  vendored at `internal/dashboard/web/vendor/`.
- No commits. Work directly on `main`, leave everything uncommitted.
- `go build ./... && go vet ./... && gofmt -l .` clean, all tests pass, before
  any task is called done.
- Match the surrounding code's comment density. Comments explain *why*, and the
  existing files are the reference for the register.

---

## Task 1: Error codes — `internal/compare/codes.go`

**Files:** create `internal/compare/codes.go`, `internal/compare/codes_test.go`

A "code" identifies a failure kind in one short token.

```go
// Code returns the short token identifying a non-OK result...
func Code(r ProxyResult) string
```

Rules, in order:
- `Outcome == OutcomeOK` → `""` (not a failure; callers skip it).
- `Status >= 400` → the decimal status, e.g. `"403"`, `"429"`.
- `ErrorKind != ""` → the kind, e.g. `"timeout"`, `"eof"`.
- otherwise → `"unknown"`.

Status takes precedence over kind: a 403 that also carries an error string is
a block, and the block is the more specific fact.

```go
// CodeGroup is one failure code within one run.
type CodeGroup struct {
	Code     string   `json:"code"`
	Count    int      `json:"count"`
	Proxies  []string `json:"proxies"`  // ProxyIDs carrying this code, input order
	Messages []string `json:"messages"` // raw errors, ONLY for code "other"/"unknown"
}

// CodeBreakdown groups non-OK results by Code, ordered by descending count
// and then by code for a stable tie-break.
func CodeBreakdown(results []ProxyResult) []CodeGroup
```

`Messages` is populated *only* for `other` and `unknown`, deduplicated,
preserving first-seen order. Every other code suppresses it — see the design
doc. Messages must be capped at 20 entries; a pathological run must not ship an
unbounded payload.

**Tests must cover:** OK results excluded; status wins over kind; the four real
mobile.csv error strings bucketing to `timeout` x4 and `eof` x3; `other`
carrying messages while `timeout` carries none; message dedup; the 20-cap;
descending-count ordering with an alphabetical tie-break; empty input returning
an empty (not nil-panicking) result.

---

## Task 2: Wilson interval — `internal/compare/proportion.go`

**Files:** create `internal/compare/proportion.go`, `proportion_test.go`

```go
// Interval is a proportion with a confidence interval.
type Interval struct {
	Rate  float64 `json:"rate"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
}

// WilsonInterval returns the 95% Wilson score interval for k successes in n
// trials. n <= 0 returns a zero Interval.
func WilsonInterval(k, n int) Interval
```

Wilson, not normal approximation: the latter gives a zero-width interval at
k == n, which is where three of the four reference providers sit. Use z =
1.96. Clamp `Low` at 0 and `High` at 1.

**Tests must cover:** `n == 0` → zero value, no NaN, no panic; `k == n`
(100/100) → `High == 1` and `Low` strictly below 1 and above 0.9; `k == 0` →
`Low == 0`, `High` above 0; 93/100 → interval containing 0.93 and excluding
1.0; a known reference value checked to 4dp against a hand-computed figure
stated in the test; wider interval for n=10 than n=1000 at the same rate.

Assert on values, never on a value recomputed by the formula under test.

---

## Task 3: API surface — `internal/dashboard/api.go`

**Files:** modify `internal/dashboard/api.go`, `api_test.go`

Extend `runOverview` with:

```go
Codes    []compare.CodeGroup `json:"codes"`
Success  compare.Interval    `json:"success"`
```

`Success` is `WilsonInterval(summary.OK, summary.Total)`.

Leave the existing `Errors map[string]int` field alone — `crosstool.js` reads
it, and this task does not touch that view.

**Tests must cover:** the new fields present and correct in the `/api/compare`
payload for a fixture run; a run with no failures shipping an empty `codes`
array (not `null` — the frontend maps over it).

---

## Task 4: Selection routing — `registry.js`, `timeline.js`, `paired.js`

**Files:** modify `internal/dashboard/web/views/registry.js`, `timeline.js`,
`paired.js`

Add to the Selection built by `selectionFrom`:

```js
proxyFiles,     // distinct non-empty proxyFile values
sameProxyFile,  // true when every run records a proxy file and all are equal
```

`sameProxyFile` requires *every* run to carry one, mirroring how `sameTool`
requires every run to carry a tool. A run with no recorded proxy file makes the
answer unknown, not true.

Then narrow both existing matches:

- `timeline`: `selection.sameTool && selection.runs.length >= 2 && !selection.differentProxyFiles`
- `paired`: same, plus `length === 2`

where `differentProxyFiles` is `proxyFiles.length > 1`. Note the asymmetry and
keep it: the guard is on *known-different*, so pre-metadata runs (no proxy
file recorded at all) keep today's behaviour rather than silently losing their
views.

Update `unmatched()` in registry.js to describe Head-to-head alongside the
others.

**Tests:** there is no JS test harness in this repo. Verify by driving the real
dashboard — see Task 11.

---

## Task 5: Head-to-head shell — `views/headtohead.js`

**Files:** create `internal/dashboard/web/views/headtohead.js`; modify
`internal/dashboard/web/views/index.js`

```js
register({
  id: 'headtohead',
  title: 'Head-to-head',
  match: (s) => s.sameTool && s.runs.length >= 2 && s.proxyFiles.length > 1,
  mount,
});
```

`mount` fetches `/api/compare`, orders runs by provider name, and appends the
panels from Tasks 6-10 in the design doc's order. Return a teardown composing
every chart teardown, following `paired.js`.

Provider label: `run.proxyFile` with its extension stripped, falling back to
`run.file`. Put this in `views/provider.js` as `providerName(run)` — Tasks 6-10
all need it and it must not be reimplemented four times.

Above everything, reuse the existing comparability banner from
`views/comparability.js`. A differing `Target` or `Workers` between providers
invalidates the comparison outright, and it matters more here than in Timeline,
not less. If the banner reports a mismatch, it stays above the scoreboard.

Import it into `views/index.js` with a comment stating that it is mutually
exclusive with timeline and paired.

---

## Task 6: Scoreboard — `views/scoreboard.js`

A column per provider, a row per metric: Total, Working, Blocked, Errored.
Each cell carries the count and the share of that provider's total. The leader
per row is marked (a class plus a visually-hidden "best" for screen readers,
never colour alone).

Under the Working row, print each provider's Wilson interval, and where two
providers' intervals overlap, say plainly that the gap is not yet
distinguishable at these sample sizes. Do **not** suppress the numbers — state
the gap and the caveat together.

This is a real table (`<table>`), not a grid of divs. It is tabular data and a
screen reader user needs the row/column association.

---

## Task 7: Outcome composition — `views/composition.js`

One normalised stacked bar per provider: ok / blocked / error, to 100%. Raw
counts in the axis label. Use the semantic tokens the inventory table already
uses for those three outcomes; do not introduce a new palette.

Accessible fallback via `dataFallback`/`plainTable` from `views/datatable.js`,
carrying the same counts and shares.

---

## Task 8: Latency table — `views/latencytable.js`

Rows: min, p25, p50, p75, p90, p95, p99, max, mean, stddev, IQR. A column per
provider. Best-in-row marked as in Task 6 — lowest wins for every row here,
including stddev and IQR (less spread is better).

Above the table, one line naming the p50/p95 disagreement when it occurs: if
the provider with the best p50 is not the provider with the best p95, say so
explicitly. That is the finding the whole view exists to surface, and a reader
scanning a grid of numbers will miss it.

A provider whose `summary.latencyCount == 0` renders as not-recorded, never as
zero. `latencyCount`, not `ok` — see the note in `timeline.js`.

---

## Task 9: Distribution overlay — modify `views/ecdf.js`

Add an optional fifth parameter:

```js
export function ecdfPanel(root, dated, undated, hasTime, labelOf = defaultLabel)
```

`defaultLabel` is today's behaviour exactly — `runTime(run.runAt) || 'undated'`
— so Timeline is unchanged. Head-to-head passes `providerName`. The label is
used in the key list *and* in the accessible summary; both must change
together, or the fallback describes curves by a name the chart does not use.

Head-to-head calls it as `ecdfPanel(root, runs, [], () => true, providerName)`.

---

## Task 10: Error codes panel — `views/codes.js`

A table: one row per code seen in any provider, a column per provider, each
cell the count and the share of that provider's total.

Each row expands to the proxies carrying that code. Use a real
`<button aria-expanded>` controlling a container, not a bare `<details>` inside
a table cell — the latter breaks table semantics. Collapsed by default.

Expanded content: the proxy IDs, one per line, middle-truncated for display
with the full value in `title` and selectable for copy. For `other` and
`unknown` only, also list the distinct raw messages. No messages for any other
code.

A code absent from a provider shows `0`, not a blank — a blank reads as "not
measured" when it means "never happened", and that distinction is the point of
the panel.

Order rows by total count across all providers, descending.

---

## Task 11: Verify against real data, then document

**Files:** modify `docs/tools/compare-results.md`; create nothing new unless
the tool docs need it.

`results/` already holds four real TM Request Tester exports: `schro.csv`,
`premium.csv`, `private.csv`, `mobile.csv`. Use them.

Start the dashboard, then drive it with Playwright MCP:

1. Select schro + mobile → Head-to-head renders, Timeline and Paired do not.
2. Confirm the scoreboard shows 100% vs 93%, and that the Wilson note appears.
3. Confirm the error panel shows `timeout` 4 and `eof` 3 for mobile, 0 for
   schro, and that expanding `timeout` lists 4 proxies.
4. Select all four → four columns, and the p50/p95 disagreement line appears
   (private has the best p95 at 1253ms but not the best p50).
5. Select schro twice-exported-as-itself is not possible; instead select a
   single run and confirm Run detail still matches, and that no selection
   regressed.
6. Screenshot at 1440 and at 390 wide. Confirm no horizontal page scroll at
   390 outside the data tables.
7. Re-run the axe accessibility check as the earlier UI tasks did.

Then document the view in `docs/tools/compare-results.md`: when it appears,
what each panel answers, and the error-code taxonomy. Keep the register of the
surrounding docs.

**Report honestly.** If a panel is wrong against real data, fix it; if
something cannot be verified, say which and why.
