package main

import (
	"testing"
	"time"
)

func TestBreathEaseBoundaries(t *testing.T) {
	if got := breathEase(0); got != 0 {
		t.Errorf("breathEase(0) = %v, want 0", got)
	}
	if got := breathEase(1); got != 1 {
		t.Errorf("breathEase(1) = %v, want 1", got)
	}
	if got := breathEase(0.5); got < 0.499 || got > 0.501 {
		t.Errorf("breathEase(0.5) = %v, want ~0.5", got)
	}
}

func TestBreathEaseMonotonic(t *testing.T) {
	prev := -1.0
	for i := 0; i <= 10; i++ {
		t2 := float64(i) / 10
		got := breathEase(t2)
		if got < prev {
			t.Fatalf("breathEase not monotonic at t=%v: %v < previous %v", t2, got, prev)
		}
		prev = got
	}
}

func TestFamiliarBreathEnvelopeStartsAndEndsAtZero(t *testing.T) {
	const cycle = 10 * time.Second
	if got := familiarBreathEnvelope(0, cycle); got != 0 {
		t.Errorf("envelope(0) = %v, want 0", got)
	}
	end := cycle - time.Millisecond
	if got := familiarBreathEnvelope(end, cycle); got > 0.05 {
		t.Errorf("envelope(cycle-1ms) = %v, want near 0", got)
	}
}

func TestFamiliarBreathEnvelopePeaksAtInhaleBoundary(t *testing.T) {
	const cycle = 10 * time.Second
	inhale := time.Duration(float64(cycle) * familiarBreathInhaleFraction)
	if got := familiarBreathEnvelope(inhale, cycle); got < 0.999 {
		t.Errorf("envelope(inhale boundary) = %v, want ~1 (the peak)", got)
	}
}

func TestFamiliarBreathEnvelopeAsymmetric(t *testing.T) {
	// Inhale is the shorter leg (40%) -- halfway through elapsed time
	// overall should already be well past the peak, descending.
	const cycle = 10 * time.Second
	got := familiarBreathEnvelope(cycle/2, cycle)
	if got >= 1 {
		t.Errorf("envelope(cycle/2) = %v, want < 1 (already descending on the longer exhale leg)", got)
	}
}

func TestFamiliarBreathEnvelopeScalesWithCycleDur(t *testing.T) {
	// The same fractional position within a short cycle and a long cycle
	// should land at the same amplitude -- decouples shape from length,
	// the reason to drive the envelope off elapsed time rather than a fixed
	// step count.
	short := familiarBreathEnvelope(3*time.Second, 6*time.Second)
	long := familiarBreathEnvelope(7*time.Second, 14*time.Second)
	if diff := short - long; diff > 0.001 || diff < -0.001 {
		t.Errorf("envelope at the same fractional position differs by cycle length: %v vs %v", short, long)
	}
}

func TestFleetBreathParamsZeroWorkingIsSlowestAndDimmest(t *testing.T) {
	cycle, peak := fleetBreathParams(0)
	if cycle != familiarBreathCycleMax {
		t.Errorf("cycle at 0 working = %v, want the max (slowest) %v", cycle, familiarBreathCycleMax)
	}
	if peak != familiarBreathPeakMin {
		t.Errorf("peak at 0 working = %v, want the min %v", peak, familiarBreathPeakMin)
	}
}

func TestFleetBreathParamsSaturatesAtCeiling(t *testing.T) {
	atCeiling, peakAtCeiling := fleetBreathParams(fleetBreathSaturateAt)
	beyond, peakBeyond := fleetBreathParams(fleetBreathSaturateAt * 10)
	if atCeiling != beyond || peakAtCeiling != peakBeyond {
		t.Errorf("params should saturate at fleetBreathSaturateAt: (%v,%v) at ceiling vs (%v,%v) beyond it", atCeiling, peakAtCeiling, beyond, peakBeyond)
	}
	if atCeiling != familiarBreathCycleMin {
		t.Errorf("cycle at saturation = %v, want the min (fastest) %v", atCeiling, familiarBreathCycleMin)
	}
	if peakAtCeiling != familiarBreathPeakMax {
		t.Errorf("peak at saturation = %v, want the max %v", peakAtCeiling, familiarBreathPeakMax)
	}
}

