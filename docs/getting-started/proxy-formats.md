# Proxy Formats

All tools auto-detect the input format, so you don't need to reformat files before using them. One proxy per line. Lines starting with `#` and blank lines are skipped.

## Supported formats

| Format | Example |
|--------|---------|
| `host:port:user:pass` | `1.2.3.4:8080:admin:secret` |
| `user:pass:host:port` | `admin:secret:1.2.3.4:8080` |
| `user:pass@host:port` | `admin:secret@1.2.3.4:8080` |
| `http://user:pass@host:port` | `http://admin:secret@1.2.3.4:8080` |

The **[Proxy Parser](../tools/proxy-parser.md)** tool can convert between any of these, plus strip auth to `host:port`.

## How auto-detection works

1. **If the line contains `@`** — it's parsed as a URL (with or without the `http://` prefix), using Go's standard `net/url` parser.
2. **Otherwise** — the line is split into 4 colon-separated parts. To decide between `host:port:user:pass` and `user:pass:host:port`, the parser checks the third part: if it contains a dot (like an IP or hostname) and the first part doesn't, it's treated as `user:pass:host:port`.

This means a proxy like `admin:secret:1.2.3.4:8080` is correctly read as user=`admin`, pass=`secret`, host=`1.2.3.4`, port=`8080`.

## Comments and blank lines

```
# This is a comment — ignored
1.2.3.4:8080:user:pass

# Blank lines above and below are fine
5.6.7.8:8080:user:pass
```

## Invalid lines

Lines that don't match any format are silently skipped. If you run a tool and see `No valid proxies found`, it means every line was rejected — double-check the format.
