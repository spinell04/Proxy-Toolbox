// Outcome composition: one normalised bar per provider.
//
// Normalised to 100% because providers are disjoint populations with their own
// sample sizes. A bar drawn to the raw count would make the longest proxy file
// the widest bar, which is a fact about how many lines were bought and not
// about the provider — the same trap the share beside each count exists to
// close. The raw counts stay on the label, where they say how much evidence
// each band rests on.
//
// Drawn in CSS rather than uPlot. It is three lengths summing to one, with no
// axis, no cursor and no resampling; a canvas would cost a theme listener and
// a resize observer to draw a flex row, and would announce nothing.

import { count, el, pct } from '../format.js';
import { panel } from './chart.js';
import { OUTCOMES } from './crosstool-util.js';
import { dataFallback, plainTable } from './datatable.js';
import { providerName } from './provider.js';

/**
 * BANDS are the three outcomes a run's summary reports, in reading order.
 *
 * The tints are taken from OUTCOMES rather than restated, so this panel, the
 * cross-tool matrix and the funnel cannot drift into two palettes for one
 * vocabulary. `absent` is dropped: a summary counts what a run measured, and
 * nothing in a single run is absent from itself.
 */
const BANDS = ['ok', 'blocked', 'error'].map((key) => ({
  ...OUTCOMES.find((outcome) => outcome.key === key),
  // The summary spells the error count `errors`; the outcome vocabulary spells
  // the state `error`. One is a tally, the other is a state, and they are
  // reconciled here rather than by renaming either.
  read: (summary) => Number((key === 'error' ? summary.errors : summary[key]) || 0),
}));

/**
 * compositionPanel renders one stacked bar per provider.
 *
 * @param root the view body
 * @param runs the selected runs, already in provider order
 */
export function compositionPanel(root, runs) {
  const providers = runs.map((run) => {
    const summary = run.summary || {};
    const total = Number(summary.total || 0);
    return {
      name: providerName(run),
      total,
      bands: BANDS.map((band) => {
        const value = band.read(summary);
        return { band, value, share: total > 0 ? value / total : null };
      }),
    };
  });

  root.append(
    panel({
      label: 'Outcome composition',
      note: 'Each bar is one provider’s results normalised to 100%. The counts beside it are the raw figures the shares were taken from.',
      children: [
        el('ul', { class: 'comp' }, providers.map(bar)),
        legend(),
        dataFallback('Composition as a table', fallbackTable(providers)),
      ],
    }),
  );
}

/**
 * bar is one provider's row: name, the normalised bar, the raw counts.
 *
 * The bar carries aria-hidden="true" and no text of its own. Everything it
 * encodes is written out beside it and tabulated in the disclosure below, so
 * announcing the three <span>s would read three empty elements and then repeat
 * the line that already said it.
 */
function bar(provider) {
  if (provider.total <= 0) {
    return el('li', { class: 'comp__row' }, [
      el('p', { class: 'comp__name', text: provider.name }),
      el('p', { class: 'comp__counts comp__counts--empty', text: 'No results recorded.' }),
    ]);
  }

  return el('li', { class: 'comp__row' }, [
    el('p', { class: 'comp__name' }, [
      document.createTextNode(provider.name),
      el('span', { class: 'comp__total num', text: `${count(provider.total)} proxies` }),
    ]),
    el(
      'div',
      { class: 'comp__bar', 'aria-hidden': true },
      provider.bands
        .filter((entry) => entry.value > 0)
        .map((entry) =>
          el('span', {
            class: `comp__seg comp__seg--${entry.band.key}`,
            style: `--share:${(entry.share * 100).toFixed(3)}%;--tint:${entry.band.tint}`,
          }),
        ),
    ),
    el(
      'p',
      { class: 'comp__counts' },
      provider.bands.map((entry) =>
        el('span', { class: 'comp__count' }, [
          el('span', { class: 'num', text: count(entry.value) }),
          document.createTextNode(` ${entry.band.label.toLowerCase()}`),
          el('span', { class: 'comp__count-share num', text: pct(entry.share) }),
        ]),
      ),
    ),
  ]);
}

/** legend names the three bands once, in the bars' own order. */
function legend() {
  return el(
    'ul',
    { class: 'comp__key' },
    BANDS.map((band) =>
      el('li', { class: 'comp__key-item' }, [
        el('span', {
          class: 'comp__key-swatch',
          'aria-hidden': true,
          style: `--tint:${band.tint}`,
        }),
        el('span', { text: band.label }),
      ]),
    ),
  );
}

/** fallbackTable is the same figures as rows, aligned for comparison down a column. */
function fallbackTable(providers) {
  const caption =
    'Outcome composition by provider: the count and share of results that were OK, blocked and errored.';
  const columns = [
    { key: 'name', label: 'Provider', cell: (row) => document.createTextNode(row.name) },
    {
      key: 'total',
      label: 'Total',
      num: true,
      cell: (row) => document.createTextNode(count(row.total)),
    },
    ...BANDS.map((band, i) => ({
      key: band.key,
      label: band.label,
      num: true,
      cell: (row) =>
        document.createTextNode(
          row.bands[i].share === null
            ? '—'
            : `${count(row.bands[i].value)} (${pct(row.bands[i].share)})`,
        ),
    })),
  ];
  return plainTable({ columns, rows: providers, caption });
}
