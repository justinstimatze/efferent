// mixer.go blends independent felt-signal "channels" onto the vest and Hue,
// gain/mute/solo/dial-controlled live from a physical Akai MIDImix. Gain,
// mute and solo are handled once, here, for every channel regardless of
// target; the vestChannel/hueChannel split below separates the two output
// devices.
package main

import (
	"log"
	"math"
	"sync"
	"time"
)

const mixerTick = 50 * time.Millisecond // matches familiars_breath.go's own tick rate

// mixerComfortCeiling mirrors vest_loop.go's vestIntensity -- the same
// comfort ceiling applies here regardless of how many channels are
// contributing to a motor.
const mixerComfortCeiling = 0x5

// mixerChannel is the identity every channel shares regardless of target --
// viz and logging need a name; nothing else is uniform across devices.
// Every channel factors into three stages: a sensor (fleetWorkingHandle,
// readAgentState, a MIDImix dial), processing it owns (envelope, fill,
// placement math -- target-agnostic), and output. Only output differs by
// device, so it's the one stage this interface splits on, below -- the
// same seam audio graphs like JACK and the Web Audio API use ("a
// processing unit's own logic never knows what's downstream").
type mixerChannel interface {
	Name() string
}

// vestChannel is a mixerChannel that targets the vest. RenderVest returns
// this channel's own raw per-motor intensity for the current tick, within
// whatever zone it targets -- before gain, before blending with any other
// channel. dial is threaded in (unlike gain/mute/solo, applied outside by
// blend()) because it parameterizes what a channel renders, not how loud --
// the one place a channel can't stay fully target-and-control-agnostic.
type vestChannel interface {
	mixerChannel
	RenderVest(now time.Time, dial [3]float64) map[int]byte
}

// hueChannel is a mixerChannel that targets Hue. push is true only on the
// tick a channel actually has a new target to send -- Hue's transition
// time resets on every PUT, so re-sending an unchanged target every 20Hz
// mixer tick would fight its own glide instead of producing one. transitionDS
// is the deciseconds to pass to pushLight for that one push.
type hueChannel interface {
	mixerChannel
	RenderHue(now time.Time) (briDelta int, push bool, transitionDS int)
}

// mixerChannels is the static channel-to-MIDImix-strip assignment -- a Go
// literal, like the rest of the device wiring (no config file exists in
// this repo). Index 0 is the leftmost physical channel strip.
// nil means unassigned -- 8 slots exist so a later channel is just a new
// entry here, not new infrastructure.
var mixerChannels [8]mixerChannel

// channelControl is one channel's live control state, decoded from the
// MIDImix by midimixLoop (midimix_loop.go). Dial semantics are channel-
// specific -- fleetBreathChannel.RenderVest reads all three (vertical
// center, left/right balance, spread; familiars_breath.go); hueBreathChannel
// ignores Dial entirely, since a light has no spatial position on the body.
// A dial-alt-mode/bank mechanism is not built.
type channelControl struct {
	// Gain is 0.0-1.0 from the fader. With fleet-breath's peaks capped at
	// 0x2-0x3 (familiars_breath.go), the fader's 128 positions would resolve
	// to only a handful of output levels; blend()'s dithering accumulator
	// (ditherState, below) smooths the gain/master path. It does not smooth
	// fleetBreathParams' own peak selection, which is latched once per
	// breath cycle before blend() sees it.
	Gain float64
	Mute bool       // opt-in gate: true means silent regardless of gain
	Solo bool       // exact wire signal for this is unresolved -- see midimix.go's header; unwired for now
	Dial [3]float64 // 0.0-1.0 each
}

// mixerControl is the shared, mutex-guarded live state every channel's
// gain/mute/solo/dial values live in, plus the master fader -- read once
// per tick by mixerLoop's blend pipeline, written by midimixLoop as
// MIDImix events arrive. The same "one goroutine produces, another reads"
// shape vestHandle/hueHandle use, holding decoded control values instead
// of a connection, since the MIDImix (unlike the vest/Hue) has exactly one
// consumer of its raw events -- there's nothing else for a handle-to-a-
// connection to serve here.
type mixerControl struct {
	mu      sync.Mutex
	channel [8]channelControl
	master  float64
}

