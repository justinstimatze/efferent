package main

import (
	"testing"
)

// setStateDir isolates state-file writes (mirrors negotiation_test.go's
// own setup) so dispatchMIDImixEvent's logMixerMove calls don't touch the
// real $XDG_STATE_HOME during tests.
func setStateDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func TestDispatchMIDImixEventFaderSetsGain(t *testing.T) {
	setStateDir(t)
	mc := newMixerControl()
	dispatchMIDImixEvent(MIDImixEvent{Status: 0xB0, D1: faderCC[0], D2: 127}, mc)
	channels, _ := mc.snapshot()
	if got := channels[0].Gain; got != 1.0 {
		t.Errorf("channel 0 gain = %v, want 1.0 (D2=127 -> full)", got)
	}
}

func TestDispatchMIDImixEventFaderMidValue(t *testing.T) {
	setStateDir(t)
	mc := newMixerControl()
	dispatchMIDImixEvent(MIDImixEvent{Status: 0xB0, D1: faderCC[3], D2: 0}, mc)
	channels, _ := mc.snapshot()
	if got := channels[3].Gain; got != 0 {
		t.Errorf("channel 3 gain = %v, want 0 (D2=0 -> off)", got)
	}
}

func TestDispatchMIDImixEventMasterCC(t *testing.T) {
	setStateDir(t)
	mc := newMixerControl()
	dispatchMIDImixEvent(MIDImixEvent{Status: 0xB0, D1: midimixMasterCC, D2: 64}, mc)
	_, master := mc.snapshot()
	want := 64.0 / 127.0
	if master != want {
		t.Errorf("master = %v, want %v", master, want)
	}
}

func TestDispatchMIDImixEventKnobSetsDial(t *testing.T) {
	setStateDir(t)
	mc := newMixerControl()
	// Row 2 (middle), channel 5 (index 4): CC 47 per knobCC.
	dispatchMIDImixEvent(MIDImixEvent{Status: 0xB0, D1: knobCC[1][4], D2: 127}, mc)
	channels, _ := mc.snapshot()
	if got := channels[4].Dial[1]; got != 1.0 {
		t.Errorf("channel 4 dial 1 = %v, want 1.0", got)
	}
}

func TestDispatchMIDImixEventMuteButtonTogglesOnPress(t *testing.T) {
	setStateDir(t)
	mc := newMixerControl()
	if !mc.getMute(0) {
		t.Fatal("channel 0 should start muted")
	}
	dispatchMIDImixEvent(MIDImixEvent{Status: 0x90, D1: muteNote[0], D2: 100}, mc)
	if mc.getMute(0) {
		t.Error("channel 0 should be unmuted after one mute-button press")
	}
	dispatchMIDImixEvent(MIDImixEvent{Status: 0x90, D1: muteNote[0], D2: 100}, mc)
	if !mc.getMute(0) {
		t.Error("channel 0 should be muted again after a second press")
	}
}

func TestDispatchMIDImixEventMuteButtonReleaseIsIgnored(t *testing.T) {
	setStateDir(t)
	mc := newMixerControl()
	// Note On with velocity 0 is the standard MIDI idiom for "note off" --
	// should not toggle mute, same as a real 0x80 Note Off wouldn't.
	dispatchMIDImixEvent(MIDImixEvent{Status: 0x90, D1: muteNote[0], D2: 0}, mc)
	if !mc.getMute(0) {
		t.Error("a zero-velocity Note On (release) should not toggle mute")
	}
}

func TestDispatchMIDImixEventUnknownCCIsIgnored(t *testing.T) {
	setStateDir(t)
	mc := newMixerControl()
	// 100 is a realistic MIDI data byte (0-127) that just isn't in any of
	// this file's lookup tables -- e.g. a control this project hasn't
	// mapped yet (Send All, per midimix.go's header) rather than a value
	// the wire could never actually produce.
	dispatchMIDImixEvent(MIDImixEvent{Status: 0xB0, D1: 100, D2: 64}, mc)
	channels, master := mc.snapshot()
	for i, c := range channels {
		if c.Gain != 1.0 || c.Mute != true {
			t.Errorf("channel %d changed on an unknown CC: %+v", i, c)
		}
	}
	if master != 1.0 {
		t.Errorf("master changed on an unknown CC: %v", master)
	}
}
