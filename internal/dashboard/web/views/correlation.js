// Does a cheap test predict an expensive one?
//
// This panel exists to answer one question honestly, and the honest answer is
// often "no". A ping is a raw CONNECT through the proxy; a Ticketmaster fetch
// is a full TLS-fingerprinted request at a host that is actively trying to
// refuse it. There is no reason in principle why the first should predict the
// second, and when the coefficient comes back at 0.04 that is a finding about
// the proxies worth acting on — stop screening on ping latency — not a broken
// chart to bury.
//
// So the chart deliberately has no y = x line and no fitted trend: both would
// invite the reader to treat two different quantities as one. There are points
// and two independently labelled axes, and the coefficient is written out in
// words underneath.

import { count, el, ms, plural } from '../format.js';
import { axis, baseOpts, cursorReadout, figure, keyList, mountChart, panel } from './chart.js';
import { columnLabel, columnSub, runChooser, runLabel } from './crosstool-util.js';

/**
 * STRENGTHS reads |r| back as the sentence it supports.
 *
 * The bands are the conventional ones and the wording is deliberately flat: a
 * coefficient is a description of a cloud of points, not a verdict, and the
 * panel's job is to stop a reader inferring more from 0.31 than it carries.
 */
const STRENGTHS = [
  { under: 0.2, word: 'essentially no relationship' },
  { under: 0.4, word: 'a weak relationship' },
  { under: 0.6, word: 'a moderate relationship' },
  { under: 0.8, word: 'a strong relationship' },
  { under: Infinity, word: 'a very strong relationship' },
];

/**
 * correlationPanel plots one run's latency against another's, per proxy.
 *
 * Returns the teardown. The chart is rebuilt when either chooser changes, so
 * the teardown has to reach whichever plot is current rather than the first.
 */
export function correlationPanel(root, runs, join) {
  const state = { a: 0, b: runs.length > 1 ? 1 : 0, teardown: null };
  const host = el('div', { class: 'correlation__host' });

  root.append(
    panel({
      label: 'Correlation',
      note: 'One point per proxy, timed in both runs. There is no diagonal here and no fitted line: the two axes are different measurements of different things, and a line across them would claim a relationship the chart is supposed to be testing for.',
      children: [
        el('div', { class: 'controls' }, [
          runChooser({
            id: 'corr-a',
            label: 'Horizontal',
            runs,
            value: state.a,
            onChange: (i) => {
              state.a = i;
              build();
            },
          }),
          runChooser({
            id: 'corr-b',
            label: 'Vertical',
            runs,
            value: state.b,
            onChange: (i) => {
              state.b = i;
              build();
            },
          }),
        ]),
        host,
      ],
    }),
  );

  function build() {
    if (state.teardown) state.teardown();
    state.teardown = draw(host, runs[state.a], runs[state.b], pairsOf(join, state.a, state.b));
  }

  build();
  return () => {
    if (state.teardown) state.teardown();
  };
}

/**
 * pairsOf collects the proxies with a real latency in both columns.
 *
 * A latency of 0 is what a failed row reports, not a measurement of zero
 * milliseconds, so a proxy that errored in either run has no coordinate. It is
 * excluded and counted, rather than plotted at the origin where it would drag
 * the coefficient towards a relationship that is an artefact of the encoding.
 */
function pairsOf(join, ia, ib) {
  const out = [];
  for (const row of join.rows) {
    const a = row.cells[ia];
    const b = row.cells[ib];
    if (!a.present || !b.present) continue;
    if (a.outcome !== 'ok' || b.outcome !== 'ok') continue;
    if (a.latencyMs <= 0 || b.latencyMs <= 0) continue;
    out.push({ id: row.proxyId, x: a.latencyMs, y: b.latencyMs });
  }
  return out;
}

/** pearson is the sample correlation. Zero variance returns 0, never NaN. */
export function pearson(xs, ys) {
  if (xs.length !== ys.length || xs.length === 0) return 0;
  const mean = (values) => values.reduce((sum, v) => sum + v, 0) / values.length;
  const mx = mean(xs);
  const my = mean(ys);
  let cov = 0;
  let vx = 0;
  let vy = 0;
  for (let i = 0; i < xs.length; i += 1) {
    const dx = xs[i] - mx;
    const dy = ys[i] - my;
    cov += dx * dy;
    vx += dx * dx;
    vy += dy * dy;
  }
  if (vx === 0 || vy === 0) return 0;
  return cov / Math.sqrt(vx * vy);
}

