// familiars_breath.go implements the fleet-breath mixer channel --
// "feel the fleet breathe": a continuous, spatially-filling vest signal
// whose rate and peak intensity track the live count of Claude Code
// sessions currently working across the fleet (fleet_panel.go's
// fleetWorkingHandle), not any single agent's own turn phase.
//
// It is a mixerChannel (mixer.go): opt-in via the MIDImix's per-channel mute
// button, blended with any other channel via root-sum-square, and
// parameterized live by the channel's three dials. Earlier hardcoded
// designs that ran continuously (tapping the wearer's own heart rate, or a
// phase-gated ramp) were uncomfortable worn for a whole session; see
// haptic-mixing-prior-art.md.
package main

import (
	"math"
	"time"
)

const (
	// No tick constant of its own -- RenderVest is called once per mixer
	// tick (mixer.go's mixerTick).
	familiarBreathInhaleFraction = 0.4 // inhale shorter and rising, exhale longer and settling

	// Cycle length varies with the fleet's live working-session count:
	// more sessions working means a faster
	// breath, nobody working means the slowest end of the range rather
	// than silence (silence is what the mute button is for).
	familiarBreathCycleMax = 14 * time.Second // 0 sessions working
	familiarBreathCycleMin = 6 * time.Second  // fleetBreathSaturateAt or more sessions working

	// Peaks are deliberately low (0x2-0x3 of a 0x0-0xF scale): higher values
	// read as too strong when worn for a whole session.
	familiarBreathPeakMin = byte(0x2)
	familiarBreathPeakMax = byte(0x3)

	// fleetBreathSaturateAt is the working-session count at which the
	// rate/intensity range maxes out, matching the 2-4 concurrent channels
	// haptic-mixing-prior-art.md (Thread A2) gives as the practical
	// attention ceiling.
	fleetBreathSaturateAt = 4

	// vestBack's two 10-motor columns (vest.go), top-to-bottom per
	// vest-protocol.md's corner-confirmed motor hypothesis. Row 0 is the
	// bottom (hem) of each column, row familiarBreathRows-1 the top
	// (shoulder).
	familiarBreathRows        = 10
	familiarBreathLeftBottom  = 19
	familiarBreathRightBottom = 29

	// Soft edge width (in rows) for the rising fill. A narrow edge (1.5)
	// makes almost every row snap from dark to fully saturated at once;
	// this is wide enough that each row saturates at a different point in
	// the cycle instead.
	familiarBreathEdgeWidth = 8.0

	// familiarBreathNarrowEdgeWidth is dial[2]'s narrow extreme: a sharp,
	// localized point instead of a soft wave.
	familiarBreathNarrowEdgeWidth = 1.5
)

// breathEase maps t in [0,1] to [0,1] via a raised-cosine curve: zero
// slope at both ends, so a curve built from it turns around smoothly at
// the trough and the peak instead of the sharp corner a linear ramp has
// right at the top.
func breathEase(t float64) float64 {
	return (1 - math.Cos(t*math.Pi)) / 2
}

// fleetBreathParams maps a working-session count onto this cycle's
// (cycleDur, peak) -- linear interpolation between the min/max range,
// clamped at fleetBreathSaturateAt. cycleDur genuinely graduates (the
// range spans seconds, byte-truncated at nanosecond precision -- no
// visible stepping). peak does not, despite the same formula: the min/max
// range is only one nibble wide (0x2 to 0x3), and byte() truncates rather
// than rounds, so peak stays at familiarBreathPeakMin for any frac short
// of a full 1.0 and only reaches familiarBreathPeakMax right at
// fleetBreathSaturateAt -- a step, not a ramp. A true gradient would need
// sub-nibble resolution (temporal dithering between adjacent values).
// mixer.go's blend() dithers its gain/master-scaled output that way, but
// peak is chosen once per breath cycle, before blend() sees this channel's
// output, so that dithering cannot reach it.
func fleetBreathParams(working int) (cycleDur time.Duration, peak byte) {
	if working < 0 {
		working = 0
	}
	frac := float64(working) / float64(fleetBreathSaturateAt)
	if frac > 1 {
		frac = 1
	}
	span := familiarBreathCycleMax - familiarBreathCycleMin
	cycleDur = familiarBreathCycleMax - time.Duration(frac*float64(span))
	peak = familiarBreathPeakMin + byte(frac*float64(familiarBreathPeakMax-familiarBreathPeakMin))
	return cycleDur, peak
}

