# Auto-Update

The toolbox is a single bare binary dropped into a folder. There is no installer, no package manager and nothing else on the machine that could keep it current, so it keeps itself current: on every launch it checks GitHub for a newer release and installs it.

That is unattended binary replacement, which sets the standard the whole thing is written to — **a failed update must leave the working binary in place, and must never stop the toolbox from starting.**

## What happens at launch

Before the first menu draws, in this order:

1. Delete `<binary>.old` if one is there, left by the previous update.
2. Stop here if auto-update is off, if this is a `dev` build, or if the platform has no published asset. Nothing is printed — these are steady states, and a line about them on every launch is noise.
3. `GET api.github.com/repos/spinell04/Proxy-Toolbox/releases/latest`, with a **3-second** timeout. The budget is short because it is a budget for someone else's availability.
4. If the latest tag is not newer than this binary's version, stop. Nothing is printed.
5. Download this platform's asset to `<binary>.new`. Timeout: **5 minutes**.
6. Verify its SHA-256 against the release's `SHA256SUMS` asset.
7. Make it executable, rename the running binary to `<binary>.old`, move `.new` into its place.
8. Re-exec. Same PID, same terminal, same arguments.

An update you can see:

```
[update] v1.0.1 -> v1.0.2, downloading...
[update] installed v1.0.2, restarting...
```

The whole thing is two lines and a few seconds. There is no prompt — the toolbox does not ask, because an unattended updater that asks is an updater that gets dismissed and never runs.

A failure is one line, and then the menu:

```
[update] check failed: GitHub API returned 404
```
```
[update] failed, staying on v1.0.1: checksum mismatch: downloaded 3f2a…, release says 9c41…
```

Offline, that first line arrives after the 3-second timeout and the toolbox starts normally. A repository with no published releases prints the 404 line and starts normally. There is no failure mode here that costs you more than one line and a few seconds.

## Your files are not touched

The update replaces **one file**: the binary itself.

| | |
|---|---|
| `config.txt` | Never overwritten and never migrated. The updater reads one key from it, `auto_update`, and writes nothing. Your settings survive every upgrade. |
| `proxyfiles/` | Untouched. |
| `results/` | Untouched. Monitor logs stay append-only across versions. |

A new config key added in a later version does **not** appear in your existing `config.txt` — the file is only ever written when it is missing. The key takes its built-in default until you add the line yourself. See [Configuration](../getting-started/configuration.md).

## The checksum is verified before the running binary is touched

This is the security property, and it is the reason the step order above is the design rather than an implementation detail.

The download lands at `<binary>.new`. It is hashed and compared against the digest the release's `SHA256SUMS` asset publishes for that exact filename. **Only after that comparison passes** does anything rename the binary you are running.

So a truncated transfer, a connection that died halfway, a proxy that injected something, or a release asset that does not match its own published digest all fail in the same place: before the swap. The original binary is still there, still working, and the only trace is a `.new` file that is removed on the way out.

**A release with no `SHA256SUMS` asset is refused, not trusted.** An unverifiable binary is not installed, and the cost of refusing is staying on a version that works.

The check runs unauthenticated against a public repository, which means no token is compiled into a binary you hand to someone else.

## The `.old` file

After the swap there is a `<binary>.old` beside the binary: the exact previous version, ~13 MB.

**It is not a rollback you can reach, and you should not plan around it.** Cleanup runs at startup, before anything else — and an update re-execs immediately, so the new binary's first act is deleting the `.old` it was just handed. By the time you see the new menu the file is already gone. It exists for a few milliseconds in the normal case.

What it is actually for is the case that matters most: **a new binary that cannot start.** If the re-exec fails, or the new version dies before reaching its own cleanup, the `.old` survives — because nothing got far enough to remove it. That is precisely when you need the previous version back, and it is sitting right there:

1. Delete the new binary.
2. Rename `<binary>.old` back to `<binary>` (on macOS: `mv proxytoolbox-mac-AppleSiliconCPU.old proxytoolbox-mac-AppleSiliconCPU && chmod +x proxytoolbox-mac-AppleSiliconCPU`).
3. Set `auto_update=off` in `config.txt` **before** relaunching, or the next launch reinstalls the version that would not start.

