// Compare dashboard — shell and inventory.
//
// Vendored: uPlot 1.6.32 (MIT), in web/vendor/. Loaded from the binary, never
// from a CDN: the dashboard has to work with no network at all.
//
// The shell owns exactly three things — the inventory, the selection, and the
// stat strip. Everything drawn below the table is a registered view; see
// views/registry.js for the contract and views/index.js for the manifest.

import { MAX_COMPARE_FILES, fetchRuns } from './api.js';
import { count, el } from './format.js';
import { renderTable, syncSelection } from './inventory.js';
import { renderViews, selectionFrom } from './views/registry.js';
import './views/index.js';

/**
 * state is the whole client model. `runs` is the server's inventory in its own
 * order (newest first); sorting derives a view of it rather than mutating it.
 */
const state = {
  runs: [],
  selected: new Set(),
  sort: { key: 'runAt', dir: 'descending' },
};

const dom = {
  body: document.getElementById('inventory-body'),
  note: document.getElementById('inventory-note'),
  views: document.getElementById('views'),
  status: document.getElementById('sr-status'),
  runs: document.getElementById('stat-runs'),
  selected: document.getElementById('stat-selected'),
  rows: document.getElementById('stat-rows'),
};

/** onSort toggles direction when the same column is clicked twice. */
function onSort(key) {
  const dir =
    state.sort.key === key && state.sort.dir === 'descending' ? 'ascending' : 'descending';
  state.sort = { key, dir };
  draw(true);
  // The table itself is not a live region — re-announcing six thousand
  // characters on every sort would be unusable. This is the one line that
  // stands in for it, and aria-sort on the header carries the rest.
  announce(`Sorted by ${label(key)}, ${dir === 'ascending' ? 'ascending' : 'descending'}.`);
}

/** label is the column's header text, for the sort announcement. */
function label(key) {
  const button = dom.body.querySelector(`th button[data-sort="${key}"]`);
  return button ? button.textContent : key;
}

/**
 * announce writes one line to the status region.
 *
 * The text is cleared first: a status region that receives the same string
 * twice announces it once, and sorting the same column twice is exactly the
 * case where the user most needs to hear that something happened.
 */
function announce(message) {
  dom.status.textContent = '';
  dom.status.textContent = message;
}

/**
 * onToggle adds or removes one run from the selection. The table is updated in
 * place rather than rebuilt, so the checkbox the user just operated keeps focus.
 */
function onToggle(file) {
  if (state.selected.has(file)) state.selected.delete(file);
  else state.selected.add(file);
  syncSelection(dom.body, state);
  refresh();
  const size = state.selected.size;
  announce(
    size === MAX_COMPARE_FILES
      ? `${count(size)} runs selected. That is the maximum a comparison accepts.`
      : `${count(size)} ${size === 1 ? 'run' : 'runs'} selected.`,
  );
}

/**
 * draw rebuilds the table — sort order changed — and then refreshes.
 *
 * keepFocus is true when the rebuild was triggered from the keyboard: the
 * button that was activated has just been replaced, and without this the
 * caret falls back to the document body mid-task.
 */
function draw(keepFocus) {
  const active = renderTable(dom.body, state, { onSort, onToggle });
  if (keepFocus && active) active.focus();
  refresh();
}

/** refresh updates the stat strip and remounts whichever views now match. */
function refresh() {
  dom.selected.textContent = count(state.selected.size);
  renderViews(dom.views, selectionFrom(state.runs, state.selected));
}

/**
 * emptyState explains how to produce a CSV. results/ is empty before the first
 * export, which is the state a first-time user lands in; a blank page there
 * looks like a broken dashboard rather than an accurate one.
 */
function emptyState() {
  return el('div', { class: 'notice' }, [
    el('h3', { text: 'No runs to compare yet' }),
    el('p', {
      text: 'The dashboard reads exported CSVs from the results/ directory, and that directory is empty.',
    }),
    el('ol', {}, [
      el('li', { text: 'Run any module.' }),
      el('li', { text: 'Save the results of the run in a CSV.' }),
      el('li', { text: 'Reload this page.' }),
    ]),
  ]);
}

/**
 * errorState reports a failed fetch in place. Never a silent blank.
 *
 * The two failures a user can actually hit want different advice, so the hint
 * is chosen from which one happened rather than guessing at both: the server
 * exiting under them, and results/ being unreadable.
 */
function errorState(err) {
  const message = String(err && err.message ? err.message : err);
  const unreachable = message.includes('cannot reach');
  // role="alert" on the notice itself, rather than a live region around the
  // container it replaces: the alert is this content, and nothing else that
  // lands in that container should be announced.
  return el('div', { class: 'notice notice--error', role: 'alert' }, [
    el('h3', { text: 'Could not load the run inventory' }),
    el('p', { text: message }),
    el('p', {
      text: unreachable
        ? 'The dashboard server has stopped — it exits when you press Enter in the terminal. Restart it from the Compare Results menu entry and reload.'
        : 'The server could not read the results/ directory. Check that it exists and is readable, then reload. The terminal running the dashboard logs the underlying error.',
    }),
  ]);
}

/** load fetches the inventory once and renders whichever state it implies. */
async function load() {
  try {
    state.runs = await fetchRuns();
  } catch (err) {
    dom.note.textContent = 'Failed to read results/.';
    dom.runs.textContent = '—';
    dom.body.replaceChildren(errorState(err));
    return;
  }

  const rows = state.runs.reduce((sum, run) => sum + run.summary.total, 0);
  dom.runs.textContent = count(state.runs.length);
  dom.rows.textContent = count(rows);

  if (state.runs.length === 0) {
    dom.note.textContent = 'results/ is empty.';
    dom.body.replaceChildren(emptyState());
    return;
  }

  const stale = state.runs.filter((run) => !run.hasMeta).length;
  dom.note.textContent = stale
    ? `Select runs to compare. ${stale} of ${state.runs.length} predate run metadata.`
    : 'Select runs to compare.';
  draw();
}

load();
