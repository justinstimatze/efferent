// launchpad.go is a from-scratch Go driver for the Novation Launchpad X's
// MIDI/SysEx protocol, written against Novation's "Launchpad X Programmer's
// reference manual". Talks to the ALSA rawmidi character device directly --
// the same mechanism `amidi` itself uses -- rather than through a MIDI
// library, so it is Linux-only.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// All SysEx messages share this six-byte header regardless of direction.
var lpSysExHeader = []byte{0xF0, 0x00, 0x20, 0x29, 0x02, 0x0C}

const lpSysExEnd = 0xF7

const (
	lpCmdLEDLighting    = 0x03
	lpCmdProgLiveToggle = 0x0E
)

// Lighting types for the LED lighting SysEx's <colourspec> entries.
const (
	lpLightStatic = 0 // 1 data byte: palette index
	lpLightRGB    = 3 // 3 data bytes: R, G, B (0-127 each)
)

// Launchpad is a connection to a Launchpad X over its ALSA rawmidi MIDI
// port -- specifically the MIDI port, not the DAW port: Novation's
// reference routes Programmer-mode SysEx and lighting control through the
// MIDI interface, while the DAW interface carries Ableton's fixed
// Session-mode mapping instead.
type Launchpad struct {
	f *os.File

	// mu serializes every write to f: the negotiation panel and fleetLoop
	// (fleet_panel.go) write from separate goroutines, the same
	// concurrent-writer hazard the vest has (vest.go's writeMu). sendSysEx
	// holds mu for the whole write.
	mu sync.Mutex
}