For a version that starts but misbehaves — the ordinary "this release is bad on my machine" case — `.old` will be gone. Roll back from the releases page instead: see [Every release is kept](#every-release-is-kept) below. That is the supported route, and it works at any distance, not just one launch back.

## `auto_update=off`

```
auto_update=on
```

`on`, `true`, `yes` and `1` all enable it; `off`, `false`, `no` and `0` all disable it; case does not matter. **Anything unrecognised takes the default, which is `on`** — a typo must not silently turn updates off, because a silently un-updated binary looks identical to an updated one until something goes wrong.

Two situations the key exists for:

- **A release that is broken on your machine.** Without the switch, every launch would reinstall it. Roll back, turn updates off, and you are pinned until you turn them back on.
- **A box running a monitor for days.** The [Downtime Monitor](../tools/proxy-monitor.md) and [Session Monitor](../tools/session-monitor.md) are meant to run for a long time, and a restart loses the in-memory history the `Ctrl+C` summary is built from. An update restarts the process — that must not happen underneath a run you care about.

The key is read before `config.txt` is created on a first ever run, and an absent or unreadable file yields the default. Whether the toolbox starts never hinges on reading an optional setting.

## A local build never updates itself

The version comes from a link-time flag that only the release workflow sets:

```
-ldflags "-X main.version=v1.0.2"
```

`go run .`, `go build`, and any IDE build report version `dev`, and a `dev` build is refused by the updater outright. Without that rule, `go run .` in a checkout would download the published release over the developer's own working copy — precisely when that is least wanted and with no obvious cause.

The menu title tells you which you have:

```
Proxy Toolbox v1.0.2      <- a release download, updates itself
Proxy Toolbox dev         <- locally built, never updates
```

See [Building from Source](building-from-source.md) for how to stamp a real version on a local build, and how to reproduce a release build exactly.

## No published asset, no update

Three assets are published per release:

| Platform | Asset |
|---|---|
| Apple Silicon | `proxytoolbox-mac-AppleSiliconCPU` |
| Intel Mac | `proxytoolbox-macOS-IntelCPU` |
| Windows (x64) | `proxytoolbox.exe` |

Anything else — Linux, Windows on ARM — has no asset, so the updater disables itself there silently. Guessing would be worse: downloading the `.exe` onto a Mac replaces a working binary with one that cannot run, after the original has already been renamed aside.

The asset names are deliberately the same names the [installation page](../getting-started/installation.md) tells you to download by hand, so a manual download and an auto-update produce the same file and "which version is this?" has one answer.

## Only the launched copy updates

The update replaces the binary **at its own path**. A second copy in another folder is a different file and knows nothing about the first; it updates on its own next launch.

That matters if you keep per-client or per-project folders. They do not drift silently — each one catches up the first time you run it — but at any given moment two folders can be on two versions. The menu title is the answer to which.

## Rate limiting

The check is one unauthenticated GitHub API request per launch, and unauthenticated GitHub allows **60 requests per hour per IP**.

That is sixty launches an hour from one address, which no single user reaches. It is reachable on a shared or NATed address, and it is reachable from an office. Hitting it is harmless:

```
[update] check failed: GitHub API rate limit reached, no update check this run
```

No check that run, the toolbox starts normally, and the next launch after the window rolls tries again. It is named distinctly rather than reported as a bare `403` so it does not read as a permissions problem with a public repository.

## Every release is kept

Releases are **never pruned**. Every version ever published stays downloadable at [github.com/spinell04/Proxy-Toolbox/releases](https://github.com/spinell04/Proxy-Toolbox/releases), with its binaries and its `SHA256SUMS`.

So the rollback story does not depend on catching the `.old` file in time. Download any older version by hand, drop it in the folder, and set `auto_update=off` so the next launch does not pull you forward again.

## If an update fails

Nothing. Do nothing.

The running binary was never touched, the `.new` file has already been removed, and the next launch tries again. A failure here is not a state you have to clean up or get out of — it is one line of output and a check that will happen again.

The one message worth acting on is the one that says the new binary is installed but could not be started:

```
[update] v1.0.2 is installed but could not be started: ...
[update] restart the toolbox manually to use it.
```

The swap succeeded, so launching the toolbox again runs the new version.

For the specific failure lines and what each means, see [Troubleshooting](../troubleshooting.md).

## Configuration summary

| Key | Purpose | Default |
|-----|---------|---------|
| `auto_update` | Check GitHub on startup, install a newer release and restart | `on` |

See [Configuration](../getting-started/configuration.md) for the full file.