// newMixerControl starts every channel muted -- silent until its MIDImix
// mute button says otherwise, an opt-in gate -- with gain and master
// already "up," so unmuting a channel is the one gesture needed to feel it
// at a sane default level. This ignores where the physical faders actually
// sit (the hardware has no position feedback and no fader-position read
// path, see midimix.go), so the first unmute lands at gain 1.0 even if a
// fader looks lower. Gain can never push a channel past
// mixerComfortCeiling (blend() clamps the combined total), so the worst
// case is the channel's own normal peak, immediately.
func newMixerControl() *mixerControl {
	mc := &mixerControl{master: 1.0}
	for i := range mc.channel {
		mc.channel[i].Gain = 1.0
		mc.channel[i].Mute = true
	}
	return mc
}

func (mc *mixerControl) setGain(ch int, v float64) {
	mc.mu.Lock()
	mc.channel[ch].Gain = v
	mc.mu.Unlock()
}

func (mc *mixerControl) setMute(ch int, v bool) {
	mc.mu.Lock()
	mc.channel[ch].Mute = v
	mc.mu.Unlock()
}

func (mc *mixerControl) getMute(ch int) bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.channel[ch].Mute
}

func (mc *mixerControl) setDial(ch, dial int, v float64) {
	mc.mu.Lock()
	mc.channel[ch].Dial[dial] = v
	mc.mu.Unlock()
}

func (mc *mixerControl) setMaster(v float64) {
	mc.mu.Lock()
	mc.master = v
	mc.mu.Unlock()
}

// snapshot returns a copy of the current control state, taken once per
// mixer tick so the whole blend pipeline sees one consistent view instead
// of racing live MIDImix updates mid-computation.
func (mc *mixerControl) snapshot() (channels [8]channelControl, master float64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.channel, mc.master
}

// ditherState carries blend()'s per-motor fractional remainder across
// ticks, indexed by motor number (0-39, matching the vest's 40 motors --
// vest.go). Delta-sigma style: each tick adds the true (gain/master-
// scaled, ceiling-clamped) target to the accumulator, emits its whole
// number as this tick's output, and keeps the remainder for next tick --
// so a target that never reaches 1.0 on its own still averages out
// correctly over time as evenly-spaced single-tick pulses instead of
// being silently truncated to zero (without it, at gain*master
// multipliers below roughly a third, even fleet-breath's top peak (0x3)
// never crosses 1.0 and the fader feels like a cliff). mixerTick runs at
// 20Hz; vest-protocol.md's estimate of an ERM motor's mechanical low-pass
// (~10-20Hz, ~30-40ms rise/decay) is the reason to expect this reads as
// smoother rather than as separate taps, but that estimate is recalled,
// not confirmed.
type ditherState struct {
	accum [40]float64
}

// blend combines every unmuted (or, if any channel is soloed, only soloed)
// channel's rendered output into one final per-motor frame: gain-scaled,
// then root-sum-square combined per motor -- Israr & Poupyrev's validated
// phantom-sensation model (haptic-mixing-prior-art.md's Thread A1): two
// nearby simultaneous vibrotactile sources don't stay separate, they fuse
// into one percept whose intensity is sqrt(A1^2 + A2^2), not their sum and
// not the louder one alone. Generalized here to N>2 contributions as a
// direct, undramatic extension of the same energy-summation model -- the
// paper validates the 2-source case specifically, not N>2. Master-scaled,
// clamped to the comfort ceiling, and dithered (ds, see ditherState) last
// -- ds is caller-owned rather than a package global, the same reasoning
// that already moved channels off the package global: a fresh
// &ditherState{} makes a single call's output reproducible and testable,
// while mixerLoop keeps one alive across its own ticks.
// computeAnySolo reports whether any channel that could actually contribute
// anyway (unmuted, gain up, assigned) is soloed -- soloing a muted channel
// isn't a request to hear nothing, it's a no-op, so it must not trigger
// solo-exclusivity and silence every other channel that was already
// playing. Shared by blend() and blendHue() -- solo is mixer-wide, not
// per-target: soloing the Hue channel silences vest channels too, and vice
// versa, the natural reading of solo on one physical console regardless of
// how many device types sit behind it.
func computeAnySolo(channels [8]mixerChannel, controls [8]channelControl) bool {
	for i, ch := range channels {
		if ch == nil {
			continue
		}
		c := controls[i]
		if c.Solo && !c.Mute && c.Gain > 0 {
			return true
		}
	}
	return false
}

