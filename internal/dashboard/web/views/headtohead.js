// Head-to-head — several providers, measured separately, ranked against each
// other.
//
// Timeline and Paired both assume one population measured twice: Paired joins
// on proxy ID and computes every figure on the intersection, Timeline draws a
// trend through a time axis. Two providers are disjoint populations, not one
// population sampled twice, so on that selection Paired had nothing to join
// and Timeline drew a trend line between two unrelated things — plausible, and
// therefore misleading. This view sets the providers side by side instead.
//
// The order of the panels is the order the figures narrow. The composition
// shows how each provider's results split between ok, blocked and errored;
// the latency table and the distribution overlay carry the separation when
// the outcome rates do not, which on the reference data is three providers
// out of four; the codes say why the fourth failed.

import { fetchCompare } from '../api.js';
import { el } from '../format.js';
import { register } from './registry.js';
import { compare } from './comparability.js';
import { codesPanel } from './codes.js';
import { compositionPanel } from './composition.js';
import { ecdfPanel } from './ecdf.js';
import { latencyTable } from './latencytable.js';
import { providerName } from './provider.js';

/**
 * SERIES_STROKES is the categorical palette, in the order it is handed out.
 *
 * Ordered so consecutive providers are far apart on the hue wheel rather than
 * in token order — alphabetical providers land on consecutive entries, and two
 * adjacent hues would put the comparison back where it started. tokens.css
 * says why these six hues and not the semantic ones.
 */
const SERIES_STROKES = [
  'var(--series-1)',
  'var(--series-2)',
  'var(--series-3)',
  'var(--series-4)',
  'var(--series-5)',
  'var(--series-6)',
];

/**
 * SERIES_DASHES is the other half of the identity, in uPlot's on/off pixels.
 *
 * Every provider gets a hue *and* a pattern. Hue alone collapses under
 * deuteranopia and prints grey on grey; a pattern alone is hard to follow
 * across a hundred-point curve where four lines cross. Together each curve is
 * identifiable twice over, which is the rule the latency table's
 * extreme-in-row markers already follow.
 *
 * Four patterns against six hues, so the two cycles do not come back into
 * phase until the twelfth provider: the seventh reuses the first's hue but not
 * its pattern, and is still its own curve.
 */
const SERIES_DASHES = [undefined, [5, 3], [2, 3], [6, 3, 2, 3]];

/** providerStyle gives provider i its hue and its pattern. */
const providerStyle = (run, i) => ({
  stroke: SERIES_STROKES[i % SERIES_STROKES.length],
  width: 1.75,
  dash: SERIES_DASHES[i % SERIES_DASHES.length],
});

register({
  id: 'headtohead',
  title: 'Head-to-head',
  // Known-different proxy files, which is exactly the case Timeline and Paired
  // step aside for. Two or more runs, not exactly two: four providers in one
  // table is the shape this view was designed around.
  match: (selection) =>
    selection.sameTool && selection.runs.length >= 2 && selection.proxyFiles.length > 1,
  mount,
});

async function mount(root, selection) {
  const payload = await fetchCompare(selection.files);
  const runs = byProvider(payload.runs || []);
  const withMeta = runs.filter((run) => run.hasMeta);

  const framed = framing(withMeta);
  if (framed) root.append(framed);

  compositionPanel(root, runs);
  latencyTable(root, runs);

  const teardowns = [
    // Every panel above is a column per provider in a fixed order; the
    // distribution overlay is the same providers as curves, so it is handed
    // the same naming function rather than falling back to run times, which
    // are minutes apart here and say nothing about which provider is which.
    //
    // And its own styling. The panel's default ramp runs accent to faint ink
    // in series order, which in the timeline is chronology and here is the
    // alphabet: it encoded nothing and, at four providers, gave two of them
    // near-identical grey and two near-identical accent. These are categories,
    // so they get a categorical scale.
    ecdfPanel(root, runs, [], () => true, providerName, providerStyle),
    codesPanel(root, runs, providerName),
  ];

  return () => {
    for (const teardown of teardowns) if (teardown) teardown();
  };
}

/**
 * byProvider fixes the column order for every panel below.
 *
 * By name rather than by rank. A table that reorders itself according to which
 * metric the reader is looking at cannot be read down a column, and ranking by
 * one metric would smuggle in the single composite score this view refuses to
 * compute. The file name breaks ties so two runs of one provider stay stable.
 */
function byProvider(runs) {
  return [...runs].sort(
    (a, b) => providerName(a).localeCompare(providerName(b)) || a.file.localeCompare(b.file),
  );
}

/**
 * framing checks the three fields that can invalidate this comparison, and is
 * silent when none does.
 *
 * Not the generic four-field comparability check used by Timeline. That one
 * also reports a differing proxy file, which here is the premise of the view
 * rather than a fault in it: on this selection it fired every time, and a
 * standing alarm is one nobody reads on the day the target changed.
 *
 * The target, the worker count and the IP mode are the fields that do
 * invalidate this comparison, and they matter more here than in Timeline, not
 * less: there, a changed target moves a trend the reader can still see moving;
 * here it silently becomes the whole difference between two providers. A v4 run
 * set beside a v6 run is that error in its purest form — two address spaces,
 * one column each, and nothing on screen saying so.
 */
function framing(withMeta) {
  if (withMeta.length < 2) {
    return el('p', { class: 'h2h__framing' }, [
      el('span', { class: 'h2h__framing-label', text: 'Not checked' }),
      document.createTextNode(
        'Fewer than two of these runs recorded their own metadata, so whether they used the same target, worker count and IP mode cannot be established. Every figure below assumes they did.',
      ),
    ]);
  }

  const differing = compare(withMeta).filter((field) => field.key !== 'proxyFile');
  // Both match: nothing to say. The differing proxy file is what this view is
  // for, and a line confirming the other two would be a reassurance nobody
  // asked for.
  if (differing.length === 0) return null;

  return el('p', { class: 'h2h__framing h2h__framing--alert' }, [
    el('span', { class: 'h2h__framing-label', text: 'Comparison invalid' }),
    document.createTextNode(
      `The ${list(differing.map((field) => NOUNS[field.key]))} ${
        differing.length === 1 ? 'differs' : 'differ'
      } across these runs — ${list(
        differing.map(
          (field) =>
            `${NOUNS[field.key]} ${field.groups.map((group) => group.value).join(' against ')}`,
        ),
      )}. Every provider below was therefore measured under more than one condition, and a difference between the columns may be that change rather than a difference between the providers.`,
    ),
  ]);
}

/**
 * NOUNS names the three checked fields as they read inside a sentence.
 *
 * Not the banner's column labels: "Workers" is a heading and reads as a plural
 * subject in prose, which puts the verb in the wrong number.
 */
const NOUNS = { target: 'target', workers: 'worker count', ipMode: 'IP mode' };

/** list joins clauses with commas and a final "and". */
const list = (parts) =>
  parts.length < 2 ? parts.join('') : `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`;
