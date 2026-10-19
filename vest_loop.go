// vest_loop.go holds the vest's connect/reconnect lifecycle and its one
// event-triggered signal. The back channel (motors 10-29) fires a sharp
// pulse when afferent's fatigue factor crosses its threshold -- a real
// edge, unlike a continuous quantity such as CO2. The front channel
// (motors 0-9 and 30-39) is currently silent. The suit is optional,
// often-off hardware, so its connect/reconnect lifecycle runs independently
// of Hue and of the dispatch loop that drives it (see dispatch.go).
package main

import (
	"log"
	"sync"
	"time"
)

const (
	// 0x5 of 0xF: routine intensities well below the 0xF maximum, which
	// vest-protocol.md records as mildly uncomfortable. Higher routine
	// values (0x8-0xA) read as too attention-getting when worn for a whole
	// session. Tuned by feel, not measured.
	vestIntensity      byte          = 0x5
	vestBuzzDuration   time.Duration = 500 * time.Millisecond
	vestRetryInterval  time.Duration = 90 * time.Second
	fatigueFactorLabel string        = "session_over_fatigue_threshold"
)

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// vestEdge tracks the per-connection edge-trigger state -- a rising edge
// only, never a steady state, so the suit stays silent except when
// something actually changed.
type vestEdge struct {
	lastFatigued bool
}

// seedVestEdge reads afferent's state once and returns the edge state a
// freshly connected vest should start from, so a reconnect syncs silently
// instead of replaying pre-existing state as a fresh transition.
func seedVestEdge() vestEdge {
	var e vestEdge
	if state, err := readAfferentState(); err == nil {
		e.lastFatigued = contains(state.Factors, fatigueFactorLabel)
	}
	return e
}

// buzzer is the vest capability applyVest and the mixer need, extracted so
// they can be unit-tested without real BLE hardware -- *Vest satisfies
// this. writeFrame exists because a spatial fill needs different
// intensities on different motors in the same frame, which neither Buzz nor
// Ramp can express.
type buzzer interface {
	Buzz(motors []int, intensity byte, dur time.Duration) error
	Ramp(motors []int, peak byte, cycleDur time.Duration, cycles int, stop <-chan struct{}) error
	writeFrame(active map[int]byte) error
}

// vestConn bundles a live connection with its edge state. edge is written
// only by seedVestEdge (before the conn is published via vestHandle.set)
// and thereafter only by applyVest, which always runs on the single
// dispatch goroutine -- so edge itself needs no lock of its own, only the
// handle publishing the pointer does.
type vestConn struct {
	v    buzzer
	edge vestEdge
	dead chan struct{}
	once sync.Once
}

// markDead tells vestLoop's reconnect goroutine this connection is bad.
func (c *vestConn) markDead() { c.once.Do(func() { close(c.dead) }) }

// vestHandle is the small mutex-guarded handle the dispatch loop reads to
// find out whether -- and to which connection -- it can currently buzz.
// A nil connection is the vest-equivalent of Hue's !hueOK: dispatch skips
// vest actuation gracefully. Always go through set/get, never touch the
// field directly, so the connection is never observed half-updated.
type vestHandle struct {
	mu sync.Mutex
	c  *vestConn
}

func (h *vestHandle) set(c *vestConn) {
	h.mu.Lock()
	h.c = c
	h.mu.Unlock()
}

// silence turns every motor off and blocks further vest writes. A no-op
// when no vest is connected or the connection is a test fake.
func (h *vestHandle) silence() {
	if c := h.get(); c != nil {
		if v, ok := c.v.(*Vest); ok {
			v.silence()
		}
	}
}

func (h *vestHandle) get() *vestConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.c
}

// vestLoop manages the vest's independent connect/reconnect lifecycle and
// publishes the current connection (or nil) via vh. It no longer polls or
// buzzes anything itself -- that's the dispatch loop's job via applyVest.
// Re-seeds on every successful connect, not just the first: ConnectVest is
// unbonded and rescans from scratch on every reconnect, so a mid-session
// disconnect/reconnect is a real, expected occurrence, and re-seeding each
// time is what keeps a reconnect syncing silently instead of replaying
// whatever changed while the vest was off as a spurious buzz.
func vestLoop(vh *vestHandle) {
	for {
		v, err := ConnectVest()
		if err != nil {
			log.Printf("vest: not available (%v), retrying in %s", err, vestRetryInterval)
			time.Sleep(vestRetryInterval)
			continue
		}
		log.Printf("vest: connected, driving front/back channels")
		conn := &vestConn{v: v, edge: seedVestEdge(), dead: make(chan struct{})}
		vh.set(conn)

		<-conn.dead // blocks until applyVest reports a write failure

		vh.set(nil)
		v.Close()
		log.Printf("vest: lost connection, reconnecting")
	}
}

// applyVest checks the back channel's transitions against a fresh snapshot
// and buzzes accordingly. conn == nil (vest not currently connected) is a
// graceful no-op, exactly like Hue being unavailable.
//
// A sharp discrete pulse is the right primitive for a real edge like
// fatigue crossing its threshold. CO2 is not wired here: it is a
// continuous quantity with no real edge (a bucket boundary is a line drawn
// on a smooth curve), so a pulse on it read as attention-getting however
// soft it was made (signal-vocabulary.md).
func applyVest(conn *vestConn, state afferentState, stateErr error) {
	if conn == nil || stateErr != nil {
		return
	}

	fatigued := contains(state.Factors, fatigueFactorLabel)
	if fatigued && !conn.edge.lastFatigued {
		log.Printf("vest: back channel, fatigue threshold crossed")
		if err := conn.v.Buzz(vestBack, vestIntensity, vestBuzzDuration); err != nil {
			log.Printf("vest: lost connection (%v)", err)
			conn.markDead()
			return
		}
	}
	conn.edge.lastFatigued = fatigued
}
