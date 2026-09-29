// Set operations over two runs, downloaded as a proxy file.
//
// This is the one place on the dashboard whose output leaves the page, and it
// leaves it without touching the server. Everything the operation needs is
// already in join.rows, so the file is assembled in the browser and handed to
// the download as a Blob. The server stays what it has been since Task 15: a
// read-only reader of results/, with no endpoint that accepts anything.
//
// The result is one proxy per line in the canonical user:pass@host:port form
// the CSVs carry, with a trailing newline — which is exactly what every tool
// in this toolbox accepts as its input proxy file. The point of the panel is
// that its output can be fed straight back in.

import { count, el, plural } from '../format.js';
import { panel } from './chart.js';
import { announce } from './datatable.js';
import { chooser, okIds, runChooser, runLabel } from './crosstool-util.js';

/** PREVIEW is how many lines are shown before the download; the DOM is not a file. */
const PREVIEW = 12;

/**
 * OPERATIONS is the closed set, each with the sentence it means.
 *
 * `absent` matters here and is stated in every label that depends on it: a
 * proxy that was never tested in B is not a proxy that failed in B, and
 * folding the two together silently is how a set operation produces a file
 * that looks right and is wrong.
 */
const OPERATIONS = [
  {
    value: 'a-not-b',
    label: 'OK in A, not OK in B',
    describe: (a, b) => `OK in ${a}, and either failed or was never tested in ${b}.`,
    run: (ids, A, B) => ids.filter((id) => A.ok.has(id) && !B.ok.has(id)),
  },
  {
    value: 'a-not-b-tested',
    label: 'OK in A, failed in B (tested in both)',
    describe: (a, b) => `OK in ${a} and not OK in ${b}, counting only proxies that appear in both runs.`,
    run: (ids, A, B) => ids.filter((id) => A.ok.has(id) && B.seen.has(id) && !B.ok.has(id)),
  },
  {
    value: 'both',
    label: 'OK in both A and B',
    describe: (a, b) => `OK in ${a} and OK in ${b}.`,
    run: (ids, A, B) => ids.filter((id) => A.ok.has(id) && B.ok.has(id)),
  },
  {
    value: 'either',
    label: 'OK in A or B',
    describe: (a, b) => `OK in ${a}, in ${b}, or in both.`,
    run: (ids, A, B) => ids.filter((id) => A.ok.has(id) || B.ok.has(id)),
  },
  {
    value: 'neither',
    label: 'Not OK in either',
    describe: (a, b) => `OK in neither ${a} nor ${b}, including proxies missing from one of them.`,
    run: (ids, A, B) => ids.filter((id) => !A.ok.has(id) && !B.ok.has(id)),
  },
];

/**
 * setOpsPanel renders the two run choosers, the operation, and the download.
 *
 * The download control is a real <button>: it is in the tab order, it responds
 * to Enter and Space without any handler of its own, and it announces itself
 * as a button. An <a download> with a revoked object URL would be a link that
 * goes nowhere on a second activation.
 */
export function setOpsPanel(root, runs, join) {
  const ids = join.rows.map((row) => row.proxyId);
  const indexed = runs.map((_, i) => ({
    ok: okIds(join.rows, i),
    seen: new Set(join.rows.filter((row) => row.cells[i].present).map((row) => row.proxyId)),
  }));

  const state = { a: 0, b: runs.length > 1 ? 1 : 0, op: OPERATIONS[0].value };

  const summary = el('div', { class: 'setops__result' });
  const preview = el('p', { class: 'setops__preview' });
  const button = el('button', { type: 'button', class: 'button', text: 'Download as a proxy file' });
  button.addEventListener('click', () => download());

  root.append(
    panel({
      label: 'Set operations',
      note: 'Built from the matrix already in the page and written to a file by the browser. The dashboard server is read-only and is not involved. One proxy per line, in the form every tool in this toolbox reads back.',
      children: [
        el('div', { class: 'controls' }, [
          runChooser({
            id: 'setops-a',
            label: 'Run A',
            runs,
            value: state.a,
            onChange: (i) => {
              state.a = i;
              refresh();
            },
          }),
          runChooser({
            id: 'setops-b',
            label: 'Run B',
            runs,
            value: state.b,
            onChange: (i) => {
              state.b = i;
              refresh();
            },
          }),
          chooser({
            id: 'setops-op',
            label: 'Operation',
            options: OPERATIONS.map((op) => ({ value: op.value, label: op.label })),
            value: state.op,
            onChange: (value) => {
              state.op = value;
              refresh();
            },
          }),
        ]),
        summary,
        preview,
        el('p', { class: 'setops__actions' }, [button]),
      ],
    }),
  );

  /** result recomputes the current selection. Cheap enough to do on every change. */
  function result() {
    const op = OPERATIONS.find((o) => o.value === state.op);
    return { op, ids: op.run(ids, indexed[state.a], indexed[state.b]) };
  }

  /**
   * refresh rewrites the count and the preview.
   *
   * Not announced. This runs on every change of three <select>s, and a control
   * that narrates its own value on every keystroke of a native select is
   * unusable; the browser already announces the option the user chose. The
   * download is announced, because nothing else reports that one happened.
   */
  function refresh() {
    const { op, ids: chosen } = result();
    const a = runLabel(runs[state.a]);
    const b = runLabel(runs[state.b]);
    summary.replaceChildren(
      figureLine(chosen.length, `of ${count(ids.length)} proxies match`),
      el('p', { class: 'setops__describe', text: op.describe(a, b) }),
    );
    button.disabled = chosen.length === 0;
    preview.textContent =
      chosen.length === 0
        ? 'No proxy matches, so there is nothing to download.'
        : chosen.slice(0, PREVIEW).join('\n') +
          (chosen.length > PREVIEW ? `\n… and ${count(chosen.length - PREVIEW)} more in the file.` : '');
  }

  /**
   * download assembles the file and hands it to the browser.
   *
   * The object URL is revoked on the next frame rather than immediately:
   * revoking it in the same tick as the synthetic click races the download in
   * some browsers, and the anchor is discarded either way.
   */
  function download() {
    const { ids: chosen } = result();
    if (chosen.length === 0) return;
    const name = fileName(runs[state.a], runs[state.b], state.op);
    const blob = new Blob([chosen.join('\n') + '\n'], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = el('a', { href: url, download: name });
    anchor.click();
    requestAnimationFrame(() => URL.revokeObjectURL(url));
    announce(`Downloaded ${name}, ${plural(chosen.length, 'proxy', 'proxies')}.`);
  }

  refresh();
}

/**
 * fileName names the download after what produced it.
 *
 * Reduced to a conservative character set rather than escaped: a run's file
 * name comes out of a directory listing the user controls, and this string
 * becomes a filename on their disk. Nothing here needs to survive the
 * reduction — it is a label, and the file's contents are the payload.
 */
function fileName(a, b, op) {
  const part = (run) => String(run.tool || run.file || 'run').replace(/[^A-Za-z0-9._-]+/g, '-');
  return `proxies_${op}_${part(a)}_${part(b)}.txt`;
}

/**
 * figureLine is the result stated as a figure: the count large, the unit small.
 *
 * The count is what the reader is deciding on — whether this operation selects
 * three proxies or three hundred changes what they do next — so it is set at
 * display size rather than folded into the sentence beneath it.
 */
function figureLine(value, caption) {
  return el('p', { class: 'figure-line' }, [
    el('strong', { class: 'figure-line__value num', text: count(value) }),
    el('span', { class: 'figure-line__caption', text: caption }),
  ]);
}