func blend(now time.Time, channels [8]mixerChannel, controls [8]channelControl, master float64, ds *ditherState) map[int]byte {
	anySolo := computeAnySolo(channels, controls)

	sumSq := make(map[int]float64)
	for i, ch := range channels {
		vc, ok := ch.(vestChannel)
		if !ok {
			continue
		}
		c := controls[i]
		if c.Mute || c.Gain <= 0 {
			continue
		}
		if anySolo && !c.Solo {
			continue
		}
		for motor, v := range vc.RenderVest(now, c.Dial) {
			scaled := float64(v) * c.Gain
			sumSq[motor] += scaled * scaled
		}
	}

	out := make(map[int]byte, len(sumSq))
	for motor, ss := range sumSq {
		v := math.Sqrt(ss) * master
		if v > mixerComfortCeiling {
			v = mixerComfortCeiling
		}
		ds.accum[motor] += v
		b := byte(ds.accum[motor])
		ds.accum[motor] -= float64(b)
		out[motor] = b
	}
	// Any motor not touched this tick has no live target to dither toward
	// -- drop its leftover fractional debt rather than let a channel that
	// stopped (muted, or excluded by another channel's solo) reappear
	// later and immediately cash in a pulse it never actually earned.
	for motor := range ds.accum {
		if _, touched := sumSq[motor]; !touched {
			ds.accum[motor] = 0
		}
	}
	return out
}

// blendHue mirrors blend()'s mute/solo/gain-skip logic but sums briDelta
// directly rather than via root-sum-square -- Hue has no phantom-sensation
// fusion mechanism the way co-located vibrotactile motors do, and at most
// one Hue channel exists for now. RenderHue is only called on channels that
// pass the mute/gain/solo gate first, same ordering blend() uses -- which
// is also why muting a Hue channel stops its own sensor read (e.g.
// hueBreathChannel's readAgentState) entirely rather than just discarding
// the result. push is true only when at least one contributing channel has
// a fresh target to send this tick; transitionDS is that channel's own
// half-cycle length in deciseconds (undefined, and unused, when push is
// false). N>1 simultaneous pushing Hue channels would need a real
// combination rule for transitionDS -- not needed yet, so not built: this
// takes the last one seen, same as an unhandled case rather than a
// considered choice.
func blendHue(now time.Time, channels [8]mixerChannel, controls [8]channelControl, master float64, anySolo bool) (briDelta int, push bool, transitionDS int) {
	for i, ch := range channels {
		hc, ok := ch.(hueChannel)
		if !ok {
			continue
		}
		c := controls[i]
		if c.Mute || c.Gain <= 0 {
			continue
		}
		if anySolo && !c.Solo {
			continue
		}
		delta, chPush, ds := hc.RenderHue(now)
		briDelta += int(float64(delta) * c.Gain * master)
		if chPush {
			push = true
			transitionDS = ds
		}
	}
	return briDelta, push, transitionDS
}

// frameIsActive reports whether frame carries any real (nonzero) signal --
// checked by value, not by map length. vestChannel.RenderVest has no fixed
// convention for how it represents "nothing right now": familiarBreathFrame
// (familiars_breath.go) always returns a full map including explicit
// zeros at the trough, while a sparser channel might return an empty map
// instead. len(frame) > 0 would treat those two cases completely
// differently -- effectively-empty-but-populated would read as "active"
// forever, pinning the vest at 20Hz writes indefinitely. Checking values
// makes the distinction irrelevant: either convention behaves the same way
// here.
func frameIsActive(frame map[int]byte) bool {
	for _, v := range frame {
		if v > 0 {
			return true
		}
	}
	return false
}

