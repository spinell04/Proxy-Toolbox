# Compare Results

## What it does

Opens a local web dashboard over the CSVs you have already exported to `results/`, so you can read several runs side by side instead of one terminal dump at a time.

Reach for it when the question spans more than one run:

- "Did this proxy file get worse over the last three weeks?"
- "Which proxies survived both the ping test *and* the Ticketmaster test?"
- "We changed provider — is the new list actually faster, and for which proxies?"
- "Four providers, four lists, same target — how do the four compare?"
- "This run looks bad. Which proxies were slowest, and what were the failures?"

It is a reader, not a tester. It runs nothing against your proxies and never touches the network beyond your own machine.

## Starting and stopping it

Pick **Compare Results** from the main menu. The tool prints the address and opens your browser:

```
Compare dashboard running at http://127.0.0.1:52431
Press Enter to stop the dashboard...
```

The OS picks a free port each time, so the address is rarely the same twice. If the browser doesn't open by itself, paste the printed address.

Press **Enter** in the terminal to stop the server and return to the menu. The page in your browser will report that it can no longer reach the server; that's expected, not a crash.

## Local and read-only

This matters more here than for the other tools, because the CSVs contain `user:pass@host:port` for every proxy you own, in plaintext.

- The server binds **`127.0.0.1`** only, so nothing on your network can reach it.
- Its API answers **GET** and nothing else — any other method gets `405`. Nothing it serves writes to `results/`, or anywhere else on disk.
- It refuses any request whose `Host` header is not a loopback literal (`127.0.0.1`, `[::1]`, or `localhost`), answering `403`.

That last check is the one worth understanding. Binding to loopback stops packets arriving from the network, but it does not stop a page you are *already viewing* from pointing a hostname it controls at `127.0.0.1` — at which point the browser treats this dashboard as that page's own origin and reads your credentials out of it. A rebound request still carries the attacker's hostname in its `Host` header, so checking that header is what tells the two cases apart.

The practical consequence: reach the dashboard at the printed `127.0.0.1` address (or `localhost`). Putting it behind any other hostname gets a `403`.

## What it reads

Every `.csv` in `results/`, and nothing else. There is no configuration — no keys in [`config.txt`](../getting-started/configuration.md), no setup step. Export a run, reload the page, and it appears as a row.

Files are read fresh each time the page loads, so after exporting a new run, reload rather than restarting the tool.

## The run inventory

The landing table is one row per CSV:

| Column | What it is |
|--------|------------|
| **File** | The CSV's name |
| **Tool**, **Run at**, **Proxy file**, **Target**, **Workers** | Read from the run's metadata rows (see [Exporting Results](../reference/exporting-results.md#the-metadata-block)) |
| **n** | Result rows in the file |
| **Success** | Share of proxies that succeeded |
| **p50** | Median latency, across successes that reported one. Nearest-rank: the samples are sorted and the value at rank `ceil(p × n)` is reported, never an interpolation, so the figure is always a latency some real proxy recorded. The CLI tools compute their percentiles the same way, so the two agree. |
| **Errors** | Share that never got a usable response |

Click a column header to sort by it. Values that were never recorded always sort last, whichever direction you sort in.

Tick the rows you want to compare. The views below the table change as you tick — there is no "compare" button.

A selection is capped at **50 runs**. At the cap the remaining checkboxes stop accepting ticks and a line explains why; clear one to pick another.

## The six views

Which views appear depends on what you selected:

| Selection | View | What it's for |
|-----------|------|---------------|
| *(none)* | **Run inventory** | The table itself — every export at a glance, sortable |
| One run | **Run detail** | Percentiles, latency distribution, slowest proxies, and failure breakdown |
| Two or more runs of the same tool, same proxy file | **Timeline** | Success rate, error rate and p50 against run time; which kinds of failure moved; and the latency distributions overlaid |
| Exactly two runs of the same tool, same proxy file | **Paired** | Per-proxy before/after: each proxy plotted A against B, who got faster, who got slower, top movers, and which proxies changed outcome |
| Two or more runs of the same tool, **different** proxy files | **Head-to-head** | Provider against provider: outcome composition, percentile table, distribution overlay, failure codes |
| Two or more runs that are **not** all one tool | **Cross-tool** | The same proxies seen through different targets — matrix, funnel, set operations, correlation |

