// Chart plumbing shared by the comparison views.
//
// uPlot draws into a <canvas>, and a canvas carries two problems this module
// exists to solve. It cannot read a CSS custom property, so every colour is
// resolved from the live tokens at mount and re-resolved when the colour
// scheme flips — uPlot does not react to prefers-color-scheme on its own. And
// it announces nothing, so no chart is ever mounted bare: every one is wrapped
// in a <figure> whose caption and hidden summary say what it shows.
//
// Nothing here introduces a colour. Series tints come out of tokens.css, and
// where a series needs more steps than there are tokens the ramp is mixed
// between two of them rather than invented.

import { el } from '../format.js';

const DARK_QUERY = '(prefers-color-scheme: dark)';

/**
 * probe is one off-screen element used to resolve colour expressions.
 *
 * getComputedStyle on :root hands back a custom property's *text*
 * ("oklch(55% 0.196 30)", or a color-mix() that has not been evaluated), which
 * a canvas context may or may not parse. Setting it as a real `color` and
 * reading the computed value back makes the engine do the evaluation, so what
 * reaches the canvas is a colour the same engine already resolved.
 */
let probe = null;

function resolveColor(expr) {
  if (!probe) {
    probe = el('span', { class: 'visually-hidden', 'aria-hidden': true });
    document.body.append(probe);
  }
  probe.style.color = 'transparent';
  probe.style.color = expr;
  return getComputedStyle(probe).color;
}

