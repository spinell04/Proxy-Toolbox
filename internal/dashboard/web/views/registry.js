// The seam between the inventory and the comparison views.
//
// app.js knows nothing about any individual view. It builds a Selection from
// the checked rows and hands it here; each view module declares, at import
// time, which selections it can render. Adding a view is one import in
// views/index.js and one register() call in the view's own module — no edit to
// app.js, no growing switch.
//
// A view is an object:
//
//   id      stable identifier, used as the section's DOM id
//   title   heading text
//   match   (selection) => boolean — can this view render this selection?
//   mount   (element, selection) => void | (() => void)
//
// mount receives an empty <section> and the same Selection that matched. It
// may return a teardown function, which is called before the next render; a
// view that attaches a resize listener or builds a uPlot instance must return
// one, because the section it drew into is discarded.
//
// mount may be async. A rejection is reported in place rather than left as a
// blank panel — the same rule the shell follows for a failed inventory fetch.

import { el } from '../format.js';

/** @type {Array<{id: string, title: string, match: Function, mount: Function}>} */
const views = [];

/** teardowns holds the cleanup functions returned by the currently mounted views. */
let teardowns = [];

/** register adds a view. Later registrations render below earlier ones. */
export function register(view) {
  for (const field of ['id', 'title', 'match', 'mount']) {
    if (!view || !view[field]) throw new Error(`view is missing ${field}`);
  }
  views.push(view);
}

/**
 * Selection is what a view is asked to render.
 *
 * @typedef {object} Selection
 * @property {string[]} files      selected file names, in inventory order
 * @property {object[]} runs       the matching inventory items
 * @property {string[]} tools      distinct tool names, empty entries dropped
 * @property {boolean} sameTool    every selected run is the same known tool
 * @property {string[]} proxyFiles distinct proxy file names, empty entries dropped
 * @property {boolean} sameProxyFile every selected run names the same proxy file
 * @property {object[]} withMeta   runs carrying metadata
 * @property {object[]} withoutMeta runs that predate it
 */

/** selectionFrom builds a Selection from the inventory and the checked files. */
export function selectionFrom(runs, selected) {
  const chosen = runs.filter((run) => selected.has(run.file));
  const tools = [...new Set(chosen.map((run) => run.tool).filter(Boolean))];
  const proxyFiles = [...new Set(chosen.map((run) => run.proxyFile).filter(Boolean))];
  return {
    files: chosen.map((run) => run.file),
    runs: chosen,
    tools,
    // A run with no metadata has no tool, so it cannot make a selection
    // same-tool; requiring every run to carry one keeps "same tool" honest.
    sameTool: tools.length === 1 && chosen.every((run) => run.tool),
    proxyFiles,
    // Same reasoning as sameTool: a run that records no proxy file leaves the
    // question unanswered, and unanswered is not the same as yes.
    sameProxyFile: proxyFiles.length === 1 && chosen.every((run) => run.proxyFile),
    withMeta: chosen.filter((run) => run.hasMeta),
    withoutMeta: chosen.filter((run) => !run.hasMeta),
  };
}

/**
 * renderViews tears down whatever is mounted and mounts every view matching
 * the selection. An empty selection clears the container and renders nothing:
 * the inventory is the landing view, not a placeholder panel.
 *
 * A *non-empty* selection that matches nothing is a different case entirely,
 * and gets an explanation rather than the same blank. The user deliberately
 * ticked those boxes; silence tells them the page is broken. The explanation
 * is written from the registry's own knowledge of what each view wants, so it
 * covers any future gap between the matches and not only today's.
 */
export function renderViews(container, selection) {
  for (const teardown of teardowns) {
    try {
      teardown();
    } catch (err) {
      console.error('view teardown failed', err);
    }
  }
  teardowns = [];
  container.replaceChildren();

  if (selection.files.length === 0) return;

  const matching = views.filter((view) => view.match(selection));
  if (matching.length === 0) {
    container.append(unmatched(selection));
    return;
  }

  for (const view of matching) {

    const heading = el('h2', { id: `${view.id}-heading`, text: view.title });
    const body = el('div');
    container.append(
      el('section', { id: view.id, 'aria-labelledby': heading.id }, [
        el('div', { class: 'section-head' }, [heading]),
        body,
      ]),
    );

    try {
      const result = view.mount(body, selection);
      if (typeof result === 'function') teardowns.push(result);
      else if (result && typeof result.then === 'function') {
        result.then(
          (teardown) => {
            if (typeof teardown === 'function') teardowns.push(teardown);
          },
          (err) => mountFailed(body, view, err),
        );
      }
    } catch (err) {
      mountFailed(body, view, err);
    }
  }
}

/**
 * unmatched explains a selection that no view can render.
 *
 * It states what was selected and what each view wants, so the user can act
 * rather than guess. The registry asks the views for their titles and derives
 * the rest from the Selection, which is everything it has — it deliberately
 * does not learn any view's match rule, so this stays one sentence per view
 * and not a second copy of the matching logic to keep in step.
 */
function unmatched(selection) {
  const known = selection.runs.filter((run) => run.tool).length;
  return el('div', { class: 'notice' }, [
    el('h3', { text: 'Nothing to compare in this selection' }),
    el('p', {
      text: `${selection.files.length} ${
        selection.files.length === 1 ? 'run is' : 'runs are'
      } selected, and no comparison fits them. ${
        selection.runs.length - known > 0
          ? `${selection.runs.length - known} of them predate run metadata, so their tool is unknown and they cannot be grouped with anything by tool.`
          : ''
      }`.trim(),
    }),
    el('p', { text: 'What each view needs:' }),
    el('ul', {}, [
      el('li', { text: 'Run detail — exactly one run.' }),
      el('li', {
        text: 'Timeline and Paired — two or more runs of the same known tool, not against different proxy files.',
      }),
      el('li', {
        text: 'Head-to-head — two or more runs of the same known tool, run against different proxy files.',
      }),
      el('li', { text: 'Cross-tool — two or more runs that are not all the same known tool.' }),
    ]),
  ]);
}

/** mountFailed replaces a view's body with its error, never a blank panel. */
function mountFailed(body, view, err) {
  console.error(`view ${view.id} failed`, err);
  body.replaceChildren(
    el('div', { class: 'notice notice--error' }, [
      el('h3', { text: `${view.title} could not be rendered` }),
      el('p', { text: String(err && err.message ? err.message : err) }),
    ]),
  );
}