Two runs of one tool against one proxy file match both Timeline and Paired, so you get both, Timeline first.

Runs exported before the metadata rows existed record no proxy file, so they cannot be sorted into "same list" or "different lists" and keep the Timeline and Paired they have always had.

If you tick rows that no view can render, the page says so and lists what each view needs, rather than going blank.

## The comparability banner

Above the Timeline sits a verdict on whether the runs are actually comparable. Head-to-head does not use it, and runs its own narrower check instead — see [Comparing providers](#comparing-providers).

A trend line across four ping runs looks exactly the same whether your proxies degraded or you pointed the third run at a different target, swapped the proxy file, doubled the worker count, or asked for a different address family. The chart cannot tell those apart. So before you reach it, the banner compares **Target**, **Proxy file**, **Workers** and **IP mode** across the selection and, when any of them differs, says so in the clear:

> **Not directly comparable** — One of target, proxy file, worker count and IP mode changed across these runs. A movement in the charts below may be that change rather than a change in the proxies.

…followed by which values were used and which files used each. When all four match, it says the opposite, equally plainly.

**IP mode** is in that list because an IP Uniqueness Test run that asked for IPv4 exits and one that asked for IPv6 exits measured two different address spaces. Their uniqueness figures are not two readings of one thing, and setting them on a shared axis is exactly the error this banner exists to catch.

This is the entire reason the exporters were taught to write metadata rows into the CSV. Without those rows there is nothing to compare, and a chart quietly attributes a changed test to degraded proxies.

A value that was never recorded is not treated as another opinion — it is set aside and counted separately. The IP Uniqueness Test writes no target at all, for instance, because it doesn't have one, and no tool but the IP Uniqueness Test records an IP mode. An export made before IP modes existed records none either, and that reads as unknown rather than as a difference.

## Older exports

CSVs exported before the metadata rows existed still parse and still show up in the inventory. What they lack is the description of the run:

- Their **Tool**, **Run at**, **Proxy file**, **Target** and **Workers** columns read `unknown`. The same goes for **IP mode** on a run's detail page, which reads `not recorded` for every export made before address families were selectable — and for every tool but the IP Uniqueness Test, which is the only one that looks up an exit IP. (A run that *does* carry metadata but genuinely had no target — the IP Uniqueness Test — reads `n/a` instead, so the two cases stay distinguishable.)
- With no run time, they cannot be placed on the time axis, so the Timeline's trend charts leave them out — named, not silently dropped — and they are not checked for comparability. They still appear in the latency-distribution chart, which has no time axis.
- With no tool name, they can't be grouped with anything by tool, so a selection containing one is never "same tool". Beside a ping run, such a file lands in the Cross-tool view with its column labelled `unknown`, which is the honest answer: the join is per proxy and doesn't care what produced a column.

Re-running the export is the only way to give an old file its metadata. Nothing is inferred from the filename.

## Comparing providers

Tick two or more runs of the same tool that were run against **different proxy files** and you get **Head-to-head**: the view that sets several proxy lists side by side.

It is a different question from the other two multi-run views, and they are built on the opposite assumption. Timeline and Paired both take one population measured more than once — Paired joins on the proxy itself and reports only the proxies present in both runs, Timeline puts the runs on a time axis and draws a trend through them. Two providers share no proxies, so Paired would have nothing to join and Timeline would draw a trend between two unrelated things. Head-to-head treats each proxy file as its own population and never joins them.

Head-to-head does not show the Timeline's four-field comparability banner. A differing proxy file is the premise of this view, so a banner that reported it would fire on every selection. What it checks instead is **target**, **worker count** and **IP mode**: if any differs across the runs, a line above the panels says which, and what that does to the comparison. If all three match, the view says nothing — there is nothing to warn about.

Those three matter more here than in Timeline, not less. There, a changed target moves a trend you can still see moving; here it silently *becomes* the whole difference between two providers. Comparing a v4 run of one provider against a v6 run of another is that error in its purest form — two address spaces, one column each, and nothing on screen saying so unless this line says it.

The one other case it reports is when fewer than two of the selected runs recorded any metadata at all, so the check could not be made.

Providers are named by their proxy file, minus the extension, in every panel, and the columns stay in that order whichever metric you are reading.

### The four panels

| Panel | The question it answers |
|-------|-------------------------|
| **Outcome composition** | The same outcomes as bars normalised to 100%, with the raw counts beside them |
| **Latency** | min, p25, p50, p75, p90, max, mean, std dev and IQR per provider, the lowest figure in each row marked |
| **Latency distribution** | The full shape, one curve per provider — percentile across the bottom, latency up the left on a log scale, so reading up from any percentile gives every provider's latency there at once. Each provider gets a distinct hue *and* a distinct dash pattern, so the curves stay separable in greyscale and under colour blindness. Lower is faster; drag or scroll on the chart to zoom, and a reset button appears once you have |
| **Error codes** | One row per failure code, one column per provider; expand a row for the proxies carrying it |

Every share is taken against that provider's own total, so files of different lengths stay comparable. A count alone is misleading the moment two lists differ in length: 340/500 is the larger count and the smaller rate against 80/100.

The percentile table and the distribution are not supporting detail. Providers routinely tie at 100% working, at which point the outcome rates separate none of them and the whole difference is distributional. Reading a column down, rather than any one row across, is what the table is for.

### Failure codes, not failure messages

Failures are grouped by a short **code**, never by the message text. For a failed row every tool writes the literal status `ERROR`, and the message is a boilerplate prefix repeated verbatim on each row — `Get "https://www.ticketmaster.de": unexpected EOF` — so the text separates nothing the code does not already say.

A code is the **HTTP status** where the proxy got one (`403`, `429` — a refusal, not a broken connection), and the **kind of transport failure** otherwise:

`auth`, `timeout`, `conn_refused`, `conn_reset`, `tls`, `dns`, `eof`, and `other` for anything that matched none of those.

Expanding a row lists the proxies carrying that code, grouped by provider, and nothing else. The one exception is `other` and `unknown`: those mean "matched no known pattern", so the raw messages are kept for them — without the text they are unactionable, and they are also the only signal that the taxonomy is missing an entry.

A code a provider never produced reads `0`, not a blank. A blank would read as "not measured"; here it means "never happened", and on a clean provider that is the finding.

### What the view does not decide for you

Fisher's exact test and a 95% Wilson interval on each provider's working rate are both computed server-side and served by `/api/compare`, and **neither is displayed**. The view reports the measured figures and leaves the reading to you.

If you want the significance of a working-rate gap, it is in the API payload: `pairwise` carries a two-sided p-value and a `distinguishable` flag for every pair of selected runs, and each run's `success` object carries its interval.

Two things it deliberately does not do: it does not rank the providers by a single score — which percentile matters is your call, not the dashboard's — and it runs no significance test on the latency figures, only on the working rate.

## Comparing across tools

A ping and a Ticketmaster request are different measurements. A ping is a raw CONNECT through the proxy; a Ticketmaster fetch is a full TLS-fingerprinted request at a host actively trying to refuse it. The dashboard never merges them into one series.

What it does instead is join **per proxy**, on the canonical `user:pass@host:port` identifier that every tool now writes. Counting proxies across tools is meaningful — a proxy either worked or it didn't, in each tool's own terms — so:

- The **matrix** gives one row per proxy and one column per run, tinting each column against its own p90, never against another column's. Rows can be collapsed by exit IP or by exit /24 when the selection includes an IP Uniqueness Test run to supply the exit addresses.
- The **funnel** counts proxies surviving each run in turn, cumulatively: "500 loaded, 412 ping OK, 380 Ticketmaster OK, 290 Bayern OK". It counts proxies and never milliseconds.
- **Set operations** answer "OK in A but not in B", "OK in both", and so on. The matching proxies are previewed on the page, and a button hands you the full list as a `.txt` — that is a browser download, assembled in the page; the dashboard still writes nothing itself.
- The **correlation** panel plots one run's latency against another's, per proxy, with two separately labelled axes, no diagonal and no fitted line.

### Expect the correlation to be near zero

On realistic data it usually is. Every honest pair measured while building this view landed between −0.05 and 0.16.

That is a result, not a malfunction. It says a proxy's latency in one of the two runs carries almost no information about its latency in the other: they are different measurements, and a position in one does not place a proxy in the other.

The panel writes the coefficient out in words underneath the chart, so you don't have to remember what 0.31 means.

## See also

- [Exporting Results](../reference/exporting-results.md) — the CSV format the dashboard reads
