// The latency table: the whole distribution, per provider, side by side.
//
// This panel exists because the outcome rates cannot separate the reference
// providers at all — three of the four sit at exactly 100% working. What
// separates them is entirely distributional, and the separation is not
// monotonic: on the four real exports `private` has the higher median of the
// three providers at 100% (754ms against premium's 720ms) and the lower
// spread. One row is therefore never the table; the column is.

import { el, ms } from '../format.js';
import { panel } from './chart.js';
import { providerName } from './provider.js';

/**
 * ROWS are the nine figures, in the order they are read.
 *
 * The lowest figure is marked in every one of them, spread included: a smaller
 * IQR or standard deviation is a narrower distribution at the same centre.
 */
const ROWS = [
  { key: 'min', label: 'Min' },
  { key: 'p25', label: 'p25' },
  { key: 'p50', label: 'p50 (median)' },
  { key: 'p75', label: 'p75' },
  { key: 'p90', label: 'p90' },
  { key: 'max', label: 'Max' },
  { key: 'mean', label: 'Mean' },
  { key: 'stdDev', label: 'Std dev' },
  { key: 'iqr', label: 'IQR' },
];

/**
 * latencyTable renders the percentile matrix.
 *
 * @param root the view body
 * @param runs the selected runs, already in provider order
 */
export function latencyTable(root, runs) {
  const providers = runs.map((run) => ({
    name: providerName(run),
    summary: run.summary || {},
    // latencyCount, not ok: a successful row reporting 0ms counts in ok but
    // contributes no sample to the distribution, so a run can have successes
    // and still have no percentiles. Such a provider reads as not recorded
    // throughout its column and never as a confident zero.
    recorded: Number((run.summary || {}).latencyCount || 0) > 0,
  }));

  const silent = providers.filter((provider) => !provider.recorded);

  root.append(
    panel({
      label: 'Latency',
      note: `Milliseconds. Lower is faster in the percentile rows and narrower in the spread rows. ${
        silent.length > 0
          ? `${silent.map((p) => p.name).join(', ')} recorded no latency sample, so ${
              silent.length === 1 ? 'that column reads' : 'those columns read'
            } as a dash rather than as zero.`
          : 'The lowest figure in each row is marked.'
      }`,
      children: [table(providers)],
    }),
  );
}

/** read returns a provider's figure, or null when it has no distribution. */
const read = (provider, key) =>
  provider.recorded && Number.isFinite(Number(provider.summary[key]))
    ? Number(provider.summary[key])
    : null;

/* ── The table ──────────────────────────────────────────────────────────── */

function table(providers) {
  const caption =
    'Latency distribution by provider: one row per percentile or spread figure, one column per provider, in milliseconds. The lowest figure in each row is marked.';

  const rows = ROWS.map((row) => {
    const values = providers.map((provider) => read(provider, row.key));
    const lowest = markedIndices(values, 'low');
    return el(
      'tr',
      {},
      [el('th', { scope: 'row', class: 'h2h__metric', text: row.label })].concat(
        values.map((value, i) =>
          el(
            'td',
            { class: lowest.has(i) ? 'col-num h2h__cell h2h__cell--mark' : 'col-num h2h__cell' },
            [
              el('span', { class: 'h2h__figure num', text: ms(value) }),
              lowest.has(i) ? el('span', { class: 'visually-hidden', text: ' — lowest' }) : null,
            ],
          ),
        ),
      ),
    );
  });

  return el(
    'div',
    {
      class: 'table-scroll table-scroll--compact',
      tabindex: '0',
      role: 'region',
      'aria-label': `${caption} Scrollable horizontally.`,
    },
    [
      el('table', { class: 'h2h__table' }, [
        el('caption', { class: 'visually-hidden', text: caption }),
        el('thead', {}, [
          el(
            'tr',
            {},
            [el('th', { scope: 'col' }, [el('span', { class: 'th-static', text: 'Figure' })])].concat(
              providers.map((provider) =>
                el('th', { scope: 'col', class: 'col-num' }, [
                  el('span', { class: 'th-static', text: provider.name }),
                ]),
              ),
            ),
          ),
        ]),
        el('tbody', {}, rows),
      ]),
    ],
  );
}

/**
 * markedIndices returns every index holding the row's extreme value, or an
 * empty set when the row has no direction and when every provider ties.
 *
 * Ties are all marked: singling one of several identical figures out would
 * invent a ranking the data does not contain.
 */
function markedIndices(values, direction) {
  if (!direction) return new Set();
  const present = values.filter((value) => value !== null && Number.isFinite(value));
  if (present.length < 2) return new Set();
  const target = direction === 'high' ? Math.max(...present) : Math.min(...present);
  const marked = new Set();
  values.forEach((value, i) => {
    if (value !== null && Number.isFinite(value) && value === target) marked.add(i);
  });
  // A row where every provider ties marks none of them: marking an extreme
  // that every column shares would spend the accent on a row separating
  // nothing.
  return marked.size === present.length ? new Set() : marked;
}
