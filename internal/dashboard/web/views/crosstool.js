// Cross-tool — the same proxies, seen through different targets.
//
// Every other view on this page compares like with like. This one does not,
// and that is the whole reason it exists: a proxy file is bought once and then
// used against several targets, and the only question that matters is which
// proxies survive all of them.
//
// The rule that governs every panel below: a cross-tool comparison joins **per
// proxy**, never as a shared series. A ping is a raw CONNECT through the
// proxy; a Ticketmaster request is a full TLS-fingerprinted fetch against a
// host that is trying to refuse it. Those are different quantities. Counting
// proxies across them is meaningful — a proxy either worked or it did not, in
// every tool's terms. Averaging their latencies is not, and nothing here does
// it: the matrix tints each column against its own p90, the funnel counts
// proxies and never milliseconds, and the correlation chart draws two
// separately labelled axes with no line between them.
//
// It does not repeat the comparability banner. That banner asks whether two
// runs measured the same thing, and the answer here is known in advance: they
// did not. Saying so as a warning would be noise.
//
// It matches two or more runs that are *not* all one known tool, which is
// wider than "more than one tool" and deliberately so. A run that predates run
// metadata carries no tool at all; beside a pinger run it makes a selection
// that is neither same-tool nor multi-tool, and before this match was widened
// nothing rendered for it. In substance it is the cross-tool case: the matrix
// joins per proxy and does not care what produced a column. So an unknown tool
// is shown as "unknown" — the word the inventory already uses for exactly this
// — rather than excluded.
//
// This view and the timeline remain mutually exclusive by construction —
// timeline requires sameTool, this requires its negation — so although both
// fetch /api/compare, no selection ever fetches it twice through them.

import { fetchCompare } from '../api.js';
import { UNKNOWN, count, el, plural } from '../format.js';
import { register } from './registry.js';
import { correlationPanel } from './correlation.js';
import { funnelPanel } from './funnel.js';
import { matrixPanel } from './matrix.js';
import { setOpsPanel } from './setops.js';

register({
  id: 'crosstool',
  title: 'Cross-tool',
  match: (selection) => selection.runs.length >= 2 && !selection.sameTool,
  mount,
});

async function mount(root, selection) {
  const { runs, join } = await fetchCompare(selection.files);

  root.append(coverage(runs, join));

  if (join.rows.length === 0) {
    root.append(
      el('div', { class: 'notice' }, [
        el('h3', { text: 'Nothing to join' }),
        el('p', { text: 'None of the selected runs recorded a single proxy result.' }),
      ]),
    );
    return null;
  }

  const teardowns = [matrixPanel(root, runs, join, exitIpColumn(join))];
  funnelPanel(root, runs, join);
  setOpsPanel(root, runs, join);
  teardowns.push(correlationPanel(root, runs, join));

  return () => {
    for (const teardown of teardowns) if (teardown) teardown();
  };
}

/**
 * exitIpColumn finds the run that can supply an exit address, or -1.
 *
 * Chosen by what the data actually contains rather than by the tool name: an
 * iptester run whose every row failed has no exit IP to contribute, and
 * offering a column of dashes because of a string in the metadata would be
 * worse than offering none.
 */
function exitIpColumn(join) {
  const width = join.runs.length;
  for (let i = 0; i < width; i += 1) {
    if (join.rows.some((row) => row.cells[i] && row.cells[i].exitIp)) return i;
  }
  return -1;
}

/**
 * coverage states what the whole view rests on, before any of it.
 *
 * `join.overlap` is the count of proxies present in *every* selected run, and
 * it is usually far below the union. Every conclusion a reader draws from the
 * funnel or the correlation chart is a conclusion about that intersection, and
 * a reader who has not been told its size will read it as a statement about
 * their whole proxy file.
 */
function coverage(runs, join) {
  const union = join.rows.length;
  const share = union ? Math.round((join.overlap / union) * 100) : 0;

  return el('div', { class: 'coverage' }, [
    el('p', { class: 'coverage__lead' }, [
      el('strong', { class: 'num', text: `${count(join.overlap)} of ${count(union)}` }),
      document.createTextNode(
        ` proxies appear in all ${count(runs.length)} runs — ${share}% of the union. The other ${count(
          union - join.overlap,
        )} were tested by some of these runs and not others, so a row in the matrix below may be blank in a column without that proxy having failed anything.`,
      ),
    ]),
    el(
      'ol',
      { class: 'coverage__runs' },
      runs.map((run, i) =>
        el('li', { class: 'coverage__run' }, [
          el('p', { class: 'coverage__tool', text: run.tool || UNKNOWN }),
          el('p', { class: 'coverage__file' }, [el('code', { text: run.file })]),
          el('p', {
            class: 'coverage__meta num',
            text: `${plural(present(join, i), 'proxy', 'proxies')} · ${count(run.summary.ok)} ok`,
          }),
        ]),
      ),
    ),
  ]);
}

/** present counts the proxies one run actually tested. */
function present(join, index) {
  return join.rows.filter((row) => row.cells[index] && row.cells[index].present).length;
}
