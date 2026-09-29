package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"proxytoolbox/internal/basedir"
)

//go:embed web
var webFS embed.FS

// resultsDirName is the exported-CSV directory, resolved against the project
// root. Named distinctly from the resultsDir parameters below so neither
// shadows the other.
const resultsDirName = "results"

// loopbackAddr is the only address the dashboard ever listens on. It is
// spelled out rather than left as ":0" deliberately: this is a local tool and
// the CSVs it serves contain proxy credentials in plaintext, so the listener
// must not be reachable from the network. The zero port asks the OS for a free
// one, and the listener reports the real address it got.
const loopbackAddr = "127.0.0.1:0"

// listen opens the dashboard's listener. It exists as its own function so the
// bound address can be asserted in a test.
func listen() (net.Listener, error) {
	return net.Listen("tcp", loopbackAddr)
}

// handler builds the dashboard's routing tree: the JSON API under /api/, and
// the embedded UI everywhere else. The only thing served off disk is what the
// API reads out of resultsDir; the static half comes from the binary.
func handler(resultsDir string) (http.Handler, error) {
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", newAPI(resultsDir))
	mux.Handle("/", http.FileServer(http.FS(static)))
	// Wrapped at the outermost level so the static half is covered too: the
	// HTML is what a rebinding attack loads first, and it is what then issues
	// the API calls.
	return loopbackHostOnly(mux), nil
}

// loopbackHostOnly refuses any request whose Host header is not a loopback
// literal.
//
// Binding to 127.0.0.1 stops packets from the network, but not DNS rebinding:
// a page the user is already viewing can point a hostname it controls at
// 127.0.0.1, at which point the browser treats this server as that page's own
// origin and CORS offers nothing — the attacker's origin is the origin. The
// random port raises the cost but is scannable. What makes it worth refusing
// here is the payload: these responses carry user:pass@host:port for every
// proxy the user owns.
//
// A rebound request still arrives carrying the attacker's hostname in Host, so
// checking it is what actually distinguishes the two cases.
func loopbackHostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost accepts the forms a browser sends for a local server:
// "127.0.0.1:port", "[::1]:port" and "localhost:port", with or without the
// port. An absent or malformed Host is refused.
func isLoopbackHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(name, "[]"))
	return ip != nil && ip.IsLoopback()
}

// readHeaderTimeout bounds how long a client may take to send its headers.
const readHeaderTimeout = 5 * time.Second

// Serve starts the compare dashboard on a random loopback port, opens the
// browser, and blocks until the user presses Enter.
func Serve(waitForEnter func()) error {
	ln, err := listen()
	if err != nil {
		return fmt.Errorf("cannot start local server: %w", err)
	}

	h, err := handler(basedir.Path(resultsDirName))
	if err != nil {
		_ = ln.Close()
		return err
	}

	srv := &http.Server{Handler: h, ReadHeaderTimeout: readHeaderTimeout}
	go func() { _ = srv.Serve(ln) }()

	url := "http://" + ln.Addr().String()
	fmt.Printf("\nCompare dashboard running at %s\n", url)
	openBrowser(url)
	fmt.Print("Press Enter to stop the dashboard...")
	waitForEnter()

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(ctx)
}

// shutdownTimeout bounds the wait for in-flight requests to finish.
const shutdownTimeout = 3 * time.Second

// openBrowser makes a best effort to open the dashboard. A failure is not an
// error — the URL is already printed.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, url)...).Start()
}
