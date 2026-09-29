// The paired view's two tables: which proxies moved, and which changed outcome.
//
// The charts above show the shape of the change; these name the proxies behind
// it, which is what a reader acts on. Both are computed on the same
// intersection the coverage line states.

import { count, el, ms, plural, signed } from '../format.js';
import { panel } from './chart.js';
import { sortableTable } from './datatable.js';

/** MOVERS is how many of the largest absolute changes the movers table lists. */
const MOVERS = 25;

/** PROXY_LIST_CAP bounds the proxy list under one flip; the DOM is not a log file. */
const PROXY_LIST_CAP = 200;

/**
 * moversPanel lists the largest absolute changes, sortable.
 *
 * Truncated on purpose and said so in the caption: the point is the extremes,
 * and a three-hundred-row table of mostly ±10ms buries them. Sorting reorders
 * the shortlist, not the population.
 */
export function moversPanel(root, aLabel, bLabel, timed) {
  if (timed.length === 0) return;

  const rows = timed
    .map((pair) => ({
      id: pair.id,
      a: pair.a.latencyMs,
      b: pair.b.latencyMs,
      delta: pair.b.latencyMs - pair.a.latencyMs,
      share: (pair.b.latencyMs - pair.a.latencyMs) / pair.a.latencyMs,
    }))
    .sort((p, q) => Math.abs(q.delta) - Math.abs(p.delta))
    .slice(0, MOVERS);

  root.append(
    panel({
      label: 'Top movers',
      note: `The ${rows.length} largest changes of ${count(timed.length)} proxies timed in both runs, largest absolute change first.`,
      children: sortableTable({
        caption: `Top movers: the ${rows.length} proxies with the largest change in latency between ${aLabel} and ${bLabel}, of ${count(timed.length)} timed in both runs.`,
        initial: { key: 'delta', dir: 'descending' },
        rows,
        columns: [
          { key: 'proxy', label: 'Proxy', value: (r) => r.id, cell: (r) => el('code', { text: r.id }) },
          { key: 'a', label: 'A', num: true, value: (r) => r.a, cell: (r) => document.createTextNode(ms(r.a)) },
          { key: 'b', label: 'B', num: true, value: (r) => r.b, cell: (r) => document.createTextNode(ms(r.b)) },
          {
            key: 'delta',
            label: 'Change',
            num: true,
            value: (r) => r.delta,
            cell: (r) => el('span', { class: deltaClass(r.delta), text: `${signed(r.delta)}ms` }),
          },
          {
            key: 'share',
            label: 'Change %',
            num: true,
            value: (r) => r.share,
            cell: (r) =>
              el('span', { class: deltaClass(r.delta), text: `${signed(r.share * 100)}%` }),
          },
        ],
      }),
    }),
  );
}

/** deltaClass tints a change; the sign in the text is what carries the meaning. */
function deltaClass(delta) {
  return `num delta ${delta > 0 ? 'delta--slower' : delta < 0 ? 'delta--faster' : ''}`.trim();
}

/* ── Which proxies changed outcome ──────────────────────────────────────── */

/**
 * flipsPanel counts the outcome transitions and names the proxies behind each.
 *
 * Unchanged pairs are summarised in one line rather than listed: nine rows of
 * which eight say "nothing happened" is not a table, it is noise around the
 * two rows that matter.
 */
export function flipsPanel(root, pairs) {
  const groups = new Map();
  let held = 0;
  for (const pair of pairs) {
    if (pair.a.outcome === pair.b.outcome) {
      held += 1;
      continue;
    }
    const key = `${pair.a.outcome}→${pair.b.outcome}`;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(pair.id);
  }

  const flips = [...groups].sort((x, y) => y[1].length - x[1].length);
  if (flips.length === 0) {
    root.append(
      panel({
        label: 'Status flips',
        children: el('p', {
          class: 'panel__empty',
          text: `Not one of the ${count(held)} paired proxies changed outcome between the two runs.`,
        }),
      }),
    );
    return;
  }

  root.append(
    panel({
      label: 'Status flips',
      note: `${count(pairs.length - held)} of ${count(pairs.length)} paired proxies changed outcome; the other ${count(held)} did not.`,
      children: el(
        'ol',
        { class: 'flips' },
        flips.map(([key, ids]) =>
          el('li', { class: 'flips__item' }, [
            el('p', { class: 'flips__head' }, [
              el('span', { class: 'flips__transition', text: key }),
              el('span', { class: 'flips__count num', text: plural(ids.length, 'proxy', 'proxies') }),
            ]),
            el('details', { class: 'data-fallback' }, [
              el('summary', { text: 'Show the proxies' }),
              el('p', { class: 'flips__proxies', text: ids.slice(0, PROXY_LIST_CAP).join('\n') }),
              ids.length > PROXY_LIST_CAP
                ? el('p', {
                    class: 'panel__note',
                    text: `${count(ids.length - PROXY_LIST_CAP)} more not listed.`,
                  })
                : null,
            ]),
          ]),
        ),
      ),
    }),
  );
}
