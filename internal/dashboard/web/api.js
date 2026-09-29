// Thin client over the dashboard's read-only JSON API.
//
// Every URL here is relative, and must stay that way. The server refuses any
// request whose Host header is not a loopback literal, which is what stops a
// page the user is already viewing from pointing a hostname it controls at
// 127.0.0.1 and reading proxy credentials out of this dashboard. An absolute
// URL, or a dev server proxying from another hostname, gets a 403 rather than
// silently working.

/**
 * MAX_COMPARE_FILES mirrors maxCompareFiles in api.go. The picker stops the
 * user here so a selection never turns into a 400.
 */
export const MAX_COMPARE_FILES = 50;

/** api issues a GET and returns the decoded body, or throws with the status. */
async function api(path, params) {
  const query = params ? `?${params.toString()}` : '';
  let response;
  try {
    response = await fetch(path + query);
  } catch (cause) {
    // A network-level failure means the server went away — the user closed the
    // menu entry, most likely. Say that rather than leaking a DOMException.
    throw new Error(`cannot reach the dashboard server (${path})`, { cause });
  }
  if (!response.ok) {
    throw new Error(`${path} failed: ${response.status} ${response.statusText}`.trim());
  }
  return response.json();
}

/** fetchRuns returns the inventory. The server sends [] — never null — when empty. */
export function fetchRuns() {
  return api('/api/runs');
}

/** fetchRun returns one run's detail, per-proxy rows included. */
export function fetchRun(file) {
  return api('/api/run', new URLSearchParams({ file }));
}

/**
 * fetchCompare returns {runs, join} for a selection. Entries in `runs` carry
 * chart data but no per-proxy rows: join.rows[].cells[i] is the single
 * row-level source, indexed by join.runs[i].
 */
export function fetchCompare(files) {
  const params = new URLSearchParams();
  for (const file of files.slice(0, MAX_COMPARE_FILES)) params.append('file', file);
  return api('/api/compare', params);
}
