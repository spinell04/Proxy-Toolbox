// The latency quantile overlay: one latency-by-percentile curve per run.
//
// This is the chart that separates "everything got a little slower" from "the
// tail fell off a cliff": two runs with the same median and different upper
// percentiles sit on top of each other until their curves diverge toward the
// right.
//
// It is drawn as the inverse of the ECDF the API sends — percentile on x,
// latency on y — because that is the axis the reader arrives with. The
// question is "what is each provider's latency at this percentile", and on
// this orientation it is answered by looking up one x and reading four numbers
// off one scale, instead of tracing four curves across to one y and reading
// four different numbers off the other one.
//
// It is its own module because it is the one chart in the timeline with no
// time axis — which is also why it is the one chart a run with no recorded run
// time can still appear on.

import { count, el, ms, plural, runTime } from '../format.js';
import {
  axis,
  baseOpts,
  chartHeight,
  cursorReadout,
  figure,
  keyList,
  mountChart,
  panel,
  zoomControl,
} from './chart.js';

/**
 * defaultLabel is the timeline's naming: the run time, falling back to the
 * file name for a run that has none.
 *
 * The fallback is the file and not the word "undated" because this name also
 * feeds the hidden summary, where several undated runs would otherwise all be
 * read out as the same word and become impossible to tell apart.
 */
const defaultLabel = (run) => runTime(run.runAt) || run.file;

/** latency reads a percentile, or null when the run recorded no samples at all. */
const latency = (run, key) => (run.summary.latencyCount > 0 ? run.summary[key] : null);

/**
 * rampShare places run i of n on the ink-to-accent ramp.
 *
 * The full 0..100 range, so that with five or six runs adjacent curves are
 * still told apart by more than a hair of chroma. The oldest run is plain
 * faint ink and the newest is the accent, which is the reading order.
 */
const rampShare = (i, n) => Math.round((100 * i) / Math.max(1, n - 1));

/**
 * defaultStyle is the timeline's styling: the recency ramp, in series order.
 *
 * `i` is the run's place on the ramp — its index among the dated runs — and
 * `n` is how many of those there are. A run with no recorded time is off the
 * ramp entirely and arrives as -1: it gets plain faint ink, a narrower stroke
 * and a dash, so it is visibly not a point on the chronology the other curves
 * form. That is three separate signals for one fact, which is the right number
 * for a curve the reader must not mistake for the newest run.
 *
 * Every value is a CSS colour expression rather than a resolved one, because
 * the same object styles the canvas series and its legend swatch and the
 * legend is DOM. theme().color() does the resolving for the canvas.
 */
const defaultStyle = (run, i, n) =>
  i < 0
    ? { stroke: 'var(--ink-faint)', width: 1.25, dash: [3, 3] }
    : {
        stroke: `color-mix(in oklab, var(--accent) ${rampShare(i, n)}%, var(--ink-faint))`,
        width: 1.75,
        dash: undefined,
      };

/**
 * PERCENTILE_GRID is the shared x axis: whole percentiles, 0 to 100 inclusive.
 *
 * Whole points for two reasons. It puts every printed percentile exactly on
 * the grid, so a curve passes through the figures the latency table beside it
 * prints rather than through an interpolation either side of them. And it is the
 * resolution the runs are actually measured at — a hundred proxies carry a
 * hundred quantiles, and a finer grid would resample each sample onto several
 * points, turning every observation into a plateau and putting back the
 * staircase the spline is here to remove.
 */
const PERCENTILE_GRID = Array.from({ length: 101 }, (_, i) => i);

/**
 * ecdfPanel overlays one latency quantile curve per run.
 *
 * `hasTime` is the caller's test for whether a run carries a real timestamp;
 * it decides only the drawing order and the tint here, never inclusion.
 *
 * Runs with no metadata appear here. They cannot be placed on a time axis, but
 * this chart has none — excluding them would discard a real distribution for a
 * reason that does not apply to it, so they are drawn last, in faint ink, and
 * the banner says so.
 *
 * Accessible fallback: a hidden summary carrying each run's median. A
 * hundred-step curve does not tabulate into anything a reader can hold; the
 * median is the shape's readable form, and the inventory table above carries
 * it per run as a sortable column.
 *
 * `labelOf` names a curve. The timeline names one by when it was taken, which
 * is what separates runs of one proxy file; head-to-head names one by its
 * provider, because there the run times are minutes apart and say nothing.
 * The same function feeds the key list and the hidden summary, and it has to:
 * a summary that describes a curve by a name the legend does not use leaves a
 * screen reader reading about a curve nobody else can find.
 *
 * `styleOf` colours one. It is injectable because the default ramp encodes
 * recency, which is a real quantity in the timeline and nothing at all in
 * head-to-head, where the series order is alphabetical: there the ramp put two
 * providers in near-identical grey and two more in near-identical accent, and
 * hid the comparison the reader came for. It is called as
 * `styleOf(run, i, n)` with the same `i` and `n` defaultStyle documents, and
 * returns `{stroke, width, dash}` with stroke as a CSS colour expression.
 *
 * One call per curve, feeding both the series and its swatch. Not two: the
 * stroke and the swatch were written out separately before this, in expressions
 * that had to be kept in step by hand, which is how a legend comes to disagree
 * with the chart it labels.
 */