/** strength reads |r| back as prose. */
function strength(r) {
  return STRENGTHS.find((band) => Math.abs(r) < band.under).word;
}

/** draw builds the scatter and the verdict, and returns the chart's teardown. */
function draw(host, runA, runB, pairs) {
  if (runA.file === runB.file) {
    host.replaceChildren(
      el('p', {
        class: 'panel__empty',
        text: 'Both axes are the same run, which correlates with itself perfectly and says nothing. Choose two different runs.',
      }),
    );
    return null;
  }
  if (pairs.length < 2) {
    host.replaceChildren(
      el('p', {
        class: 'panel__empty',
        text: `Only ${plural(pairs.length, 'proxy', 'proxies')} succeeded with a measured latency in both runs, which is not enough for a coefficient. A proxy that failed in either run has no latency to plot.`,
      }),
    );
    return null;
  }

  const sorted = [...pairs].sort((p, q) => p.x - q.x);
  const xs = sorted.map((p) => p.x);
  const ys = sorted.map((p) => p.y);
  const r = pearson(xs, ys);
  const nearZero = Math.abs(r) < 0.2;

  const keys = keyList([
    {
      label: 'Proxies',
      sub: plural(pairs.length, 'proxy', 'proxies'),
      color: 'var(--accent)',
      dot: true,
    },
  ]);

  const { node, host: plotHost } = figure({
    summary: `Scatter plot of ${count(pairs.length)} proxies, latency in ${runLabel(
      runA,
    )} on the horizontal axis against latency in ${runLabel(
      runB,
    )} on the vertical. Pearson's correlation coefficient is ${r.toFixed(
      2,
    )}, ${strength(r)}. The two axes measure different things and are not comparable in magnitude.`,
    keys: keys.node,
  });

  host.replaceChildren(verdict(r, nearZero, pairs.length, runA, runB), node);

  return mountChart(plotHost, (t, width) => ({
    data: [xs, ys],
    opts: {
      ...baseOpts(),
      height: Math.round(Math.min(420, Math.max(220, width * 0.62))),
      scales: { x: { time: false }, y: {} },
      axes: [
        axis(t, {
          size: 46,
          space: 60,
          label: `${columnSub(runA)} — ${columnLabel(runA)} latency`,
          values: (_, v) => v.map((n) => `${count(n)}ms`),
        }),
        axis(t, {
          side: 3,
          size: 58,
          label: `${columnSub(runB)} — ${columnLabel(runB)} latency`,
          values: (_, v) => v.map((n) => `${count(n)}ms`),
        }),
      ],
      series: [
        {},
        {
          stroke: t.accent,
          paths: () => null,
          points: { show: true, size: 5, stroke: t.accent, fill: t.wash(t.accent) },
        },
      ],
      hooks: cursorReadout(keys, (idx) => [`${ms(xs[idx])} → ${ms(ys[idx])}`]),
    },
  }));
}

/**
 * verdict states the coefficient and what it covers.
 *
 * A near-zero result gets the loud treatment rather than the quiet one: it is
 * a real result about the two measurements, and a number printed small next to
 * a shapeless cloud reads as a chart that failed.
 */
function verdict(r, nearZero, n, runA, runB) {
  return el('div', { class: nearZero ? 'verdict verdict--flat' : 'verdict' }, [
    el('p', { class: 'verdict__label', text: 'Pearson r' }),
    el('p', { class: 'verdict__value num', text: r.toFixed(2) }),
    el('p', { class: 'verdict__lead', text: `${cap(strength(r))} across ${count(n)} proxies.` }),
    el('p', {
      class: 'verdict__body',
      text: nearZero
        ? `Latency in ${runLabel(runA)} carries almost no information about latency in ${runLabel(
            runB,
          )}. That is a result, not a missing one: these are different measurements — a raw CONNECT against a full request at a hostile target — and a proxy's position in one does not place it in the other.`
        : `Proxies that are ${r > 0 ? 'slow' : 'fast'} in ${runLabel(runA)} tend to be slow in ${runLabel(
            runB,
          )}. The coefficient describes the trend in this cloud of points and nothing more — the two axes are still different measurements, and neither scale transfers to the other.`,
    }),
  ]);
}

/** cap upper-cases the first letter of a sentence fragment. */
const cap = (text) => text.charAt(0).toUpperCase() + text.slice(1);
