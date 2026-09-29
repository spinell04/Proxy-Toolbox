// The proxy matrix: one row per proxy, one column per run.
//
// This is the table the whole project exists for. Everywhere else on this page
// a run is a distribution; here a run is a column, and the thing being read is
// a single proxy's row across four different targets. Nothing is averaged
// across those columns and nothing is plotted across them — a ping is a raw
// CONNECT and a Ticketmaster fetch is a TLS-fingerprinted request at a hostile
// host, and a number that mixed them would be a number about nothing.
//
// The tint follows the same rule. Each column's latency wash is normalised
// against that column's *own* p90, so a cell is dark because it was slow for
// that tool, never because that tool is slower than another one.

import { count, el, ms, pct } from '../format.js';
import { panel } from './chart.js';
import { announce } from './datatable.js';
import { virtualTable } from './vtable.js';
import {
  OUTCOMES,
  cellRank,
  cellText,
  chooser,
  columnLabel,
  columnSub,
  outcomeOf,
} from './crosstool-util.js';

/** GROUPINGS are the three shapes the matrix can take; the last two need exit IPs. */
const GROUPINGS = [
  { value: 'none', label: 'One row per proxy' },
  { value: 'ip', label: 'Grouped by exit IP' },
  { value: 'subnet', label: 'Grouped by exit /24' },
];

/**
 * matrixPanel builds the matrix, its outcome filter and its grouping control.
 *
 * `exitIndex` is the column supplying exit IPs, or -1 when no iptester run is
 * in the selection. Returns the teardown the view contract requires.
 */
export function matrixPanel(root, runs, join, exitIndex) {
  const state = {
    outcomes: new Set(OUTCOMES.map((o) => o.key)),
    grouping: 'none',
    table: null,
  };

  const host = el('div', { class: 'matrix__host' });
  const controls = el('div', { class: 'controls' }, [
    outcomeFilter(state, () => refresh()),
    exitIndex >= 0
      ? chooser({
          id: 'matrix-grouping',
          label: 'Rows',
          options: GROUPINGS,
          value: state.grouping,
          onChange: (value) => {
            state.grouping = value;
            build();
            announce(GROUPINGS.find((g) => g.value === value).label + '.');
          },
        })
      : null,
  ]);

  const status = el('p', { class: 'matrix__status num' });

  root.append(
    panel({
      label: 'Proxy matrix',
      note:
        exitIndex >= 0
          ? 'One proxy per row, one run per column. Each column is tinted against its own p90: a dark cell was slow for that tool, not slow compared to another one. The exit IP comes from the iptester run in the selection.'
          : 'One proxy per row, one run per column. Each column is tinted against its own p90: a dark cell was slow for that tool, not slow compared to another one.',
      children: [controls, status, host],
    }),
  );

  /** build swaps the table for the current grouping, destroying the old one. */
  function build() {
    if (state.table) state.table.destroy();
    state.table =
      state.grouping === 'none'
        ? proxyTable(runs, join, exitIndex)
        : cohortTable(runs, join, exitIndex, state.grouping);
    host.replaceChildren(state.table.node);
    refresh();
  }

  /** refresh re-applies the outcome filter without rebuilding the table shell. */
  function refresh() {
    const rows = state.table.filter(state.outcomes);
    state.table.setRows(rows);
    status.textContent = `${count(rows.length)} of ${count(state.table.total)} rows shown.`;
  }

  build();
  return () => {
    if (state.table) state.table.destroy();
  };
}

/**
 * outcomeFilter is a checkbox group, not a select.
 *
 * The filter is genuinely a set — a reader wants "errors and blocked, not OK"
 * — and a multi-select is the least usable control in HTML. Four checkboxes in
 * a named fieldset announce themselves correctly and work on touch.
 */
