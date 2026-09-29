// The funnel: how many proxies survive each run in turn.
//
// "500 loaded, 412 ping OK, 380 Ticketmaster OK, 290 Bayern OK" is the
// sentence a reader came here to write, and it is a genuinely cumulative
// figure: each step counts the proxies that were OK in *every* run up to and
// including that one. That is not the same as each run's own success count,
// and the difference is the whole point — a run can succeed on 80% of what it
// saw while sharing almost nothing with the run before it.
//
// So both numbers are shown, side by side, and the one that is cumulative says
// so. Nothing is averaged across the steps: this counts proxies, which is a
// quantity every tool measures identically, and never latency, which is not.

import { count, el, pct, plural } from '../format.js';
import { panel } from './chart.js';
import { outcomeOf, runLabel } from './crosstool-util.js';

/**
 * funnelPanel renders the survival chain in the order the runs were selected.
 *
 * The order is the user's, taken from join.runs, and it is stated rather than
 * inferred: reordering the selection reorders the funnel, and a funnel read in
 * the wrong order tells a story that never happened.
 */
export function funnelPanel(root, runs, join) {
  const total = join.rows.length;
  const steps = [];
  let survivors = new Set(join.rows.map((row) => row.proxyId));

  for (let i = 0; i < runs.length; i += 1) {
    const ok = new Set();
    let ownOk = 0;
    for (const row of join.rows) {
      if (outcomeOf(row.cells[i]) !== 'ok') continue;
      ownOk += 1;
      if (survivors.has(row.proxyId)) ok.add(row.proxyId);
    }
    steps.push({
      run: runs[i],
      surviving: ok.size,
      lost: survivors.size - ok.size,
      ownOk,
      ownSeen: join.rows.filter((row) => row.cells[i].present).length,
    });
    survivors = ok;
  }

  const last = steps[steps.length - 1];

  root.append(
    panel({
      label: 'Funnel',
      note: 'Cumulative, in the order the runs were selected. Each step keeps only the proxies that were OK in every run above it as well, which is why a step can fall further than its own failure rate.',
      children: [
        el('p', { class: 'funnel__lead' }, [
          el('strong', { class: 'num', text: count(last.surviving) }),
          document.createTextNode(
            ` of ${count(total)} proxies were OK in all ${count(runs.length)} runs — ${pct(
              total ? last.surviving / total : 0,
            )} of the set, end to end.`,
          ),
        ]),
        el('ol', { class: 'funnel' }, [
          el('li', { class: 'funnel__step funnel__step--seed' }, [
            el('p', { class: 'funnel__head' }, [
              el('span', { class: 'funnel__name', text: 'Loaded' }),
              el('span', { class: 'funnel__count num', text: count(total) }),
            ]),
            bar(1),
            el('p', {
              class: 'funnel__note',
              text: 'The union of every proxy seen in any selected run.',
            }),
          ]),
          ...steps.map((step) => stepItem(step, total)),
        ]),
      ],
    }),
  );
}

/** stepItem is one run's rung: its cumulative survivors, and its own rate beside them. */
function stepItem(step, total) {
  const share = total ? step.surviving / total : 0;
  return el('li', { class: 'funnel__step' }, [
    el('p', { class: 'funnel__head' }, [
      el('span', { class: 'funnel__name', text: runLabel(step.run) }),
      el('span', { class: 'funnel__count num', text: count(step.surviving) }),
    ]),
    bar(share),
    el('p', { class: 'funnel__note' }, [
      el('span', {
        class: 'num',
        text: `${pct(share)} of the set kept`,
      }),
      document.createTextNode(
        ` — ${plural(step.lost, 'proxy', 'proxies')} dropped here. On its own this run was OK for ${count(
          step.ownOk,
        )} of the ${count(step.ownSeen)} proxies it tested.`,
      ),
    ]),
  ]);
}

/**
 * bar is the rung's length. A scaled element rather than a width, so it stays
 * on the compositor with the rest of the page's motion.
 */
function bar(share) {
  return el('span', { class: 'funnel__track', 'aria-hidden': true }, [
    el('span', { class: 'funnel__fill', style: `--v:${Math.max(0, Math.min(1, share)).toFixed(4)}` }),
  ]);
}
