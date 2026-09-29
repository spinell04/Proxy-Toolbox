// A sortable table that only builds the rows you can see.
//
// The cross-tool matrix is the one surface on this page whose row count is not
// bounded by anything the user chose: it is the union of every proxy across
// every selected run, and a 5000-line proxy file produces 5000 rows times up
// to fifty columns. Building that eagerly is a quarter of a million cells, and
// re-sorting it is the same again — so the tbody holds a window of rows with a
// spacer above and below standing in for the rest.
//
// Three deliberate decisions make that safe for a screen reader and a
// keyboard, and none of them is free:
//
//  1. Below VIRTUALIZE_MIN rows nothing is virtualized at all. A 180-row
//     matrix is cheap to build whole, and a whole table is strictly better to
//     read, so the windowing only switches on where it actually buys anything.
//  2. aria-rowcount and aria-rowindex are set from the *full* sorted list, not
//     from the window. Without them a screen reader announces "row 3 of 40"
//     halfway down five thousand proxies, which is worse than no count.
//  3. Nothing inside the tbody is focusable — the cells are text, the sort
//     buttons live in the thead, which is never unmounted, and the scroll
//     container itself carries the tab stop. Focus therefore cannot be
//     standing on a row when the window scrolls out from under it.
//
// The header is built once and mutated in place on sort, rather than replaced.
// That is the same problem inventory.js solves by re-focusing the rebuilt
// button; not destroying it in the first place is simpler and does not move
// the caret at all.

import { el } from '../format.js';
import { announce, sortRows } from './datatable.js';

/** OVERSCAN is how many rows are rendered beyond the viewport at each end. */
const OVERSCAN = 8;

/**
 * VIRTUALIZE_MIN is the row count below which the whole table is built.
 *
 * Generous on purpose: virtualization is an accessibility cost, so it is only
 * paid where the alternative is a page that stutters.
 */
const VIRTUALIZE_MIN = 200;

/** FALLBACK_ROW_HEIGHT seeds the first paint; the real height is measured after it. */
const FALLBACK_ROW_HEIGHT = 28;

/**
 * virtualTable renders columns over rows inside a named scroll frame.
 *
 * columns is `[{key, label, sub, num, title, value(row), cell(row), attrs(row)}]`
 * — datatable.js's shape plus the optional `sub` and `attrs`, so a column
 * definition can otherwise move between the two. `initial` is `{key, dir}`.
 *
 * Returns `{node, setRows(rows), count()}`. setRows is what a filter calls; it
 * resets the scroll position, because a filtered list has no relationship to
 * the offset the previous one was scrolled to.
 */