export function ecdfPanel(
  root,
  dated,
  undated,
  hasTime,
  labelOf = defaultLabel,
  styleOf = defaultStyle,
) {
  const points = new Map([...dated, ...undated].map((run) => [run, plottable(run)]));
  const ordered = [...points.keys()].filter((run) => points.get(run).length > 0);
  const dropped = [...points.keys()].filter((run) => points.get(run).length === 0);

  if (ordered.length === 0) {
    root.append(
      panel({
        label: 'Latency distribution',
        children: el('p', {
          class: 'panel__empty',
          text: 'No selected run recorded a latency sample, so there is no distribution to draw.',
        }),
      }),
    );
    return null;
  }

  const curves = ordered.map((run) => quantiles(points.get(run)));
  const datedCount = ordered.filter(hasTime).length;
  const home = { x: [0, 100], y: latencyDomain(curves) };
  const zoom = zoomControl(home);

  // Undated runs are appended after the dated ones, so a dated run's index in
  // `ordered` is already its index among the dated runs — which is what the
  // ramp is a position on.
  const styles = ordered.map((run, i) => styleOf(run, hasTime(run) ? i : -1, datedCount));

  const keys = keyList(
    ordered.map((run, i) => ({
      label: labelOf(run),
      // Only when it adds something. A run with no recorded time is labelled
      // by its file already, and printing it twice reads as two facts.
      sub: labelOf(run) === run.file ? undefined : run.file,
      color: styles[i].stroke,
      dash: styles[i].dash,
    })),
  );

  const { node, host } = figure({
    summary: `Latency by percentile, one curve per run, ${plural(ordered.length, 'curve', 'curves')}, on a logarithmic latency scale. ${ordered
      .map(
        (run) =>
          `${labelOf(run)}: ${plural(run.summary.latencyCount, 'sample', 'samples')}, median ${ms(
            latency(run, 'p50'),
          )}`,
      )
      .join('. ')}. A curve lower than another is faster at the same percentile.${
      dropped.length > 0
        ? ` ${dropped.length} selected ${dropped.length === 1 ? 'run has' : 'runs have'} no latency samples and ${
            dropped.length === 1 ? 'is' : 'are'
          } not drawn.`
        : ''
    }`,
    keys: keys.node,
    actions: zoom.node,
  });

  root.append(
    panel({
      label: 'Latency distribution',
      note:
        dropped.length > 0
          ? `Lower is faster. ${dropped.length} run${dropped.length === 1 ? '' : 's'} recorded no latency sample and ${dropped.length === 1 ? 'is' : 'are'} not drawn: ${dropped.map((r) => r.file).join(', ')}.`
          : 'Lower is faster. A curve that separates only toward the right has lost its tail, not its median.',
      children: node,
    }),
  );

  return mountChart(host, (t, width) => ({
    data: [PERCENTILE_GRID, ...curves],
    opts: {
      ...baseOpts(),
      ...zoom.opts,
      height: chartHeight(width, 0.4),
      // Both ranges are the identity, so a zoom lands exactly where it was
      // asked to and the opening view is whatever zoomControl sets. Latency is
      // logarithmic because these distributions are heavy-tailed: the medians
      // here sit inside one 300ms band and the slowest tail is thirty times
      // that, so a linear axis spends nine tenths of its height on the tail
      // and renders the part being compared unreadable. Log also makes the
      // vertical gap between two curves a ratio, which is the honest way to
      // say one provider is half as fast as another.
      scales: {
        x: { time: false, range: (_, min, max) => [min, max] },
        y: { distr: 3, range: (_, min, max) => [min, max] },
      },
      axes: [
        axis(t, { size: 34, values: (_, ticks) => ticks.map((v) => `${count(v)}%`), space: 70 }),
        axis(t, {
          side: 3,
          size: 62,
          splits: (_, __, min, max) => latencySplits(min, max),
          // A null tick is uPlot's log axis saying two labels would collide
          // and it is dropping this one. It has to stay null: ms() renders an
          // absent figure as an em dash, which on an axis reads as a tick
          // whose latency is unknown rather than as no tick at all.
          values: (_, ticks) => ticks.map((v) => (v === null ? null : ms(v))),
        }),
      ],
      series: [
        {},
        ...ordered.map((_, i) => ({
          stroke: t.color(styles[i].stroke),
          width: styles[i].width,
          dash: styles[i].dash,
          // A spline, where the ECDF orientation took a step function. What is
          // drawn here is a quantile function of a latency distribution that
          // is continuous — every value between two observed latencies was a
          // latency the network could have produced — so a curve through the
          // samples no longer asserts what the step function was protecting
          // against. The samples are still discrete, and the line between two
          // of them is an interpolation, not a measurement: read the values at
          // the cursor, not the shape between them.
          paths: window.uPlot.paths.spline(),
          points: { show: false },
        })),
      ],
      hooks: cursorReadout(keys, (idx) => curves.map((curve) => ms(curve[idx]))),
    },
  }));
}

