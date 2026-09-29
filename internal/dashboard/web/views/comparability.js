// The comparability check that sits above the timeline.
//
// This is the reason the exporters were taught to record their own metadata.
// A trend line across four pinger runs looks the same whether the proxies got
// worse or the operator pointed the third run at a different target, doubled
// the worker count, or swapped the proxy file. The chart cannot tell those
// apart and neither can the reader, so the difference is named here, in the
// clear, before the chart is reached.
//
// Nothing is inferred from a run that has no metadata: its target and proxy
// file are empty and its worker count is zero because they were never
// recorded, not because they were different. Folding those in would invent a
// difference. Such runs are reported separately, as excluded.

import { el } from '../format.js';

/** FIELDS are the three facts that decide whether two runs measured the same thing. */
const FIELDS = [
  { key: 'target', label: 'Target', read: (run) => run.target },
  { key: 'proxyFile', label: 'Proxy file', read: (run) => run.proxyFile },
  { key: 'workers', label: 'Workers', read: (run) => (run.workers > 0 ? String(run.workers) : '') },
];

/**
 * compare groups the metadata-carrying runs by each field's value.
 *
 * A field is only reported when two or more *recorded* values disagree. An
 * unrecorded value — iptester writes no target at all, and an older exporter
 * wrote no worker count — is not a third opinion, so it is set aside and
 * counted rather than compared.
 *
 * @returns {Array<{key, label, groups: Array<{value, files}>, unrecorded: string[]}>}
 */
export function compare(runs) {
  const differing = [];
  for (const field of FIELDS) {
    const groups = new Map();
    const unrecorded = [];
    for (const run of runs) {
      const value = field.read(run);
      if (!value) {
        unrecorded.push(run.file);
        continue;
      }
      if (!groups.has(value)) groups.set(value, []);
      groups.get(value).push(run.file);
    }
    if (groups.size < 2) continue;
    differing.push({
      key: field.key,
      label: field.label,
      groups: [...groups].map(([value, files]) => ({ value, files })),
      unrecorded,
    });
  }
  return differing;
}

/**
 * banner renders the verdict.
 *
 * It is not a live region. #views deliberately stopped being one, and wrapping
 * a panel in another would have the whole view read out on every tick of a
 * checkbox. The banner is instead the first thing inside the view's own
 * section, ahead of every chart, and its text stands alone.
 *
 * @param dated    runs placed on the time axis, chronological
 * @param undated  runs excluded from it
 * @param withMeta runs whose metadata can be compared at all
 */
export function banner(dated, undated, withMeta) {
  const differing = compare(withMeta);
  const children = [];

  if (differing.length > 0) {
    children.push(
      el('p', { class: 'banner__label', text: 'Not directly comparable' }),
      el('p', {
        class: 'banner__lead',
        text: `${differing.length === 1 ? 'One' : String(differing.length)} of target, proxy file and worker count changed across these runs. A movement in the charts below may be that change rather than a change in the proxies.`,
      }),
      el(
        'dl',
        { class: 'banner__fields' },
        differing.flatMap((field) => [
          el('dt', { text: field.label }),
          el('dd', {}, [
            el(
              'ul',
              { class: 'banner__values' },
              field.groups.map((group) =>
                el('li', {}, [
                  el('code', { class: 'banner__value', text: group.value }),
                  el('span', {
                    class: 'banner__where',
                    text: `${group.files.length} ${group.files.length === 1 ? 'run' : 'runs'}: ${group.files.join(', ')}`,
                  }),
                ]),
              ),
            ),
            field.unrecorded.length > 0
              ? el('p', {
                  class: 'banner__where',
                  text: `Not recorded in ${field.unrecorded.length} ${
                    field.unrecorded.length === 1 ? 'run' : 'runs'
                  }: ${field.unrecorded.join(', ')}. Left out of this comparison rather than treated as a third value.`,
                })
              : null,
          ]),
        ]),
      ),
    );
  } else if (withMeta.length > 1) {
    children.push(
      el('p', { class: 'banner__label', text: 'Comparable' }),
      el('p', {
        class: 'banner__lead',
        text: `Target, proxy file and worker count match across all ${withMeta.length} runs carrying metadata, so a movement in the charts below is a movement in the proxies.`,
      }),
    );
  }

  if (undated.length > 0) {
    children.push(
      el('p', { class: 'banner__excluded' }, [
        el('strong', {
          text: `${undated.length} ${undated.length === 1 ? 'run has' : 'runs have'} no recorded run time`,
        }),
        document.createTextNode(
          ` — ${undated
            .map((run) => run.file)
            .join(', ')}. ${
            undated.length === 1 ? 'It is' : 'They are'
          } left off the two time-axis charts entirely rather than given an invented position, and ${
            undated.length === 1 ? 'its' : 'their'
          } target, proxy file and worker count are not compared above. ${
            undated.length === 1 ? 'It still appears' : 'They still appear'
          } in the latency distribution, which has no time axis.`,
        ),
      ]),
    );
  }

  if (children.length === 0) return null;
  return el(
    'div',
    { class: differing.length > 0 ? 'banner banner--alert' : 'banner' },
    children,
  );
}

/** placedOnTimeAxis splits a selection into the runs that carry a real timestamp and those that do not. */
export function placedOnTimeAxis(runs, hasTime) {
  const dated = runs.filter((run) => hasTime(run)).sort((a, b) => Date.parse(a.runAt) - Date.parse(b.runAt));
  const undated = runs.filter((run) => !hasTime(run));
  return { dated, undated };
}
