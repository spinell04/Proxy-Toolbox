package tools

import (
	"sync"
	"time"
)

// repeatDelay is the gap left between two checks of the same proxy string.
const repeatDelay = time.Second

// repeatGate splits a run into the proxies that appear once and the proxies
// that appear more than once.
//
// Why this exists: a residential gateway keys its sticky session off the
// username, so the same line repeated N times asks for the same session N
// times. Fired concurrently by the worker pool, those N requests are one
// instant of the provider's behaviour — which says nothing about whether the
// exit rotates. Spacing them turns a repeated line into a probe of one proxy
// over time, which is the only reason to repeat a line at all.
//
// The repeats are deliberately kept *out* of the worker pool. Holding a pool
// worker across each wait would let one repeated proxy starve the pool: with
// 100 workers and a file of 100 copies plus 900 distinct proxies, every worker
// would block on the one repeated proxy while the 900 others waited out the
// whole sequential run. Each repeated proxy gets its own goroutine instead, so
// the distinct proxies keep the pool at full width and the repeats trickle
// alongside them.
type repeatGate struct {
	delay time.Duration
	// once holds the indices of proxies appearing exactly once, in input order.
	once []int
	// groups maps a repeated proxy's id to its indices, in input order.
	groups map[string][]int
}

// newRepeatGate partitions ids, which is in input order and may contain
// duplicates. The indices it hands back are positions in that slice.
func newRepeatGate(ids []string, delay time.Duration) *repeatGate {
	at := make(map[string][]int, len(ids))
	for i, id := range ids {
		at[id] = append(at[id], i)
	}

	g := &repeatGate{delay: delay, groups: make(map[string][]int)}
	// Input order, not map order: `once` drives the pool's dispatch, and a
	// results table that reorders itself between runs is hard to read.
	for i, id := range ids {
		if len(at[id]) == 1 {
			g.once = append(g.once, i)
		}
	}
	for id, idx := range at {
		if len(idx) > 1 {
			g.groups[id] = idx
		}
	}
	return g
}

// repeated reports whether any proxy in the file occurs more than once.
func (g *repeatGate) repeated() bool { return len(g.groups) > 0 }

// repeats counts the checks that will be spaced rather than run at pool speed.
func (g *repeatGate) repeats() int {
	n := 0
	for _, idx := range g.groups {
		n += len(idx)
	}
	return n
}

// runRepeats checks every repeated proxy — one goroutine per proxy string,
// leaving at least delay between two checks of the same string — and returns
// once they are all done.
//
// check is called concurrently across different proxy strings, and never
// concurrently for the same one.
func (g *repeatGate) runRepeats(check func(index int)) {
	var wg sync.WaitGroup
	for _, idx := range g.groups {
		wg.Add(1)
		go func(idx []int) {
			defer wg.Done()
			for n, i := range idx {
				// The first occurrence waits for nothing: the delay sits
				// between checks rather than in front of them.
				if n > 0 {
					time.Sleep(g.delay)
				}
				check(i)
			}
		}(idx)
	}
	wg.Wait()
}
