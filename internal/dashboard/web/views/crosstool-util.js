// Vocabulary shared by the cross-tool panels.
//
// The four panels below this file all describe the same three things — a run,
// an outcome, and one cell of the join matrix — and they have to describe them
// identically, or the funnel's "OK" and the matrix's "OK" quietly stop being
// the same population.

import { UNKNOWN, el, ms, runTime } from '../format.js';

/**
 * OUTCOMES is the closed vocabulary of the `outcome` field, plus the fourth
 * state a *cell* can be in that an outcome never is: absent.
 *
 * Every entry carries the word the cell prints. Colour reinforces the outcome
 * on this page but never carries it alone — the matrix cell says "blocked" or
 * "timeout", not just a red wash — which is also what keeps the tint free of
 * the 3:1 non-text contrast obligation.
 */
export const OUTCOMES = [
  { key: 'ok', label: 'OK', tint: 'var(--good)' },
  { key: 'blocked', label: 'Blocked', tint: 'var(--warn)' },
  { key: 'error', label: 'Error', tint: 'var(--bad)' },
  { key: 'absent', label: 'Absent', tint: 'var(--ink-faint)' },
];

/** outcomeOf reads a cell's state, collapsing "not in this run" into `absent`. */
export function outcomeOf(cell) {
  if (!cell || !cell.present) return 'absent';
  return cell.outcome || 'error';
}

/**
 * cellText is what one matrix cell prints.
 *
 * An OK cell prints its latency, which is the measurement. Everything else
 * prints why there is no measurement — the error kind where the parser found
 * one, the status where the target refused — because "—" in three different
 * colours is not a table anyone can read.
 */
export function cellText(cell) {
  switch (outcomeOf(cell)) {
    case 'ok':
      // A genuine 0ms success is real but carries no sample; saying "ok" is
      // honest where printing "0ms" would claim a measurement of zero.
      return cell.latencyMs > 0 ? ms(cell.latencyMs) : 'ok';
    case 'blocked':
      return cell.status > 0 ? `blocked ${cell.status}` : 'blocked';
    case 'error':
      return cell.errorKind || 'error';
    default:
      return '—';
  }
}

/**
 * cellRank is the sort key for a run column.
 *
 * Latency orders the measurements; the two non-measurements sort past every
 * one of them, worst last, so an ascending sort reads fastest-to-broken.
 * Absent is null, which sortRows always places last in either direction —
 * a proxy that was not in the run is not a bad proxy.
 */
export function cellRank(cell) {
  switch (outcomeOf(cell)) {
    case 'ok':
      return cell.latencyMs;
    case 'blocked':
      return Number.MAX_SAFE_INTEGER - 1;
    case 'error':
      return Number.MAX_SAFE_INTEGER;
    default:
      return null;
  }
}

/**
 * runLabel names a run in prose: its tool, then when it ran.
 *
 * Both halves are always present, because both can be missing and a label that
 * silently collapses to one of them stops being unique. A run with no metadata
 * has no tool and no time, so it reads "unknown · legacy.csv" — which names
 * the same absence the inventory names, and is still distinct from every other
 * run, because the file name is.
 */
export function runLabel(run) {
  const tool = run.tool || UNKNOWN;
  return `${tool} · ${columnSub(run)}`;
}

/** columnLabel is the tool alone, compressed to fit a column header. */
export function columnLabel(run) {
  return run.tool || UNKNOWN;
}

/**
 * columnSub is the run's identity beneath its tool: when it ran, or, when that
 * was never recorded, the file it came from. The file is always there and
 * always unique, which is what makes it the right fallback.
 */
export function columnSub(run) {
  return runTime(run.runAt) || run.file;
}

/** okIds returns the proxies a run's column marks OK, as a Set. */
export function okIds(rows, index) {
  const set = new Set();
  for (const row of rows) {
    if (outcomeOf(row.cells[index]) === 'ok') set.add(row.proxyId);
  }
  return set;
}

/**
 * runChooser builds a labelled <select> over the runs.
 *
 * A real <select> rather than a listbox widget: it is keyboard operable,
 * announces its own state, and works on a touch screen, none of which a
 * hand-built one gets for free.
 */
export function runChooser({ id, label, runs, value, onChange }) {
  const select = el(
    'select',
    { id, class: 'control__input' },
    runs.map((run, i) =>
      el('option', { value: String(i), selected: i === value ? true : null, text: runLabel(run) }),
    ),
  );
  select.addEventListener('change', () => onChange(Number(select.value)));
  return el('p', { class: 'control' }, [
    el('label', { class: 'control__label', for: id, text: label }),
    select,
  ]);
}

/** chooser is the same control over an arbitrary list of {value, label}. */
export function chooser({ id, label, options, value, onChange }) {
  const select = el(
    'select',
    { id, class: 'control__input' },
    options.map((option) =>
      el('option', {
        value: option.value,
        selected: option.value === value ? true : null,
        text: option.label,
      }),
    ),
  );
  select.addEventListener('change', () => onChange(select.value));
  return el('p', { class: 'control' }, [
    el('label', { class: 'control__label', for: id, text: label }),
    select,
  ]);
}
