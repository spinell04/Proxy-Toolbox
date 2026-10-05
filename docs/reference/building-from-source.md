# Building from Source

Prefer the [pre-built binaries](https://github.com/spinell04/Proxy-Toolbox/releases/latest) unless you're modifying the code — they keep themselves up to date, and a local build does not.

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

## A local build reports `dev` and never updates itself

The version in the menu title comes from a link-time flag that only the release workflow sets. Any build that skips the workflow — `go build`, `go run .`, an IDE — reports `dev`:

```
Proxy Toolbox dev
```

A `dev` build is refused by the updater outright. That is the point: without the rule, `go run .` in a checkout would download the published release over your own working copy, at exactly the moment you least want it and with no obvious cause. See [Auto-Update](auto-update.md).

The flip side is that a local build stays whatever you built until you build again. It will not quietly become the release.

## Reproducing a release build

The release workflow builds with these flags, and nothing else:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=v1.0.0" -o proxytoolbox .
```

| Flag | Why |
|---|---|
| `-X main.version=v1.0.0` | Stamps the version. This is what makes the binary a release rather than a `dev` build, so a binary built with it **will** auto-update. |
| `-s -w` | Drops the symbol table and DWARF. Worth a few MB on a file every user downloads; release binaries land at roughly 12.5–13.4 MB. |
| `-trimpath` | Strips absolute source paths out of the binary. |
| `CGO_ENABLED=0` | Matches the workflow, and keeps cross-compilation toolchain-free. |

`-trimpath` is the one that matters beyond size. Without it the build embeds the absolute path of whatever directory it ran in, so the same commit built on two machines produces two different files and two different digests. With it, two builds of the same target from the same source are **bit-identical**.

That is what makes a release verifiable after the fact. Check out an old tag, rebuild it with the command above substituting that tag, hash the result, and you get the digest that release's `SHA256SUMS` published:

```bash
git checkout v1.0.0
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath \
  -ldflags "-s -w -X main.version=v1.0.0" \
  -o proxytoolbox-mac-AppleSiliconCPU .
shasum -a 256 proxytoolbox-mac-AppleSiliconCPU
```

Drop `-trimpath` and the digest changes, so the reproduction stops proving anything. The same `SHA256SUMS` is what the updater checks a download against before it replaces the running binary.

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
│   ├── update/                # Startup self-update: check, verify, swap, re-exec
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