// findLaunchpadCard locates the Launchpad X's ALSA card index by reading
// /proc/asound/cards, rather than hardcoding one -- the card index shifts
// depending on what else is plugged in and in what order (webcams, mics
// and other USB-Audio devices share the same numbering).
func findLaunchpadCard() (int, error) {
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
		// continuation line -- either can name the device, so consume and
		// check both before moving to the next card.
		continuation := ""
		if scanner.Scan() {
			continuation = scanner.Text()
		}
		if strings.Contains(line, "Launchpad X") || strings.Contains(continuation, "Launchpad X") {
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
	return 0, fmt.Errorf("Launchpad X not found in /proc/asound/cards -- is it plugged in?")
}

// sndrvCtlIoctlRawmidiPreferSubdevice is SNDRV_CTL_IOCTL_RAWMIDI_PREFER_SUBDEVICE
// (_IOW('U', 0x42, int)) = 0x40045542, as defined in
// /usr/include/sound/asound.h.
const sndrvCtlIoctlRawmidiPreferSubdevice = 0x40045542

// launchpadMIDISubdevice is the Launchpad X's "MIDI" port (see `amidi -l`):
// hw:X,0,0 reports as "... DAW Out" and hw:X,0,1 as
// "... MIDI Out" -- both are one ALSA rawmidi device (card 3's single
// /proc/asound/card3/midi0) with two substreams, not two separate device
// nodes. Novation's own manual says Programmer-mode SysEx and note/CC
// events route through the MIDI port specifically, not the DAW port.
const launchpadMIDISubdevice = 1

// preferSubdevice tells the kernel that the *next* open() of a
// multi-substream rawmidi device on this card should bind to a specific
// substream instead of grabbing whichever one is currently free. Without
// this, opening the Launchpad's single /dev/snd/midiC<card>D0 node is
// non-deterministic between its two substreams -- and substream 0 (DAW)
// happily accepts Novation's SysEx (mode toggle, LED lighting are
// apparently substream-agnostic at the firmware level) while never
// emitting incoming Note On/Control Change for button presses, since those
// only arrive on substream 1 (MIDI). That silent split is exactly what
// made input look broken while output kept working.
//
// Returns the open control-device fd rather than closing it here: the
// preference is scoped to that fd staying open (cmd/lpdiag checks which
// substream a given open actually bound to). Closing it immediately gives
// no error and still binds to subdevice 0. The caller must keep it open across its own
// rawmidi open() and close it only after.
func preferSubdevice(cardIndex, subdevice int) (*os.File, error) {
	path := fmt.Sprintf("/dev/snd/controlC%d", cardIndex)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	// IoctlSetPointerInt, not IoctlSetInt: _IOW(..., int) is a "write"
	// ioctl the kernel services with copy_from_user, so arg must be a
	// pointer to the int -- IoctlSetInt instead passes the value itself as
	// the raw arg (fine for ioctls that treat arg as an immediate, wrong
	// here), and fails with EFAULT as the kernel tries to copy_from_user(1)
	// as if it were a pointer.
	if err := unix.IoctlSetPointerInt(int(f.Fd()), sndrvCtlIoctlRawmidiPreferSubdevice, subdevice); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// launchpadConnectTimeout bounds ConnectLaunchpad: ALSA rawmidi's open() on
// a specific (non-default) substream blocks in-kernel (snd_rawmidi_open) if
// that substream hasn't finished being released yet -- e.g. a just-restarted
// efferent racing the previous process's teardown. A stuck connect must
// never stall the rest of the daemon, the same way Hue or the vest being
// unavailable doesn't.
const launchpadConnectTimeout = 5 * time.Second

// ConnectLaunchpad opens the device's MIDI port and switches it into
// Programmer mode -- the mode Novation's reference recommends for
// scripted control, where every pad press reports a MIDI message and the
// LED lighting SysEx addresses pads by the same fixed note numbering
// regardless of whatever layout was showing before. Bounded by
// launchpadConnectTimeout; see its doc comment for why that's needed at
// all. A timeout leaks the inner goroutine (still blocked in the kernel,
// with no way to cancel an in-flight open()) -- accepted as a bounded,
// rare cost against hanging the entire daemon indefinitely.
func ConnectLaunchpad() (*Launchpad, error) {
	type result struct {
		lp  *Launchpad
		err error
	}
	done := make(chan result, 1)
	go func() {
		lp, err := connectLaunchpad()
		done <- result{lp, err}
	}()
	select {
	case r := <-done:
		return r.lp, r.err
	case <-time.After(launchpadConnectTimeout):
		return nil, fmt.Errorf("timed out after %s opening the rawmidi device (a substream may still be held by a just-exited process)", launchpadConnectTimeout)
	}
}

func connectLaunchpad() (*Launchpad, error) {
	card, err := findLaunchpadCard()
	if err != nil {
		return nil, err
	}

	ctl, err := preferSubdevice(card, launchpadMIDISubdevice)
	if err != nil {
		return nil, fmt.Errorf("selecting MIDI substream: %w", err)
	}
	defer ctl.Close() // kept open until after the rawmidi open below -- see preferSubdevice's doc comment

	path := fmt.Sprintf("/dev/snd/midiC%dD0", card)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	lp := &Launchpad{f: f}
	if err := lp.setProgrammerMode(true); err != nil {
		f.Close()
		return nil, fmt.Errorf("entering programmer mode: %w", err)
	}
	return lp, nil
}

func (lp *Launchpad) setProgrammerMode(on bool) error {
	mode := byte(0)
	if on {
		mode = 1
	}
	return lp.sendSysEx(lpCmdProgLiveToggle, []byte{mode})
}

func (lp *Launchpad) sendSysEx(command byte, payload []byte) error {
	msg := make([]byte, 0, len(lpSysExHeader)+2+len(payload))
	msg = append(msg, lpSysExHeader...)
	msg = append(msg, command)
	msg = append(msg, payload...)
	msg = append(msg, lpSysExEnd)
	lp.mu.Lock()
	defer lp.mu.Unlock()
	_, err := lp.f.Write(msg)
	return err
}

// padNote returns the Programmer-mode note number for the 8x8 grid pad at
// (row, col), both 1-8, row 1 at the bottom -- matching Novation's own
// diagram, where note 11 (0Bh) is bottom-left and note 88 is top-right.
func padNote(row, col int) byte {
	return byte(10*row + col)
}

// LightIndex sets one addressable LED -- grid pad or a top-row/right-
// column/corner control button, all of which share the same colourspec
// address-byte scheme (see Off, which lights every one of them in a single
// batch) -- to a static RGB color (0-127 per channel, the device's own
// native range).
func (lp *Launchpad) LightIndex(index, r, g, b byte) error {
	colourspec := []byte{lpLightRGB, index, r & 0x7F, g & 0x7F, b & 0x7F}
	return lp.sendSysEx(lpCmdLEDLighting, colourspec)
}

// LightPad sets one grid pad to a static RGB color via LightIndex.
func (lp *Launchpad) LightPad(row, col int, r, g, b byte) error {
	return lp.LightIndex(padNote(row, col), r, g, b)
}

// Frame batches several LED writes into a single SysEx message -- the same
// move Off already makes by hand for a full clear, generalized so a caller
// that changes many LEDs at once (fleetLoop's per-tick grid render) sends
// one message instead of one per LED.
type Frame struct{ spec []byte }

// Set stages one addressable LED's RGB color in the frame.
func (fr *Frame) Set(index byte, r, g, b byte) {
	fr.spec = append(fr.spec, lpLightRGB, index, r&0x7F, g&0x7F, b&0x7F)
}

// Flush sends every staged LED write in fr as one SysEx message. A frame
// with nothing staged is a no-op rather than an empty SysEx write.
func (lp *Launchpad) Flush(fr *Frame) error {
	if len(fr.spec) == 0 {
		return nil
	}
	return lp.sendSysEx(lpCmdLEDLighting, fr.spec)
}

// Off sets every one of the surface's 81 addressable LEDs (the 8x8 grid,
// the top control row, the right-side column, and the top-right corner
// pad) to palette index 0 (black in Novation's colour
// palette table) in one batch LED lighting SysEx message.
func (lp *Launchpad) Off() error {
	colourspec := make([]byte, 0, 81*3)
	for row := 1; row <= 8; row++ {
		for col := 1; col <= 8; col++ {
			colourspec = append(colourspec, lpLightStatic, padNote(row, col), 0)
		}
	}
	for col := 1; col <= 8; col++ {
		colourspec = append(colourspec, lpLightStatic, byte(90+col), 0) // top row, CC91-98
	}
	for row := 1; row <= 8; row++ {
		colourspec = append(colourspec, lpLightStatic, byte(10*row+9), 0) // right column, CC19-89
	}
	colourspec = append(colourspec, lpLightStatic, 99, 0) // top-right corner, CC99
	return lp.sendSysEx(lpCmdLEDLighting, colourspec)
}

// PadEventType distinguishes a grid-pad press/release from a top-row or
// right-column control button press/release. Programmer mode reports the
// two differently on the wire: grid pads as Note On/Off, control buttons as
// Control Change -- see dispatchMIDIEvent.
type PadEventType int

const (
	PadDown PadEventType = iota
	PadUp
	ControlDown
	ControlUp
)

// LaunchpadEvent is one decoded incoming message. For PadDown/PadUp, Row
// and Col identify the grid pad (matching padNote's numbering) and
// Velocity carries the press velocity (0 on release). For
// ControlDown/ControlUp, Control carries the raw CC number (91-98 for the
// top row, 19/29/.../89 for the right column, 99 for the top-right corner)
// -- left undecoded since no consumer exists yet to say which mapping it
// wants.
type LaunchpadEvent struct {
	Type     PadEventType
	Row, Col int
	Control  byte
	Velocity byte
}

// Listen starts a background goroutine decoding incoming MIDI messages and
// returns a channel of events. The channel closes when the device is
// closed or a read otherwise fails. This is push-only groundwork: nothing
// in efferent consumes it yet (signal-vocabulary.md's Launchpad
// acknowledgment/invoke role is what will, once that's designed), so this
// is deliberately just "decode what the hardware sends," no dispatch.
func (lp *Launchpad) Listen() <-chan LaunchpadEvent {
	events := make(chan LaunchpadEvent, 16)
	go readMIDIEvents(bufio.NewReader(lp.f), events)
	return events
}

// readMIDIEvents decodes what Programmer mode actually sends (Note On/Off,
// Control Change) into LaunchpadEvent values, via the shared byte-stream
// decoder in rawmidi.go -- extracted there once midimix.go needed the
// identical decode for a different device.
func readMIDIEvents(r *bufio.Reader, events chan<- LaunchpadEvent) {
	defer close(events)
	readMIDIMessages(r, func(status, d1, d2 byte) {
		dispatchMIDIEvent(status, d1, d2, events)
	})
}

// dispatchMIDIEvent decodes one complete 3-byte channel message into a
// LaunchpadEvent. A Note On with velocity 0 is the standard MIDI idiom for
// "note off" (lets a stream use running status for a whole press-release
// pair), so it's treated identically to an explicit Note Off.
func dispatchMIDIEvent(status, d1, d2 byte, events chan<- LaunchpadEvent) {
	switch status & 0xF0 {
	case 0x90: // Note On (grid pad)
		row, col := int(d1)/10, int(d1)%10
		if d2 == 0 {
			events <- LaunchpadEvent{Type: PadUp, Row: row, Col: col}
		} else {
			events <- LaunchpadEvent{Type: PadDown, Row: row, Col: col, Velocity: d2}
		}
	case 0x80: // Note Off (grid pad)
		row, col := int(d1)/10, int(d1)%10
		events <- LaunchpadEvent{Type: PadUp, Row: row, Col: col}
	case 0xB0: // Control Change (top row / right column / corner)
		if d2 == 0 {
			events <- LaunchpadEvent{Type: ControlUp, Control: d1}
		} else {
			events <- LaunchpadEvent{Type: ControlDown, Control: d1, Velocity: d2}
		}
	// 0xA0 (polyphonic aftertouch) is intentionally undecoded -- efferent
	// only needs press/release, not held pressure.
	default:
	}
}

// Close returns the device to Live mode (Novation's reference calls this
// out explicitly: "Remember to switch the device back to Live mode once
// done") and closes the underlying device file.
func (lp *Launchpad) Close() error {
	_ = lp.setProgrammerMode(false)
	return lp.f.Close()
}