// mixerLoop ticks the blend pipeline and writes the result to the vest and
// to Hue -- the two output adapters run independently within one tick, on
// their own connectivity (vh, hh), so one device being unreachable never
// blocks the other's channels from being processed. Vest side: writes only
// while at least one channel is actually contributing, plus exactly one
// all-zero frame on the active-to-inactive transition to clear its own
// motors -- writing an empty frame every tick regardless would fight
// applyVest's fatigue buzz and vestLoop's keepalive for the bus during
// genuinely silent periods (vest.go's writeFrame semantics: any write
// claims every motor it doesn't mention as "off"), the same discipline
// the other vest writers already follow. Hue side: pushLight only on
// the tick blendHue reports a fresh target (see hueChannel's doc comment)
// -- "channels render, the mixer writes" holds for both adapters, so
// neither hueBreathChannel nor fleetBreathChannel ever calls writeFrame or
// pushLight itself. Also snapshots control state into mixer_state.json
// every tick regardless of either device's connectivity -- MIDImix state
// (fader/mute/dial positions) is independent of whether the vest or Hue is
// reachable.
func mixerLoop(vh *vestHandle, hh *hueHandle, mc *mixerControl) {
	ticker := time.NewTicker(mixerTick)
	defer ticker.Stop()
	wasActive := false
	ds := &ditherState{}
	for now := range ticker.C {
		controls, master := mc.snapshot()
		recordMixerStateViz(mixerChannels, controls, master)

		if conn := vh.get(); conn != nil {
			frame := blend(now, mixerChannels, controls, master, ds)
			active := frameIsActive(frame)
			if active || wasActive {
				if err := conn.v.writeFrame(frame); err != nil {
					log.Printf("efferent: mixer: %v", err)
					conn.markDead()
				}
			}
			wasActive = active
		} else {
			wasActive = false
		}

		if bridge, ok := hh.get(); ok {
			anySolo := computeAnySolo(mixerChannels, controls)
			if delta, push, transitionDS := blendHue(now, mixerChannels, controls, master, anySolo); push {
				ls := lightState{CT: normalCT, Bri: clampBri(normalBri + delta)}
				for _, id := range targetLights {
					if err := pushLight(bridge, id, ls, transitionDS); err != nil {
						log.Printf("efferent: mixer: hue: light %s: %v", id, err)
					}
				}
			}
		}
	}
}

// mixerStateVizMinInterval matches vest.go's vestVizMinInterval -- same
// reasoning: mixerLoop's ticker fires at mixerTick (20Hz) for the life of
// the daemon regardless of whether any channel is active, and nothing
// reading a live debug view needs faster than this.
const mixerStateVizMinInterval = 100 * time.Millisecond

// lastMixerStateViz throttles recordMixerStateViz. Only ever touched from
// mixerLoop's own single goroutine, so -- like vest.go's lastVizWrite under
// writeMu -- it needs no lock of its own.
var lastMixerStateViz time.Time

// mixerChannelViz is one channel strip's state for the viz snapshot below.
type mixerChannelViz struct {
	Name string     `json:"name"` // "" for an unassigned strip (mixerChannels[i] == nil)
	Gain float64    `json:"gain"`
	Mute bool       `json:"mute"`
	Solo bool       `json:"solo"`
	Dial [3]float64 `json:"dial"`
}

// mixerStateViz is the mixer's full live control state. The moves log alone can't answer "what's the mixer doing
// right now": mute is a software toggle that newMixerControl resets to true
// on daemon restart, with nothing in the log marking the restart. This file
// is the live answer, the same role vest_frame.json plays for the vest's
// state; a reader checks Timestamp for staleness.
type mixerStateViz struct {
	Timestamp string             `json:"timestamp"`
	Channels  [8]mixerChannelViz `json:"channels"`
	Master    float64            `json:"master"`
}

