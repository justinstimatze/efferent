package main

import (
	"bufio"
	"bytes"
	"testing"
)

// The shared byte-stream decode (running status, real-time bytes, SysEx
// skip) is already covered by launchpad_test.go's TestReadMIDIEvents*
// suite against readMIDIMessages -- these tests only cover MIDImix's own
// thin layer on top of it: raw status/d1/d2 bytes arrive as MIDImixEvent
// with nothing decoded away.

func TestReadMIDImixEventsControlChange(t *testing.T) {
	raw := []byte{0xB0, 12, 100}
	events := make(chan MIDImixEvent, 1)
	readMIDImixEvents(bufio.NewReader(bytes.NewReader(raw)), events)
	got := <-events
	want := MIDImixEvent{Status: 0xB0, D1: 12, D2: 100}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadMIDImixEventsNoteOn(t *testing.T) {
	raw := []byte{0x90, 1, 127}
	events := make(chan MIDImixEvent, 1)
	readMIDImixEvents(bufio.NewReader(bytes.NewReader(raw)), events)
	got := <-events
	want := MIDImixEvent{Status: 0x90, D1: 1, D2: 127}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadMIDImixEventsRunningStatus(t *testing.T) {
	// A Control Change status byte followed by two data-byte pairs with no
	// repeated status -- running status should decode both as CC.
	raw := []byte{0xB0, 12, 64, 13, 65}
	events := make(chan MIDImixEvent, 2)
	readMIDImixEvents(bufio.NewReader(bytes.NewReader(raw)), events)
	first, second := <-events, <-events
	if want := (MIDImixEvent{Status: 0xB0, D1: 12, D2: 64}); first != want {
		t.Errorf("first: got %+v, want %+v", first, want)
	}
	if want := (MIDImixEvent{Status: 0xB0, D1: 13, D2: 65}); second != want {
		t.Errorf("second: got %+v, want %+v", second, want)
	}
}

func TestReadMIDImixEventsClosesChannelOnEOF(t *testing.T) {
	events := make(chan MIDImixEvent)
	readMIDImixEvents(bufio.NewReader(bytes.NewReader(nil)), events)
	if _, ok := <-events; ok {
		t.Fatal("expected channel to be closed on EOF with no events sent")
	}
}