func TestFleetBreathParamsNegativeWorkingClampsToZero(t *testing.T) {
	// Defensive: fwh.get() should never actually return negative, but a
	// stray -1 shouldn't produce a faster-than-slowest cycle via an
	// out-of-range fraction.
	got, _ := fleetBreathParams(-1)
	if got != familiarBreathCycleMax {
		t.Errorf("cycle at working=-1 = %v, want the same as working=0 (%v)", got, familiarBreathCycleMax)
	}
}

// zeroDial is the untouched-knob hardware default (mixer.go's
// newMixerControl never sets Dial, so a fresh channelControl's Dial is
// [0,0,0]) -- deliberately chosen, in familiarBreathFrame, to be the value
// that gives the default frame (hem fill, balanced, wide edge).
var zeroDial [3]float64

func TestFamiliarBreathFrameZeroAmplitudeIsAllDark(t *testing.T) {
	frame := familiarBreathFrame(0, familiarBreathPeakMax, zeroDial)
	for _, v := range frame {
		if v != 0 {
			t.Errorf("frame at a=0 has a nonzero motor: %+v", frame)
			break
		}
	}
}

func TestFamiliarBreathFrameFullAmplitudeLightsEveryRow(t *testing.T) {
	frame := familiarBreathFrame(1, familiarBreathPeakMax, zeroDial)
	for row := 0; row < familiarBreathRows; row++ {
		left := familiarBreathLeftBottom - row
		right := familiarBreathRightBottom - row
		if frame[left] != familiarBreathPeakMax {
			t.Errorf("row %d left motor %d = %v, want peak %v", row, left, frame[left], familiarBreathPeakMax)
		}
		if frame[right] != familiarBreathPeakMax {
			t.Errorf("row %d right motor %d = %v, want peak %v", row, right, frame[right], familiarBreathPeakMax)
		}
	}
}

func TestFamiliarBreathFrameMidAmplitudeIsGradedNotBinary(t *testing.T) {
	// The defect a narrow edge produces: most
	// rows sat at either fully-on or fully-off, with only a sliver of the
	// column ever at an intermediate value. A wide edge should put a real
	// gradient across the column at a mid-cycle amplitude, easing from
	// hem to shoulder rather than jumping between the two extremes.
	frame := familiarBreathFrame(0.5, 4, zeroDial)
	sawIntermediate := false
	prev := byte(255)
	for row := 0; row < familiarBreathRows; row++ {
		v := frame[familiarBreathLeftBottom-row]
		if v > 0 && v < 4 {
			sawIntermediate = true
		}
		if v > prev {
			t.Errorf("row %d = %v is brighter than the row below it (%v) -- fill should ease down from hem to shoulder", row, v, prev)
		}
		prev = v
	}
	if !sawIntermediate {
		t.Error("want at least one row at an intermediate intensity at a=0.5, got only fully-on/fully-off rows")
	}
}

func TestFamiliarBreathFrameBottomRowLitFirst(t *testing.T) {
	// A small amplitude should light only the bottom-most row(s), not the
	// shoulder -- confirms the fill direction is hem-up, not shoulder-down.
	frame := familiarBreathFrame(0.2, 4, zeroDial)
	if frame[familiarBreathLeftBottom] == 0 {
		t.Error("bottom row should be the first to light as amplitude rises from zero")
	}
	if frame[familiarBreathLeftBottom-familiarBreathRows+1] != 0 {
		t.Error("top row (shoulder) should still be dark at low amplitude")
	}
}

// TestFamiliarBreathFrameDialCenterShiftsFillOrigin guards its core
// promise: dial[0] moves where the fill starts from, inverting
// BottomRowLitFirst's own low-amplitude case when the center is placed at
// the shoulder instead of the hem.
func TestFamiliarBreathFrameDialCenterShiftsFillOrigin(t *testing.T) {
	dial := [3]float64{1, 0, 0} // center at the shoulder (row familiarBreathRows-1)
	frame := familiarBreathFrame(0.2, 4, dial)
	if frame[familiarBreathLeftBottom-familiarBreathRows+1] == 0 {
		t.Error("top row (shoulder) should be first to light when dial[0] places the center there")
	}
	if frame[familiarBreathLeftBottom] != 0 {
		t.Error("bottom row (hem) should still be dark at low amplitude with the center at the shoulder")
	}
}

