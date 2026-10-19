package main

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// buzzCall records one Buzz invocation so a test can assert on it.
type buzzCall struct {
	motors    []int
	intensity byte
}

// rampCall records one Ramp invocation so a test can assert on it.
type rampCall struct {
	motors []int
	peak   byte
	cycles int
}

// fakeBuzzer stands in for a live *Vest so applyVest's edge-triggering
// logic (and the breathing pacer's invoke/toggle logic) can be tested
// without real BLE hardware. Guarded by a mutex because, unlike applyVest
// (always called from a single goroutine in both production and these
// tests), the breathing pacer's Ramp runs from launchpadLoop's own
// background goroutine while a test observes it from the test goroutine.
// frameCall records one writeFrame invocation so a test can assert on it.
type frameCall struct {
	active map[int]byte
}

type fakeBuzzer struct {
	mu     sync.Mutex
	calls  []buzzCall
	failOn int // 1-indexed call number to fail on; 0 means never fail

	ramps  []rampCall
	frames []frameCall
}

func (f *fakeBuzzer) Buzz(motors []int, intensity byte, dur time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, buzzCall{motors, intensity})
	if f.failOn != 0 && len(f.calls) == f.failOn {
		return errors.New("fake buzz failure")
	}
	return nil
}

// Ramp mirrors real Ramp's actual contract -- it runs until stop fires --
// rather than returning immediately, so a test driving the breathing
// pacer's toggle logic can observe a ramp genuinely "in progress" between
// the invoke press that starts it and the press that stops it.
func (f *fakeBuzzer) Ramp(motors []int, peak byte, cycleDur time.Duration, cycles int, stop <-chan struct{}) error {
	f.mu.Lock()
	f.ramps = append(f.ramps, rampCall{motors, peak, cycles})
	f.mu.Unlock()
	<-stop
	return nil
}

func (f *fakeBuzzer) writeFrame(active map[int]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, frameCall{maps.Clone(active)})
	if f.failOn != 0 && len(f.frames) == f.failOn {
		return errors.New("fake writeFrame failure")
	}
	return nil
}

func newTestConn(fb *fakeBuzzer, edge vestEdge) *vestConn {
	return &vestConn{v: fb, edge: edge, dead: make(chan struct{})}
}

func TestApplyVestNilConnNoOp(t *testing.T) {
	// Must not panic -- this is the graceful "vest not connected" path.
	applyVest(nil, afferentState{}, nil)
}

func TestApplyVestStateErrNoOp(t *testing.T) {
	fb := &fakeBuzzer{}
	conn := newTestConn(fb, vestEdge{})

	applyVest(conn, afferentState{}, errors.New("read failed"))

	if len(fb.calls) != 0 {
		t.Errorf("got %d buzz calls on a state read error, want 0", len(fb.calls))
	}
}

func TestApplyVestFatigueRisingEdgeOnly(t *testing.T) {
	fb := &fakeBuzzer{}
	conn := newTestConn(fb, vestEdge{})
	fatigued := afferentState{Factors: []string{fatigueFactorLabel}}

	applyVest(conn, fatigued, nil)
	if len(fb.calls) != 1 {
		t.Fatalf("fatigue rising edge: got %d calls, want 1", len(fb.calls))
	}

	// Still fatigued on the next read -- must not re-fire.
	applyVest(conn, fatigued, nil)
	if len(fb.calls) != 1 {
		t.Errorf("fatigue held steady: got %d calls, want still 1", len(fb.calls))
	}

	// Fatigue clears, then crosses again -- should fire a second time.
	applyVest(conn, afferentState{}, nil)
	applyVest(conn, fatigued, nil)
	if len(fb.calls) != 2 {
		t.Errorf("fatigue re-crossing: got %d calls, want 2", len(fb.calls))
	}
}

func TestApplyVestBuzzFailureMarksDead(t *testing.T) {
	fb := &fakeBuzzer{failOn: 1}
	conn := newTestConn(fb, vestEdge{})

	applyVest(conn, afferentState{Factors: []string{fatigueFactorLabel}}, nil)

	select {
	case <-conn.dead:
	default:
		t.Error("a Buzz failure should close conn.dead")
	}
}

func writeAfferentEdgeFixture(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "afferent"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "afferent", "state.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSeedVestEdge(t *testing.T) {
	writeAfferentEdgeFixture(t, `{"co2_bucket":"high","factors":["session_over_fatigue_threshold"]}`)

	edge := seedVestEdge()

	if !edge.lastFatigued {
		t.Error("lastFatigued = false, want true")
	}
}

func TestSeedVestEdgeNoStateFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	// No afferent/state.json at all -- must return the zero-value edge,
	// not panic, so a vest that connects before afferent has ever run
	// starts from a sane "nothing has happened yet" baseline.
	edge := seedVestEdge()
	if edge.lastFatigued {
		t.Errorf("seedVestEdge() with no state file = %+v, want zero value", edge)
	}
}

func TestContains(t *testing.T) {
	list := []string{"a", "b", "c"}
	if !contains(list, "b") {
		t.Error(`contains(list, "b") = false, want true`)
	}
	if contains(list, "z") {
		t.Error(`contains(list, "z") = true, want false`)
	}
	if contains(nil, "a") {
		t.Error(`contains(nil, "a") = true, want false`)
	}
}
