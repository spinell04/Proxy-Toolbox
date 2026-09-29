// Timeline — two or more runs of one tool, read as a trend.
//
// Three charts, in the order a regression is actually diagnosed: what moved,
// what kind of failure moved it, and whether the whole latency distribution
// shifted or only its tail. Above all three sits the comparability banner,
// which is the point of the view: a trend line is only evidence about the
// proxies if the test did not change underneath it.

import { fetchCompare } from '../api.js';
import { count, el, errorRate, ms, pct, runTime } from '../format.js';
import { register } from './registry.js';
import { banner, placedOnTimeAxis } from './comparability.js';
import { axis, baseOpts, chartHeight, cursorReadout, figure, keyList, mountChart, panel } from './chart.js';
import { dataFallback, plainTable } from './datatable.js';
import { ecdfPanel } from './ecdf.js';

/** hasTime reports whether a run can be placed on a time axis at all. */
const hasTime = (run) => Boolean(run.hasMeta && runTime(run.runAt));

/** seconds is the x value uPlot's time scale wants. */
const seconds = (run) => Date.parse(run.runAt) / 1000;

/** latency reads a percentile, or null when the run has no samples at all.
 *  latencyCount, not ok: a successful row reporting 0ms counts in ok but
 *  contributes nothing to a distribution, and a confident 0ms on a trend line
 *  is a lie the chart cannot walk back. */
const latency = (run, key) => (run.summary.latencyCount > 0 ? run.summary[key] : null);

register({
  id: 'timeline',
  title: 'Timeline',
  // Guarded on *known-different* proxy files rather than on not-same: runs
  // exported before run metadata record no proxy file at all, so proxyFiles is
  // empty for them and they keep the trend they have always had. Tightening
  // this to sameProxyFile would silently strip the view from every legacy
  // export, which is the opposite of the fix.
  match: (selection) =>
    selection.sameTool && selection.runs.length >= 2 && selection.proxyFiles.length <= 1,
  mount,
});

async function mount(root, selection) {
  const { runs } = await fetchCompare(selection.files);
  const { dated, undated } = placedOnTimeAxis(runs, hasTime);
  const withMeta = runs.filter((run) => run.hasMeta);

  const teardowns = [];
  const note = banner(dated, undated, withMeta);
  if (note) root.append(note);

  if (dated.length < 2) {
    root.append(
      el('div', { class: 'notice' }, [
        el('h3', { text: 'Not enough dated runs for a trend' }),
        el('p', {
          text: `A trend needs at least two runs with a recorded run time, and this selection has ${dated.length}. The latency distribution below still compares every selected run, because it has no time axis.`,
        }),
      ]),
    );
  } else {
    teardowns.push(trendPanel(root, dated));
    teardowns.push(errorPanel(root, dated));
  }

  teardowns.push(ecdfPanel(root, dated, undated, hasTime));

  return () => {
    for (const teardown of teardowns) if (teardown) teardown();
  };
}

/* ── 1. What moved ──────────────────────────────────────────────────────── */

/**
 * trendPanel plots the four headline figures against run time.
 *
 * Dual axis because two quantities are involved and averaging them would be
 * meaningless: rates are shares on the left, latencies are milliseconds on the
 * right. Success and errors carry the semantic tokens they already carry in
 * the inventory table; the two latency lines stay in ink, so the accent is not
 * spent on a series that is not a signal.
 *
 * Accessible fallback: a hidden summary and nothing more. Every figure on this
 * chart — success rate, error rate, p50, per run — is already a column of
 * the inventory table above, which is a real, sorted, named table. Repeating
 * it here would be a second copy to keep in step, not an improvement.
 */
function trendPanel(root, dated) {
  const first = dated[0];
  const last = dated[dated.length - 1];
  const swing = last.summary.successRate - first.summary.successRate;

  const keys = keyList([
    { label: 'Success', color: 'var(--good)' },
    { label: 'Errors', color: 'var(--bad)', dashed: true },
    { label: 'p50', color: 'var(--ink)' },
  ]);

  const { node, host } = figure({
    summary: `Line chart of four figures across ${dated.length} runs from ${runTime(first.runAt)} to ${runTime(
      last.runAt,
    )}. Success rate moves from ${pct(first.summary.successRate)} to ${pct(last.summary.successRate)}, a ${
      swing >= 0 ? 'rise' : 'fall'
    } of ${pct(Math.abs(swing))}; error rate from ${pct(errorRate(first.summary))} to ${pct(
      errorRate(last.summary),
    )}; median latency from ${ms(latency(first, 'p50'))} to ${ms(
      latency(last, 'p50'),
    )}. Every run's figures are also columns of the run inventory table above.`,
    keys: keys.node,
  });

  root.append(
    panel({
      label: 'Trend',
      note: 'Rates left, milliseconds right. The two are different quantities and never share a scale.',
      children: node,
    }),
  );

  const xs = dated.map(seconds);
  const series = [
    dated.map((run) => run.summary.successRate * 100),
    dated.map((run) => errorRate(run.summary) * 100),
    dated.map((run) => latency(run, 'p50')),
  ];

  return mountChart(host, (t, width) => ({
    data: [xs, ...series],
    opts: {
      ...baseOpts(),
      height: chartHeight(width),
      scales: { x: { time: true }, rate: { range: [0, 100] } },
      axes: [
        axis(t, { space: 70 }),
        axis(t, { scale: 'rate', side: 3, size: 46, values: (_, ticks) => ticks.map((v) => `${v}%`) }),
        axis(t, {
          scale: 'ms',
          side: 1,
          size: 56,
          grid: { show: false },
          values: (_, ticks) => ticks.map((v) => `${v}ms`),
        }),
      ],
      series: [
        {},
        line(t.good, 'rate'),
        line(t.bad, 'rate', [5, 3]),
        line(t.ink, 'ms'),
      ],
      hooks: cursorReadout(keys, (idx) => [
        pct(dated[idx].summary.successRate),
        pct(errorRate(dated[idx].summary)),
        ms(latency(dated[idx], 'p50')),
      ]),
    },
  }));
}

