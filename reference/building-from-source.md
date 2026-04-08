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
│   ├── config/                # config.txt loader
│   ├── proxy/                 # Parse + file selection
│   ├── tools/                 # The 5 tools
│   └── util/                  # Colors, export, error helpers
├── go.mod
└── go.sum
```
