// Detail — one run, read on its own terms.
//
// The only view backed by /api/run rather than /api/compare, because it is the
// only one that needs the per-proxy rows: /api/compare deliberately withholds
// them, since join.rows already carries the same facts for a selection and two
// copies of one truth is one copy too many.
//
// Reading order is the order a run is actually diagnosed. Headline figures,
// then the shape of the distribution, then the percentiles behind that shape,
// then the individual proxies at the tail, then what the failures were. Each
// answer narrows the last.

import { fetchRun } from '../api.js';
import { count, el, ms, pct, plural, runTime } from '../format.js';
import { panel } from './chart.js';
import { plainTable, sortableTable } from './datatable.js';
import { histogramPanel } from './detail-charts.js';
import { register } from './registry.js';

/** SLOWEST is how many of the slowest successful proxies get a row. */
const SLOWEST = 25;

/** PERCENTILES is the table, in the order a distribution is read. */
const PERCENTILES = [
  { key: 'min', label: 'Minimum' },
  { key: 'p25', label: 'p25' },
  { key: 'p50', label: 'Median (p50)' },
  { key: 'p75', label: 'p75' },
  { key: 'p90', label: 'p90' },
  { key: 'max', label: 'Maximum' },
];

register({
  id: 'detail',
  title: 'Run detail',
  match: (selection) => selection.runs.length === 1,
  mount,
});

async function mount(root, selection) {
  const run = await fetchRun(selection.files[0]);

  root.append(headline(run));

  const teardown = histogramPanel(root, run);
  percentilePanel(root, run);
  slowestPanel(root, run);
  errorsPanel(root, run);

  return teardown;
}

/**
 * headline is the run in one paragraph, with its identity beside it.
 *
 * It repeats figures the inventory row above already shows, on purpose: the
 * inventory is a list of fifty runs read as a list, and this is one run read
 * on its own. A reader who has scrolled past the table should not have to
 * scroll back to learn which file they are looking at.
 */
function headline(run) {
  const timed = run.summary.latencyCount;
  return el('div', { class: 'coverage' }, [
    el('p', { class: 'coverage__lead' }, [
      el('strong', { class: 'num', text: `${count(run.summary.ok)} of ${count(run.summary.total)}` }),
      document.createTextNode(
        ` proxies succeeded — ${pct(run.summary.successRate)}. ${count(
          run.summary.blocked,
        )} were blocked by the target and ${count(run.summary.errors)} never got a usable response. ${
          timed === 0
            ? 'Not one success reported an elapsed time, so this run has no latency distribution at all.'
            : `${count(timed)} of the successes reported an elapsed time and make up every latency figure below.`
        }`,
      ),
    ]),
    el('dl', { class: 'facts' }, [
      fact('Tool', run.tool || 'unknown'),
      fact('Run at', runTime(run.runAt) || 'not recorded'),
      fact('Target', run.target || 'not recorded'),
      fact('Proxy file', run.proxyFile || 'not recorded'),
      fact('Workers', run.workers > 0 ? count(run.workers) : 'not recorded'),
      fact('File', run.file),
    ]),
  ]);
}

/** fact is one label/value pair of the identity strip. */
function fact(label, value) {
  return el('div', { class: 'facts__item' }, [
    el('dt', { text: label }),
    el('dd', { class: 'num', text: value }),
  ]);
}

/* ── Percentiles ────────────────────────────────────────────────────────── */

/**
 * percentilePanel is the distribution as figures rather than as a shape.
 *
 * Branching on latencyCount rather than on ok is the whole guard here: a run
 * whose successes all reported 0ms has a non-zero ok count and no samples, and
 * printing a column of confident zeros for its percentiles would be the most
 * plausible-looking wrong answer on the page.
 */
function percentilePanel(root, run) {
  const s = run.summary;
  if (s.latencyCount === 0) {
    root.append(
      panel({
        label: 'Percentiles',
        children: el('p', {
          class: 'panel__empty',
          text: 'No success in this run reported an elapsed time, so there are no percentiles. A zero here would be a measurement that was never taken.',
        }),
      }),
    );
    return;
  }

  const rows = [
    ...PERCENTILES.map((p) => ({ label: p.label, value: ms(s[p.key]), note: '' })),
    { label: 'Mean', value: ms(s.mean), note: 'Pulled by the tail; the median is the typical proxy.' },
    { label: 'Standard deviation', value: ms(s.stdDev), note: 'Spread around the mean, in the same units.' },
    { label: 'IQR', value: ms(s.iqr), note: 'p75 minus p25 — the width of the middle half.' },
  ];

  root.append(
    panel({
      label: 'Percentiles',
      note: `Computed on the ${count(s.latencyCount)} successes that reported an elapsed time, not on all ${count(
        s.total,
      )} results.`,
      children: plainTable({
        caption: `Latency percentiles over ${count(s.latencyCount)} timed successes.`,
        columns: [
          { key: 'stat', label: 'Statistic', cell: (row) => document.createTextNode(row.label) },
          {
            key: 'value',
            label: 'Latency',
            num: true,
            cell: (row) => el('span', { class: 'stat-value', text: row.value }),
          },
          {
            key: 'note',
            label: 'Reading',
            cell: (row) => el('span', { class: 'stat-note', text: row.note }),
          },
        ],
        rows,
      }),
    }),
  );
}

