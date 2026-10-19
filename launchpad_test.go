package main

import (
	"bufio"
	"bytes"
	"testing"
	"time"
)

func TestDispatchMIDIEventPadDown(t *testing.T) {
	events := make(chan LaunchpadEvent, 1)
	dispatchMIDIEvent(0x90, 23, 100, events)
	got := <-events
	want := LaunchpadEvent{Type: PadDown, Row: 2, Col: 3, Velocity: 100}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDispatchMIDIEventPadUpViaZeroVelocity(t *testing.T) {
	events := make(chan LaunchpadEvent, 1)
	dispatchMIDIEvent(0x90, 23, 0, events)
	got := <-events
	want := LaunchpadEvent{Type: PadUp, Row: 2, Col: 3}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDispatchMIDIEventPadUpViaNoteOff(t *testing.T) {
	events := make(chan LaunchpadEvent, 1)
	dispatchMIDIEvent(0x80, 88, 64, events)
	got := <-events
	want := LaunchpadEvent{Type: PadUp, Row: 8, Col: 8}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDispatchMIDIEventControlChange(t *testing.T) {
	events := make(chan LaunchpadEvent, 2)
	dispatchMIDIEvent(0xB0, 91, 127, events)
	dispatchMIDIEvent(0xB0, 91, 0, events)

	down := <-events
	if want := (LaunchpadEvent{Type: ControlDown, Control: 91, Velocity: 127}); down != want {
		t.Errorf("got %+v, want %+v", down, want)
	}
	up := <-events
	if want := (LaunchpadEvent{Type: ControlUp, Control: 91}); up != want {
		t.Errorf("got %+v, want %+v", up, want)
	}
}

func TestDispatchMIDIEventIgnoresAftertouch(t *testing.T) {
	events := make(chan LaunchpadEvent, 1)
	dispatchMIDIEvent(0xA0, 23, 100, events) // polyphonic aftertouch, deliberately undecoded
	select {
	case got := <-events:
		t.Errorf("expected no event, got %+v", got)
	default:
	}
}

func readEventsWithTimeout(t *testing.T, events <-chan LaunchpadEvent, n int) []LaunchpadEvent {
	t.Helper()
	got := make([]LaunchpadEvent, 0, n)
	for i := 0; i < n; i++ {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("channel closed early after %d of %d events", i, n)
			}
			got = append(got, ev)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for event %d of %d", i+1, n)
		}
	}
	return got
}

func TestReadMIDIEventsParsesStream(t *testing.T) {
	raw := []byte{0x90, 23, 100, 0x80, 23, 0}
	events := make(chan LaunchpadEvent, 8)
	readMIDIEvents(bufio.NewReader(bytes.NewReader(raw)), events)

	got := readEventsWithTimeout(t, events, 2)
	want := []LaunchpadEvent{
		{Type: PadDown, Row: 2, Col: 3, Velocity: 100},
		{Type: PadUp, Row: 2, Col: 3},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReadMIDIEventsRunningStatus(t *testing.T) {
	// Second Note On omits the repeated 0x90 status byte -- a real device
	// (and the MIDI spec) can do this for back-to-back same-status messages.
	raw := []byte{0x90, 23, 100, 24, 90}
	events := make(chan LaunchpadEvent, 8)
	readMIDIEvents(bufio.NewReader(bytes.NewReader(raw)), events)

	got := readEventsWithTimeout(t, events, 2)
	want := LaunchpadEvent{Type: PadDown, Row: 2, Col: 4, Velocity: 90}
	if got[1] != want {
		t.Errorf("running-status event: got %+v, want %+v", got[1], want)
	}
}

func TestReadMIDIEventsSkipsSysEx(t *testing.T) {
	raw := []byte{0xF0, 0x00, 0x20, 0x29, 0x02, 0x0C, 0x0E, 0x01, 0xF7, 0x90, 11, 50}
	events := make(chan LaunchpadEvent, 8)
	readMIDIEvents(bufio.NewReader(bytes.NewReader(raw)), events)

	got := readEventsWithTimeout(t, events, 1)
	want := LaunchpadEvent{Type: PadDown, Row: 1, Col: 1, Velocity: 50}
	if got[0] != want {
		t.Errorf("got %+v, want %+v", got[0], want)
	}
}

func TestReadMIDIEventsIgnoresRealtimeBytes(t *testing.T) {
	// 0xF8 (timing clock) lands mid-stream and must not disturb running
	// status or produce an event of its own.
	raw := []byte{0x90, 23, 100, 0xF8, 0x80, 23, 0}
	events := make(chan LaunchpadEvent, 8)
	readMIDIEvents(bufio.NewReader(bytes.NewReader(raw)), events)

	got := readEventsWithTimeout(t, events, 2)
	want := []LaunchpadEvent{
		{Type: PadDown, Row: 2, Col: 3, Velocity: 100},
		{Type: PadUp, Row: 2, Col: 3},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReadMIDIEventsClosesChannelOnEOF(t *testing.T) {
	raw := []byte{0x90, 23, 100}
	events := make(chan LaunchpadEvent, 8)
	readMIDIEvents(bufio.NewReader(bytes.NewReader(raw)), events)

	readEventsWithTimeout(t, events, 1)
	select {
	case _, ok := <-events:
		if ok {
			t.Fatalf("expected channel closed, got another event")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for channel close")
	}
}
