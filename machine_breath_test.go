package main

import (
	"testing"
	"time"
)

func TestBreathHalfCycleForPhaseWorking(t *testing.T) {
	if got := breathHalfCycleForPhase("working"); got != breathBusyHalfCycle {
		t.Errorf("breathHalfCycleForPhase(%q) = %v, want %v", "working", got, breathBusyHalfCycle)
	}
}

func TestBreathHalfCycleForPhaseAwaitingUser(t *testing.T) {
	if got := breathHalfCycleForPhase("awaiting_user"); got != breathIdleHalfCycle {
		t.Errorf("breathHalfCycleForPhase(%q) = %v, want %v", "awaiting_user", got, breathIdleHalfCycle)
	}
}

func TestBreathHalfCycleForPhaseZeroValueIsIdle(t *testing.T) {
	if got := breathHalfCycleForPhase(""); got != breathIdleHalfCycle {
		t.Errorf("breathHalfCycleForPhase(\"\") = %v, want %v (idle -- no live turn, not an error)", got, breathIdleHalfCycle)
	}
}

func TestBreathHalfCycleForPhaseUnknownIsIdle(t *testing.T) {
	if got := breathHalfCycleForPhase("some_future_phase"); got != breathIdleHalfCycle {
		t.Errorf("breathHalfCycleForPhase(unknown) = %v, want %v (idle default)", got, breathIdleHalfCycle)
	}
}

func TestDeciseconds(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want int
	}{
		{6 * time.Second, 60},
		{1500 * time.Millisecond, 15},
		{50 * time.Millisecond, 1}, // floors at 1, never 0 ("instant snap")
		{0, 1},
	}
	for _, c := range cases {
		if got := deciseconds(c.d); got != c.want {
			t.Errorf("deciseconds(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

func TestClampBri(t *testing.T) {
	cases := []struct{ in, want int }{
		{-10, 1},
		{0, 1},
		{1, 1},
		{130, 130},
		{254, 254},
		{300, 254},
	}
	for _, c := range cases {
		if got := clampBri(c.in); got != c.want {
			t.Errorf("clampBri(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