// familiarBreathEnvelope returns the breath's amplitude in [0,1] at
// position t within one cycle of length cycleDur: eased rise over the
// inhale fraction, eased fall over the rest. cycleDur is a parameter
// because fleetBreathParams can change it between cycles as the fleet's
// activity changes.
func familiarBreathEnvelope(t, cycleDur time.Duration) float64 {
	inhale := time.Duration(float64(cycleDur) * familiarBreathInhaleFraction)
	if t < inhale {
		return breathEase(float64(t) / float64(inhale))
	}
	exhale := cycleDur - inhale
	return 1 - breathEase(float64(t-inhale)/float64(exhale))
}

// familiarBreathFrame builds one tick's worth of per-motor intensity: a
// soft fill expanding outward from a dial-placed center as amplitude a
// rises, scaled to peak. The dials are built so that their hardware rest
// state (an untouched MIDImix knob reads 0, which is also a fresh
// newMixerControl() channel's zero value) gives the default frame: a fill
// from the hem, both columns equal, wide soft edge. dial[0] (0-1): vertical
// center -- 0 = hem (row 0). dial[1] (0-1): pans from balanced (0) toward
// the right column as it increases. This is one-sided because a unipolar
// 0-127 knob has one rest position; a true left<->right pan would put that
// rest position at one extreme instead of at center. dial[2] (0-1):
// interpolates the soft edge from the wide fill (familiarBreathEdgeWidth)
// toward a narrow, snappier point (familiarBreathNarrowEdgeWidth).
func familiarBreathFrame(a float64, peak byte, dial [3]float64) map[int]byte {
	center := dial[0] * float64(familiarBreathRows-1)
	edgeWidth := familiarBreathEdgeWidth - dial[2]*(familiarBreathEdgeWidth-familiarBreathNarrowEdgeWidth)
	leftGain, rightGain := panGains(dial[1])

	// The fill's own multiplier stays fixed at the wide edge width
	// regardless of dial[2] or dial[0] -- at the narrow edge and an
	// off-center placement, a=1 still needs to comfortably overshoot the
	// farthest row's threshold so "full amplitude lights every row" holds
	// everywhere, not just at the defaults.
	fill := a * (float64(familiarBreathRows) + familiarBreathEdgeWidth)
	active := make(map[int]byte, familiarBreathRows*2)
	for row := 0; row < familiarBreathRows; row++ {
		dist := math.Abs(float64(row) - center)
		level := (fill - dist) / edgeWidth
		switch {
		case level < 0:
			level = 0
		case level > 1:
			level = 1
		}
		active[familiarBreathLeftBottom-row] = byte(level * float64(peak) * leftGain)
		active[familiarBreathRightBottom-row] = byte(level * float64(peak) * rightGain)
	}
	return active
}

// panGains maps dial[1] (0 = balanced, matching the hardware rest state;
// 1 = fully panned right)
// to the left/right column gains. Unity on both sides at 0 is load-bearing:
// an equal-power law (constant leftGain^2+rightGain^2) would dip both
// sides below unity even at rest.
func panGains(pan float64) (left, right float64) {
	return 1 - pan, 1 + pan
}

// fleetBreathChannel is the fleet-breath vestChannel. One instance, held
// by mixerChannels (mixer.go). RenderVest is called once per mixer tick;
// cycleDur/peak are latched once per breath cycle (at the boundary), not
// recomputed every tick, so a single breath's shape never jumps mid-cycle
// even if the fleet's working count changes while it's playing.
type fleetBreathChannel struct {
	fwh *fleetWorkingHandle

	started    bool
	cycleStart time.Time
	cycleDur   time.Duration
	peak       byte
}

func newFleetBreathChannel(fwh *fleetWorkingHandle) *fleetBreathChannel {
	return &fleetBreathChannel{fwh: fwh}
}

func (c *fleetBreathChannel) Name() string { return "fleet-breath" }

// RenderVest implements vestChannel (mixer.go). dial comes from the live
// MIDImix snapshot blend() passes in, not from anything this channel
// stores -- see familiarBreathFrame for what each of the three values does.
func (c *fleetBreathChannel) RenderVest(now time.Time, dial [3]float64) map[int]byte {
	if !c.started || now.Sub(c.cycleStart) >= c.cycleDur {
		c.cycleDur, c.peak = fleetBreathParams(c.fwh.get())
		c.cycleStart = now
		c.started = true
	}
	a := familiarBreathEnvelope(now.Sub(c.cycleStart), c.cycleDur)
	return familiarBreathFrame(a, c.peak, dial)
}
