package session

import "testing"

func TestReplayWindow(t *testing.T) {
	var w replayWindow
	must := func(c uint32) {
		if err := w.accept(c); err != nil {
			t.Fatalf("accept %d unexpectedly failed: %v", c, err)
		}
	}
	reject := func(c uint32) {
		if err := w.accept(c); err == nil {
			t.Fatalf("accept %d unexpectedly succeeded", c)
		}
	}

	must(100)   // baseline
	reject(100) // duplicate of max
	must(101)   // advance
	must(90)    // in-window, out of order (diff 11)
	reject(90)  // in-window duplicate
	reject(68)  // diff 33 > 32: stale
	must(1000)  // big jump forward, window resets
	reject(101) // now far below max: stale
	reject(1000)
	must(1001)
}

func TestReplayWindowOutOfOrder(t *testing.T) {
	var w replayWindow
	seq := []uint32{10, 12, 11, 13, 9, 8}
	for _, c := range seq {
		if err := w.accept(c); err != nil {
			t.Fatalf("accept %d: %v", c, err)
		}
	}
	for _, c := range seq {
		if err := w.accept(c); err == nil {
			t.Fatalf("duplicate %d was accepted", c)
		}
	}
}
