# Building from Source

Prefer the pre-built binaries unless you're modifying the code.

## Requirements

- **Go 1.25+**
- A terminal (macOS/Linux/WSL/PowerShell)

## Clone and build

```bash
git clone https://github.com/spinell04/Proxy-Toolbox.git
cd Proxy-Toolbox
go build -o proxytoolbox .
./proxytoolbox
```

The first build will download dependencies (`huh` for menus, `tls-client` for TLS fingerprinting, etc.) — expect it to take a minute or two.

## Cross-compiling

Go builds for any target from any host. From a Mac you can build all three in one go:

```bash
# Apple Silicon Mac
GOOS=darwin GOARCH=arm64 go build -o proxytoolbox-mac-AppleSiliconCPU .

# Intel Mac
GOOS=darwin GOARCH=amd64 go build -o proxytoolbox-macOS-IntelCPU .

# Windows
GOOS=windows GOARCH=amd64 go build -o proxytoolbox.exe .
```

No `CGO` is required, so cross-compilation works out of the box without extra toolchains.

## Running without building

During development you can use `go run`:

```bash
go run .
```

This compiles to a temp directory and runs directly. The toolbox detects this case and falls back to the current working directory for resolving `config.txt` and `proxyfiles/`.

## Project layout

```
Proxy-Toolbox/
├── main.go                    # Menu entrypoint
├── config.txt                 # Runtime config
├── proxyfiles/                # User proxy lists
├── results/                   # CSV exports
├── docs/                      # This documentation
├── internal/
│   ├── basedir/               # Resolves binary's own directory
│   ├── compare/               # CSV parsing and comparison statistics
│   ├── config/                # config.txt loader
│   ├── dashboard/             # Compare Results: HTTP server, JSON API, embedded web/
│   ├── proxy/                 # Parse + file selection
│   ├── tools/                 # The 8 menu tools other than Compare Results
│   └── util/                  # Colors, export, percentiles, error helpers
├── go.mod
└── go.sum
```

## Changing a config key

`config.txt` is **generated**, not tracked. It is written beside the binary on
first run from `internal/bootstrap/config.default.txt`, which is compiled in
with `go:embed`.

So a config change is two edits, always:

| Edit | File |
|---|---|
| Parse the key | `internal/config/config.go` — the `switch` in `Load`, plus a `Default*` constant if it has one |
| Ship the key | `internal/bootstrap/config.default.txt` — same key, same default value |

**Skipping the second is invisible in testing and broken in the field.** Your
own `config.txt` already exists and is never rewritten, so a missing template
line changes nothing locally — but every new install gets a config with no
line for that setting, and most users never read the docs to find out it
exists.

Three tests make this mechanical rather than a matter of memory:

* `TestDefaultConfig_CoversEveryKeyTheParserKnows` — fails if the parser
  understands a key the template does not mention. It reads the keys out of
  `config.go` itself rather than a hand-kept list, because a hand-kept list
  drifts exactly the way the template would.
* `TestDefaultConfig_MatchesTheBuiltInDefaults` — fails if a template value
  disagrees with its `Default*` constant, in either direction. These had
  already diverged once: `workers` shipped as 100 in the template while
  `DefaultWorkers` was 20, so the same binary ran at five times the
  concurrency depending only on whether a file happened to exist beside it.
* `TestDefaultConfig_ShipsNoCredential` — fails if any key in the template
  ships a non-empty URL or credential. The template is compiled into every
  binary you hand out.
