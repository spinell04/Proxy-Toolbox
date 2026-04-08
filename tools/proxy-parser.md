# Proxy Parser

## What it does

Converts the format of every proxy in a file to a format you pick. Input format is auto-detected (see [Proxy Formats](../getting-started/proxy-formats.md)), so you can freely mix or convert between any supported format.

## Output formats

You'll be prompted to pick one of:

| Format | Example |
|--------|---------|
| `host:port:user:pass` | `1.2.3.4:8080:admin:secret` |
| `user:pass:host:port` | `admin:secret:1.2.3.4:8080` |
| `user:pass@host:port` | `admin:secret@1.2.3.4:8080` |
| `http://user:pass@host:port` | `http://admin:secret@1.2.3.4:8080` |
| `host:port` (no auth) | `1.2.3.4:8080` |

The last option **strips authentication** — useful if you need a public-format list and the proxies work via IP whitelisting.

## How input auto-detection works

You don't need to tell the tool what format your file is in — it figures it out line by line:

- Lines containing `@` are parsed as URLs
- Lines split cleanly into 4 colon-separated parts are parsed as either `host:port:user:pass` or `user:pass:host:port`, picked based on which side looks like a hostname/IP

So you can convert a file from `http://user:pass@host:port` to `host:port:user:pass` in one pass with no prep work.

## ⚠️ In-place conversion

The tool **overwrites the original file**. There's no automatic backup. If you want to keep the original format, copy the file first:

```bash
cp proxyfiles/my-list.txt proxyfiles/my-list-backup.txt
```

Then run the parser on the copy, or on the original.

## Output

```
Converted 1000 proxies to http://user:pass@host:port
File: /path/to/proxyfiles/my-list.txt
```

That's it — no CSV export, no summary. The file itself is the result.
