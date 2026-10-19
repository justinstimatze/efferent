package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// testTimeout/waitFor are shared async-test helpers -- also used by
// fleet_panel_test.go.
const testTimeout = 2 * time.Second

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// negotiationLightCall records one LightIndex invocation.
type negotiationLightCall struct{ index, r, g, b byte }

// fakeNegotiationLighter stands in for a live *Launchpad so
// negotiationLoop's dispatch/lighting logic can be tested without real
// MIDI hardware. Guarded by a mutex: flash (negotiation.go) relights a pad
// from its own goroutine while a test observes from the test goroutine.
type fakeNegotiationLighter struct {
	mu    sync.Mutex
	calls []negotiationLightCall
}

func (f *fakeNegotiationLighter) LightIndex(index, r, g, b byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, negotiationLightCall{index, r, g, b})
	return nil
}

func (f *fakeNegotiationLighter) lightCalls() []negotiationLightCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func setEfferentStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	return filepath.Join(dir, "efferent")
}

func TestNegotiationLoopBootLightsAllSevenButtons(t *testing.T) {
	setEfferentStateDir(t)
	fl := &fakeNegotiationLighter{}
	events := make(chan LaunchpadEvent)
	go negotiationLoop(fl, events)

	waitFor(t, func() bool { return len(fl.lightCalls()) >= 7 }, "all seven boot lights")

	wantCCs := []byte{holdCC, calmCC, okCC, frayedCC, doneCC, landedCC, missedCC}
	calls := fl.lightCalls()
	for _, cc := range wantCCs {
		found := false
		for _, c := range calls {
			if c.index == cc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no boot light call for CC%d", cc)
		}
	}
	close(events)
}

func TestNegotiationLoopHoldTogglesAndWritesState(t *testing.T) {
	stateDir := setEfferentStateDir(t)
	fl := &fakeNegotiationLighter{}
	events := make(chan LaunchpadEvent, 4)
	go negotiationLoop(fl, events)
	waitFor(t, func() bool { return len(fl.lightCalls()) >= 7 }, "boot lights")

	events <- LaunchpadEvent{Type: ControlDown, Control: holdCC}
	waitFor(t, func() bool {
		data, err := os.ReadFile(filepath.Join(stateDir, "hold.json"))
		return err == nil && strings.Contains(string(data), `"active":true`)
	}, "hold.json to report active")

	events <- LaunchpadEvent{Type: ControlDown, Control: holdCC}
	waitFor(t, func() bool {
		data, err := os.ReadFile(filepath.Join(stateDir, "hold.json"))
		return err == nil && strings.Contains(string(data), `"active":false`)
	}, "hold.json to report released")
	close(events)
}

func TestNegotiationLoopSelfReportAppendsLabel(t *testing.T) {
	stateDir := setEfferentStateDir(t)
	fl := &fakeNegotiationLighter{}
	events := make(chan LaunchpadEvent, 4)
	go negotiationLoop(fl, events)
	waitFor(t, func() bool { return len(fl.lightCalls()) >= 7 }, "boot lights")

	events <- LaunchpadEvent{Type: ControlDown, Control: frayedCC}
	path := filepath.Join(stateDir, "labels.jsonl")
	waitFor(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, "labels.jsonl to be created")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading labels.jsonl: %v", err)
	}
	var entry selfReportEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil { // trim trailing newline
		t.Fatalf("unmarshaling label entry: %v (data: %s)", err, data)
	}
	if entry.Label != "frayed" {
		t.Errorf("label = %q, want %q", entry.Label, "frayed")
	}
	close(events)
}

func TestNegotiationLoopTurnRatingAppends(t *testing.T) {
	stateDir := setEfferentStateDir(t)
	fl := &fakeNegotiationLighter{}
	events := make(chan LaunchpadEvent, 4)
	go negotiationLoop(fl, events)
	waitFor(t, func() bool { return len(fl.lightCalls()) >= 7 }, "boot lights")

	events <- LaunchpadEvent{Type: ControlDown, Control: landedCC}
	path := filepath.Join(stateDir, "turn_ratings.jsonl")
	waitFor(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, "turn_ratings.jsonl to be created")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading turn_ratings.jsonl: %v", err)
	}
	var entry turnRatingEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("unmarshaling rating entry: %v (data: %s)", err, data)
	}
	if entry.Rating != "landed" {
		t.Errorf("rating = %q, want %q", entry.Rating, "landed")
	}
	close(events)
}

func TestNegotiationLoopIgnoresPadEvents(t *testing.T) {
	setEfferentStateDir(t)
	fl := &fakeNegotiationLighter{}
	events := make(chan LaunchpadEvent, 4)
	go negotiationLoop(fl, events)
	waitFor(t, func() bool { return len(fl.lightCalls()) >= 7 }, "boot lights")

	before := len(fl.lightCalls())
	events <- LaunchpadEvent{Type: PadDown, Row: 1, Col: 1}
	time.Sleep(50 * time.Millisecond)
	if got := len(fl.lightCalls()); got != before {
		t.Errorf("a PadDown event changed light calls: %d -> %d, want no change", before, got)
	}
	close(events)
}