/** token reads a custom property off the document root, untouched. */
function token(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/**
 * theme resolves everything a chart draws with, for the scheme in force now.
 *
 * `mix` builds the ramps the views need. Where a chart has one series per run,
 * chronology is the thing being read: the oldest run sits back in ink and the
 * newest carries the accent. That keeps the one-accent rule — the accent still
 * marks the thing to look at — without inventing a second palette to tell
 * fifty runs apart.
 */
export function theme() {
  const mono = token('--face-mono');
  return {
    ink: resolveColor(token('--ink')),
    muted: resolveColor(token('--ink-muted')),
    faint: resolveColor(token('--ink-faint')),
    rule: resolveColor(token('--rule-hair')),
    ruleMid: resolveColor(token('--rule-mid')),
    accent: resolveColor(token('--accent')),
    good: resolveColor(token('--good')),
    warn: resolveColor(token('--warn')),
    bad: resolveColor(token('--bad')),
    font: `11px ${mono}`,
    // color resolves any CSS colour expression the caller already wrote for
    // the DOM — a token reference, a color-mix() — into something the canvas
    // takes. It is what lets one style function feed both a series stroke and
    // its legend swatch, instead of the two being written out twice and
    // drifting apart.
    color: resolveColor,
    // mix blends two already-resolved colours, for ramps a view builds from
    // two semantic tokens rather than from new ones.
    //
    // The space is the caller's because it changes what the ramp means. oklch
    // travels the hue arc, which is right between two warm tokens — bad to
    // warn passes through orange, as a reader expects. Between the accent and
    // the near-neutral ink it is wrong: the shorter arc from blue-grey to
    // vermilion runs through magenta, inventing a hue the palette does not
    // have. oklab interpolates straight through and simply desaturates.
    mix: (a, b, share, space = 'oklch') =>
      resolveColor(`color-mix(in ${space}, ${a} ${share}%, ${b})`),
    // A tint at the alpha the fills use. Separate from the stroke so a stacked
    // band never reaches the opacity of a line.
    wash: (color) => resolveColor(`color-mix(in oklch, ${color} 34%, transparent)`),
  };
}

/** MIN_WIDTH is the floor a chart is drawn at; below 320px the host is narrower. */
const MIN_WIDTH = 240;

/**
 * mountChart builds a uPlot into host and keeps it in step with the viewport
 * and the colour scheme.
 *
 * build(theme, width) returns `{opts, data}`; it is called again — and the
 * plot rebuilt — whenever the width changes or the scheme flips, because
 * uPlot resolves its strokes once at construction. Returns the teardown the
 * view contract requires: without it the scheme listener outlives the section
 * it drew into.
 */
export function mountChart(host, build) {
  let plot = null;
  let lastWidth = 0;
  let frame = 0;

  const draw = () => {
    const width = Math.max(MIN_WIDTH, Math.floor(host.clientWidth) || MIN_WIDTH);
    lastWidth = width;
    const { opts, data } = build(theme(), width);
    if (plot) plot.destroy();
    plot = new window.uPlot({ ...opts, width }, data, host);
  };

  draw();

  // Width only. The plot sets its own height, so observing both axes would
  // have the observer fire on the change it just caused.
  const observer = new ResizeObserver(() => {
    if (Math.floor(host.clientWidth) === lastWidth) return;
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(draw);
  });
  observer.observe(host);

  const scheme = window.matchMedia(DARK_QUERY);
  scheme.addEventListener('change', draw);

  return () => {
    cancelAnimationFrame(frame);
    observer.disconnect();
    scheme.removeEventListener('change', draw);
    if (plot) plot.destroy();
  };
}

/** chartHeight keeps a wide chart in proportion and a phone-width one usable. */
export function chartHeight(width, ratio = 0.42) {
  return Math.round(Math.min(420, Math.max(190, width * ratio)));
}

/**
 * panel is the view's unit of composition: a marginal label column and a body.
 *
 * Not a card — the rule above it and the whitespace around it are the whole
 * frame, which is what the rest of the page does.
 */
export function panel({ label, note, children }) {
  return el('section', { class: 'panel', 'aria-label': label }, [
    el('div', { class: 'panel__head' }, [
      el('h3', { class: 'panel__label', text: label }),
      note ? el('p', { class: 'panel__note', text: note }) : null,
    ]),
    el('div', { class: 'panel__body' }, [].concat(children)),
  ]);
}

/**
 * figure wraps a chart in the semantics the canvas itself cannot supply.
 *
 * `summary` is the sentence a screen reader gets in the canvas's place, and is
 * required — a figure with no summary is a figure that announces nothing.
 *
 * `actions` are controls that operate on the plot — today only the zoom reset.
 * They sit inside the figure and above the canvas, because a control that
 * changes what the chart shows belongs with the chart and not in the panel
 * head, which is prose.
 *
 * Returns the node plus the host the plot mounts into.
 */
export function figure({ summary, keys, children, actions }) {
  const host = el('div', { class: 'chart' });
  const node = el('figure', { class: 'figure' }, [
    actions ? el('div', { class: 'figure__actions' }, [].concat(actions)) : null,
    host,
    keys || null,
    el('figcaption', { class: 'visually-hidden', text: summary }),
    ...[].concat(children || []),
  ]);
  return { node, host };
}

/** DASHED is the canvas dash the `dashed: true` shorthand stands for. */
const DASHED = [3, 3];

/**
 * swatchDash paints a swatch with the same dash the canvas strokes with.
 *
 * A legend that carried only the colour could not tell apart two series that
 * differ by pattern, which on a categorical chart is half of what separates
 * them — and it is the half that survives greyscale and colour blindness.
 *
 * The dash is uPlot's own array, on/off in canvas pixels, so the legend cannot
 * describe a pattern the chart is not drawing. Each drawn run is widened by a
 * pixel: the swatch is 3px tall, and a 3px run at that height reads as a
 * square dot rather than as a dash. Returns null for a solid line, which the
 * stylesheet already draws.
 */
function swatchDash(dash) {
  if (!dash || dash.length === 0) return null;
  const stops = [];
  let at = 0;
  dash.forEach((run, i) => {
    const on = i % 2 === 0;
    const end = at + run + (on ? 1 : 0);
    const color = on ? 'var(--tint)' : 'transparent';
    stops.push(`${color} ${at}px`, `${color} ${end}px`);
    at = end;
  });
  return `repeating-linear-gradient(to right, ${stops.join(', ')})`;
}

/**
 * keyList is the legend, built here rather than taken from uPlot.
 *
 * uPlot's own legend is an unnamed <table>, which is exactly the defect the
 * last accessibility pass removed from this page. A list of terms is the right
 * shape anyway: each entry is a name and its value at the cursor.
 *
 * Returns the node and an update(values) that writes the readout. The readout
 * is pointer-driven and deliberately not a live region.
 */
export function keyList(items) {
  const values = [];
  const node = el(
    'ul',
    { class: 'keys' },
    items.map((item) => {
      const value = el('span', { class: 'keys__value num', text: '' });
      values.push(value);
      const swatch = ['keys__swatch'];
      if (item.dot) swatch.push('keys__swatch--dot');
      const fill = swatchDash(item.dashed ? DASHED : item.dash);
      return el('li', { class: 'keys__item' }, [
        el('span', {
          class: swatch.join(' '),
          'aria-hidden': true,
          // The shorthand, not background-image: the stylesheet's own
          // `background: var(--tint)` would otherwise sit behind the gradient
          // and fill in the gaps, leaving a dashed swatch looking solid.
          style: `--tint:${item.color}${fill ? `;background:${fill}` : ''}`,
        }),
        el('span', { class: 'keys__name' }, [
          document.createTextNode(item.label),
          item.sub ? el('span', { class: 'keys__sub', text: item.sub }) : null,
        ]),
        value,
      ]);
    }),
  );
  return {
    node,
    update(readouts) {
      values.forEach((span, i) => {
        span.textContent = readouts[i] === undefined || readouts[i] === null ? '' : readouts[i];
      });
    },
  };
}

/**
 * cursorReadout wires a uPlot cursor to a keyList.
 *
 * readouts(idx, plot) returns one string per key, in the keys' own order —
 * which is not always the series order: a stacked chart draws its bands back
 * to front and carries cumulative values, so the legend has to be fed from the
 * raw figures instead of from plot.data.
 *
 * Returns a hooks object to spread into the chart's opts.
 */
export function cursorReadout(keys, readouts) {
  const clear = () => keys.update([]);
  return {
    setCursor: [
      (plot) => {
        const idx = plot.cursor.idx;
        if (idx === null || idx === undefined) {
          clear();
          return;
        }
        keys.update(readouts(idx, plot));
      },
    ],
  };
}

/**
 * baseOpts is the house style: hairline grid, monospaced axes, no uPlot
 * legend, and a cursor that does not drag-zoom — these charts are read, not
 * explored, and a stray drag that rescales an axis is a trap on a touch
 * screen.
 *
 * A chart that is genuinely explored rather than read spreads zoomControl()
 * over this, which is deliberately per-panel: the trap is real, and the price
 * of escaping it is a visible control saying how to get back.
 */
export function baseOpts() {
  return {
    legend: { show: false },
    cursor: { drag: { x: false, y: false }, points: { size: 6 } },
  };
}

/* ── Zoom ─────────────────────────────────────────────────────────────── */

/** WHEEL_STEP is the share of the current range one wheel notch keeps. */
const WHEEL_STEP = 0.82;

/**
 * wheelZoom is the plugin uPlot does not ship: a wheel over the plot rescales
 * both axes about the pointer.
 *
 * The listener is non-passive, because stopping the page from scrolling out
 * from under the pointer is the whole point of it, and it is bound to `u.over`
 * — the plot's own overlay and nothing wider — so a wheel anywhere else still
 * scrolls the page. A ctrl-wheel is left alone: that is the browser's own page
 * zoom, and a chart must not be allowed to take it away.
 *
 * The new bounds are read back through posToVal rather than computed from the
 * current min and max. That is what keeps this correct on a log axis, where
 * the pixel halfway between two values is not the value halfway between them.
 */
function wheelZoom(home) {
  return {
    hooks: {
      ready: (u) => {
        u.over.addEventListener(
          'wheel',
          (e) => {
            if (e.ctrlKey) return;
            e.preventDefault();
            const rect = u.over.getBoundingClientRect();
            const keep = e.deltaY < 0 ? WHEEL_STEP : 1 / WHEEL_STEP;
            zoomAxis(u, home, 'x', e.clientX - rect.left, rect.width, keep);
            zoomAxis(u, home, 'y', e.clientY - rect.top, rect.height, keep);
          },
          { passive: false },
        );
      },
    },
  };
}

/**
 * zoomAxis scales one axis about `at`, keeping `keep` of the current extent,
 * and never past the range the chart opened at.
 *
 * The clamp is what makes the reset button's own test honest: a wheel that
 * could wander outside the data would leave the plot "zoomed" at a range no
 * reader asked for and could not read their way out of.
 */
function zoomAxis(u, home, key, at, extent, keep) {
  const [floor, ceiling] = home[key];
  const a = u.posToVal(at - at * keep, key);
  const b = u.posToVal(at + (extent - at) * keep, key);
  const min = Math.max(floor, Math.min(a, b));
  const max = Math.min(ceiling, Math.max(a, b));
  if (!(max > min)) return;
  u.setScale(key, { min, max });
}

/**
 * zoomControl is the opt-in this module's cursor deliberately does not give
 * every chart: drag to zoom, wheel to zoom, and a named way back.
 *
 * `home` is the range each scale opens at, keyed by scale — `{x: [0, 100], y:
 * [500, 20000]}`. It is supplied rather than read off the plot because reset
 * has to land exactly where the chart opened, and because mountChart rebuilds
 * the plot on a resize and has to open it there again.
 *
 * Double-click already resets a uPlot and nothing on the page says so, which
 * makes it a feature only its author can find. The button is the same action
 * with a name and a tab stop. It is absent until there is something to undo,
 * because a control that can only ever do nothing is furniture.
 *
 * Returns `{node, opts}`: the button for the figure's actions, and the opts to
 * spread into the chart after baseOpts().
 */
export function zoomControl(home) {
  const node = el('button', {
    type: 'button',
    class: 'zoom-reset',
    hidden: true,
    text: 'Reset zoom',
  });
  const scales = Object.entries(home);
  let plot = null;

  const goHome = (u) => {
    for (const [key, [min, max]] of scales) u.setScale(key, { min, max });
  };
  const sync = (u) => {
    node.hidden = scales.every(([key, [min, max]]) => u.scales[key].min === min && u.scales[key].max === max);
  };

  node.addEventListener('click', () => {
    if (plot) goHome(plot);
  });

  return {
    node,
    opts: {
      // A box drag: both scales, always. uPlot's `uni` would let a sweep
      // along one axis leave the other alone, but it only holds in one
      // direction here — a shallow horizontal sweep still crushed the y
      // scale — and a control that works one way round is worse than one
      // that always does the same thing.
      cursor: { drag: { x: true, y: true }, points: { size: 6 } },
      plugins: [
        wheelZoom(home),
        {
          hooks: {
            // The opening ranges are set here rather than left to uPlot's own
            // fit, so that "where the chart opened" is one value this module
            // owns and reset cannot drift from.
            //
            // Out of the microtask queue, not inline: `ready` fires from
            // inside uPlot's first commit, and a scale set during a commit is
            // discarded by the end of it. The queue runs before the frame is
            // painted, so the chart is never seen at the range it is leaving.
            ready: (u) => {
              plot = u;
              queueMicrotask(() => {
                if (plot !== u) return;
                goHome(u);
                sync(u);
              });
            },
            setScale: sync,
          },
        },
      ],
    },
  };
}

/** axis builds one axis in the house style. */
export function axis(t, extra = {}) {
  return {
    stroke: t.muted,
    grid: { stroke: t.rule, width: 1 },
    ticks: { stroke: t.rule, width: 1, size: 4 },
    font: t.font,
    labelFont: t.font,
    labelSize: 22,
    ...extra,
  };
}