export function virtualTable({ columns, rows, caption, initial, label }) {
  const state = {
    key: initial.key,
    dir: initial.dir,
    sorted: [],
    start: -1,
    end: -1,
    rowHeight: 0,
  };

  const body = el('tbody');
  const headCells = new Map();
  const headRow = el('tr', { 'aria-rowindex': '1' }, columns.map((column) => headCell(column)));
  const table = el('table', { class: 'vtable__table' }, [
    el('caption', { class: 'visually-hidden', text: caption }),
    el('thead', {}, [headRow]),
    body,
  ]);
  const frame = el(
    'div',
    {
      class: 'table-scroll table-scroll--compact vtable',
      tabindex: '0',
      role: 'region',
      'aria-label': `${caption} Scrollable, and scrolled with the arrow keys.`,
    },
    [table],
  );

  function headCell(column) {
    const th = el('th', { scope: 'col', class: column.num ? 'col-num' : null });
    const button = el(
      'button',
      {
        type: 'button',
        title: column.title ? `${column.title} — sort by this column` : `Sort by ${column.label}`,
        dataset: { sort: column.key },
      },
      [
        document.createTextNode(column.label),
        // `sub` is a second header line, and it is part of the button's
        // accessible name rather than a title: a title reaches a mouse and
        // nothing else, so two columns distinguished only by one would be
        // indistinguishable to a keyboard or touch user. Callers pass it only
        // when the labels would otherwise collide.
        // The leading space is load-bearing and invisible: it separates the two
        // lines in the button's accessible name, which is one concatenated
        // string. Without it the name reads "pinger2026-09-17 11:12". The
        // space collapses at the start of a block box, so nothing moves.
        column.sub ? el('span', { class: 'th-sub', text: ` ${column.sub}` }) : null,
      ],
    );
    button.addEventListener('click', () => {
      state.dir = state.key === column.key && state.dir === 'descending' ? 'ascending' : 'descending';
      state.key = column.key;
      applySort();
      announce(`Sorted by ${column.label}, ${state.dir}.`);
    });
    th.append(button);
    headCells.set(column.key, th);
    return th;
  }

  /** applySort re-orders the full list and repaints the window from the top. */
  function applySort() {
    for (const [key, th] of headCells) {
      if (key === state.key) th.setAttribute('aria-sort', state.dir);
      else th.removeAttribute('aria-sort');
    }
    state.sorted = sortRows(state.sorted, columns, state);
    frame.scrollTop = 0;
    state.start = -1;
    paint();
  }

  /**
   * window computes the slice to render.
   *
   * Under VIRTUALIZE_MIN it is the whole list, which is what makes the small
   * case an ordinary table with no spacers and no aria bookkeeping to get
   * wrong.
   */
  function windowFor() {
    const total = state.sorted.length;
    if (total < VIRTUALIZE_MIN) return { start: 0, end: total };
    const height = state.rowHeight || FALLBACK_ROW_HEIGHT;
    const viewport = frame.clientHeight || height * 20;
    const visible = Math.ceil(viewport / height) + OVERSCAN * 2;
    const start = Math.max(0, Math.floor(frame.scrollTop / height) - OVERSCAN);
    return { start, end: Math.min(total, start + visible) };
  }

  /** paint rebuilds the tbody for the current window, if the window moved. */
  function paint(force) {
    const { start, end } = windowFor();
    if (!force && start === state.start && end === state.end) return;
    state.start = start;
    state.end = end;

    const height = state.rowHeight || FALLBACK_ROW_HEIGHT;
    const children = [];
    if (start > 0) children.push(spacer(start * height));
    for (let i = start; i < end; i += 1) {
      // aria-rowindex is one-based and counts the header row, so a data row at
      // list position i is row i + 2.
      children.push(
        el(
          'tr',
          { 'aria-rowindex': String(i + 2) },
          columns.map((column) => {
            // `attrs` is how the matrix tints a cell by outcome and latency.
            // It lands on the <td> rather than on a wrapper inside it so the
            // tint fills the cell instead of hugging four characters of text.
            const extra = column.attrs ? column.attrs(state.sorted[i]) : null;
            const classes = [column.num ? 'col-num' : null, extra && extra.class]
              .filter(Boolean)
              .join(' ');
            return el('td', { class: classes || null, style: extra && extra.style }, [
              column.cell(state.sorted[i]),
            ]);
          }),
        ),
      );
    }
    const trailing = state.sorted.length - end;
    if (trailing > 0) children.push(spacer(trailing * height));
    body.replaceChildren(...children);

    // One measurement, taken from a row the browser has actually laid out.
    // A guessed height that is off by two pixels drifts a thousand rows down
    // into a visible gap, so the first real paint corrects it and repaints.
    if (!state.rowHeight) {
      const first = body.querySelector('tr:not([aria-hidden])');
      if (first && first.offsetHeight > 0) {
        state.rowHeight = first.offsetHeight;
        if (state.rowHeight !== height) paint(true);
      }
    }
  }

  /** spacer stands in for the rows outside the window, at their exact height. */
  function spacer(height) {
    return el('tr', { class: 'vtable__spacer', 'aria-hidden': true }, [
      el('td', { colspan: String(columns.length), style: `height:${Math.round(height)}px` }),
    ]);
  }

  // Scroll is the hot path: coalesced into one frame, and a frame that does
  // not move the window does no DOM work at all.
  let frameId = 0;
  const onScroll = () => {
    if (frameId) return;
    frameId = requestAnimationFrame(() => {
      frameId = 0;
      paint();
    });
  };
  frame.addEventListener('scroll', onScroll, { passive: true });

  const api = {
    node: frame,
    /** setRows replaces the data. The scroll offset of the old list means nothing for the new one. */
    setRows(next) {
      state.sorted = sortRows(next, columns, state);
      table.setAttribute('aria-rowcount', String(state.sorted.length + 1));
      frame.scrollTop = 0;
      state.start = -1;
      paint();
    },
    count: () => state.sorted.length,
    /** destroy releases the scroll listener; the view contract requires it. */
    destroy() {
      cancelAnimationFrame(frameId);
      frame.removeEventListener('scroll', onScroll);
    },
  };

  headCells.get(state.key)?.setAttribute('aria-sort', state.dir);
  api.setRows(rows);
  return api;
}
