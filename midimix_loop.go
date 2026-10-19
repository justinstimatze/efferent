// midimix_loop.go owns the MIDImix's connect/reconnect lifecycle and
// decodes its incoming events directly into mixerControl (mixer.go).
// Mirrors vestLoop/launchpadSupervisor's connect-retry shape, but the
// MIDImix is read-only with exactly one consumer of its raw events -- this
// loop -- so there's no handle object other goroutines reach through.
// mixerControl is what they read, already decoded into gain/mute/solo/dial
// values.
package main

import (
	"log"
	"time"
)

// midimixRetryInterval matches vestRetryInterval/launchpadRetryInterval --
// same convention for optional, often-unplugged hardware.
const midimixRetryInterval = 90 * time.Second

func midimixLoop(mc *mixerControl) {
	for {
		m, err := ConnectMIDImix()
		if err != nil {
			log.Printf("efferent: midimix: not available (%v), retrying in %s", err, midimixRetryInterval)
			time.Sleep(midimixRetryInterval)
			continue
		}
		log.Printf("efferent: midimix: connected")
		for ev := range m.Listen() {
			dispatchMIDImixEvent(ev, mc)
		}
		m.Close()
		log.Printf("efferent: midimix: lost connection, reconnecting")
	}
}

// dispatchMIDImixEvent decodes one raw MIDImixEvent into a mixerControl
// update and logs it. Fader/knob CC values are 0-127 on the wire,
// normalized to 0.0-1.0. Mute buttons are assumed momentary electrically
// (Note On on press, Note Off on release) with the toggle logic done here
// in software -- an assumption, not confirmed for the MIDImix. Solo's wire
// signal is unresolved (midimix.go's header) so it's not dispatched.
//
// Every CC/note number below is from a single, only-partially-confirmed
// community source (midimix.go's header). A wrong mute-note number means
// "press mute and feel nothing," indistinguishable from the mute
// intentionally doing nothing -- unless the miss is logged, which is what
// the fallthrough below is for.
func dispatchMIDImixEvent(ev MIDImixEvent, mc *mixerControl) {
	switch ev.Status & 0xF0 {
	case 0xB0: // Control Change: a fader or a knob
		v := float64(ev.D2) / 127.0
		if ev.D1 == midimixMasterCC {
			mc.setMaster(v)
			logMixerMove(-1, "master", v)
			return
		}
		for ch, cc := range faderCC {
			if ev.D1 == cc {
				mc.setGain(ch, v)
				logMixerMove(ch, "gain", v)
				return
			}
		}
		for dial, row := range knobCC {
			for ch, cc := range row {
				if ev.D1 == cc {
					mc.setDial(ch, dial, v)
					logMixerMove(ch, dialControlName(dial), v)
					return
				}
			}
		}
		log.Printf("efferent: midimix: unrecognized CC %d (value %d) -- not in faderCC/knobCC/midimixMasterCC, check the control map against the real device", ev.D1, ev.D2)
	case 0x90: // Note On: a mute button press (Note Off is ignored -- see doc comment)
		if ev.D2 == 0 {
			return
		}
		for ch, note := range muteNote {
			if ev.D1 == note {
				newState := !mc.getMute(ch)
				mc.setMute(ch, newState)
				logMixerMoveBool(ch, "mute", newState)
				return
			}
		}
		log.Printf("efferent: midimix: unrecognized note %d (velocity %d) -- not in muteNote, check the control map against the real device", ev.D1, ev.D2)
	}
}

func dialControlName(dial int) string {
	switch dial {
	case 0:
		return "dial1"
	case 1:
		return "dial2"
	default:
		return "dial3"
	}
}