func recordMixerStateViz(channels [8]mixerChannel, controls [8]channelControl, master float64) {
	now := time.Now()
	if now.Sub(lastMixerStateViz) < mixerStateVizMinInterval {
		return
	}
	lastMixerStateViz = now

	var viz mixerStateViz
	viz.Timestamp = now.UTC().Format(time.RFC3339Nano)
	viz.Master = master
	for i, ch := range channels {
		name := ""
		if ch != nil {
			name = ch.Name()
		}
		c := controls[i]
		viz.Channels[i] = mixerChannelViz{Name: name, Gain: c.Gain, Mute: c.Mute, Solo: c.Solo, Dial: c.Dial}
	}
	writeJSON(statePath("efferent", "mixer_state.json"), viz)
}

// mixerMoveEntry logs one fader/knob/mute change, mirroring negotiation.go's
// labels.jsonl/turn_ratings.jsonl exactly (same appendJSONLine/statePath
// helpers, new filename). A fader move is a self-report with a magnitude
// attached (haptic-mixing-prior-art.md's design implication #9), from a
// control that has an independent reason to be touched.
type mixerMoveEntry struct {
	Timestamp string `json:"timestamp"`
	Channel   int    `json:"channel"` // 0-7, or -1 for master
	Control   string `json:"control"` // "gain", "master", "mute", "dial1".."dial3"
	Value     any    `json:"value"`   // float64 for gain/master/dial moves, bool for mute -- real JSON types, not pre-stringified, so mixer-viz's jq can read .value directly
}

// mixerMoveLogInterval throttles logMixerMove the same way vest.go's
// recordVizFrame throttles the viz snapshot (vestVizMinInterval): a
// continuous control (fader, knob, master) emits a stream of MIDI CC
// messages per gesture (a fader sweep is 100+ messages), and
// negotiation.go's append-per-event pattern (built for discrete button
// presses) would turn one gesture into 100+ file opens. This caps it to one write per
// control per interval; the tradeoff, same as recordVizFrame's own, is
// that the logged value is roughly where a gesture settled rather than its
// exact final sample -- acceptable since nothing currently reads this file
// at sub-second precision. Mute (logMixerMoveBool) is a discrete,
// one-message-per-press control and isn't throttled.
const mixerMoveLogInterval = 150 * time.Millisecond

type moveKey struct {
	channel int
	control string
}

// moveThrottle tracks the last-logged time per (channel, control) key.
// Its own mutex, rather than relying on dispatchMIDImixEvent's single-
// caller-goroutine invariant, so it's equally safe called directly from a
// test.
type moveThrottle struct {
	mu   sync.Mutex
	last map[moveKey]time.Time
}

func newMoveThrottle() *moveThrottle {
	return &moveThrottle{last: make(map[moveKey]time.Time)}
}

// allow reports whether now is far enough past the last allowed call for
// key (or is the first call for it), and if so records now as the new
// last-allowed time.
func (t *moveThrottle) allow(key moveKey, interval time.Duration, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.last[key]; ok && now.Sub(last) < interval {
		return false
	}
	t.last[key] = now
	return true
}

var mixerMoveThrottle = newMoveThrottle()

func logMixerMove(channel int, control string, value float64) {
	now := time.Now()
	if !mixerMoveThrottle.allow(moveKey{channel, control}, mixerMoveLogInterval, now) {
		return
	}
	appendJSONLine(statePath("efferent", "mixer_moves.jsonl"), mixerMoveEntry{
		Timestamp: now.UTC().Format(time.RFC3339),
		Channel:   channel,
		Control:   control,
		Value:     value,
	})
}

func logMixerMoveBool(channel int, control string, value bool) {
	appendJSONLine(statePath("efferent", "mixer_moves.jsonl"), mixerMoveEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   channel,
		Control:   control,
		Value:     value,
	})
}