// TestFamiliarBreathFrameZeroDialPansNeitherSide guards the hardware-rest
// backward-compat claim directly: dial[1]=0 must give identical left/right
// intensity.
func TestFamiliarBreathFrameZeroDialPansNeitherSide(t *testing.T) {
	frame := familiarBreathFrame(1, familiarBreathPeakMax, zeroDial)
	if frame[familiarBreathLeftBottom] != frame[familiarBreathRightBottom] {
		t.Errorf("left %v != right %v at dial[1]=0, want equal (balanced, the hardware rest state)", frame[familiarBreathLeftBottom], frame[familiarBreathRightBottom])
	}
}

// TestFamiliarBreathFramePanFavorsRightColumn guards dial[1]'s one-sided pan:
// turning it up must darken the left column relative to the right, never
// the reverse (there's no left-pan direction on this one dial -- see
// familiarBreathFrame's doc comment for why).
func TestFamiliarBreathFramePanFavorsRightColumn(t *testing.T) {
	dial := [3]float64{0, 1, 0} // fully panned right
	frame := familiarBreathFrame(1, familiarBreathPeakMax, dial)
	if frame[familiarBreathLeftBottom] != 0 {
		t.Errorf("left = %v at dial[1]=1 (fully panned right), want 0", frame[familiarBreathLeftBottom])
	}
	if frame[familiarBreathRightBottom] == 0 {
		t.Error("right should still be lit at dial[1]=1")
	}
}

// TestFamiliarBreathFrameNarrowSpreadLightsFewerRows guards dial[2]: turning
// it up should narrow the soft edge, leaving fewer rows at an intermediate
// (neither fully-on nor fully-off) intensity than the wide default does at
// the same mid-cycle amplitude.
func TestFamiliarBreathFrameNarrowSpreadLightsFewerRows(t *testing.T) {
	countIntermediate := func(dial [3]float64) int {
		frame := familiarBreathFrame(0.5, familiarBreathPeakMax, dial)
		n := 0
		for row := 0; row < familiarBreathRows; row++ {
			v := frame[familiarBreathLeftBottom-row]
			if v > 0 && v < familiarBreathPeakMax {
				n++
			}
		}
		return n
	}
	wide := countIntermediate(zeroDial)
	narrow := countIntermediate([3]float64{0, 0, 1})
	if narrow >= wide {
		t.Errorf("intermediate rows at dial[2]=1 (narrow) = %d, want fewer than at dial[2]=0 (wide) = %d", narrow, wide)
	}
}

func TestFleetBreathChannelRenderLatchesOncePerCycle(t *testing.T) {
	fwh := &fleetWorkingHandle{}
	fwh.set(0) // slowest/dimmest: familiarBreathCycleMax, familiarBreathPeakMin
	c := newFleetBreathChannel(fwh)

	start := time.Now()
	c.RenderVest(start, zeroDial) // latches cycleDur/peak for this cycle

	// Change the fleet's working count mid-cycle -- the in-flight cycle's
	// own shape shouldn't jump, per this file's "latched once per cycle"
	// discipline.
	fwh.set(fleetBreathSaturateAt)
	midCycle := start.Add(familiarBreathCycleMax / 2)
	c.RenderVest(midCycle, zeroDial)
	if c.cycleDur != familiarBreathCycleMax {
		t.Errorf("cycleDur changed mid-cycle: got %v, want the latched %v", c.cycleDur, familiarBreathCycleMax)
	}

	// Past the cycle boundary, the new fleet count should take effect.
	nextCycle := start.Add(familiarBreathCycleMax + time.Second)
	c.RenderVest(nextCycle, zeroDial)
	if c.cycleDur != familiarBreathCycleMin {
		t.Errorf("cycleDur after the cycle boundary = %v, want the now-current %v", c.cycleDur, familiarBreathCycleMin)
	}
}

func TestFleetBreathChannelName(t *testing.T) {
	c := newFleetBreathChannel(&fleetWorkingHandle{})
	if got := c.Name(); got != "fleet-breath" {
		t.Errorf("Name() = %q, want %q", got, "fleet-breath")
	}
}