/**
 * plottable is a run's ECDF points, minus any at or below zero.
 *
 * The API already builds the ECDF from successful results with a positive
 * latency, so this removes nothing in practice. It is here because the y axis
 * is logarithmic and a single zero would make uPlot drop that run's entire
 * curve without saying so — a silent absence is the one failure this chart
 * must not have, since an absent curve reads as a provider nobody tested.
 */
function plottable(run) {
  return (run.ecdf || []).filter((point) => point.value > 0);
}

/**
 * quantiles inverts one run's ECDF onto the shared percentile grid.
 *
 * Nearest rank, the same rule as util.Percentile: the value at a percentile is
 * the smallest observed value whose cumulative probability reaches it. Every
 * percentile on this page is computed that way, and a curve that disagreed
 * with the latency table printed beside it would be a bug, not a smoothing
 * choice.
 *
 * Both arrays ascend, so one cursor walks the points once.
 */
function quantiles(points) {
  const out = new Array(PERCENTILE_GRID.length);
  let cursor = 0;
  for (let i = 0; i < PERCENTILE_GRID.length; i += 1) {
    const target = PERCENTILE_GRID[i] / 100;
    while (cursor < points.length - 1 && points[cursor].p < target) cursor += 1;
    out[i] = points[cursor].value;
  }
  return out;
}

/** TICK_LADDER is the 1-2-5 decade the latency axis prefers to land on. */
const TICK_LADDER = [1, 2, 5];

/** TICK_LADDER_FINE is what it falls back to once a zoom is inside one decade. */
const TICK_LADDER_FINE = [1, 1.5, 2, 2.5, 3, 4, 5, 6, 7, 8, 9];

/**
 * latencyDomain is the range the chart opens at: the pooled data, rounded out
 * to the ladder so both ends of the axis carry a labelled tick.
 */
function latencyDomain(curves) {
  let lo = Infinity;
  let hi = -Infinity;
  for (const curve of curves) {
    lo = Math.min(lo, curve[0]);
    hi = Math.max(hi, curve[curve.length - 1]);
  }
  return [ladderBelow(lo), ladderAbove(hi)];
}

/**
 * latencySplits is where the latency axis puts its ticks.
 *
 * uPlot's own log splits are whole decades, which over the 500ms-to-11s span
 * these runs cover produces two labels. The ladder is what a reader expects to
 * see on a log axis — 500, 1000, 2000, 5000 — and it degrades twice: to ninths
 * of a decade once a zoom is inside one, and to an even split once it is
 * inside that, where the axis is narrow enough for linear spacing to read
 * correctly anyway.
 */
function latencySplits(min, max) {
  const coarse = ladderBetween(min, max, TICK_LADDER);
  if (coarse.length >= 3) return coarse;
  const fine = ladderBetween(min, max, TICK_LADDER_FINE);
  if (fine.length >= 3) return fine;
  const step = (max - min) / 4;
  return [0, 1, 2, 3, 4].map((i) => min + i * step);
}

/** ladderBetween is every ladder value inside [min, max], ascending. */
function ladderBetween(min, max, mantissas) {
  const out = [];
  for (let decade = Math.floor(Math.log10(min)); decade <= Math.ceil(Math.log10(max)); decade += 1) {
    for (const mantissa of mantissas) {
      const value = mantissa * 10 ** decade;
      if (value >= min && value <= max) out.push(value);
    }
  }
  return out;
}

/** ladderBelow is the largest ladder value at or below v. */
function ladderBelow(v) {
  const decade = Math.floor(Math.log10(v));
  const mantissa = v / 10 ** decade;
  const step = [...TICK_LADDER].reverse().find((m) => m <= mantissa);
  return step * 10 ** decade;
}

/** ladderAbove is the smallest ladder value at or above v. */
function ladderAbove(v) {
  const decade = Math.floor(Math.log10(v));
  const mantissa = v / 10 ** decade;
  const step = TICK_LADDER.find((m) => m >= mantissa);
  return step === undefined ? 10 ** (decade + 1) : step * 10 ** decade;
}
