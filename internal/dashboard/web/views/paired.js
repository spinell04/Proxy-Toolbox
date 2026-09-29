// Paired — exactly two runs of one tool, compared proxy by proxy.
//
// Everything here is built from join.rows where *both* cells are present. A
// proxy that only appears in one of the two runs has no delta, no point on the
// scatter and no flip; including it with a zero would invent a measurement. So
// the coverage line comes first and stays first: every figure below it is
// computed on the intersection, and the intersection is often much smaller
// than either run.
//
// The timeline view also matches a two-run same-tool selection and renders
// above this one, which is why the comparability banner is not repeated here.

import { fetchCompare } from '../api.js';
import { count, el, runTime } from '../format.js';
import { register } from './registry.js';
import { deltaPanel, scatterPanel } from './paired-charts.js';
import { flipsPanel, moversPanel } from './paired-tables.js';

register({
  id: 'paired',
  title: 'Paired',
  // Known-different proxy files mean disjoint populations, and the join this
  // view is built on has nothing to work with. The guard is on known-different
  // rather than on sameProxyFile deliberately: pre-metadata exports record no
  // proxy file, so proxyFiles is empty and they keep today's behaviour instead
  // of losing the view to a question nobody can answer for them.
  match: (selection) =>
    selection.sameTool && selection.runs.length === 2 && selection.proxyFiles.length <= 1,
  mount,
});

async function mount(root, selection) {
  const { runs, join } = await fetchCompare(selection.files);
  const [a, b] = chronological(runs);
  const ia = join.runs.indexOf(a.file);
  const ib = join.runs.indexOf(b.file);

  const pairs = join.rows
    .filter((row) => row.cells[ia].present && row.cells[ib].present)
    .map((row) => ({ id: row.proxyId, a: row.cells[ia], b: row.cells[ib] }));

  root.append(coverage(a, b, pairs.length, join.rows.length));

  if (pairs.length === 0) {
    root.append(
      el('div', { class: 'notice' }, [
        el('h3', { text: 'These two runs share no proxies' }),
        el('p', {
          text: 'Not one proxy appears in both runs, so there is nothing to pair. They were almost certainly run against different proxy files.',
        }),
      ]),
    );
    return null;
  }

  // A latency of 0 is what an errored row reports, not a measurement of zero
  // milliseconds. Only proxies timed in *both* runs can be plotted or differenced.
  const timed = pairs.filter((pair) => pair.a.latencyMs > 0 && pair.b.latencyMs > 0);

  const teardowns = [scatterPanel(root, timed), deltaPanel(root, timed)];
  moversPanel(root, label(a), label(b), timed);
  flipsPanel(root, pairs);

  return () => {
    for (const teardown of teardowns) if (teardown) teardown();
  };
}

/** chronological puts the earlier run first when both carry a run time. */
function chronological(runs) {
  const [x, y] = runs;
  if (!runTime(x.runAt) || !runTime(y.runAt)) return runs;
  return Date.parse(x.runAt) <= Date.parse(y.runAt) ? [x, y] : [y, x];
}

/** label names a run the way the charts do: its time if it has one, else its file. */
const label = (run) => runTime(run.runAt) || run.file;

/**
 * when is the same fact for the coverage strip, which prints the file name on
 * its own line underneath. Falling back to the file there would print it twice
 * and still not say what is missing.
 */
const when = (run) => runTime(run.runAt) || 'No recorded run time';

/**
 * coverage states what the whole view is computed on, before anything else.
 *
 * It also fixes which run is A and which is B, because every delta below is
 * signed and a reader who has the direction backwards reads every conclusion
 * backwards with it.
 */
function coverage(a, b, both, union) {
  return el('div', { class: 'coverage' }, [
    el('p', { class: 'coverage__lead' }, [
      el('strong', { class: 'num', text: `${count(both)} of ${count(union)}` }),
      document.createTextNode(
        ` proxies appear in both runs. Every figure below is computed on those ${count(
          both,
        )} alone — the ${count(union - both)} that appear in only one run have no pair and are left out entirely.`,
      ),
    ]),
    el('dl', { class: 'coverage__pair' }, [
      el('div', { class: 'coverage__run' }, [
        el('dt', { text: 'A — before' }),
        el('dd', { class: 'num', text: when(a) }),
        el('dd', {}, [el('code', { text: a.file })]),
      ]),
      el('div', { class: 'coverage__run' }, [
        el('dt', { text: 'B — after' }),
        el('dd', { class: 'num', text: when(b) }),
        el('dd', {}, [el('code', { text: b.file })]),
      ]),
    ]),
  ]);
}