function outcomeFilter(state, onChange) {
  return el('fieldset', { class: 'chips' }, [
    el('legend', { class: 'control__label', text: 'Show rows with' }),
    ...OUTCOMES.map((outcome) => {
      const box = el('input', {
        type: 'checkbox',
        id: `matrix-outcome-${outcome.key}`,
        checked: true,
      });
      box.addEventListener('change', () => {
        if (box.checked) state.outcomes.add(outcome.key);
        else state.outcomes.delete(outcome.key);
        onChange();
        announce(`${outcome.label} ${box.checked ? 'shown' : 'hidden'}.`);
      });
      return el('span', { class: 'chips__item' }, [
        box,
        el('label', { for: box.id }, [
          el('span', { class: 'chips__swatch', 'aria-hidden': true, style: `--tint:${outcome.tint}` }),
          document.createTextNode(outcome.label),
        ]),
      ]);
    }),
    el('p', {
      class: 'chips__note',
      text: 'A proxy is listed when at least one of its runs has a checked outcome.',
    }),
  ]);
}

/* ── One row per proxy ──────────────────────────────────────────────────── */

/**
 * ambiguous names the tools that appear more than once in the selection.
 *
 * A column header is the tool name, which is the fact a reader scans for. Two
 * pinger runs in a cross-tool selection would then carry identical headers, so
 * those columns — and only those — take a second line naming the run. The
 * matrix is the widest table on the page and every header line costs it row
 * height, so the disambiguation is paid exactly where it is needed.
 */
function ambiguous(runs) {
  const seen = new Map();
  for (const run of runs) {
    const tool = columnLabel(run);
    seen.set(tool, (seen.get(tool) || 0) + 1);
  }
  return new Set([...seen].filter(([, n]) => n > 1).map(([tool]) => tool));
}

/** proxyTable is the matrix proper. */
function proxyTable(runs, join, exitIndex) {
  const scales = runs.map((run) => (run.summary.latencyCount > 0 ? run.summary.p90 || 1 : 0));
  const repeated = ambiguous(runs);

  const columns = [
    {
      key: 'proxy',
      label: 'Proxy',
      value: (row) => row.proxyId,
      cell: (row) => el('code', { text: row.proxyId }),
    },
    ...join.runs.map((file, i) => ({
      key: `run-${i}`,
      label: columnLabel(runs[i]),
      sub: repeated.has(columnLabel(runs[i])) ? columnSub(runs[i]) : null,
      title: `${columnSub(runs[i])} — ${file}`,
      num: true,
      value: (row) => cellRank(row.cells[i]),
      cell: (row) => document.createTextNode(cellText(row.cells[i])),
      attrs: (row) => tint(row.cells[i], scales[i]),
    })),
  ];

  if (exitIndex >= 0) {
    columns.push({
      key: 'exit',
      label: 'Exit IP',
      value: (row) => row.cells[exitIndex].exitIp || null,
      cell: (row) =>
        row.cells[exitIndex].exitIp
          ? el('code', { text: row.cells[exitIndex].exitIp })
          : el('span', { class: 'cell--absent', text: '—' }),
    });
  }

  const table = virtualTable({
    columns,
    rows: [],
    initial: { key: 'proxy', dir: 'ascending' },
    caption: `Proxy matrix: ${count(join.rows.length)} proxies across ${count(runs.length)} runs, one column per run.`,
  });

  table.total = join.rows.length;
  table.filter = (outcomes) =>
    join.rows.filter((row) => row.cells.some((cell) => outcomes.has(outcomeOf(cell))));
  return table;
}

/** tint washes a cell by outcome, at an intensity set by that column's own p90. */
function tint(cell, scale) {
  const outcome = outcomeOf(cell);
  if (outcome === 'absent') return { class: 'cell--absent' };
  const meta = OUTCOMES.find((o) => o.key === outcome);
  const share = outcome === 'ok' && scale > 0 ? Math.min(1, cell.latencyMs / scale) : 1;
  return { class: `cell cell--${outcome}`, style: `--tint:${meta.tint};--v:${share.toFixed(3)}` };
}

