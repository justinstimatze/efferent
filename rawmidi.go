// rawmidi.go decodes a raw MIDI byte stream -- shared by every device this
// project talks to directly over ALSA rawmidi (launchpad.go, midimix.go).
// Running status, single-byte real-time messages, and SysEx framing are
// all part of the wire format itself, not anything device-specific, so
// this is extracted once rather than duplicated per device. Originally
// lived only in launchpad.go (readMIDIEvents/skipSysEx); pulled out here
// when midimix.go needed the identical decode with a different consumer of
// the decoded messages.
package main

import "bufio"

const midiSysExEnd = 0xF7

// readMIDIMessages decodes a raw MIDI byte stream into complete 3-byte
// channel messages, calling handle(status, d1, d2) for each one. Handles
// running status (a repeated status byte may be omitted), single-byte
// System Real-Time messages (0xF8-0xFF, e.g. Active Sensing) that don't
// affect running status, and SysEx blocks (0xF0...0xF7), skipped whole.
// Takes an io.Reader-backed *bufio.Reader (not a device handle) so it's
// testable against a byte stream without real hardware. Returns when the
// reader fails (device closed).
func readMIDIMessages(r *bufio.Reader, handle func(status, d1, d2 byte)) {
	var status byte
	pending := make([]byte, 0, 2)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return
		}
		if b&0x80 == 0 {
			pending = append(pending, b)
			if len(pending) == 2 {
				handle(status, pending[0], pending[1])
				pending = pending[:0]
			}
			continue
		}
		if b == 0xF0 {
			skipSysEx(r)
			status = 0
			pending = pending[:0]
			continue
		}
		if b >= 0xF8 {
			continue // real-time byte, no data bytes, running status unaffected
		}
		status = b
		pending = pending[:0]
	}
}

// skipSysEx consumes bytes up to and including the terminating 0xF7.
func skipSysEx(r *bufio.Reader) {
	for {
		b, err := r.ReadByte()
		if err != nil || b == midiSysExEnd {
			return
		}
	}
}
