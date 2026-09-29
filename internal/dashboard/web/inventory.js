// The inventory table: every exported CSV as one row, sortable, selectable.
//
// Rendering is a pure function of state. The table tops out at whatever is in
// results/ and a comparison is capped at 50 files, so rebuilding the tbody on
// every sort or tick is cheaper than reconciling it.

import { MAX_COMPARE_FILES } from './api.js';
import {
  count,
  el,
  errorRate,
  hints,
  ms,
  notRecordedCell,
  pct,
  runTime,
  unknownCell,
} from './format.js';

/**
 * COLUMNS drives both the header and the body, so a column can never be
 * defined in one place and forgotten in the other.
 *
 *   key    sort identity, also the dataset key on the <th>
 *   label  the header text
 *   num    right-aligned, monospaced figure
 *   value  sort value; null sorts last regardless of direction
 *   cell   builds the <td> content
 */
const COLUMNS = [
  {
    key: 'file',
    label: 'File',
    className: 'col-file',
    value: (r) => r.file.toLowerCase(),
    cell: (r) => el('span', { text: r.file, title: r.file }),
  },
  {
    key: 'tool',
    label: 'Tool',
    value: (r) => (r.tool ? r.tool.toLowerCase() : null),
    cell: (r) => (r.tool ? el('span', { class: 'tool', text: r.tool }) : unknownCell()),
  },
  {
    key: 'runAt',
    label: 'Run at',
    value: (r) => (runTime(r.runAt) ? Date.parse(r.runAt) : null),
    cell: (r) => {
      const when = runTime(r.runAt);
      return when ? el('span', { class: 'num', text: when }) : unknownCell();
    },
  },
  {
    key: 'proxyFile',
    label: 'Proxy file',
    className: 'col-proxyfile',
    value: (r) => (r.proxyFile ? r.proxyFile.toLowerCase() : null),
    cell: (r) => (r.proxyFile ? el('span', { text: r.proxyFile }) : unknownCell()),
  },
  {
    key: 'target',
    label: 'Target',
    className: 'col-target',
    value: (r) => (r.target ? r.target.toLowerCase() : null),
    // iptester records no target even when it does carry metadata, so an empty
    // target is not on its own evidence that the export is old.
    cell: (r) => {
      if (r.target) return el('span', { text: r.target, title: r.target });
      return r.hasMeta
        ? notRecordedCell()
        : unknownCell();
    },
  },
  {
    key: 'workers',
    label: 'Workers',
    className: 'col-workers col-num',
    num: true,
    value: (r) => (r.workers > 0 ? r.workers : null),
    cell: (r) => (r.workers > 0 ? document.createTextNode(count(r.workers)) : unknownCell()),
  },
  {
    key: 'total',
    label: 'n',
    className: 'col-num',
    num: true,
    value: (r) => r.summary.total,
    cell: (r) => document.createTextNode(count(r.summary.total)),
  },
  {
    key: 'successRate',
    label: 'Success',
    className: 'col-num',
    num: true,
    value: (r) => r.summary.successRate,
    cell: (r) => rateCell(r.summary.successRate, 'var(--good)'),
  },
  {
    key: 'p50',
    label: 'p50',
    className: 'col-num',
    num: true,
    value: (r) => latency(r, 'p50'),
    cell: (r) => document.createTextNode(ms(latency(r, 'p50'))),
  },
  {
    key: 'errorRate',
    label: 'Errors',
    className: 'col-num',
    num: true,
    value: (r) => errorRate(r.summary),
    cell: (r) => rateCell(errorRate(r.summary), 'var(--bad)'),
  },
];

/**
 * latency reads a percentile, or null when the run has no samples.
 *
 * The branch is on latencyCount rather than ok on purpose: a successful result
 * that reported 0ms counts in ok but contributes no sample, so ok > 0 does not
 * mean there is a distribution to show.
 */
function latency(run, key) {
  return run.summary.latencyCount > 0 ? run.summary[key] : null;
}

/** rateCell renders a ratio as a figure over a hairline bar. */
function rateCell(ratio, tint) {
  return el('span', { class: 'rate' }, [
    el('span', {
      class: 'rate__bar',
      'aria-hidden': true,
      style: `--v:${Math.max(0, Math.min(1, ratio || 0))};--tint:${tint}`,
    }),
    el('span', { class: 'rate__text', text: pct(ratio) }),
  ]);
}

/** sortRuns orders a copy of runs by key and direction; nulls always sort last. */
export function sortRuns(runs, key, dir) {
  const column = COLUMNS.find((c) => c.key === key);
  if (!column) return runs.slice();
  const sign = dir === 'ascending' ? 1 : -1;
  return runs.slice().sort((a, b) => {
    const [x, y] = [column.value(a), column.value(b)];
    if (x === null && y === null) return a.file < b.file ? -1 : 1;
    if (x === null) return 1;
    if (y === null) return -1;
    if (x === y) return a.file < b.file ? -1 : 1;
    return x < y ? -sign : sign;
  });
}

