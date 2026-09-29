// What a provider is called, everywhere in the head-to-head view.
//
// The whole view is a matrix of providers against metrics, spread over five
// panels. A provider that is "mobile" in one panel and "mobile.txt" in
// the distribution key is two providers to a reader scanning columns, so the
// name is derived once, here, and every panel is handed this function rather
// than deriving its own.
//
// The proxy file is the identity: it is the thing that differs between the
// runs this view matches, and it is what the user typed when they bought the
// list. The CSV's own name is the fallback and not the default — exports are
// named after the run, so two runs of one provider carry two file names and
// would split one column into two.

/**
 * providerName names the provider a run tested.
 *
 * @param {{proxyFile?: string, file?: string}} run
 * @returns {string}
 */
export function providerName(run) {
  const source = (run && run.proxyFile) || '';
  if (!source) return (run && run.file) || 'unknown';
  // Only a real extension is stripped. A provider list called "de.residential"
  // has no extension, and cutting at the last dot would rename it "de".
  return source.replace(/\.[A-Za-z0-9]{1,8}$/, '') || source;
}
