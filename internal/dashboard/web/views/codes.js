// The head-to-head error-code panel: one row per failure code, one column per
// provider.
//
// Providers are disjoint populations, so a raw count is not comparable across
// them the moment two proxy files differ in length. Every cell therefore
// carries the count *and* that count as a share of that provider's own total.
//
// A code a provider never produced shows 0, never a blank. A blank reads as
// "not measured"; here it means "never happened", and that distinction is the
// whole point of the panel — three of the four reference providers are clean,
// and the panel has to say so rather than say nothing.
//
// The grouping key is the code, not the message. For a failed row the tools
// write the literal string ERROR and the message is a boilerplate prefix
// repeated verbatim on every row, so the text is suppressed everywhere except
// `other` and `unknown`, which mean "matched no known pattern" and are
// unactionable without it.

import { count, el, pct, plural } from '../format.js';
import { panel } from './chart.js';
import { announce } from './datatable.js';

/** PROXY_CAP bounds one disclosure's proxy list; the DOM is not a log file. */
const PROXY_CAP = 200;

/**
 * Middle-truncation bounds, in characters.
 *
 * A proxy ID is credential@host:port and both ends distinguish it — gateway
 * pools share one host and differ only in a session credential, so cutting the
 * tail can leave every line looking identical. The width is measured from the
 * container so a 390px phone and a 1440px desktop each show what they can
 * hold, rather than one of them being wrong.
 */
const ID_MIN_CHARS = 24;
const ID_MAX_CHARS = 110;

/**
 * advance measures one monospace glyph in the font `node` is rendered with.
 *
 * Measured rather than assumed at 0.6em: the page names five fallback faces
 * and their advances differ, and being 6% optimistic about the glyph is
 * enough to run the last characters of every line under the frame's edge.
 * A 2d context is the cheapest measurement that does not touch the document.
 */
let pen = null;

function advance(node) {
  if (!pen) pen = document.createElement('canvas').getContext('2d');
  const style = getComputedStyle(node);
  pen.font = `${style.fontSize} ${style.fontFamily}`;
  return pen.measureText('0'.repeat(10)).width / 10 || 7;
}

/** uid keeps aria-controls targets unique across every panel on the page. */
let uid = 0;

/**
 * codesPanel renders the code breakdown for `runs` into `root`.
 *
 * `providerName(run)` supplies the display name; this module deliberately does
 * not derive one, so the whole view names a provider the same way in every
 * panel.
 *
 * Returns a teardown, or null when there is nothing to tear down.
 */
export function codesPanel(root, runs, providerName) {
  const providers = runs.map((run) => ({
    name: providerName(run),
    total: Number(run.summary?.total || 0),
    codes: new Map((run.codes || []).map((group) => [group.code, group])),
  }));

  const rows = collate(providers);
  if (rows.length === 0) {
    root.append(
      panel({
        label: 'Error codes',
        children: el('p', { class: 'panel__empty', text: cleanSweep(providers) }),
      }),
    );
    return null;
  }

  const failures = rows.reduce((sum, row) => sum + row.total, 0);
  const { node, teardown } = codesTable(providers, rows);

  root.append(
    panel({
      label: 'Error codes',
      note: `${plural(failures, 'failure', 'failures')} across ${plural(
        providers.length,
        'provider',
        'providers',
      )}, by code. Each count is also a share of that provider's own total, so files of different lengths stay comparable. Expand a row for the proxies behind it.`,
      children: node,
    }),
  );
  return teardown;
}

/**
 * collate merges the per-provider breakdowns into one row per code seen
 * anywhere, ordered by total count descending and then by code so ties are
 * stable rather than dependent on selection order.
 */
function collate(providers) {
  const totals = new Map();
  for (const provider of providers) {
    for (const group of provider.codes.values()) {
      totals.set(group.code, (totals.get(group.code) || 0) + group.count);
    }
  }
  return [...totals]
    .sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1))
    .map(([code, total]) => ({ code, total }));
}

/** cleanSweep is the sentence for a selection in which nothing failed at all. */
function cleanSweep(providers) {
  const proxies = providers.reduce((sum, provider) => sum + provider.total, 0);
  return `Every one of the ${count(proxies)} proxies across all ${plural(
    providers.length,
    'selected provider',
    'selected providers',
  )} succeeded. Not one was blocked and not one errored, so there is no code to break down.`;
}