/** line is one trend series: never spanning a gap, because a gap is a run with no samples. */
function line(stroke, scale, dash) {
  return { scale, stroke, width: 1.75, dash, spanGaps: false, points: { show: true, size: 6 } };
}

/* ── 2. What kind of failure ────────────────────────────────────────────── */

/**
 * errorPanel stacks the error taxonomy across runs.
 *
 * The count alone hides the interesting case: forty errors that were timeouts
 * last week and are 407s today is a credential problem wearing a capacity
 * problem's clothes. Stacking shows the mix and the total at once.
 *
 * Accessible fallback: an adjacent data table, behind a disclosure. This is
 * the only place the taxonomy appears anywhere on the page, the exact counts
 * are what a reader acts on, and the table is small — one row per run, one
 * column per kind.
 */
function errorPanel(root, dated) {
  const kinds = rankedKinds(dated);
  if (kinds.length === 0) {
    root.append(
      panel({
        label: 'Error mix',
        children: el('p', { class: 'panel__empty', text: 'No errors were recorded in any dated run.' }),
      }),
    );
    return null;
  }

  // counts[k][i] is kind k's count in run i; stacked[k][i] is the running sum
  // through kind k, which is what the bands are actually drawn from.
  const counts = kinds.map((kind) => dated.map((run) => run.errors[kind] || 0));
  const stacked = counts.map((_, k) =>
    dated.map((_, i) => counts.slice(0, k + 1).reduce((sum, row) => sum + row[i], 0)),
  );
  const totals = kinds.map((_, k) => counts[k].reduce((a, b) => a + b, 0));

  const keys = keyList(
    kinds.map((kind, k) => ({
      label: kind,
      sub: `${count(totals[k])} across ${dated.length} runs`,
      color: `color-mix(in oklch, var(--bad) ${tintShare(k, kinds.length)}%, var(--warn))`,
    })),
  );

  const { node, host } = figure({
    summary: `Stacked area chart of error kinds across ${dated.length} runs. ${kinds
      .map((kind, k) => `${kind}, ${count(totals[k])} in total`)
      .join('; ')}. The same figures are in the table below.`,
    keys: keys.node,
    children: dataFallback(
      'Error counts per run',
      plainTable({
        caption: `Error kinds per run, ${dated.length} runs by ${kinds.length} kinds.`,
        columns: [
          { key: 'run', label: 'Run at', cell: (run) => el('span', { class: 'num', text: runTime(run.runAt) }) },
          { key: 'file', label: 'File', cell: (run) => el('code', { text: run.file }) },
          ...kinds.map((kind) => ({
            key: kind,
            label: kind,
            num: true,
            cell: (run) => document.createTextNode(count(run.errors[kind] || 0)),
          })),
          {
            key: 'total',
            label: 'All',
            num: true,
            cell: (run) => document.createTextNode(count(run.summary.errors)),
          },
        ],
        rows: dated,
      }),
    ),
  });

  root.append(
    panel({
      label: 'Error mix',
      note: 'Bands stack to the run’s total. A shift between bands is a change of cause, not of volume.',
      children: node,
    }),
  );

  const xs = dated.map(seconds);
  // Drawn back to front: the largest cumulative band first, each smaller one
  // painting over it. uPlot renders series in array order, so the order here
  // is the z-order, and the legend is fed separately from the raw counts.
  const order = kinds.map((_, k) => k).reverse();

  return mountChart(host, (t, width) => ({
    data: [xs, ...order.map((k) => stacked[k])],
    opts: {
      ...baseOpts(),
      height: chartHeight(width, 0.34),
      // Forced to zero. uPlot would fit the axis to the data, and a stacked
      // band measured from a floating baseline is not the share it looks like.
      scales: { y: { range: (_, __, max) => [0, max] } },
      axes: [
        axis(t, { space: 70 }),
        axis(t, { side: 3, size: 46, values: (_, ticks) => ticks.map((v) => count(v)) }),
      ],
      series: [
        {},
        ...order.map((k) => {
          const stroke = t.mix(t.bad, t.warn, tintShare(k, kinds.length));
          return { stroke, width: 1, fill: t.wash(stroke), spanGaps: true };
        }),
      ],
      hooks: cursorReadout(keys, (idx) => kinds.map((_, k) => count(counts[k][idx]))),
    },
  }));
}

/** tintShare spreads the kinds between the two alarm tokens — bad through warn. */
function tintShare(index, total) {
  return total <= 1 ? 100 : Math.round(100 - (70 * index) / (total - 1));
}

/** rankedKinds is every error kind seen in the selection, commonest first. */
function rankedKinds(runs) {
  const totals = new Map();
  for (const run of runs) {
    for (const [kind, n] of Object.entries(run.errors || {})) {
      totals.set(kind, (totals.get(kind) || 0) + n);
    }
  }
  return [...totals].sort((a, b) => b[1] - a[1]).map(([kind]) => kind);
}