/* ── The tail ───────────────────────────────────────────────────────────── */

/**
 * slowestPanel names the proxies at the far right of the histogram.
 *
 * Only successes: a failed row reports a latency of zero, and sorting by
 * latency descending would otherwise rank the broken proxies as the fastest.
 */
function slowestPanel(root, run) {
  const timed = (run.results || [])
    .map(normalise)
    .filter((result) => result.outcome === 'ok' && result.latencyMs > 0)
    .sort((a, b) => b.latencyMs - a.latencyMs)
    .slice(0, SLOWEST);

  if (timed.length === 0) {
    root.append(
      panel({
        label: 'Slowest proxies',
        children: el('p', {
          class: 'panel__empty',
          text: 'No successful result reported an elapsed time, so there is no tail to list.',
        }),
      }),
    );
    return;
  }

  root.append(
    panel({
      label: 'Slowest proxies',
      note: `The ${timed.length} slowest successes of ${count(
        run.summary.latencyCount,
      )} timed. Failures are not listed here — they report no time and would sort as the fastest.`,
      children: sortableTable({
        caption: `The ${timed.length} slowest successful proxies in ${run.file}.`,
        initial: { key: 'latency', dir: 'descending' },
        rows: timed,
        columns: [
          {
            key: 'proxy',
            label: 'Proxy',
            value: (r) => r.proxyId,
            cell: (r) => el('code', { text: r.proxyId }),
          },
          {
            key: 'latency',
            label: 'Latency',
            num: true,
            value: (r) => r.latencyMs,
            cell: (r) => document.createTextNode(ms(r.latencyMs)),
          },
          {
            key: 'status',
            label: 'Status',
            num: true,
            value: (r) => r.status,
            cell: (r) => document.createTextNode(r.status > 0 ? String(r.status) : 'ok'),
          },
          {
            key: 'exit',
            label: 'Exit IP',
            value: (r) => r.exitIp || null,
            cell: (r) =>
              r.exitIp ? el('code', { text: r.exitIp }) : el('span', { class: 'cell--absent', text: '—' }),
          },
        ],
      }),
    }),
  );
}

/**
 * normalise reads a per-proxy row from /api/run.
 *
 * ProxyResult carries no JSON tags in Go, so it marshals with exported Go
 * field names — ProxyID, LatencyMs — while every other structure on this API
 * is lower-camel. Rather than depend on which, both spellings are read here,
 * in one place, so the rest of the view sees one shape.
 */
function normalise(result) {
  return {
    proxyId: result.proxyId ?? result.ProxyID ?? '',
    latencyMs: result.latencyMs ?? result.LatencyMs ?? 0,
    status: result.status ?? result.Status ?? 0,
    outcome: result.outcome ?? result.Outcome ?? '',
    errorKind: result.errorKind ?? result.ErrorKind ?? '',
    exitIp: result.exitIp ?? result.ExitIP ?? '',
  };
}

/* ── What failed ────────────────────────────────────────────────────────── */

/**
 * errorsPanel breaks the failures down by taxonomy kind.
 *
 * The share is given against the errors, not against the run: "half of the
 * failures were 407s" is the actionable sentence, and dividing by the total
 * instead turns every kind into a small number that reads as fine.
 */
function errorsPanel(root, run) {
  const kinds = Object.entries(run.errors || {}).sort((a, b) => b[1] - a[1]);
  const total = kinds.reduce((sum, [, n]) => sum + n, 0);

  if (kinds.length === 0) {
    root.append(
      panel({
        label: 'Failures',
        children: el('p', {
          class: 'panel__empty',
          text: `Not one of the ${count(run.summary.total)} results in this run failed with an error.`,
        }),
      }),
    );
    return;
  }

  root.append(
    panel({
      label: 'Failures',
      note: `${plural(total, 'error', 'errors')} across ${count(
        run.summary.total,
      )} results. Blocked is not an error — the proxy worked and the target refused it — and is counted separately above.`,
      children: el(
        'ol',
        { class: 'taxonomy' },
        kinds.map(([kind, n]) =>
          el('li', { class: 'taxonomy__item' }, [
            el('p', { class: 'taxonomy__head' }, [
              el('span', { class: 'taxonomy__kind', text: kind }),
              el('span', { class: 'taxonomy__count num', text: `${count(n)} · ${pct(n / total)}` }),
            ]),
            el('span', { class: 'taxonomy__track', 'aria-hidden': true }, [
              el('span', { class: 'taxonomy__fill', style: `--v:${(n / total).toFixed(4)}` }),
            ]),
          ]),
        ),
      ),
    }),
  );
}
