// midimix.go is a from-scratch Go driver for the Akai MIDImix's raw MIDI
// output -- talks to the ALSA rawmidi character device directly, the same
// approach launchpad.go already uses for the Launchpad X (see its header
// for why: matching this project's existing pattern of talking to a
// protocol directly rather than pulling in a MIDI library). Unlike the
// Launchpad X, the MIDImix enumerates as a single ALSA MIDI port -- `amidi
// -l` shows one "MIDI Mix" port with no second DAW-mode port -- so none of
// launchpad.go's subdevice-preference handling (preferSubdevice, the
// SNDRV_CTL_IOCTL_RAWMIDI_PREFER_SUBDEVICE dance) is needed here.
//
// The CC/note number for every fader, knob, and mute button is not an
// Akai-published spec: Akai's MIDImix User Guide contains no CC table, and
// describes every control as "assignable" via Ableton's bundled
// integration or a separate MIDImix Editor app, not a fixed factory map.
//
// The numbers below come from a working Ableton Live control script
// (github.com/AodhSully/MIDIMIX-ABLETON-SCRIPT, MIDI_Map.py, credited
// there to "st4rchild"). One source, not independently corroborated; only
// fader 1 -> CC19 was spot-checked against a real device (`amidi -d -p
// hw:<card>,0,0`). The rest of the table is taken as good on the strength
// of that check plus its consistent structure. The master fader's CC and
// the Solo button's message are unresolved.
//
//	Knobs (Control Change, channel 1), row/channel:
//	  row 1 (top):    CC 16 20 24 28 46 50 54 58  (channels 1-8)
//	  row 2 (middle): CC 17 21 25 29 47 51 55 59
//	  row 3 (bottom): CC 18 22 26 30 48 52 56 60
//	Faders (channels 1-8): CC 19 23 27 31 49 53 57 61
//	  -- gap between channel 4 and 5 (CC 32-45 unused) is in the source,
//	  not a transcription error.
//	Master fader: CC 62 -- inferred (the one otherwise-unassigned number
//	  where the pattern would put it), NOT stated outright by the source.
//	Mute buttons (MIDI Note, channel 1): 1 4 7 10 13 16 19 22
//	Record-arm buttons: 3 6 9 12 15 18 21 24
//	Bank Left/Right: notes 25/26
//	Solo button: whether it sends its own message, or only changes what a
//	  Mute press sends while held, is NOT resolved by this source.
//
// This file is deliberately transport only: open the device and decode the
// raw byte stream into MIDImixEvent values, still carrying the raw status/
// CC/note numbers -- the actual decode into semantic gain/mute/solo/dial
// values, using the control map above, lives in midimix_loop.go's
// dispatchMIDImixEvent instead.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// The tables below are the mapping documented in this file's header, kept
// in the same file as its provenance so a corrected CC number has one place
// to edit.
const midimixMasterCC = 62 // inferred, not directly stated by the source -- see this file's header

var faderCC = [8]byte{19, 23, 27, 31, 49, 53, 57, 61}

var knobCC = [3][8]byte{
	{16, 20, 24, 28, 46, 50, 54, 58}, // row 1 (top)
	{17, 21, 25, 29, 47, 51, 55, 59}, // row 2 (middle)
	{18, 22, 26, 30, 48, 52, 56, 60}, // row 3 (bottom)
}

var muteNote = [8]byte{1, 4, 7, 10, 13, 16, 19, 22}

// findMIDIMixCard locates the Akai MIDImix's ALSA card index by reading
// /proc/asound/cards, the same approach findLaunchpadCard uses and for the
// same reason: the card index shifts depending on what else is plugged in
// and in what order.
func findMIDIMixCard() (int, error) {
	f, err := os.Open("/proc/asound/cards")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/asound/cards: %w", err)
	}
	defer f.Close()

	cardHeader := regexp.MustCompile(`^\s*(\d+)\s+\[`)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := cardHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Each card entry is its header line plus one indented
		// continuation line -- either can name the device.
		continuation := ""
		if scanner.Scan() {
			continuation = scanner.Text()
		}
		if strings.Contains(line, "MIDI Mix") || strings.Contains(continuation, "MIDI Mix") {
			card, err := strconv.Atoi(m[1])
			if err != nil {
				return 0, fmt.Errorf("parsing card index from %q: %w", line, err)
			}
			return card, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scanning /proc/asound/cards: %w", err)
	}
	return 0, fmt.Errorf("akai MIDImix not found in /proc/asound/cards -- is it plugged in?")
}

// MIDImix is a connection to the Akai MIDImix's single ALSA rawmidi port.
type MIDImix struct {
	f *os.File
}

// ConnectMIDImix opens the device's rawmidi port. No subdevice-preference
// ioctl is needed: the MIDImix has exactly one port, unlike the Launchpad
// X's split DAW/MIDI substreams.
func ConnectMIDImix() (*MIDImix, error) {
	card, err := findMIDIMixCard()
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/dev/snd/midiC%dD0", card)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &MIDImix{f: f}, nil
}

// MIDImixEvent is one incoming message, left undecoded past the raw MIDI
// status/data bytes; midimix_loop.go maps CC/note numbers to controls.
type MIDImixEvent struct {
	Status byte // status byte with channel nibble intact: 0xB0-0xBF Control Change, 0x90-0x9F/0x80-0x8F Note On/Off
	D1, D2 byte // controller/note number and value, in wire order
}

// Listen starts a background goroutine decoding incoming MIDI messages and
// returns a channel of events.
func (m *MIDImix) Listen() <-chan MIDImixEvent {
	events := make(chan MIDImixEvent, 16)
	go readMIDImixEvents(bufio.NewReader(m.f), events)
	return events
}

// readMIDImixEvents decodes raw MIDI messages into MIDImixEvent values via
// the shared byte-stream decoder in rawmidi.go. Takes a *bufio.Reader (not
// *MIDImix) so it's testable against a byte stream without real hardware,
// the same split launchpad.go's readMIDIEvents uses.
func readMIDImixEvents(r *bufio.Reader, events chan<- MIDImixEvent) {
	defer close(events)
	readMIDIMessages(r, func(status, d1, d2 byte) {
		events <- MIDImixEvent{Status: status, D1: d1, D2: d2}
	})
}

func (m *MIDImix) Close() error {
	return m.f.Close()
}
