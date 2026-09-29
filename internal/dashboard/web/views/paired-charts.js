// The paired view's two charts: A against B, and the distribution of the change.
//
// Both are built from the proxies timed in *both* runs. A latency of 0 is what
// an errored row reports, not a measurement of zero milliseconds, so a proxy
// that failed in either run has no point on the scatter and no delta — it is
// counted in the status flips instead, which is where it means something.

import { count, el, ms, plural, signed } from '../format.js';
import { axis, baseOpts, cursorReadout, figure, keyList, mountChart, panel } from './chart.js';
import { dataFallback, plainTable } from './datatable.js';

/** DELTA_BUCKETS is odd so one bucket is centred on zero rather than straddling an edge. */
const DELTA_BUCKETS = 21;

/** OUTCOME_CLASSES splits a pair by what happened, not by how fast it was. */
const OUTCOME_CLASSES = [
  {
    key: 'held',
    label: 'OK in both',
    tint: (t) => t.good,
    css: 'var(--good)',
    test: (p) => p.a.outcome === 'ok' && p.b.outcome === 'ok',
  },
  {
    key: 'flipped',
    label: 'Outcome changed',
    tint: (t) => t.accent,
    css: 'var(--accent)',
    test: (p) => p.a.outcome !== p.b.outcome,
  },
  {
    key: 'failed',
    label: 'Not OK in either',
    tint: (t) => t.bad,
    css: 'var(--bad)',
    test: () => true,
  },
];

/** classify returns the first matching outcome class; the last one matches everything. */
const classify = (pair) => OUTCOME_CLASSES.findIndex((c) => c.test(pair));

/**
 * scatterPanel plots each proxy's latency in A against its latency in B.
 *
 * The y = x line is the whole reading: on it, nothing changed; above it, the
 * proxy got slower; below it, faster. The accent marks the points whose
 * outcome flipped, because that is the signal — a proxy that stopped
 * succeeding matters more than one that gained 40ms.
 *
 * Accessible fallback: a hidden summary with the counts either side of the
 * line and per outcome. Three hundred coordinate pairs are not a table
 * anyone reads; the movers table below is the tabular form of the same data,
 * showing the points a reader would actually go looking for.
 */
export function scatterPanel(root, timed) {
  if (timed.length === 0) {
    root.append(
      panel({
        label: 'A against B',
        children: el('p', {
          class: 'panel__empty',
          text: 'No proxy recorded a latency in both runs, so there is nothing to plot against the diagonal — and nothing to difference either, which is why there is no change-in-latency chart or top-movers table below. The status flips are still counted.',
        }),
      }),
    );
    return null;
  }

  // Sorted by A so the shared x array ascends, which is what uPlot's renderer
  // and cursor both assume; the y = x reference series then ascends with it.
  const sorted = [...timed].sort((p, q) => p.a.latencyMs - q.a.latencyMs);
  const bound = Math.round(Math.max(...sorted.map((p) => Math.max(p.a.latencyMs, p.b.latencyMs))) * 1.04);
  // Padded with the two corners of the plot. uPlot draws every series against
  // one x array, so without them the reference line would start at the fastest
  // proxy and stop at the slowest instead of crossing the whole square — and a
  // diagonal that does not reach the corners does not read as y = x.
  const xs = [0, ...sorted.map((p) => p.a.latencyMs), bound];
  const groups = OUTCOME_CLASSES.map((_, k) => [
    null,
    ...sorted.map((pair) => (classify(pair) === k ? pair.b.latencyMs : null)),
    null,
  ]);
  const present = OUTCOME_CLASSES.map((_, k) => groups[k].filter((v) => v !== null).length);
  const slower = sorted.filter((p) => p.b.latencyMs > p.a.latencyMs).length;
  const faster = sorted.filter((p) => p.b.latencyMs < p.a.latencyMs).length;

  const keys = keyList(
    OUTCOME_CLASSES.map((cls, k) => ({
      label: cls.label,
      sub: plural(present[k], 'proxy', 'proxies'),
      color: cls.css,
      dot: true,
    })),
  );

  const { node, host } = figure({
    summary: `Scatter plot of ${count(timed.length)} proxies, latency in run A on the horizontal axis against run B on the vertical, with a diagonal reference line. ${count(
      slower,
    )} points sit above the line and are slower in B; ${count(faster)} sit below and are faster. By outcome: ${OUTCOME_CLASSES.map(
      (cls, k) => `${cls.label.toLowerCase()}, ${count(present[k])}`,
    ).join('; ')}. The largest individual movements are listed in the top movers table below.`,
    keys: keys.node,
  });

  root.append(
    panel({
      label: 'A against B',
      note: 'One point per proxy. Above the diagonal is slower in B, below it faster.',
      children: node,
    }),
  );

  return mountChart(host, (t, width) => ({
    data: [xs, xs, ...groups],
    opts: {
      ...baseOpts(),
      // Square: a diagonal that is not at 45° cannot be read as equality.
      height: Math.round(Math.min(440, Math.max(220, width))),
      scales: { x: { time: false, range: [0, bound] }, y: { range: [0, bound] } },
      axes: [
        axis(t, { size: 34, space: 60, values: (_, v) => v.map((n) => `${count(n)}ms`), label: 'Run A' }),
        axis(t, { side: 3, size: 54, values: (_, v) => v.map((n) => `${count(n)}ms`), label: 'Run B' }),
      ],
      series: [
        {},
        // --ink-muted, not a rule token: the diagonal is not page furniture,
        // it is the chart's reference, and a graphical object that carries
        // meaning has to clear 3:1 against the field. The rules do not.
        { stroke: t.muted, width: 1, dash: [4, 4], points: { show: false } },
        ...OUTCOME_CLASSES.map((cls) => ({
          stroke: cls.tint(t),
          paths: () => null,
          points: { show: true, size: 5, stroke: cls.tint(t), fill: t.wash(cls.tint(t)) },
        })),
      ],
      hooks: cursorReadout(keys, (idx) =>
        OUTCOME_CLASSES.map((_, k) => (groups[k][idx] === null ? '' : ms(groups[k][idx]))),
      ),
    },
  }));
}

