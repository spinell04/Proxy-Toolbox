package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir lays out a results directory with one real run, and a second CSV
// one level above it that no request is allowed to reach.
func fixtureDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	results := filepath.Join(base, "results")
	if err := os.Mkdir(results, 0o755); err != nil {
		t.Fatal(err)
	}

	src, err := os.ReadFile("../compare/testdata/pinger_full.csv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(results, "pinger_run.csv"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	secret := strings.ReplaceAll(string(src), "alice:pw@1.2.3.4:8080", secretMarker+":pw@1.2.3.4:8080")
	if err := os.WriteFile(filepath.Join(base, "secret.csv"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	return results
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHandleRuns_ListsInventory(t *testing.T) {
	h := newAPI(fixtureDir(t))

	rec := do(t, h, http.MethodGet, "/api/runs")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []runInventoryItem
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	item := got[0]
	if item.File != "pinger_run.csv" {
		t.Errorf("File = %q, want %q", item.File, "pinger_run.csv")
	}
	if item.Tool != "pinger" {
		t.Errorf("Tool = %q, want %q", item.Tool, "pinger")
	}
	if item.Target != "https://google.com" || item.Workers != 100 || item.ProxyFile != "residential_de.txt" {
		t.Errorf("metadata not carried through: %+v", item)
	}
	if !item.HasMeta {
		t.Error("HasMeta = false, want true")
	}
	if item.RunAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-09-19T14:32:07Z" {
		t.Errorf("RunAt = %v, want 2026-09-19T14:32:07Z", item.RunAt)
	}
	// Total 5, OK 1, Blocked 1, Errors 3 — see compare/testdata/pinger_full.csv.
	if item.Summary.Total != 5 || item.Summary.OK != 1 || item.Summary.Blocked != 1 || item.Summary.Errors != 3 {
		t.Errorf("Summary = %+v", item.Summary)
	}
}

func TestHandleRuns_EmptyDirIsEmptyArray(t *testing.T) {
	h := newAPI(t.TempDir())

	rec := do(t, h, http.MethodGet, "/api/runs")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// "null" would make the UI branch on a missing array instead of rendering
	// an empty inventory, which is the state before a first export.
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %q, want %q", body, "[]")
	}
}

// TestSummaryJSON_CarriesLatencyCountAndP25 pins the two fields the UI needs to
// tell "no samples" apart from "all zero". LatencyCount is not OK: a result can
// succeed and still contribute no latency sample.
func TestSummaryJSON_CarriesLatencyCountAndP25(t *testing.T) {
	dir := t.TempDir()
	const csv = `Summary,Value
Tool,pinger
Run at,2026-09-19T14:32:07Z
,
#,Proxy,Latency,Status,Error
1,a:pw@1.2.3.4:8080,100ms,HTTP 200,
2,b:pw@1.2.3.5:8080,0ms,HTTP 200,
`
	if err := os.WriteFile(filepath.Join(dir, "zero.csv"), []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newAPI(dir)

	rec := do(t, h, http.MethodGet, "/api/run?file=zero.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	summary, ok := raw["summary"].(map[string]any)
	if !ok {
		t.Fatalf("no summary object in %v", raw)
	}
	for field, want := range map[string]float64{
		"ok":           2, // both rows succeeded
		"latencyCount": 1, // but only one carried a latency sample
		"p25":          100,
		"p50":          100,
	} {
		got, present := summary[field]
		if !present {
			t.Errorf("summary has no %q field", field)
			continue
		}
		if got != want {
			t.Errorf("summary[%q] = %v, want %v", field, got, want)
		}
	}
}

func TestHandleRun_ReturnsDetail(t *testing.T) {
	h := newAPI(fixtureDir(t))

	rec := do(t, h, http.MethodGet, "/api/run?file=pinger_run.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got runDetail
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Results) != 5 {
		t.Errorf("got %d results, want 5", len(got.Results))
	}
	// One sample survives the OK-and-nonzero filter: row 1 at 300ms.
	if got.Summary.LatencyCount != 1 {
		t.Errorf("LatencyCount = %d, want 1", got.Summary.LatencyCount)
	}
	if len(got.ECDF) != 1 || got.ECDF[0].Value != 300 {
		t.Errorf("ECDF = %+v, want one point at 300ms", got.ECDF)
	}
	if len(got.Histogram.Buckets) != 30 {
		t.Errorf("got %d histogram buckets, want 30", len(got.Histogram.Buckets))
	}
	wantErrors := map[string]int{"timeout": 1, "auth": 1, "eof": 1}
	if len(got.Errors) != len(wantErrors) {
		t.Errorf("Errors = %v, want %v", got.Errors, wantErrors)
	}
	for kind, want := range wantErrors {
		if got.Errors[kind] != want {
			t.Errorf("Errors[%q] = %d, want %d", kind, got.Errors[kind], want)
		}
	}
}

func TestHandleRun_MissingFileIsNotFound(t *testing.T) {
	h := newAPI(fixtureDir(t))

	rec := do(t, h, http.MethodGet, "/api/run?file=nope.csv")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleCompare_JoinsRequestedRuns(t *testing.T) {
	h := newAPI(fixtureDir(t))

	rec := do(t, h, http.MethodGet, "/api/compare?file=pinger_run.csv&file=pinger_run.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got compareResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Join.Overlap != 5 {
		t.Errorf("Overlap = %d, want 5 (a run joined with itself)", got.Join.Overlap)
	}
	if len(got.Join.Rows) != 5 {
		t.Errorf("got %d join rows, want 5", len(got.Join.Rows))
	}
	if len(got.Join.Runs) != 2 {
		t.Errorf("Join.Runs = %v, want two entries", got.Join.Runs)
	}
	if len(got.Runs) != 2 {
		t.Fatalf("got %d run overviews, want 2", len(got.Runs))
	}
	for i, r := range got.Runs {
		if r.File != "pinger_run.csv" || r.Summary.Total != 5 {
			t.Errorf("Runs[%d] = %s with summary %+v", i, r.File, r.Summary)
		}
		if len(r.ECDF) != 1 || len(r.Histogram.Buckets) != histogramBuckets {
			t.Errorf("Runs[%d] is missing the distributions the charts need", i)
		}
	}
}

// TestHandleCompare_OmitsPerProxyRows pins the shape of the compare response
// against the duplication it was trimmed of: Join.Rows already carries every
// proxy's latency, status, outcome, error kind and exit IP for every selected
// run, so a per-run Results slice would encode the same facts a second time.
// Drill-down is /api/run's job.
func TestHandleCompare_OmitsPerProxyRows(t *testing.T) {
	h := newAPI(fixtureDir(t))

	rec := do(t, h, http.MethodGet, "/api/compare?file=pinger_run.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var raw struct {
		Runs []map[string]any `json:"runs"`
		Join struct {
			Rows []map[string]any `json:"rows"`
		} `json:"join"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(raw.Runs))
	}

	if _, present := raw.Runs[0]["results"]; present {
		t.Error(`compare response carries a per-run "results" key; Join.Rows is the row-level source`)
	}
	// The overview fields the charts read must still be there, so the check
	// above cannot be satisfied by returning less than the views need.
	for _, field := range []string{"summary", "histogram", "ecdf", "errors", "codes", "success", "file", "runAt"} {
		if _, present := raw.Runs[0][field]; !present {
			t.Errorf("compare response run is missing %q", field)
		}
	}
	// And the row-level truth is still reachable, in exactly one place.
	if len(raw.Join.Rows) != 5 {
		t.Errorf("got %d join rows, want 5", len(raw.Join.Rows))
	}
}

// TestHandleCompare_MissingFileIsLoggedNotReflected pins both halves of the
// error handling: the operator gets the name in the terminal, the client gets a
// fixed string with nothing of its own input echoed back.
func TestHandleCompare_MissingFileIsLoggedNotReflected(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	h := newAPI(fixtureDir(t))

	rec := do(t, h, http.MethodGet, "/api/compare?file=ghostrun.csv")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "ghostrun") {
		t.Errorf("response body reflects the requested name back: %q", rec.Body.String())
	}
	if !strings.Contains(logged.String(), "ghostrun.csv") {
		t.Errorf("log %q does not name the file that failed to load", logged.String())
	}
}

func TestHandleCompare_RejectsTooManyFiles(t *testing.T) {
	h := newAPI(fixtureDir(t))

	one := "file=pinger_run.csv"
	atLimit := strings.Repeat("&"+one, maxCompareFiles-1)
	overLimit := strings.Repeat("&"+one, maxCompareFiles)

	t.Run("at the limit", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/compare?"+one+atLimit)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 — %d files is within the cap", rec.Code, maxCompareFiles)
		}
	})

	t.Run("one over the limit", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/compare?"+one+overLimit)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 — %d files exceeds the cap", rec.Code, maxCompareFiles+1)
		}
	})
}

func TestHandleCompare_NoSelectionIsBadRequest(t *testing.T) {
	h := newAPI(fixtureDir(t))

	rec := do(t, h, http.MethodGet, "/api/compare")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestAPI_DegenerateRunsDoNotPanic covers the shapes a hand-edited or
// pre-metadata export can take.
func TestAPI_DegenerateRunsDoNotPanic(t *testing.T) {
	dir := t.TempDir()
	legacy, err := os.ReadFile("../compare/testdata/legacy_no_meta.csv")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"empty.csv":      "",
		"metaonly.csv":   "Summary,Value\nTool,pinger\nRun at,2026-09-19T14:32:07Z\n",
		"headeronly.csv": "Summary,Value\nTool,pinger\n,\n#,Proxy,Latency,Status,Error\n",
		"legacy.csv":     string(legacy),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := newAPI(dir)

	rec := do(t, h, http.MethodGet, "/api/runs")
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/runs status = %d, want 200", rec.Code)
	}
	var items []runInventoryItem
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != len(files) {
		t.Errorf("got %d items, want %d", len(items), len(files))
	}

	for name := range files {
		t.Run(name, func(t *testing.T) {
			if rec := do(t, h, http.MethodGet, "/api/run?file="+name); rec.Code != http.StatusOK {
				t.Errorf("/api/run status = %d, want 200", rec.Code)
			}
			if rec := do(t, h, http.MethodGet, "/api/compare?file="+name); rec.Code != http.StatusOK {
				t.Errorf("/api/compare status = %d, want 200", rec.Code)
			}
		})
	}
}

// writeRun drops a single CSV into a fresh results directory and returns the
// directory, for tests that need an outcome mix the shared fixture does not have.
func writeRun(t *testing.T, name, csv string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// mixedOutcomesCSV is 8 proxies: 5 working, 1 blocked with a 403, 2 timing out.
// The three outcome counts are deliberately all different, so a success interval
// computed from the wrong field cannot coincide with the right answer.
const mixedOutcomesCSV = `Summary,Value
Tool,pinger
Run at,2026-09-19T14:32:07Z
Proxy file,mixed.txt
Target,https://google.com
Workers,10
,
#,Proxy,Latency,Status,Error
1,a:pw@1.2.3.1:8080,100ms,HTTP 200,
2,b:pw@1.2.3.2:8080,110ms,HTTP 200,
3,c:pw@1.2.3.3:8080,120ms,HTTP 200,
4,d:pw@1.2.3.4:8080,130ms,HTTP 200,
5,e:pw@1.2.3.5:8080,140ms,HTTP 200,
6,f:pw@1.2.3.6:8080,,HTTP 403,
7,g:pw@1.2.3.7:8080,,ERROR,dial tcp 1.2.3.7:8080: i/o timeout
8,h:pw@1.2.3.8:8080,,ERROR,dial tcp 1.2.3.8:8080: i/o timeout
`

// TestHandleCompare_CarriesCodeBreakdown pins the per-run failure codes the
// head-to-head view groups on. Status wins over kind, so the blocked row files
// under "403" and not under a transport bucket, and the ordering is by count.
func TestHandleCompare_CarriesCodeBreakdown(t *testing.T) {
	h := newAPI(writeRun(t, "mixed.csv", mixedOutcomesCSV))

	rec := do(t, h, http.MethodGet, "/api/compare?file=mixed.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got compareResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(got.Runs))
	}
	codes := got.Runs[0].Codes
	if len(codes) != 2 {
		t.Fatalf("Codes = %+v, want two groups (timeout, 403)", codes)
	}
	if codes[0].Code != "timeout" || codes[0].Count != 2 {
		t.Errorf("Codes[0] = %+v, want timeout x2 first (descending count)", codes[0])
	}
	if codes[1].Code != "403" || codes[1].Count != 1 {
		t.Errorf("Codes[1] = %+v, want 403 x1", codes[1])
	}
	wantProxies := []string{"g:pw@1.2.3.7:8080", "h:pw@1.2.3.8:8080"}
	if len(codes[0].Proxies) != len(wantProxies) {
		t.Fatalf("Codes[0].Proxies = %v, want %v", codes[0].Proxies, wantProxies)
	}
	for i, want := range wantProxies {
		if codes[0].Proxies[i] != want {
			t.Errorf("Codes[0].Proxies[%d] = %q, want %q", i, codes[0].Proxies[i], want)
		}
	}
}

// TestHandleCompare_CarriesSuccessInterval pins the Wilson bounds, not just the
// rate. The expected numbers are the closed form evaluated by hand for k=5, n=8,
// so the test fails if the handler feeds WilsonInterval the wrong counts or the
// interval stops being a Wilson one.
func TestHandleCompare_CarriesSuccessInterval(t *testing.T) {
	h := newAPI(writeRun(t, "mixed.csv", mixedOutcomesCSV))

	rec := do(t, h, http.MethodGet, "/api/compare?file=mixed.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got compareResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(got.Runs))
	}
	success := got.Runs[0].Success
	const tolerance = 0.0005
	for _, c := range []struct {
		field string
		got   float64
		want  float64
	}{
		{"rate", success.Rate, 0.625}, // 5 OK of 8 tested
		{"low", success.Low, 0.3057},
		{"high", success.High, 0.8632},
	} {
		if diff := c.got - c.want; diff > tolerance || diff < -tolerance {
			t.Errorf("success.%s = %v, want %v", c.field, c.got, c.want)
		}
	}
}

// TestHandleCompare_FullSuccessKeepsFiniteLowerBound covers the case the whole
// interval exists for: providers that tie at 100%. A normal approximation would
// report 1.0 to 1.0 here and assert a certainty eight trials do not support.
func TestHandleCompare_FullSuccessKeepsFiniteLowerBound(t *testing.T) {
	const csv = `Summary,Value
Tool,pinger
Run at,2026-09-19T14:32:07Z
,
#,Proxy,Latency,Status,Error
1,a:pw@1.2.3.1:8080,100ms,HTTP 200,
2,b:pw@1.2.3.2:8080,110ms,HTTP 200,
3,c:pw@1.2.3.3:8080,120ms,HTTP 200,
`
	h := newAPI(writeRun(t, "perfect.csv", csv))

	rec := do(t, h, http.MethodGet, "/api/compare?file=perfect.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.Bytes()
	var got compareResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(got.Runs))
	}
	success := got.Runs[0].Success
	if success.Rate != 1 || success.High != 1 {
		t.Errorf("success = %+v, want rate and high both exactly 1", success)
	}
	if success.Low >= 1 {
		t.Errorf("success.Low = %v, want strictly below 1 — three trials do not prove 100%%", success.Low)
	}
	if success.Low <= 0 {
		t.Errorf("success.Low = %v, want above 0", success.Low)
	}

	// A run with no failures must ship an empty JSON array. The head-to-head
	// view maps over codes directly, and a nil slice marshals to null, which is
	// a crash there — a check on the decoded Go value cannot tell the two apart.
	if !bytes.Contains(body, []byte(`"codes":[]`)) {
		t.Errorf(`payload does not contain "codes":[]; it has %q`, codesFragment(body))
	}
}

// codesFragment pulls the "codes" key and a little of what follows out of a
// payload, so a failure names the shape that was shipped without printing the
// entire compare response.
func codesFragment(body []byte) string {
	i := bytes.Index(body, []byte(`"codes":`))
	if i < 0 {
		return "no codes key at all"
	}
	end := i + 32
	if end > len(body) {
		end = len(body)
	}
	return string(body[i:end])
}

// ratesCSV builds a run of total proxies of which ok succeed and the rest time
// out, so a test can pin an exact (ok, total) pair without depending on a real
// export. The latencies vary so the distributions stay non-degenerate.
func ratesCSV(ok, total int) string {
	var b strings.Builder
	b.WriteString("Summary,Value\nTool,pinger\nRun at,2026-09-19T14:32:07Z\n,\n#,Proxy,Latency,Status,Error\n")
	for i := 1; i <= total; i++ {
		proxy := fmt.Sprintf("u%d:pw@10.0.%d.%d:8080", i, i/256, i%256)
		if i <= ok {
			fmt.Fprintf(&b, "%d,%s,%dms,HTTP 200,\n", i, proxy, 100+i)
			continue
		}
		fmt.Fprintf(&b, "%d,%s,,ERROR,dial tcp %d: i/o timeout\n", i, proxy, i)
	}
	return b.String()
}

// writeRuns drops several CSVs into one fresh results directory, for the
// multi-provider selections the head-to-head view is built on.
func writeRuns(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, csv := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(csv), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestHandleCompare_PairwiseCoversEveryPairInResponseOrder pins the contract the
// frontend indexes on: one entry per unordered pair, i < j over the runs in the
// order they appear in the response. Three distinct rates keep an entry from
// matching the wrong pair by coincidence, and the three file names are distinct
// so a swapped A/B is visible.
func TestHandleCompare_PairwiseCoversEveryPairInResponseOrder(t *testing.T) {
	dir := writeRuns(t, map[string]string{
		"alpha.csv": ratesCSV(10, 10),
		"beta.csv":  ratesCSV(8, 10),
		"gamma.csv": ratesCSV(6, 10),
	})
	h := newAPI(dir)

	rec := do(t, h, http.MethodGet, "/api/compare?file=alpha.csv&file=beta.csv&file=gamma.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got compareResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Runs) != 3 {
		t.Fatalf("got %d runs, want 3", len(got.Runs))
	}
	// Three runs make exactly three unordered pairs. Six would mean the
	// implementation emits ordered pairs and the UI would draw each gap twice.
	if len(got.Pairwise) != 3 {
		t.Fatalf("got %d pairs, want 3: %+v", len(got.Pairwise), got.Pairwise)
	}
	want := [][2]string{
		{"alpha.csv", "beta.csv"},
		{"alpha.csv", "gamma.csv"},
		{"beta.csv", "gamma.csv"},
	}
	for i, w := range want {
		if got.Pairwise[i].A != w[0] || got.Pairwise[i].B != w[1] {
			t.Errorf("Pairwise[%d] = (%q, %q), want (%q, %q)", i, got.Pairwise[i].A, got.Pairwise[i].B, w[0], w[1])
		}
	}
}

// TestHandleCompare_PairwiseSeparatesTheReferenceProviders is the case the whole
// statistic exists for. schro is 100 working of 100 and mobile is 93 of 100; the
// two Wilson intervals overlap in [0.9630, 0.9657], so the interval-overlap rule
// this replaced called the gap unreal. Fisher's exact test on the same table
// gives two-sided p = 0.014018 — computed independently in exact rationals, not
// by calling compare.FisherExact — so the gap is real and the worse provider is
// correctly rejected.
//
// The counts come from results/schro.csv and results/mobile.csv, reproduced as a
// fixture rather than read from results/: that directory is gitignored and its
// rows carry live proxy credentials, so a test reading it would fail on any
// clean checkout.
func TestHandleCompare_PairwiseSeparatesTheReferenceProviders(t *testing.T) {
	dir := writeRuns(t, map[string]string{
		"schro.csv":  ratesCSV(100, 100),
		"mobile.csv": ratesCSV(93, 100),
	})
	h := newAPI(dir)

	rec := do(t, h, http.MethodGet, "/api/compare?file=schro.csv&file=mobile.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got compareResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Pairwise) != 1 {
		t.Fatalf("got %d pairs, want 1: %+v", len(got.Pairwise), got.Pairwise)
	}
	pair := got.Pairwise[0]
	if pair.A != "schro.csv" || pair.B != "mobile.csv" {
		t.Errorf("pair = (%q, %q), want (schro.csv, mobile.csv)", pair.A, pair.B)
	}
	// Three decimals: 0.0140. Feeding the test the totals instead of the OK
	// counts makes this 1, and either count off by one moves it past 0.0005.
	if diff := pair.P - 0.014018; diff > 0.0005 || diff < -0.0005 {
		t.Errorf("p = %v, want 0.0140", pair.P)
	}
	if !pair.Distinguishable {
		t.Error("distinguishable = false, want true — p = 0.014 is below the 5% level")
	}
}

// TestHandleCompare_PairwiseCallsIdenticalRatesIndistinguishable is the other
// end: two providers with the same counts have no gap at all, so no amount of
// data could separate them and p is exactly 1.
func TestHandleCompare_PairwiseCallsIdenticalRatesIndistinguishable(t *testing.T) {
	dir := writeRuns(t, map[string]string{
		"left.csv":  ratesCSV(93, 100),
		"right.csv": ratesCSV(93, 100),
	})
	h := newAPI(dir)

	rec := do(t, h, http.MethodGet, "/api/compare?file=left.csv&file=right.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got compareResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Pairwise) != 1 {
		t.Fatalf("got %d pairs, want 1: %+v", len(got.Pairwise), got.Pairwise)
	}
	if got.Pairwise[0].P != 1 {
		t.Errorf("p = %v, want exactly 1", got.Pairwise[0].P)
	}
	if got.Pairwise[0].Distinguishable {
		t.Error("distinguishable = true, want false — the two rates are identical")
	}
}

// TestHandleCompare_PairwiseIsEmptyArrayForOneRun asserts on the marshalled
// bytes on purpose. A nil slice and an empty slice are both len() == 0 in Go, so
// decoding into compareResponse and checking the length would pass either way —
// but nil marshals to null, and the frontend maps over this field, so null is a
// crash on the single-run selection that is the dashboard's default state.
func TestHandleCompare_PairwiseIsEmptyArrayForOneRun(t *testing.T) {
	h := newAPI(writeRun(t, "solo.csv", ratesCSV(9, 10)))

	rec := do(t, h, http.MethodGet, "/api/compare?file=solo.csv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"pairwise":[]`) {
		t.Errorf("body does not carry `\"pairwise\":[]`; it has %q", pairwiseFragment(body))
	}
}

// pairwiseFragment pulls the pairwise field out of a response body for a failure
// message, so the assertion above does not print a whole compare payload.
func pairwiseFragment(body string) string {
	i := strings.Index(body, `"pairwise":`)
	if i < 0 {
		return "no pairwise field at all"
	}
	if end := i + 40; end < len(body) {
		return body[i:end]
	}
	return body[i:]
}
