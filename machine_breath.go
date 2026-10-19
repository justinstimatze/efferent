// machine_breath.go implements "Machine's Breath" from signal-vocabulary.md:
// a slow brightening/dimming rhythm on a fixed soft-white baseline, faster
// while the agent is working (about 3s per breath) and slower while it
// awaits the user (about 12s). The phase comes from agenthook's
// agent_state.json, which tracks the agent's own turn rather than system
// CPU load.
//
// Two states only, deliberately: no "awaiting_permission" state (see
// agenthook/main.go's header), and no staleness handling on the read. A
// stale phase after a hard crash costs a few seconds of the wrong rhythm.
//
// The Hue-availability gate lives in mixer.go's mixerLoop, which calls
// hh.get() once per tick before touching any hueChannel.
package main

import "time"

const (
	breathIdleHalfCycle = 6 * time.Second         // ~12s full breath while awaiting the user
	breathBusyHalfCycle = 1500 * time.Millisecond // ~3s full breath while the agent is working
	breathBriDelta      = 20                      // +/- around the fixed soft-white baseline
)

// agentPhaseWorking must match agenthook/main.go's phaseWorking string --
// agenthook and efferent are separate compiled binaries in this module
// with no shared package between them, so this string contract is pinned
// by the tests on both sides, not the compiler.
const agentPhaseWorking = "working"

// breathHalfCycleForPhase maps the agent's turn phase to the breathing
// rhythm's half-cycle duration -- quick while the agent is actively
// working, steady otherwise. Anything other than agentPhaseWorking,
// including "" (agentState's zero value, or a missing/unreadable state
// file), reads as idle: there's no live turn happening, so idle is the
// honest default, not an error case.
func breathHalfCycleForPhase(phase string) time.Duration {
	if phase == agentPhaseWorking {
		return breathBusyHalfCycle
	}
	return breathIdleHalfCycle
}

// deciseconds converts a Duration to Hue's transitiontime unit (1/10s),
// floored at 1 -- 0 would mean "instant snap," never what a breath wants.
func deciseconds(d time.Duration) int {
	if ds := int(d / (100 * time.Millisecond)); ds >= 1 {
		return ds
	}
	return 1
}

func clampBri(b int) int {
	switch {
	case b < 1:
		return 1
	case b > 254:
		return 254
	default:
		return b
	}
}

// hueBreathChannel is Machine's Breath as a mixer hueChannel. It reads
// readAgentState() itself, but only when the mixer's mute/gain/solo gate
// lets RenderHue get called at all (mixer.go's blendHue), and reports its
// brightness delta as data rather than pushing it: the mixer performs the
// pushLight call, the same "channels render, the mixer writes" split vest
// channels follow.
//
// newMixerControl() starts every channel muted, so this is silent until
// unmuted on the MIDImix.
type hueBreathChannel struct {
	started  bool
	high     bool
	lastFlip time.Time
}

func newHueBreathChannel() *hueBreathChannel {
	return &hueBreathChannel{}
}

func (c *hueBreathChannel) Name() string { return "machine's-breath" }

// RenderHue implements mixer.go's hueChannel. push is true only on the
// tick a half-cycle boundary is crossed -- Hue's own transition time
// (deciseconds(half)) would restart on every redundant PUT otherwise,
// fighting its own glide instead of producing one. A missing or corrupt
// agent_state.json is expected, not fatal (no Claude Code session has
// necessarily run yet, or the hook hasn't fired since boot) --
// readAgentState's error just reads as idle phase.
func (c *hueBreathChannel) RenderHue(now time.Time) (briDelta int, push bool, transitionDS int) {
	phase := ""
	if state, err := readAgentState(); err == nil {
		phase = state.Phase
	}
	half := breathHalfCycleForPhase(phase)

	if !c.started || now.Sub(c.lastFlip) >= half {
		c.high = !c.high
		c.lastFlip = now
		c.started = true
		push = true
		transitionDS = deciseconds(half)
	}
	if c.high {
		return breathBriDelta, push, transitionDS
	}
	return -breathBriDelta, push, transitionDS
}