/* ── Grouped by exit IP ─────────────────────────────────────────────────── */

/**
 * cohortTable groups proxies by the exit address the iptester run saw.
 *
 * This is the enrichment that makes a proxy list legible: fifty entries that
 * all leave through one address are one machine sold fifty times, and the
 * matrix cannot show that because it is keyed on the proxy the user bought.
 *
 * Each run keeps its own column and its own median. Nothing is pooled across
 * runs here either — the same rule as everywhere else on this page.
 */
function cohortTable(runs, join, exitIndex, grouping) {
  const repeated = ambiguous(runs);
  const groups = new Map();
  for (const row of join.rows) {
    const key = groupKey(row.cells[exitIndex].exitIp, grouping);
    if (!groups.has(key)) groups.set(key, { key, rows: [] });
    groups.get(key).rows.push(row);
  }

  const cohorts = [...groups.values()].map((group) => ({
    ...group,
    stats: runs.map((_, i) => cohortStats(group.rows, i)),
  }));

  const columns = [
    {
      key: 'group',
      label: grouping === 'ip' ? 'Exit IP' : 'Exit /24',
      value: (g) => g.key,
      cell: (g) => el('code', { text: g.key }),
    },
    {
      key: 'size',
      label: 'Proxies',
      num: true,
      value: (g) => g.rows.length,
      cell: (g) => document.createTextNode(count(g.rows.length)),
    },
    ...runs.map((run, i) => ({
      key: `run-${i}`,
      label: columnLabel(run),
      sub: repeated.has(columnLabel(run)) ? columnSub(run) : null,
      title: `${columnSub(run)} — OK share and median latency within the cohort`,
      num: true,
      value: (g) => (g.stats[i].present > 0 ? g.stats[i].ok / g.stats[i].present : null),
      cell: (g) => cohortCell(g.stats[i]),
    })),
  ];

  const table = virtualTable({
    columns,
    rows: [],
    initial: { key: 'size', dir: 'descending' },
    caption: `Exit address cohorts: ${count(cohorts.length)} ${
      grouping === 'ip' ? 'exit addresses' : 'exit /24 blocks'
    } across ${count(runs.length)} runs, largest cohort first.`,
  });

  table.total = cohorts.length;
  table.filter = (outcomes) =>
    cohorts.filter((group) =>
      group.rows.some((row) => row.cells.some((cell) => outcomes.has(outcomeOf(cell)))),
    );
  return table;
}

/** groupKey is the exit address, or its /24. An unknown address is its own bucket. */
function groupKey(exitIp, grouping) {
  if (!exitIp) return 'no exit IP recorded';
  if (grouping === 'ip') return exitIp;
  const parts = exitIp.split('.');
  return parts.length === 4 ? `${parts[0]}.${parts[1]}.${parts[2]}.0/24` : exitIp;
}

/** cohortStats counts one cohort's outcomes in one run and takes its median. */
function cohortStats(rows, index) {
  let present = 0;
  let ok = 0;
  const latencies = [];
  for (const row of rows) {
    const cell = row.cells[index];
    if (!cell.present) continue;
    present += 1;
    if (cell.outcome === 'ok') {
      ok += 1;
      if (cell.latencyMs > 0) latencies.push(cell.latencyMs);
    }
  }
  latencies.sort((a, b) => a - b);
  return {
    present,
    ok,
    p50: latencies.length ? latencies[Math.floor((latencies.length - 1) / 2)] : null,
  };
}

/** cohortCell prints the share and the median, both in words, never colour alone. */
function cohortCell(stats) {
  if (stats.present === 0) return el('span', { class: 'cell--absent', text: '—' });
  return el('span', { class: 'cohort-cell' }, [
    el('span', { class: 'num', text: `${count(stats.ok)}/${count(stats.present)}` }),
    el('span', {
      class: 'cohort-cell__sub num',
      text: `${pct(stats.ok / stats.present)} · ${ms(stats.p50)}`,
    }),
  ]);
}