/* ── The table ──────────────────────────────────────────────────────────── */

/**
 * codesTable builds the table and its disclosures.
 *
 * A real <table>: this is a matrix of codes against providers, and a cell means
 * nothing without its row and column, which only a table conveys. The expander
 * is a <button aria-expanded> over a sibling row rather than a <details> in a
 * cell — a <details> inside a <td> puts a non-table element between the row and
 * its content and the row/column association is lost with it.
 */
function codesTable(providers, rows) {
  const caption = `Failure codes by provider: one row per code, one column per provider, each cell the count and its share of that provider's total.`;
  const body = el('tbody');
  const eliding = elider();

  for (const row of rows) {
    const id = `codes-detail-${(uid += 1)}`;
    const detail = el('td', { colspan: providers.length + 1 }, [
      el('div', { class: 'codes__detail', id }),
    ]);
    const detailRow = el('tr', { class: 'codes__detail-row', hidden: true }, [detail]);

    const button = el('button', {
      type: 'button',
      class: 'codes__toggle',
      'aria-expanded': 'false',
      'aria-controls': id,
    }, [
      el('span', { class: 'codes__code', text: row.code }),
      el('span', {
        class: 'visually-hidden',
        text: `: show the ${plural(row.total, 'proxy', 'proxies')} carrying this code`,
      }),
    ]);

    button.addEventListener('click', () => {
      const open = button.getAttribute('aria-expanded') === 'true';
      // Built on first expand: most rows are never opened, and a hundred
      // proxy lines per row per provider is a DOM nobody asked for.
      if (!open && detail.firstChild.childElementCount === 0) {
        detail.firstChild.append(...detailContent(providers, row.code, eliding));
      }
      button.setAttribute('aria-expanded', open ? 'false' : 'true');
      detailRow.hidden = open;
      // Focus stays on the button — it is never replaced — so collapsing
      // leaves the caret where the reader put it.
      announce(
        open
          ? `${row.code} collapsed.`
          : `${row.code} expanded, ${plural(row.total, 'proxy', 'proxies')}.`,
      );
    });

    body.append(
      el('tr', { class: 'codes__row' }, [
        el('th', { scope: 'row', class: 'codes__key' }, [button]),
        ...providers.map((provider) => cell(provider, row.code)),
      ]),
      detailRow,
    );
  }

  const node = el(
    'div',
    {
      class: 'table-scroll table-scroll--compact',
      tabindex: '0',
      role: 'region',
      'aria-label': `${caption} Scrollable horizontally.`,
    },
    [
      el('table', { class: 'codes' }, [
        el('caption', { class: 'visually-hidden', text: caption }),
        el('thead', {}, [
          el('tr', {}, [
            el('th', { scope: 'col' }, [el('span', { class: 'th-static', text: 'Code' })]),
            ...providers.map((provider) =>
              el('th', { scope: 'col', class: 'col-num' }, [
                el('span', { class: 'th-static', text: provider.name }),
              ]),
            ),
          ]),
        ]),
        body,
      ]),
    ],
  );

  return { node, teardown: eliding.observe(node) };
}

/**
 * cell writes one provider's count for one code, and that count as a share.
 *
 * Zero is written out, muted but not faded: the share is dropped because
 * "0.0%" beside a 0 is the same fact twice, and the digit alone is what the
 * reader compares down the column.
 */
function cell(provider, code) {
  const group = provider.codes.get(code);
  if (!group) {
    return el('td', { class: 'col-num codes__cell codes__cell--zero', text: '0' });
  }
  return el('td', { class: 'col-num codes__cell' }, [
    el('span', { class: 'codes__count', text: count(group.count) }),
    el('span', {
      class: 'codes__share',
      text: provider.total ? pct(group.count / provider.total) : '—',
    }),
  ]);
}

/* ── The disclosure ─────────────────────────────────────────────────────── */

/**
 * detailContent names the proxies behind one code, grouped by provider.
 *
 * Providers with none are omitted: the row above already states their zero in
 * a cell the reader has just looked at, and repeating "none" under every code
 * triples the length of the disclosure to say it again.
 */
