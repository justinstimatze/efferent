package main

import (
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeGattWriter stands in for the vest's real BLE characteristic so
// writeMu's locking behavior (Buzz holding it across a whole burst) is
// testable without real hardware -- records every frame written, in order.
type fakeGattWriter struct {
	mu    sync.Mutex
	calls [][]byte
}

func (f *fakeGattWriter) WriteWithoutResponse(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, slices.Clone(p))
	return len(p), nil
}

func (f *fakeGattWriter) snapshot() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// TestVestBuzzHoldsWriteMuAcrossWholeBurst guards against Buzz releasing
// writeMu between its "on" and "off" writes, which would let a concurrent
// mixer tick land in the 500ms sleep gap and stomp the alert. A concurrent
// writeFrame call must block until Buzz's whole on/sleep/off sequence
// completes.
func TestVestBuzzHoldsWriteMuAcrossWholeBurst(t *testing.T) {
	fw := &fakeGattWriter{}
	v := &Vest{cmdChar: fw, stop: make(chan struct{})}

	const buzzDur = 60 * time.Millisecond
	buzzDone := make(chan struct{})
	go func() {
		defer close(buzzDone)
		if err := v.Buzz([]int{0}, 0xF, buzzDur); err != nil {
			t.Errorf("Buzz: %v", err)
		}
	}()

	time.Sleep(15 * time.Millisecond) // let Buzz issue its "on" write and enter the sleep

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		if err := v.writeFrame(map[int]byte{5: 0x1}); err != nil {
			t.Errorf("writeFrame: %v", err)
		}
	}()

	select {
	case <-writeDone:
		t.Fatal("concurrent writeFrame completed before Buzz released writeMu -- it interleaved mid-burst")
	case <-time.After(25 * time.Millisecond):
	}

	<-buzzDone
	<-writeDone

	calls := fw.snapshot()
	if len(calls) != 3 {
		t.Fatalf("got %d writes, want 3 (buzz-on, buzz-off, the concurrent write): %v", len(calls), calls)
	}
	if isAllZero(calls[0]) {
		t.Errorf("first write = %v, want buzz's nonzero 'on' frame", calls[0])
	}
	if !isAllZero(calls[1]) {
		t.Errorf("second write = %v, want buzz's all-zero 'off' frame, not displaced by the concurrent writer", calls[1])
	}
}

func TestVestSinceLastWriteBeforeAnyWrite(t *testing.T) {
	v := &Vest{stop: make(chan struct{})}
	if got := v.sinceLastWrite(); got < vestKeepalive {
		t.Errorf("sinceLastWrite() before any write = %v, want >= vestKeepalive so a fresh connection still pings once", got)
	}
}

func TestVestSinceLastWriteAfterAWrite(t *testing.T) {
	fw := &fakeGattWriter{}
	v := &Vest{cmdChar: fw, stop: make(chan struct{})}
	if err := v.writeFrame(map[int]byte{0: 0x1}); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	if got := v.sinceLastWrite(); got >= vestKeepalive {
		t.Errorf("sinceLastWrite() right after a write = %v, want well under vestKeepalive (%v) so keepalive skips its own ping", got, vestKeepalive)
	}
}

func TestRampStepsShape(t *testing.T) {
	got := rampSteps(10, 4)
	want := []byte{0, 2, 5, 7, 10, 7, 5, 2, 0}
	if len(got) != len(want) {
		t.Fatalf("len(rampSteps(10, 4)) = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rampSteps(10, 4)[%d] = %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestRampStepsPeaksAtSpecifiedIntensity(t *testing.T) {
	seq := rampSteps(0x8, rampStepsPerCycle)
	max := byte(0)
	for _, v := range seq {
		if v > max {
			max = v
		}
	}
	if max != 0x8 {
		t.Errorf("peak of rampSteps(0x8, %d) = %#x, want %#x", rampStepsPerCycle, max, byte(0x8))
	}
	if seq[0] != 0 || seq[len(seq)-1] != 0 {
		t.Errorf("rampSteps should start and end at 0, got start=%d end=%d", seq[0], seq[len(seq)-1])
	}
}

func TestRampStepsZeroPeak(t *testing.T) {
	seq := rampSteps(0, 5)
	for i, v := range seq {
		if v != 0 {
			t.Errorf("rampSteps(0, 5)[%d] = %d, want 0", i, v)
		}
	}
}

func TestVestWriteFrameClampsToHardCeiling(t *testing.T) {
	fw := &fakeGattWriter{}
	v := &Vest{cmdChar: fw, stop: make(chan struct{})}
	in := map[int]byte{0: 0xF, 1: 0x2}
	if err := v.writeFrame(in); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	if in[0] != 0xF {
		t.Errorf("writeFrame mutated the caller's map: %v", in)
	}
	want := vestFrame(map[int]byte{0: vestHardCeiling, 1: 0x2})
	if got := fw.snapshot(); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Errorf("frames = %v, want one frame %v with motor 0 clamped to %#x", got, want, vestHardCeiling)
	}
}

func TestVestSilenceWritesOffThenBlocksFurtherWrites(t *testing.T) {
	fw := &fakeGattWriter{}
	v := &Vest{cmdChar: fw, stop: make(chan struct{})}
	v.silence()
	if err := v.writeFrame(map[int]byte{0: 0x3}); err != nil {
		t.Fatalf("writeFrame after silence: %v", err)
	}
	calls := fw.snapshot()
	if len(calls) != 1 || !isAllZero(calls[0]) {
		t.Errorf("frames = %v, want exactly one all-zero frame (later writes dropped)", calls)
	}
}
