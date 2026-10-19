package main

import (
	"testing"
	"time"
)

// fakeFlusher records every Frame flushed to it, so boot()'s write
// sequence and renderCore's output can be asserted without real MIDI
// hardware -- mirrors negotiation_test.go's fakeNegotiationLighter role.
type fakeFlusher struct {
	flushes []Frame
}

func (f *fakeFlusher) Flush(fr *Frame) error {
	f.flushes = append(f.flushes, *fr)
	return nil
}

type ledWrite struct{ index, r, g, b byte }

// decodeFrame unpacks a Frame's staged writes back into (index, r, g, b)
// tuples, mirroring Frame.Set's own 5-byte layout.
func decodeFrame(fr Frame) []ledWrite {
	var out []ledWrite
	for i := 0; i+5 <= len(fr.spec); i += 5 {
		out = append(out, ledWrite{fr.spec[i+1], fr.spec[i+2], fr.spec[i+3], fr.spec[i+4]})
	}
	return out
}

func noSleep(time.Duration) {}

// ---- demux ----

func TestDemuxLaunchpadEventsSplitsByType(t *testing.T) {
	events := make(chan LaunchpadEvent, 4)
	control, grid := demuxLaunchpadEvents(events)

	events <- LaunchpadEvent{Type: ControlDown, Control: 91}
	select {
	case ev := <-control:
		if ev.Control != 91 {
			t.Errorf("control event = %+v, want Control=91", ev)
		}
	case ev := <-grid:
		t.Fatalf("a ControlDown event landed on grid: %+v", ev)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the control event")
	}

	events <- LaunchpadEvent{Type: PadDown, Row: 2, Col: 3}
	select {
	case ev := <-grid:
		if ev.Row != 2 || ev.Col != 3 {
			t.Errorf("grid event = %+v, want Row=2 Col=3", ev)
		}
	case ev := <-control:
		t.Fatalf("a PadDown event landed on control: %+v", ev)
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the grid event")
	}

	close(events)
	select {
	case _, ok := <-control:
		if ok {
			t.Error("control channel should be closed once events closes")
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for control to close")
	}
	select {
	case _, ok := <-grid:
		if ok {
			t.Error("grid channel should be closed once events closes")
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for grid to close")
	}
}

// ---- boot ----

func TestBootSequence(t *testing.T) {
	s1 := &sessionSlot{row: 1, col: 1, foreground: true, main: &tstate{path: writeTranscript(t, assistantToolUse("2026-09-07T10:00:00Z"))}, subs: map[string]*tstate{}}
	s2 := &sessionSlot{row: 1, col: 2, main: &tstate{path: writeTranscript(t, assistantEndTurn("2026-09-07T10:00:00Z"))}, subs: map[string]*tstate{}}
	fl := &fleet{slots: []*sessionSlot{s1, s2}}

	ff := &fakeFlusher{}
	boot(ff, fl, time.Now(), noSleep)

	// 2 pad-TESTING writes (one per slot), 8 core-TESTING writes (levels
	// 1..8), 1 CONFIRMED batch write (all 8 core levels at once), 2
	// pad-resolve writes -- the staged order of the boot ritual.
	const wantFlushes = 2 + 8 + 1 + 2
	if len(ff.flushes) != wantFlushes {
		t.Fatalf("got %d flushes, want %d", len(ff.flushes), wantFlushes)
	}

	checkSingle := func(i int, wantIndex byte, wantColor rgb) {
		t.Helper()
		w := decodeFrame(ff.flushes[i])
		if len(w) != 1 {
			t.Fatalf("flush %d: got %d LED writes, want 1", i, len(w))
		}
		if w[0].index != wantIndex {
			t.Errorf("flush %d: index = %d, want %d", i, w[0].index, wantIndex)
		}
		if w[0] != (ledWrite{wantIndex, wantColor[0], wantColor[1], wantColor[2]}) {
			t.Errorf("flush %d: got %+v, want color %v", i, w[0], wantColor)
		}
	}

	checkSingle(0, padNote(1, 1), colorTest)
	checkSingle(1, padNote(1, 2), colorTest)
	for level := 1; level <= 8; level++ {
		checkSingle(1+level, padNote(corePad(level)), colorTest)
	}

	confirmed := decodeFrame(ff.flushes[10])
	if len(confirmed) != 8 {
		t.Fatalf("CONFIRMED flush: got %d LED writes, want 8", len(confirmed))
	}
	for level := 1; level <= 8; level++ {
		w := confirmed[level-1]
		if w.index != padNote(corePad(level)) || w != (ledWrite{padNote(corePad(level)), colorConfirmed[0], colorConfirmed[1], colorConfirmed[2]}) {
			t.Errorf("CONFIRMED level %d: got %+v, want index %d color %v", level, w, padNote(corePad(level)), colorConfirmed)
		}
	}

	resolve1 := decodeFrame(ff.flushes[11])
	resolve2 := decodeFrame(ff.flushes[12])
	if len(resolve1) != 1 || resolve1[0].index != padNote(1, 1) {
		t.Errorf("resolve 1: got %+v, want one write to pad(1,1)", resolve1)
	}
	if len(resolve2) != 1 || resolve2[0].index != padNote(1, 2) {
		t.Errorf("resolve 2: got %+v, want one write to pad(1,2)", resolve2)
	}
}

// ---- sessionSlot.color: the branches with an exact, non-breathing output ----

func TestSessionSlotColorGoneIsBlack(t *testing.T) {
	s := &sessionSlot{gone: true}
	if got := s.color(time.Now(), 0); got != (rgb{}) {
		t.Errorf("color = %v, want black", got)
	}
}

func TestSessionSlotColorForegroundIsHere(t *testing.T) {
	s := &sessionSlot{foreground: true}
	if got := s.color(time.Now(), 0); got != colorHere {
		t.Errorf("color = %v, want colorHere %v", got, colorHere)
	}
}

func TestSessionSlotColorConfirmFlash(t *testing.T) {
	now := time.Now()
	s := &sessionSlot{confirmAt: now}
	if got := s.color(now, 0); got != colorFlash {
		t.Errorf("color = %v, want colorFlash %v (just inside the 250ms confirm window)", got, colorFlash)
	}
}

func TestSessionSlotColorErrorTick(t *testing.T) {
	now := time.Now()
	s := &sessionSlot{errStreak: 1, lastErrAt: now}
	if got := s.color(now, 0); got != colorTick {
		t.Errorf("color = %v, want colorTick %v (just inside the 300ms error-tick window)", got, colorTick)
	}
}

// ---- renderCore ----

func TestRenderCoreNoWorkShowsOnlyPilotLED(t *testing.T) {
	fr := &Frame{}
	lit, period := renderCore(fr, 0, 0, 0)
	if lit != 0 || period != 3.0 {
		t.Errorf("lit,period = %d,%.1f, want 0,3.0 when nothing is working", lit, period)
	}
	w := decodeFrame(*fr)
	if len(w) != 8 {
		t.Fatalf("got %d LED writes, want 8 (the whole column)", len(w))
	}
	if w[0].index != padNote(corePad(1)) {
		t.Errorf("level 1 index = %d, want %d", w[0].index, padNote(corePad(1)))
	}
	for i := 1; i < 8; i++ {
		if w[i].r != 0 || w[i].g != 0 || w[i].b != 0 {
			t.Errorf("level %d = %+v, want dark", i+2, w[i])
		}
	}
}

func TestRenderCoreMaxRateLightsWholeColumn(t *testing.T) {
	fr := &Frame{}
	lit, period := renderCore(fr, 20000, 3, 0)
	if lit != 8 {
		t.Errorf("lit = %d, want 8 at the top of the log scale", lit)
	}
	if period <= 0 {
		t.Errorf("period = %v, want positive", period)
	}
}

func TestFleetWorkingHandleGetSet(t *testing.T) {
	var h fleetWorkingHandle
	if got := h.get(); got != 0 {
		t.Errorf("zero-value get() = %d, want 0", got)
	}
	if h.validNow() {
		t.Error("zero-value validNow() = true, want false (unknown until the first set)")
	}
	h.set(3)
	if got := h.get(); got != 3 {
		t.Errorf("get() after set(3) = %d, want 3", got)
	}
	if !h.validNow() {
		t.Error("validNow() after set(3) = false, want true")
	}
}

// TestFleetWorkingHandleResetClearsStaleCount guards against a Launchpad
// disconnect (routine -- the device is shared with music) stopping fleetLoop
// without resetting the count, which would leave fleet-breath pulsing at
// the last-seen rate/intensity forever instead of falling back to its calm
// default.
func TestFleetWorkingHandleResetClearsStaleCount(t *testing.T) {
	var h fleetWorkingHandle
	h.set(4)
	h.reset()
	if got := h.get(); got != 0 {
		t.Errorf("get() after reset() = %d, want 0 (fleetBreathParams' safe/calm default), not the stale 4", got)
	}
	if h.validNow() {
		t.Error("validNow() after reset() = true, want false")
	}
}
