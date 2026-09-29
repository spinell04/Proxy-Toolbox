// The view manifest.
//
// Importing a view module runs its register() call, which is the whole
// registration mechanism. This file is the single place a new view is wired
// in; app.js imports it once and never learns any view's name.
//
// Order matters only in that views render in registration order. Timeline is
// first because a two-run same-tool selection matches both of the views below,
// and the comparability banner timeline draws has to be read before the
// paired view's deltas are.
//
// Timeline and paired are the only pair that render together. Every other
// match is exclusive of them and of each other: a selection is either one run,
// or two or more that are not all one known tool, or two or more of one known
// tool split by whether their proxy files are known to differ — head-to-head
// takes that case, timeline and paired take the rest. So although crosstool
// and headtohead also fetch /api/compare, no selection ever mounts either
// alongside timeline or paired, and the payload is never fetched twice on
// their account.

import './timeline.js'; // two or more runs of one tool, not against differing proxy files
import './paired.js'; // exactly two runs of one tool, not against differing proxy files
// Two or more runs of one tool against differing proxy files. Mutually
// exclusive with timeline and paired: those two require the proxy files not to
// be known-different, so no selection ever mounts this view beside them.
import './headtohead.js';
import './crosstool.js'; // a selection spanning more than one tool
import './detail.js'; // exactly one run
