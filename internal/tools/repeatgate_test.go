package tools

import (
	"sort"
	"sync"
	"testing"
	"time"
)

// A short delay keeps the suite fast; the production value is repeatDelay.
const testDelay = 40 * time.Millisecond

func TestRepeatGate_PartitionsOnceFromRepeated(t *testing.T) {
	g := newRepeatGate([]string{"a", "b", "a", "c"}, testDelay)

	if want := []int{1, 3}; !equalInts(g.once, want) {
		t.Errorf("once = %v, want %v (b and c, in input order)", g.once, want)
	}
	if want := []int{0, 2}; !equalInts(g.groups["a"], want) {
		t.Errorf("groups[a] = %v, want %v", g.groups["a"], want)
	}
	if len(g.groups) != 1 {
		t.Errorf("groups has %d entries, want 1", len(g.groups))
	}
	if !g.repeated() {
		t.Error("repeated() = false, want true when a proxy occurs twice")
	}
	if g.repeats() != 2 {
		t.Errorf("repeats() = %d, want 2", g.repeats())
	}
}

// TestRepeatGate_NoDuplicatesLeavesEveryProxyInThePool is the property that
// makes this safe to leave switched on: an ordinary file must not change shape.
func TestRepeatGate_NoDuplicatesLeavesEveryProxyInThePool(t *testing.T) {
	g := newRepeatGate([]string{"a", "b", "c"}, testDelay)

	if want := []int{0, 1, 2}; !equalInts(g.once, want) {
		t.Errorf("once = %v, want %v", g.once, want)
	}
	if len(g.groups) != 0 {
		t.Errorf("groups has %d entries, want 0 when every proxy is distinct", len(g.groups))
	}
	if g.repeated() {
		t.Error("repeated() = true, want false when every proxy is distinct")
	}
	if g.repeats() != 0 {
		t.Errorf("repeats() = %d, want 0", g.repeats())
	}
}

// TestRepeatGate_OnceKeepsInputOrder: `once` drives dispatch, and map iteration
// order is randomised, so building it from the map would shuffle the results
// table between runs of the same file.
func TestRepeatGate_OnceKeepsInputOrder(t *testing.T) {
	g := newRepeatGate([]string{"z", "y", "x", "w"}, testDelay)
	if want := []int{0, 1, 2, 3}; !equalInts(g.once, want) {
		t.Errorf("once = %v, want %v", g.once, want)
	}
}

// TestRepeatGate_RunRepeatsChecksEveryOccurrenceExactlyOnce guards the case a
// count-only assertion would miss: every index checked, none twice, none lost.
func TestRepeatGate_RunRepeatsChecksEveryOccurrenceExactlyOnce(t *testing.T) {
	g := newRepeatGate([]string{"a", "b", "a", "b", "a", "c"}, time.Millisecond)

	var mu sync.Mutex
	var seen []int
	g.runRepeats(func(i int) {
		mu.Lock()
		seen = append(seen, i)
		mu.Unlock()
	})

	sort.Ints(seen)
	// c is index 5 and occurs once, so it is the pool's job and not a repeat.
	if want := []int{0, 1, 2, 3, 4}; !equalInts(seen, want) {
		t.Errorf("checked %v, want %v", seen, want)
	}
}

// TestRepeatGate_LeavesTheDelayBetweenRepeats is the behaviour that was asked
// for. It asserts the gap between consecutive checks of one proxy rather than
// the total runtime, so it cannot pass by being slow for some other reason.
func TestRepeatGate_LeavesTheDelayBetweenRepeats(t *testing.T) {
	g := newRepeatGate([]string{"a", "a", "a"}, testDelay)

	var mu sync.Mutex
	var at []time.Time
	g.runRepeats(func(int) {
		mu.Lock()
		at = append(at, time.Now())
		mu.Unlock()
	})

	if len(at) != 3 {
		t.Fatalf("recorded %d checks, want 3", len(at))
	}
	for i := 1; i < len(at); i++ {
		if gap := at[i].Sub(at[i-1]); gap < testDelay {
			t.Errorf("gap between check %d and %d was %v, want at least %v", i-1, i, gap, testDelay)
		}
	}
}

// TestRepeatGate_FirstOccurrenceDoesNotWait: the delay sits *between* checks,
// so a repeated proxy must not pay it before its first one.
func TestRepeatGate_FirstOccurrenceDoesNotWait(t *testing.T) {
	g := newRepeatGate([]string{"a", "a"}, 400*time.Millisecond)

	var mu sync.Mutex
	var first time.Time
	start := time.Now()
	g.runRepeats(func(int) {
		mu.Lock()
		if first.IsZero() {
			first = time.Now()
		}
		mu.Unlock()
	})

	if delay := first.Sub(start); delay > 200*time.Millisecond {
		t.Errorf("first occurrence waited %v, want no wait", delay)
	}
}

// TestRepeatGate_RepeatsOfOneProxyDoNotOverlap. Two concurrent requests for one
// session defeat the spacing however long each waited first.
func TestRepeatGate_RepeatsOfOneProxyDoNotOverlap(t *testing.T) {
	g := newRepeatGate([]string{"a", "a", "a", "a"}, time.Millisecond)

	var mu sync.Mutex
	inFlight, maxInFlight := 0, 0
	g.runRepeats(func(int) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()

		time.Sleep(5 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()
	})

	if maxInFlight != 1 {
		t.Errorf("max concurrent checks of one proxy = %d, want 1", maxInFlight)
	}
}

// TestRepeatGate_DifferentProxiesRepeatConcurrently is the other half: spacing
// one proxy must not serialise the others.
func TestRepeatGate_DifferentProxiesRepeatConcurrently(t *testing.T) {
	g := newRepeatGate([]string{"a", "a", "b", "b"}, time.Millisecond)

	var mu sync.Mutex
	inFlight, maxInFlight := 0, 0
	g.runRepeats(func(int) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()
	})

	if maxInFlight != 2 {
		t.Errorf("max concurrent checks across two proxies = %d, want 2", maxInFlight)
	}
}

func TestRepeatGate_RunRepeatsOnCleanFileDoesNothing(t *testing.T) {
	g := newRepeatGate([]string{"a", "b"}, time.Hour)

	called := 0
	start := time.Now()
	g.runRepeats(func(int) { called++ })

	if called != 0 {
		t.Errorf("check called %d times, want 0", called)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("runRepeats took %v on a file with no duplicates, want no wait", elapsed)
	}
}

func TestRepeatDelayIsOneSecond(t *testing.T) {
	if repeatDelay != time.Second {
		t.Errorf("repeatDelay = %v, want 1s", repeatDelay)
	}
}

func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
