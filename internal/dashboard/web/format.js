// Formatting and DOM helpers shared by every view.
//
// Nothing here uses innerHTML. Run metadata — proxy file, target, tool — comes
// out of a CSV the user may have hand-edited, and the only reason this page is
// safe to serve credentials on is that it never executes what it reads.

/** UNKNOWN is what a field reads when the export predates run metadata. */
export const UNKNOWN = 'unknown';

/**
 * The two reasons a metadata cell is empty, each written once.
 *
 * A `title` reaches a mouse user and nobody else — not the keyboard, not a
 * screen reader, not touch. The text therefore lives in the document, once,
 * and every muted cell points at it with aria-describedby. A description is
 * announced on demand rather than inline, so pointing 55 rows x 5 columns at
 * the same paragraph costs nothing in verbosity. The `title` stays for the
 * sighted mouse user who expects a tooltip.
 */
export const UNKNOWN_HINT =
  'This CSV was exported before runs recorded their own metadata, so the tool, time, proxy file, target and worker count are not recoverable.';
export const UNKNOWN_HINT_ID = 'hint-unknown';

export const NOT_RECORDED_HINT =
  'This run recorded no target; the tool does not test one.';
export const NOT_RECORDED_HINT_ID = 'hint-not-recorded';

/**
 * el builds an element. attrs maps attribute names to values, except `class`,
 * `text` and `dataset`, which are handled specially; children may be nodes or
 * strings.
 *
 * `true` serialises as the string "true", not as the empty string. ARIA is a
 * string vocabulary: aria-hidden="" reads as *undefined*, not as true, so the
 * empty form silently does nothing — and for aria-expanded, aria-pressed,
 * aria-selected and aria-current it reads as the opposite of what was meant.
 * `false` is dropped before reaching here, so "true" is the only string this
 * branch can produce. `hidden` is the one boolean HTML attribute in use and
 * keeps its empty-string form, which is what HTML wants.
 */
export function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key === 'text') node.textContent = value;
    else if (key === 'dataset') Object.assign(node.dataset, value);
    else if (value !== true) node.setAttribute(key, String(value));
    else node.setAttribute(key, key === 'hidden' ? '' : 'true');
  }
  for (const child of [].concat(children)) {
    if (child === null || child === undefined) continue;
    node.append(child);
  }
  return node;
}

/** unknownCell renders the muted, explained placeholder for missing metadata. */
export function unknownCell() {
  return el('span', {
    class: 'unknown',
    title: UNKNOWN_HINT,
    'aria-describedby': UNKNOWN_HINT_ID,
    text: UNKNOWN,
  });
}

/**
 * hints renders the descriptions the muted cells point at. It is placed once,
 * off-screen, wherever the table that uses them is rendered.
 */
export function hints() {
  return el('div', { class: 'visually-hidden' }, [
    el('p', { id: UNKNOWN_HINT_ID, text: UNKNOWN_HINT }),
    el('p', { id: NOT_RECORDED_HINT_ID, text: NOT_RECORDED_HINT }),
  ]);
}

/**
 * notRecordedCell is the other reason a metadata field is empty: the run does
 * carry metadata, but that tool has nothing to put there. iptester records no
 * target, and labelling it "unknown" would blame the export format for a fact
 * about the tool.
 */
export function notRecordedCell() {
  return el('span', {
    class: 'unknown',
    title: NOT_RECORDED_HINT,
    'aria-describedby': NOT_RECORDED_HINT_ID,
    text: 'n/a',
  });
}

/**
 * count formats an integer, grouping thousands with a narrow no-break space
 * from five digits up.
 *
 * Deliberately not toLocaleString for the grouping character: a German locale
 * groups with a full stop, so a 49960-row inventory reads as "49.960" beside
 * latencies written in ms. The space cannot be mistaken for a decimal
 * separator in any locale.
 *
 * Four-digit numbers are left unbroken, which is the SI/ISO 31-0 convention
 * and here a necessity rather than a nicety: these figures are set in the mono
 * face, where every glyph takes one identical advance. U+202F is only narrow
 * in a proportional font; in mono it is a full space, and "1 329ms" reads as
 * two values rather than one.
 */
const GROUP_FROM = 10000;

export function count(n) {
  const v = Number(n || 0);
  const grouped = v.toLocaleString('en-US');
  return Math.abs(v) < GROUP_FROM ? grouped.replace(/,/g, '') : grouped.replace(/,/g, '\u202f');
}

/** pct formats a 0..1 ratio as a percentage with one decimal. */
export function pct(ratio) {
  return `${(Number(ratio || 0) * 100).toFixed(1)}%`;
}

/**
 * ms formats a millisecond figure. A run whose latencyCount is zero has no
 * distribution at all, and callers pass null so it reads as a dash rather than
 * a confident "0ms".
 */
export function ms(value) {
  if (value === null || value === undefined) return '—';
  return `${count(Math.round(value))}ms`;
}

/**
 * runTime formats an RFC 3339 timestamp for the table. Go marshals a zero
 * time.Time as "0001-01-01T00:00:00Z", which is how a run with no metadata
 * arrives; that is not a date and must not be printed as one.
 */
export function runTime(iso) {
  const t = Date.parse(iso);
  if (!iso || Number.isNaN(t) || new Date(t).getUTCFullYear() < 1970) return null;
  const d = new Date(t);
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(
    d.getMinutes(),
  )}`;
}

/**
 * signed writes a change with an explicit sign and a real minus sign.
 *
 * The sign is what carries the direction. Every delta on the page is also
 * tinted, but colour is a reinforcement here and never the only channel.
 */
export function signed(value) {
  const rounded = Math.round(value);
  if (rounded > 0) return `+${count(rounded)}`;
  if (rounded < 0) return `\u2212${count(Math.abs(rounded))}`;
  return '0';
}

/** plural picks the noun for a count. One proxy is not "1 proxies". */
export function plural(n, one, many) {
  return `${count(n)} ${n === 1 ? one : many}`;
}

/**
 * errorRate is the share of results that neither succeeded nor were blocked.
 * Blocked is a distinct outcome — the proxy worked and the target refused — so
 * folding it in here would overstate proxy failure.
 */
export function errorRate(summary) {
  if (!summary.total) return 0;
  return summary.errors / summary.total;
}
