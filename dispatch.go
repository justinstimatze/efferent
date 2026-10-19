// dispatch.go is the daemon's top-level wiring. It watches afferent's state
// file (fsnotify, with a 60s fallback tick) and, on any change, reads it once
// and drives the vest's fatigue alert from that snapshot. Hue does not read
// afferent state: Machine's Breath breathes around a fixed baseline, and
// setupHue/hueHandle exist only to feed it. Everything else polls
// independently on its own tick rather than through this watch:
// hueBreathChannel (machine_breath.go) polls agenthook's agent_state.json
// from the mixer's tick, and fleetLoop (fleet_panel.go) reads
// ~/.claude/projects and tmux on its own render/poll/refresh ticks and
// publishes the working-session count through fleetWorkingHandle for the
// fleet-breath channel (familiars_breath.go).
package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// silenceOnSignal turns the vest's motors off when the daemon is asked to
// stop (systemctl stop, Ctrl-C), then exits. A vest left mid-frame keeps
// whatever motors were last commanded. The write runs in a goroutine with a
// timeout so a wedged BLE stack cannot block shutdown. A SIGKILL or crash
// cannot be caught; the vest's behavior then is whatever its firmware does.
func silenceOnSignal(vh *vestHandle) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sig
		log.Printf("efferent: %v received, silencing vest and exiting", s)
		done := make(chan struct{})
		go func() { vh.silence(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			log.Printf("efferent: vest silence timed out")
		}
		os.Exit(0)
	}()
}

const fallbackInterval = 60 * time.Second // dead-man's-switch only, not the drive mechanism

// hueBackoffSteps is retryHue's escalating wait between attempts while a
// paired bridge stays unreachable -- checked against fallback's fixed
// 60s tick rather than replacing it, since that tick also drives the
// afferent dispatch fallback and can't be slowed down just because Hue is
// having a bad day. Index N-1 is used after the Nth consecutive failure;
// the last entry repeats as the ceiling.
var hueBackoffSteps = []time.Duration{
	60 * time.Second,
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
}

// hueLogEveryNFailures caps how often a continuing outage re-announces
// itself once it's past the first failure (which setupHue already logs) --
// otherwise a long outage says nothing further until it recovers.
const hueLogEveryNFailures = 10

func runDispatch() {
	hh := &hueHandle{}
	hh.set(setupHue())

	vh := &vestHandle{}
	silenceOnSignal(vh)
	go vestLoop(vh)

	// The mixer (mixer.go) blends channels onto the vest and Hue, controlled
	// from the MIDImix. Strip 0 is fleet-breath (vest), strip 1 is Machine's
	// Breath (Hue). fleetWorkingHandle is threaded down through
	// launchpadSupervisor to fleetLoop, which publishes the live
	// working-session count that fleet-breath reads.
	fwh := &fleetWorkingHandle{}
	mc := newMixerControl()
	mixerChannels[0] = newFleetBreathChannel(fwh)
	mixerChannels[1] = newHueBreathChannel()
	go midimixLoop(mc)
	go mixerLoop(vh, hh, mc)

	go launchpadSupervisor(fwh)

	afferentDir := filepath.Dir(statePath("afferent", "state.json"))
	aw, awErr := NewWatcher(afferentDir, "state.json") // afferent's dir -- may not exist yet, not fatal
	if awErr != nil {
		log.Printf("efferent: afferent state not watchable yet (%v), falling back to the %s tick until it appears", awErr, fallbackInterval)
	} else {
		defer aw.Close()
	}

	fallback := time.NewTicker(fallbackInterval)
	defer fallback.Stop()

	dispatch := func() {
		// Retry establishing the afferent watch on the fallback tick if it
		// never succeeded at startup -- afferent may simply not have run yet.
		if aw == nil {
			if w, err := NewWatcher(afferentDir, "state.json"); err == nil {
				aw = w
				log.Printf("efferent: afferent state now watchable, watch established")
			}
		}
		state, stateErr := readAfferentState()
		applyVest(vh.get(), state, stateErr)
	}

	// retryHue re-attempts setupHue -- only ever called from the slow
	// fallback tick, never from a real afferent event. resolveBridge can
	// block on up to three sequential 4s HTTP timeouts during a genuine
	// outage, and that latency must never leak into the vest's reaction
	// time on a live event -- the whole point of the event-driven refactor
	// was Hue and the vest reacting at the same instant.
	//
	// Backed off separately from the fallback tick itself (hueBackoffSteps):
	// the tick stays fixed at fallbackInterval because it also drives the
	// afferent dispatch fallback, so retryHue just skips most ticks while
	// an outage runs long, rather than the ticker slowing down.
	var hueFailStreak int
	var hueNextRetry, hueOutageStart time.Time
	retryHue := func() {
		if !hh.needsRetry() {
			hueFailStreak = 0
			return
		}
		if now := time.Now(); now.Before(hueNextRetry) {
			return
		}
		if b, ok, retryable := setupHue(); ok {
			hh.set(b, true, false)
			log.Printf("efferent: hue connection recovered")
			hueFailStreak = 0
			return
		} else {
			hh.set(hueBridge{}, false, retryable)
			if hueFailStreak == 0 {
				hueOutageStart = time.Now()
			}
			hueFailStreak++
			step := hueBackoffSteps[min(hueFailStreak, len(hueBackoffSteps))-1]
			hueNextRetry = time.Now().Add(step)
			if hueFailStreak%hueLogEveryNFailures == 0 {
				log.Printf("efferent: hue still unreachable after %d attempts (down %s, next retry in %s)",
					hueFailStreak, time.Since(hueOutageStart).Round(time.Second), step)
			}
		}
	}

	log.Printf("efferent: dispatch loop started (event-driven, %s fallback)", fallbackInterval)
	dispatch() // pick up whatever's already on disk before waiting for the first event
	for {
		var afferentEvents <-chan struct{}
		if aw != nil {
			afferentEvents = aw.Events()
		}
		select {
		case <-afferentEvents:
			dispatch()
		case <-fallback.C:
			retryHue()
			dispatch()
		}
	}
}