/**
 * renderTable builds the inventory into container.
 *
 * state carries { runs, sort: {key, dir}, selected: Set<file> }; onSort and
 * onToggle are called with the column key and the file name respectively.
 *
 * A full rebuild happens on sort only. Ticking a checkbox goes through
 * syncSelection instead, because replacing the tbody under a keyboard user
 * destroys the element they are standing on and drops focus to the body.
 *
 * Returns the sort button for the active column, so the caller can put focus
 * back on it after a rebuild for the same reason.
 */
export function renderTable(container, state, { onSort, onToggle }) {
  const head = el('tr', {}, [
    el('th', { scope: 'col' }, [el('span', { class: 'visually-hidden', text: 'Select' })]),
    ...COLUMNS.map((column) => {
      const sorted = state.sort.key === column.key;
      const th = el('th', {
        scope: 'col',
        class: column.className,
        'aria-sort': sorted ? state.sort.dir : null,
      });
      const button = el('button', {
        type: 'button',
        text: column.label,
        title: `Sort by ${column.label}`,
        dataset: { sort: column.key },
      });
      button.addEventListener('click', () => onSort(column.key));
      th.append(button);
      return th;
    }),
  ]);

  const body = el('tbody');
  for (const run of sortRuns(state.runs, state.sort.key, state.sort.dir)) {
    const row = el('tr', {
      dataset: { file: run.file },
      // hasMeta is the server's own verdict; the tooltip hangs off the whole
      // row so it is reachable from any of the muted cells.
      title: run.hasMeta ? null : 'Exported before runs recorded metadata.',
    });

    const pick = el('td', { class: 'col-pick' });
    const box = el('input', { type: 'checkbox', 'aria-label': `Select ${run.file}` });
    box.addEventListener('change', () => {
      // aria-disabled is advisory, so the refusal has to be enforced here:
      // put the box back the way it was and leave the selection alone.
      if (box.hasAttribute('aria-disabled')) {
        box.checked = false;
        return;
      }
      onToggle(run.file);
    });
    pick.append(box);
    row.append(pick);

    for (const column of COLUMNS) {
      row.append(el('td', { class: column.className }, [column.cell(run)]));
    }
    body.append(row);
  }

  container.replaceChildren(
    el('fieldset', {}, [
      el('legend', { class: 'visually-hidden', text: 'Select runs to compare' }),
      // A scroll container with no focusable descendant of its own cannot be
      // reached or panned from the keyboard, and below 860px the rightmost
      // columns — success, p50, error rate — are off-screen. One tab stop and a
      // role make it operable with the arrow keys in every browser.
      el(
        'div',
        {
          class: 'table-scroll',
          tabindex: '0',
          role: 'region',
          'aria-label': 'Run inventory table, scrollable horizontally',
        },
        [table(head, body)],
      ),
      hints(),
    ]),
    el('p', { class: 'cap-warning', id: CAP_WARNING_ID, hidden: true }),
  );
  syncSelection(container, state);
  return container.querySelector(`th button[data-sort="${state.sort.key}"]`);
}

/**
 * table assembles the element, kept separate only to keep renderTable short.
 *
 * The caption is what names the table in the accessibility tree; without it
 * the table is anonymous in a screen reader's element list, which is the one
 * place a user goes to find it.
 */
function table(head, body) {
  return el('table', {}, [
    el('caption', {
      class: 'visually-hidden',
      text: 'Exported runs. Select rows to compare them.',
    }),
    el('thead', {}, [head]),
    body,
  ]);
}

/** CAP_WARNING_ID lets the capped checkboxes point at the reason they are. */
const CAP_WARNING_ID = 'cap-warning';

/** setCapped marks a checkbox unavailable without removing it from the tab order. */
function setCapped(box, capped) {
  box.toggleAttribute('data-capped', capped);
  if (capped) {
    box.setAttribute('aria-disabled', 'true');
    box.setAttribute('aria-describedby', CAP_WARNING_ID);
    return;
  }
  box.removeAttribute('aria-disabled');
  box.removeAttribute('aria-describedby');
}

/**
 * syncSelection pushes the selection onto an already-rendered table: row state,
 * checkbox state, and the cap that stops a selection from becoming a 400.
 */
export function syncSelection(container, state) {
  const atCap = state.selected.size >= MAX_COMPARE_FILES;

  for (const row of container.querySelectorAll('tbody tr')) {
    const selected = state.selected.has(row.dataset.file);
    const box = row.querySelector('input[type="checkbox"]');
    row.classList.toggle('is-selected', selected);
    box.checked = selected;
    // aria-disabled rather than the disabled property, deliberately. `disabled`
    // drops the control out of the tab order, so at the cap a keyboard user
    // tabs straight past the unselectable rows with nothing telling them those
    // rows exist or why they cannot be picked. This way the box stays
    // reachable, announces itself as unavailable, and carries the reason as a
    // description. The cost is that the tick no longer bounces off natively,
    // so the change handler refuses it instead.
    setCapped(box, !selected && atCap);
  }

  const warning = container.querySelector('.cap-warning');
  warning.hidden = !atCap;
  warning.textContent = atCap
    ? `Selection is capped at ${MAX_COMPARE_FILES} runs — the comparison API refuses more. Clear one to pick another.`
    : '';
}