function detailContent(providers, code, eliding) {
  const nodes = [];
  for (const provider of providers) {
    const group = provider.codes.get(code);
    if (!group || group.proxies.length === 0) continue;

    const shown = group.proxies.slice(0, PROXY_CAP);
    const list = el(
      'ul',
      { class: 'codes__proxies' },
      shown.map((proxy) => el('li', { class: 'codes__proxy', title: proxy }, [proxy])),
    );
    eliding.register(list, shown);

    nodes.push(
      el('div', { class: 'codes__group' }, [
        el('p', { class: 'codes__group-head' }, [
          el('span', { class: 'codes__group-name', text: provider.name }),
          el('span', {
            class: 'codes__group-count num',
            text: plural(group.count, 'proxy', 'proxies'),
          }),
        ]),
        list,
        group.proxies.length > shown.length
          ? el('p', {
              class: 'panel__note',
              text: `${count(group.proxies.length - shown.length)} more not listed.`,
            })
          : null,
        // Messages exist only for `other` and `unknown`; for every classified
        // code the server sends [] by design, so this renders nothing.
        messages(group.messages),
      ]),
    );
  }
  return nodes;
}

/** messages lists the distinct raw errors behind an unclassified bucket. */
function messages(raw) {
  const distinct = [...new Set(raw || [])];
  if (distinct.length === 0) return null;
  return el('div', { class: 'codes__messages' }, [
    el('p', {
      class: 'codes__messages-head',
      text: `${plural(distinct.length, 'distinct message', 'distinct messages')}:`,
    }),
    el(
      'ul',
      { class: 'codes__message-list' },
      distinct.map((message) => el('li', { text: message })),
    ),
  ]);
}

/* ── Middle truncation ──────────────────────────────────────────────────── */

/**
 * elider owns the display truncation of every proxy list in one table.
 *
 * The elision has to be done in the text, because no CSS property elides a
 * middle. Doing it once at mount would then be wrong at every other width, so
 * the frame is observed and each list is rewritten from the full IDs it was
 * registered with — the truncation is display only and never loses the value.
 */
function elider() {
  const lists = [];
  let frame = null;

  // The list sits inside a cell that may be wider than the frame, so the
  // budget is the frame's width less the disclosure's own inset — doubled,
  // because that padding is symmetric and the right edge is the one that clips.
  const widthChars = (list) => {
    const inset = list.getBoundingClientRect().left - frame.getBoundingClientRect().left;
    const budget = (frame.clientWidth || 0) - inset * 2;
    const fits = Math.floor(budget / advance(list));
    return Math.max(ID_MIN_CHARS, Math.min(ID_MAX_CHARS, fits));
  };

  const write = ({ node, ids }) => {
    const width = widthChars(node);
    for (let i = 0; i < ids.length; i += 1) {
      node.children[i].textContent = middleTruncate(ids[i], width);
    }
  };

  return {
    register(node, ids) {
      const entry = { node, ids };
      lists.push(entry);
      // A list built on expand has missed every resize so far, so it is
      // written once here rather than waiting for the next one.
      if (frame) write(entry);
    },
    /** observe starts watching `node` and returns the teardown. */
    observe(node) {
      frame = node;
      const observer = new ResizeObserver(() => lists.forEach(write));
      observer.observe(node);
      return () => observer.disconnect();
    },
  };
}

/**
 * middleTruncate elides the centre of an ID, keeping both ends.
 *
 * The JS counterpart of util.TruncateID, and it has to agree with it: the same
 * proxy elided one way in the terminal and another in the browser is two
 * different-looking strings for one thing. Surrogate pairs are split with the
 * spread, so an ID outside the BMP counts characters the way Go's rune slice
 * does rather than UTF-16 code units.
 */
function middleTruncate(id, width) {
  const chars = [...String(id)];
  if (width <= 0 || chars.length <= width) return String(id);
  if (width < 4) return chars.slice(0, width).join('');
  const remaining = width - 3;
  const head = Math.ceil(remaining / 2);
  const tail = remaining - head;
  return `${chars.slice(0, head).join('')}...${chars.slice(chars.length - tail).join('')}`;
}
