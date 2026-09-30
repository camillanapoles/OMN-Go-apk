package logx

import (
	"fmt"
	"sync"
	"testing"
)

// ----------------------------------------------------------------------
// The history ring
// ----------------------------------------------------------------------
//
// Each test makes its own Hub. The ring of a test thus starts empty, and no
// other test writes into it.

// The ring keeps the lines in the order that they arrived.
func TestLogHistoryKeepsTheOrder(t *testing.T) {
	h := &Hub{}
	for _, line := range []string{"first\n", "second\n", "third\n"} {
		h.Broadcast(line, false)
	}

	got := h.Snapshot()
	want := []string{"first\n", "second\n", "third\n"}
	if len(got) != len(want) {
		t.Fatalf("the ring holds %d lines, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// The ring writes over the oldest line, and it keeps the newest.
//
// A log that stops at its cap keeps the start of the session and loses
// the fault. The fault is the half that a person needs.
func TestLogHistoryKeepsTheNewestLines(t *testing.T) {
	h := &Hub{}
	for i := 0; i < HistoryCap+25; i++ {
		h.Broadcast(fmt.Sprintf("line %d\n", i), false)
	}

	got := h.Snapshot()
	if len(got) != HistoryCap {
		t.Fatalf("the ring holds %d lines, want the cap of %d", len(got), HistoryCap)
	}
	if want := fmt.Sprintf("line %d\n", 25); got[0] != want {
		t.Errorf("the oldest line is %q, want %q", got[0], want)
	}
	if want := fmt.Sprintf("line %d\n", HistoryCap+24); got[len(got)-1] != want {
		t.Errorf("the newest line is %q, want %q", got[len(got)-1], want)
	}
}

// The snapshot is a copy. A caller that changes it changes no line of
// the ring.
func TestLogHistorySnapshotIsACopy(t *testing.T) {
	h := &Hub{}
	h.Broadcast("the real line\n", false)

	first := h.Snapshot()
	if len(first) != 1 {
		t.Fatalf("the ring holds %d lines, want 1", len(first))
	}
	first[0] = "a line that a caller wrote"

	second := h.Snapshot()
	if second[0] != "the real line\n" {
		t.Errorf("the ring now holds %q, thus the snapshot shares its memory", second[0])
	}
}

// Two goroutines that write at the same time must not race, and the ring
// must keep each line.
//
// Run this one with -race. Broadcast takes mu, and record runs under it. A
// ring outside that lock is a data race that a test without -race never
// reports.
func TestLogHistoryUnderConcurrentWriters(t *testing.T) {
	h := &Hub{}
	const writers, each = 8, 20

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				h.Broadcast(fmt.Sprintf("writer %d line %d\n", w, i), false)
			}
		}(w)
	}
	// A reader at the same time, so the snapshot path is under the race
	// detector as well.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = h.Snapshot()
		}
	}()
	wg.Wait()

	got := h.Snapshot()
	if len(got) != writers*each {
		t.Fatalf("the ring holds %d lines, want %d. A line was lost.",
			len(got), writers*each)
	}
	seen := map[string]bool{}
	for _, line := range got {
		if seen[line] {
			t.Errorf("the line %q is in the ring two times", line)
		}
		seen[line] = true
	}
}
