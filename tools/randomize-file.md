# Randomize File

## What it does

Shuffles the order of proxies in a file. Pick a file, confirm, done — the file is overwritten with the lines in a new random order.

## Why you'd use it

- **Avoid deterministic ordering.** If multiple processes read the same proxy list in top-to-bottom order, they'll hit the same proxies at the same time. Shuffling spreads load evenly.
- **Retest fairness.** When comparing speed/uniqueness runs over time, shuffling between runs prevents any order-dependent bias.
- **Break provider ordering.** Proxies from some vendors come grouped by subnet. Shuffling mixes them for more diverse rotation.

## Behaviour

- Reads the file, shuffles the lines, writes back to the **same file**.
- Comments (`#`) and blank lines are skipped during load — if the original file had them, they will **not** be preserved after shuffling.
- Safe on any format supported by the toolbox (it works line-by-line without parsing).

> **Tip:** Keep a master copy of your proxy lists somewhere else if you care about preserving the original order or any in-file comments.
