# Proxy Toolbox

A CLI toolbox for testing, analyzing and managing proxy lists. One binary, arrow-key menus, no installer.

Drop it in a folder and run it — `proxyfiles/`, `results/` and `config.txt` appear beside it on first launch, and it keeps itself up to date from then on.

## Start here

| If you want to | Go to |
|---|---|
| Get it running | [Installation](getting-started/installation.md) |
| Know what each tool does | [Tools](tools/) |
| Change workers, intervals, webhooks | [Configuration](getting-started/configuration.md) |
| Understand a CSV you exported | [Exporting Results](reference/exporting-results.md) |
| Stop it updating itself | [Auto-Update](reference/auto-update.md) |
| Fix something that went wrong | [Troubleshooting](troubleshooting.md) |

## The tools, in menu order

Nine tools. **Site Request Test** and **Monitor** are submenus of two each.

| Tool | What it answers |
|---|---|
| [IP Uniqueness Test](tools/ip-uniqueness-test.md) | What exit IP does each proxy have, and are any duplicated? |
| [Site Request Test](tools/tm-request-tester.md) | Can each proxy actually load a real page, and how fast? |
| [Monitor](tools/proxy-monitor.md) | Is the pool still up — and is each proxy still on the same IP? |
| [Ping Test](tools/ping-test.md) | What is the latency to a domain through each proxy? |
| [Randomize File](tools/randomize-file.md) | — shuffles the order of a proxy file |
| [Proxy Parser](tools/proxy-parser.md) | — converts between proxy formats |
| [Compare Results](tools/compare-results.md) | How do two providers compare, across every exported run? |

## Two things worth knowing early

**Your settings survive upgrades.** `config.txt` is written once, on first run, and never overwritten or migrated. A key added in a later version takes its built-in default until you add the line yourself.

**The toolbox updates itself, silently.** Every launch checks GitHub for a newer release and installs it. The checksum is verified before the running binary is touched, and a failed check costs one line of output and a few seconds. Set `auto_update=off` to pin a version — see [Auto-Update](reference/auto-update.md).
