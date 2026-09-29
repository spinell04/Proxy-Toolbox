// A small sortable table for the comparison views.
//
// Deliberately not a second table component: the markup, the sort buttons and
// the scroll frame are the same shapes inventory.js builds, so they pick up
// the same rules from table.css and look like one system rather than two. What
// differs is only that these tables are read-only — no selection column, no
// cap — so the state lives here instead of in app.js.

import { el } from '../format.js';

/**
 * announce writes one line to the page's single status region.
 *
 * Re-sorting replaces the whole tbody, and the table is not a live region for
 * the same reason the inventory is not: announcing a few hundred cells on
 * every sort is unusable. This one line stands in for it, and aria-sort on the
 * header carries the rest. The text is cleared first so that sorting the same
 * column twice is still announced twice.
 *
 * Exported because the virtualized matrix in vtable.js announces its own sort
 * and filter changes the same way, and two copies of this would drift.
 */
export function announce(message) {
  const status = document.getElementById('sr-status');
  if (!status) return;
  status.textContent = '';
  status.textContent = message;
}

/**
 * sortableTable renders rows into a named, keyboard-operable scroll frame.
 *
 * columns is `[{key, label, num, value(row), cell(row)}]`; `value` supplies the
 * sort key and null sorts last in either direction, matching the inventory.
 * `initial` is `{key, dir}`.
 *
 * The caption is what names the table in the accessibility tree — an unnamed
 * table is invisible in a screen reader's element list, which is precisely
 * where a user goes looking for one.
 */
export function sortableTable({ columns, rows, caption, initial }) {
  const state = { key: initial.key, dir: initial.dir };
  const frame = el('div', {
    class: 'table-scroll table-scroll--compact',
    tabindex: '0',
    role: 'region',
    'aria-label': `${caption} Scrollable horizontally.`,
  });

  const render = (focusKey) => {
    const head = el(
      'tr',
      {},
      columns.map((column) => {
        const sorted = state.key === column.key;
        const th = el('th', {
          scope: 'col',
          class: column.num ? 'col-num' : null,
          'aria-sort': sorted ? state.dir : null,
        });
        const button = el('button', {
          type: 'button',
          text: column.label,
          title: `Sort by ${column.label}`,
          dataset: { sort: column.key },
        });
        button.addEventListener('click', () => {
          state.dir = state.key === column.key && state.dir === 'descending' ? 'ascending' : 'descending';
          state.key = column.key;
          render(column.key);
          announce(`Sorted by ${column.label}, ${state.dir}.`);
        });
        th.append(button);
        return th;
      }),
    );

    const body = el(
      'tbody',
      {},
      sortRows(rows, columns, state).map((row) =>
        el(
          'tr',
          {},
          columns.map((column) => el('td', { class: column.num ? 'col-num' : null }, [column.cell(row)])),
        ),
      ),
    );

    frame.replaceChildren(
      el('table', {}, [
        el('caption', { class: 'visually-hidden', text: caption }),
        el('thead', {}, [head]),
        body,
      ]),
    );
    // The button that was just activated has been replaced; without this the
    // caret drops to the document body mid-task, the same reason inventory.js
    // hands its active sort button back to app.js.
    if (focusKey) {
      const active = frame.querySelector(`th button[data-sort="${focusKey}"]`);
      if (active) active.focus();
    }
  };

  render(null);
  return frame;
}

/**
 * sortRows orders a copy; nulls sort last regardless of direction.
 *
 * Exported for vtable.js, which sorts the full list once and then renders a
 * window of it. The ordering rule has to be identical across the page's
 * tables, so it is defined here once rather than reimplemented there.
 */
export function sortRows(rows, columns, state) {
  const column = columns.find((c) => c.key === state.key);
  if (!column) return rows.slice();
  const sign = state.dir === 'ascending' ? 1 : -1;
  return rows.slice().sort((a, b) => {
    const [x, y] = [column.value(a), column.value(b)];
    if (x === null && y === null) return 0;
    if (x === null) return 1;
    if (y === null) return -1;
    if (x === y) return 0;
    return x < y ? -sign : sign;
  });
}

/**
 * dataFallback is the tabular counterpart to a chart, behind a disclosure.
 *
 * <details> rather than a custom toggle: it is keyboard operable and announces
 * its own expanded state with no ARIA at all, and the figures stay out of the
 * way of a sighted reader who is looking at the chart instead.
 */
export function dataFallback(label, table) {
  return el('details', { class: 'data-fallback' }, [el('summary', { text: label }), table]);
}

/** plainTable builds an unsorted table with the same frame and naming rules. */
export function plainTable({ columns, rows, caption }) {
  return el(
    'div',
    {
      class: 'table-scroll table-scroll--compact',
      tabindex: '0',
      role: 'region',
      'aria-label': `${caption} Scrollable horizontally.`,
    },
    [
      el('table', {}, [
        el('caption', { class: 'visually-hidden', text: caption }),
        el('thead', {}, [
          el(
            'tr',
            {},
            columns.map((column) =>
              el('th', { scope: 'col', class: column.num ? 'col-num' : null }, [
                el('span', { class: 'th-static', text: column.label }),
              ]),
            ),
          ),
        ]),
        el(
          'tbody',
          {},
          rows.map((row) =>
            el(
              'tr',
              {},
              columns.map((column) =>
                el('td', { class: column.num ? 'col-num' : null }, [column.cell(row)]),
              ),
            ),
          ),
        ),
      ]),
    ],
  );
}