/* ── How much it moved ──────────────────────────────────────────────────── */

/**
 * deltaPanel bins the per-proxy change in latency.
 *
 * The scatter shows where the mass sits; this shows how far it moved and in
 * which direction. Faster and slower carry the same two semantic tokens they
 * carry everywhere else on the page, and the sign is written out in the axis
 * so the direction never rests on colour alone.
 *
 * Accessible fallback: an adjacent data table, behind a disclosure. Twenty-one
 * rows of "range, count" is exactly what this chart is, and no other element
 * on the page carries the delta distribution.
 */
export function deltaPanel(root, timed) {
  if (timed.length === 0) return null;

  const deltas = timed.map((pair) => pair.b.latencyMs - pair.a.latencyMs);
  const bound = Math.max(1, ...deltas.map(Math.abs));
  const width = (2 * bound) / DELTA_BUCKETS;
  const middle = (DELTA_BUCKETS - 1) / 2;

  const counts = new Array(DELTA_BUCKETS).fill(0);
  for (const delta of deltas) {
    const index = Math.min(DELTA_BUCKETS - 1, Math.max(0, Math.floor((delta + bound) / width)));
    counts[index] += 1;
  }
  const centres = counts.map((_, k) => Math.round(-bound + (k + 0.5) * width));

  // Three series over one bucket axis, so a bar's colour is its direction.
  const bucketsOf = (test) => counts.map((n, k) => (test(k) ? n : null));
  const faster = bucketsOf((k) => k < middle);
  const same = bucketsOf((k) => k === middle);
  const slower = bucketsOf((k) => k > middle);
  const totals = [faster, same, slower].map((row) => row.reduce((sum, n) => sum + (n || 0), 0));

  const keys = keyList([
    { label: 'Faster in B', sub: plural(totals[0], 'proxy', 'proxies'), color: 'var(--good)' },
    { label: `Within ±${count(Math.round(width / 2))}ms`, sub: plural(totals[1], 'proxy', 'proxies'), color: 'var(--ink-muted)' },
    { label: 'Slower in B', sub: plural(totals[2], 'proxy', 'proxies'), color: 'var(--bad)' },
  ]);

  const { node, host } = figure({
    summary: `Histogram of per-proxy latency change from run A to run B across ${count(
      timed.length,
    )} proxies, in ${DELTA_BUCKETS} buckets spanning minus ${count(Math.round(bound))} to plus ${count(
      Math.round(bound),
    )} milliseconds. ${count(totals[0])} proxies got faster, ${count(totals[2])} got slower, and ${count(
      totals[1],
    )} changed by less than the bucket width either way. The same figures are in the table below.`,
    keys: keys.node,
    children: dataFallback(
      'Bucket counts',
      plainTable({
        caption: `Latency change buckets, ${DELTA_BUCKETS} rows.`,
        columns: [
          {
            key: 'range',
            label: 'Change',
            cell: (row) =>
              el('span', {
                class: 'num',
                text: `${signed(Math.round(-bound + row.k * width))} to ${signed(
                  Math.round(-bound + (row.k + 1) * width),
                )} ms`,
              }),
          },
          {
            key: 'direction',
            label: 'Direction',
            cell: (row) =>
              document.createTextNode(
                row.k < middle ? 'faster in B' : row.k > middle ? 'slower in B' : 'unchanged',
              ),
          },
          { key: 'count', label: 'Proxies', num: true, cell: (row) => document.createTextNode(count(counts[row.k])) },
        ],
        rows: counts.map((_, k) => ({ k })),
      }),
    ),
  });

  root.append(
    panel({
      label: 'Change in latency',
      note: 'Run B minus run A, per proxy. The centre bucket straddles zero and holds the proxies that did not really move.',
      children: node,
    }),
  );

  return mountChart(host, (t, plotWidth) => ({
    data: [centres, faster, same, slower],
    opts: {
      ...baseOpts(),
      height: Math.round(Math.min(320, Math.max(190, plotWidth * 0.34))),
      scales: { x: { time: false } },
      axes: [
        axis(t, { size: 34, space: 60, values: (_, v) => v.map((n) => `${signed(n)}ms`) }),
        axis(t, { side: 3, size: 46, values: (_, v) => v.map((n) => count(n)) }),
      ],
      series: [
        {},
        bar(t.good, t),
        bar(t.muted, t),
        bar(t.bad, t),
      ],
      hooks: cursorReadout(keys, (idx) =>
        [faster, same, slower].map((row) => (row[idx] === null ? '' : plural(row[idx], 'proxy', 'proxies'))),
      ),
    },
  }));
}

/** bar is one histogram series in the house style. */
function bar(stroke, t) {
  return {
    stroke,
    fill: t.wash(stroke),
    width: 1,
    paths: window.uPlot.paths.bars({ size: [0.92, Infinity] }),
    points: { show: false },
  };
}
