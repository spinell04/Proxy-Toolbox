// The detail view's graphic: the latency histogram.
//
// Split from detail.js so that file carries the shape of the page and not also
// the arithmetic behind the chart.

import { count, el, ms, pct, plural } from '../format.js';
import { axis, baseOpts, cursorReadout, figure, keyList, mountChart, panel } from './chart.js';
import { dataFallback, plainTable } from './datatable.js';

/**
 * histogramPanel draws the run's latency distribution as bars.
 *
 * Plotted against bucket *index*, not against the bucket's lower bound. The
 * bounds are integer truncations of float edges, so when the bucket count
 * exceeds the value range several adjacent buckets report identical bounds —
 * a known and accepted behaviour of compare.Histogram. An x axis built from
 * them would collapse those bars on top of each other; the index is unique by
 * construction and the bounds are printed as the axis labels instead.
 */
export function histogramPanel(root, run) {
  const buckets = (run.histogram && run.histogram.buckets) || [];
  const samples = run.summary.latencyCount;

  if (samples === 0 || buckets.length === 0) {
    root.append(
      panel({
        label: 'Latency distribution',
        note: 'Successful results that reported no elapsed time are counted as successes and contribute no sample, which is why a run can succeed and still have no distribution.',
        children: el('p', {
          class: 'panel__empty',
          text: 'This run recorded no latency sample, so there is no distribution to draw.',
        }),
      }),
    );
    return null;
  }

  const xs = buckets.map((_, i) => i);
  const counts = buckets.map((bucket) => bucket.count);
  const peak = buckets.reduce((best, b) => (b.count > best.count ? b : best), buckets[0]);
  const label = (i) => `${count(buckets[i].lo)}–${count(buckets[i].hi)}ms`;

  const keys = keyList([
    { label: 'Proxies', sub: plural(samples, 'timed sample', 'timed samples'), color: 'var(--accent)' },
  ]);

  const { node, host } = figure({
    summary: `Histogram of successful latencies across ${count(samples)} samples in ${count(
      buckets.length,
    )} buckets, from ${ms(run.summary.min)} to ${ms(run.summary.max)}. The tallest bucket is ${count(
      peak.lo,
    )} to ${count(peak.hi)} milliseconds with ${plural(
      peak.count,
      'proxy',
      'proxies',
    )}. The median is ${ms(run.summary.p50)}. The same figures are in the table below.`,
    keys: keys.node,
    children: dataFallback(
      'Bucket counts',
      plainTable({
        caption: `Latency histogram, ${buckets.length} buckets.`,
        columns: [
          {
            key: 'range',
            label: 'Latency',
            cell: (row) => el('span', { class: 'num', text: label(row.i) }),
          },
          {
            key: 'count',
            label: 'Proxies',
            num: true,
            cell: (row) => document.createTextNode(count(counts[row.i])),
          },
          {
            key: 'share',
            label: 'Share',
            num: true,
            cell: (row) => document.createTextNode(pct(counts[row.i] / samples)),
          },
        ],
        rows: buckets.map((_, i) => ({ i })),
      }),
    ),
  });

  root.append(
    panel({
      label: 'Latency distribution',
      note: 'Successful results only. A long right tail with a tight body is a few slow proxies rather than a uniformly slow file; the percentiles below give the same shape as figures.',
      children: node,
    }),
  );

  return mountChart(host, (t, width) => ({
    data: [xs, counts],
    opts: {
      ...baseOpts(),
      height: Math.round(Math.min(340, Math.max(190, width * 0.36))),
      scales: { x: { time: false }, y: { range: (_, __, max) => [0, max] } },
      axes: [
        axis(t, {
          size: 40,
          space: 70,
          values: (_, ticks) => ticks.map((v) => (buckets[v] ? `${count(buckets[v].lo)}ms` : '')),
        }),
        axis(t, { side: 3, size: 46, values: (_, ticks) => ticks.map((v) => count(v)) }),
      ],
      series: [
        {},
        {
          stroke: t.accent,
          fill: t.wash(t.accent),
          width: 1,
          paths: window.uPlot.paths.bars({ size: [0.9, Infinity] }),
          points: { show: false },
        },
      ],
      hooks: cursorReadout(keys, (idx) => [`${label(idx)} · ${count(counts[idx])}`]),
    },
  }));
}
