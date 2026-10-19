// fleet_panel.go is the fleet-health panel's hardware-facing half: the
// boot sequence, the core-column gauge, and the steady-state render/poll/
// refresh loop that owns the Launchpad's grid pads and right-hand column.
// fleet.go holds the hardware-free discovery/state-machine logic this
// drives.
//
// launchpad_loop.go's launchpadSupervisor already owns the daemon's one
// *Launchpad connection and its one Listen() event channel, for
// negotiation.go's control-row panel. Listen() can only be called once
// -- a second call would race two goroutines reading the same fd and
// split the MIDI byte stream. launchpadSupervisor calls
// demuxLaunchpadEvents once on the single real event channel and hands
// negotiationLoop the control half and fleetLoop (below) the grid half, so
// the two features never contend for the read side. Both write to the
// device from separate goroutines, so Launchpad.sendSysEx (launchpad.go)
// is mutex-guarded.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"
)

const (
	fleetRenderTick      = 150 * time.Millisecond
	fleetPollInterval    = 1500 * time.Millisecond
	fleetRefreshInterval = 10 * time.Second
)

// frameFlusher is the one Launchpad capability the fleet panel needs,
// extracted so boot/fleetLoop are unit-testable without real MIDI
// hardware.
type frameFlusher interface {
	Flush(fr *Frame) error
}

// renderCore lights the top-left gauge cluster (corePad, fleet.go): fill
// from fleet-wide transcript growth rate (log-scaled), pulse period
// shortening with it -- the reactor core that visibly changes with
// ship-wide draw. Nothing working = one dim pilot LED. Lives on reserved
// grid pads, not the right-edge control column.
func renderCore(fr *Frame, rate float64, working int, t float64) (lit int, period float64) {
	level := math.Log10(1+rate) / math.Log10(1+20000)
	level = math.Max(0, math.Min(1, level))
	if working == 0 {
		c := scaled(colorPilot, breathe(t, 3.0, 0, 0.4, 1.0))
		fr.Set(padNote(corePad(1)), c[0], c[1], c[2])
		for l := 2; l <= 8; l++ {
			fr.Set(padNote(corePad(l)), 0, 0, 0)
		}
		return 0, 3.0
	}
	lit = int(math.Ceil(level * 8))
	if lit < 1 {
		lit = 1
	}
	period = 2.4 - 1.8*level
	s := breathe(t, period, 0, 0.5, 1.0)
	for l := 1; l <= 8; l++ {
		if l <= lit {
			// upper LEDs run hotter, the cluster reads as a plasma level
			c := scaled(colorCore, s*(0.6+0.4*float64(l)/float64(lit)))
			fr.Set(padNote(corePad(l)), c[0], c[1], c[2])
		} else {
			fr.Set(padNote(corePad(l)), 0, 0, 0)
		}
	}
	return lit, period
}

// ---- Boot ----
//
// Timing is modeled on the boot sequence of the saturday cockpit
// (github.com/justinstimatze/saturday): fixed, deliberately unsorted
// duration sets (not runtime randomness) so no sweep is visible, an
// initial dark beat before anything lights, and stages long enough to
// watch (the first version finished in under 2s and read as too fast).

// bootPadStagger/bootResolveStagger are fixed, deliberately unsorted
// per-pad delays (ms), cycled by pad index -- the cockpit's "no sweep" move,
// so 8 pads lighting up (or resolving) reads as a ripple, not one flash.
var bootPadStagger = []time.Duration{90, 150, 70, 180, 110, 160, 80, 140}
var bootResolveStagger = []time.Duration{170, 120, 210, 90, 190, 140, 230, 100}

// bootCoreStep is the core column's per-LED fill duration (ms), also fixed
// and unsorted, same shape as the cockpit boot's per-bar durations.
var bootCoreStep = []time.Duration{260, 190, 310, 150, 240, 130, 200, 170}

// boot plays the startup ritual once: peripherals (session pads) light
// TESTING red in a staggered ripple, the core column fills bottom-up with
// a real pause at the 70% START gate, flips to CONFIRMED blue, then the
// pads resolve to their real color in a staggered wave. sleep is a
// parameter (not time.Sleep called directly) so tests can pass a no-op.
func boot(lp frameFlusher, fl *fleet, now time.Time, sleep func(time.Duration)) {
	// A dark beat before anything lights, not an instant cut to TESTING.
	sleep(400 * time.Millisecond)

	for i, s := range fl.slots {
		fr := &Frame{}
		fr.Set(padNote(s.row, s.col), colorTest[0], colorTest[1], colorTest[2])
		lp.Flush(fr)
		sleep(bootPadStagger[i%len(bootPadStagger)] * time.Millisecond)
	}
	sleep(650 * time.Millisecond) // hold in TESTING -- a checklist pauses, it doesn't blink

	for l := 1; l <= 8; l++ {
		fr := &Frame{}
		fr.Set(padNote(corePad(l)), colorTest[0], colorTest[1], colorTest[2])
		lp.Flush(fr)
		if l == 6 { // the manual's 70% gate, rounded to a LED
			sleep(1100 * time.Millisecond)
		} else {
			sleep(bootCoreStep[l-1] * time.Millisecond)
		}
	}
	fr := &Frame{}
	for l := 1; l <= 8; l++ {
		fr.Set(padNote(corePad(l)), colorConfirmed[0], colorConfirmed[1], colorConfirmed[2])
	}
	lp.Flush(fr)
	sleep(500 * time.Millisecond)

	for i, s := range fl.slots {
		s.poll(now)
		fr := &Frame{}
		c := s.color(now, 0)
		fr.Set(padNote(s.row, s.col), c[0], c[1], c[2])
		lp.Flush(fr)
		sleep(bootResolveStagger[i%len(bootResolveStagger)] * time.Millisecond)
	}
}

// ---- Wiring: demux and the steady-state loop ----

// demuxLaunchpadEvents reads the Launchpad's single event channel (there
// can only be one live consumer of Listen() -- see this file's header) and
// forwards control-button events (top row/right column/corner) to control
// and grid pad events to grid. Both output channels close when events
// closes.
func demuxLaunchpadEvents(events <-chan LaunchpadEvent) (control, grid <-chan LaunchpadEvent) {
	c := make(chan LaunchpadEvent, 16)
	g := make(chan LaunchpadEvent, 16)
	go func() {
		defer close(c)
		defer close(g)
		for ev := range events {
			switch ev.Type {
			case ControlDown, ControlUp:
				c <- ev
			case PadDown, PadUp:
				g <- ev
			}
		}
	}()
	return c, g
}

// ---- Legend: what does this pad mean right now ----
//
// A button with no legend is not one a person should reasonably press.
// fleetLegendPath is written on every poll tick (1.5s) so a plain `jq`/
// `watch` (or, later, a dedicated TUI) can show "what is pad (row,col)
// right now, and what does CC91 do" before anyone presses anything --
// same statePath convention as hue_bridge.json/agent_state.json, no new
// protocol.

var fleetLegendPath = statePath("efferent", "fleet_legend.json")

type fleetLegendPad struct {
	Row     int    `json:"row"`
	Col     int    `json:"col"`
	Session string `json:"session,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Note    string `json:"note,omitempty"`
}

type fleetLegend struct {
	UpdatedAt   string               `json:"updated_at"`
	ControlRow  []controlDescription `json:"control_row"`  // the top row's 7 negotiation buttons + the 1 still dark, left to right
	LegendPad   string               `json:"legend_pad"`   // what the reserved bottom-right pad does -- also fixed
	OverviewPad string               `json:"overview_pad"` // what the pad next to it does -- also fixed
	GaugePad    string               `json:"gauge_pad"`    // where the fleet-wide gauge lives now -- also fixed
	Pads        []fleetLegendPad     `json:"pads"`
}

// legendPadDescription documents the reserved bottom-right pad's one
// action. Placement is ergonomics, not a protocol constraint: the most
// frequent inputs sit at the bottom right, where a right hand rests on the
// device.
var legendPadDescription = fmt.Sprintf(
	"pad(%d,%d), bottom-right: press to jump the tmux cockpit to the fleet-legend pane you're reading right now, zoomed. Press again to un-zoom. Dim teal, always on -- never a session.",
	legendPadRow, legendPadCol,
)

// overviewPadDescription documents the reserved pad immediately left of
// the legend pad -- same cluster, forces the cockpit back to its tiled
// overview (mirrors saturday-cockpit's own Alt+0).
var overviewPadDescription = fmt.Sprintf(
	"pad(%d,%d), one left of the legend pad: press to un-zoom (if zoomed) and re-tile the whole cockpit -- same as saturday-cockpit's Alt+0. Dim purple, always on -- never a session.",
	overviewPadRow, overviewPadCol,
)

// gaugePadDescription documents the reserved top-left cluster the
// fleet-wide gauge occupies.
var gaugePadDescription = fmt.Sprintf(
	"pads (%d,%d)-(%d,%d), top-left, 8 cells: the reactor-core gauge -- fleet-wide transcript growth, filled live by the daemon on the device itself. Never a session.",
	coreClusterRowLo, coreClusterCol0, coreClusterRowHi, coreClusterCol0+3,
)

// legendNote is the one-line reason a pad looks the way it does --
// mirrors color's own branch order for foreground/stuck/phase, minus the
// sub-250ms confirm-flash/error-tick edges (too fast for a 1.5s-refreshed
// legend to usefully describe).
func legendNote(s *sessionSlot) string {
	switch {
	case s.gone:
		return "gone"
	case s.foreground:
		return "you are here"
	case s.errStreak >= 3:
		return fmt.Sprintf("stuck x%d errored tool calls", s.errStreak)
	case s.phase == sessionPhaseWorking:
		return "working"
	case s.phase == sessionPhaseAwaiting:
		return "waiting on you"
	}
	return "unknown"
}

// writeFleetLegend snapshots fl to fleetLegendPath. Soft-fails (logged,
// not fatal) the same way every other state write in this daemon does --
// a legend a human can't currently read is a degraded experience, not a
// reason to stop driving the actual lights.
func writeFleetLegend(fl *fleet) {
	legend := fleetLegend{
		UpdatedAt:   time.Now().Format(time.RFC3339),
		ControlRow:  negotiationLegendEntries(),
		LegendPad:   legendPadDescription,
		OverviewPad: overviewPadDescription,
		GaugePad:    gaugePadDescription,
	}
	for _, s := range fl.slots {
		legend.Pads = append(legend.Pads, fleetLegendPad{
			Row: s.row, Col: s.col, Session: s.short(), Phase: s.phase.String(), Note: legendNote(s),
		})
	}
	data, err := json.MarshalIndent(legend, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(fleetLegendPath, data, 0600); err != nil {
		log.Printf("efferent: fleet: writing legend: %v", err)
	}
}

// fleetWorkingHandle is the small mutex-guarded handle for the fleet's
// live working-session count -- fleetLoop already computes this once per
// render tick for the core gauge; publishing it here lets a second,
// independent goroutine (the fleet-breath mixer channel, familiars_breath.go)
// read the same number without re-deriving it or coupling directly to
// fleetLoop. Modeled on vestHandle/hueHandle's get/set-behind-a-mutex
// shape, but unlike a bare int copy, it carries its own invalidity: the
// Launchpad is shared with music and gets unplugged routinely
// (launchpad-shared-with-music.md), which stops fleetLoop and would
// otherwise leave working frozen at whatever it last was -- fleet-breath
// would then keep pulsing at that stale rate/intensity forever, the worst
// failure mode for a signal whose entire value is being trustworthy.
// reset() (called from launchpadSupervisor on disconnect) clears it; get()
// reports 0 either way (fleetBreathParams already treats <=0 as the
// slowest/dimmest end, the safe direction to degrade toward), while
// validNow() keeps "known zero" and "unknown" distinguishable for any
// future consumer that cares about the difference. The zero-value struct
// (as dispatch.go constructs it) is already valid=false, so a fresh handle
// defaults to "unknown, assume calm" for free before the Launchpad ever
// connects the first time.
type fleetWorkingHandle struct {
	mu      sync.Mutex
	working int
	valid   bool
}

func (h *fleetWorkingHandle) set(n int) {
	h.mu.Lock()
	h.working = n
	h.valid = true
	h.mu.Unlock()
}

// reset marks the count as no longer current -- call when whatever was
// publishing it stops (the Launchpad disconnects), so a stale number
// doesn't outlive the connection that produced it.
func (h *fleetWorkingHandle) reset() {
	h.mu.Lock()
	h.working = 0
	h.valid = false
	h.mu.Unlock()
}

func (h *fleetWorkingHandle) get() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.working
}

// validNow reports whether the working count currently reflects a live
// Launchpad connection, as opposed to the post-reset/never-set default.
func (h *fleetWorkingHandle) validNow() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.valid
}

// fleetLoop owns the fleet-health grid and core column: boots once, then
// on a single 150ms tick renders every pad, gated-checks a 1.5s transcript
// poll and a 10s cockpit/membership refresh inside that same tick (one
// ticker with elapsed-time gates, not three independent tickers). gridEvents
// is demuxLaunchpadEvents's grid half; PadDown on a session pad jumps the
// cockpit there. Logging is sparse -- arrivals and phase edges only,
// nothing on every render tick.
func fleetLoop(lp frameFlusher, gridEvents <-chan LaunchpadEvent, fwh *fleetWorkingHandle) {
	root := claudeProjectsRoot()
	ck := readCockpit()
	fl := newFleet(discoverActive(root, fleetActiveWindow), ck)
	log.Printf("efferent: fleet: %d active session(s), %d in cockpit", len(fl.slots), len(ck.order))

	start := time.Now()
	boot(lp, fl, start, time.Sleep)
	reservedFr := &Frame{}
	reservedFr.Set(padNote(legendPadRow, legendPadCol), colorLegend[0], colorLegend[1], colorLegend[2])
	reservedFr.Set(padNote(overviewPadRow, overviewPadCol), colorOverview[0], colorOverview[1], colorOverview[2])
	lp.Flush(reservedFr) // steady, not part of the per-tick render -- fl.place never assigns a session to either
	log.Printf("efferent: fleet: steady state (press a cockpit pad to jump there; bottom-right pads: legend, overview)")
	writeFleetLegend(fl) // so the legend exists immediately, not only after the first poll tick

	tick := time.NewTicker(fleetRenderTick)
	defer tick.Stop()
	lastPoll, lastRefresh := start, start
	var rate float64 // EMA bytes/s, tau ~8s

	for {
		select {
		case ev, ok := <-gridEvents:
			if !ok {
				log.Printf("efferent: fleet: launchpad disconnected")
				return
			}
			if ev.Type == PadDown {
				switch {
				case ev.Row == legendPadRow && ev.Col == legendPadCol:
					jumpToLegendPane()
				case ev.Row == overviewPadRow && ev.Col == overviewPadCol:
					jumpToCockpitOverview()
				default:
					jumpToSession(fl, ev.Row, ev.Col)
				}
			}
		case now := <-tick.C:
			if now.Sub(lastRefresh) >= fleetRefreshInterval {
				lastRefresh = now
				ck = readCockpit()
				for _, s := range fl.refresh(discoverActive(root, fleetActiveWindow), ck) {
					log.Printf("efferent: fleet: arrived %s -> pad(%d,%d)", s.short(), s.row, s.col)
					fr := &Frame{}
					fr.Set(padNote(s.row, s.col), colorTest[0], colorTest[1], colorTest[2])
					lp.Flush(fr)
					time.Sleep(200 * time.Millisecond) // mini spool-up for a newcomer
					s.poll(now)
				}
			}
			if now.Sub(lastPoll) >= fleetPollInterval {
				dt := now.Sub(lastPoll).Seconds()
				lastPoll = now
				var grew int64
				for _, s := range fl.slots {
					if s.gone {
						continue
					}
					before := s.phase
					grew += s.poll(now)
					if before != s.phase && before != sessionPhaseUnknown {
						log.Printf("efferent: fleet: %s: %s -> %s", s.short(), before, s.phase)
					}
				}
				inst := float64(grew) / dt
				a := math.Min(1, dt/8.0)
				rate = rate*(1-a) + inst*a
				writeFleetLegend(fl)
			}

			t := now.Sub(start).Seconds()
			working := 0
			fr := &Frame{}
			for _, s := range fl.slots {
				if !s.gone && !s.foreground && s.phase == sessionPhaseWorking {
					working++
				}
				c := s.color(now, t)
				fr.Set(padNote(s.row, s.col), c[0], c[1], c[2])
			}
			fwh.set(working)
			renderCore(fr, rate, working, t)
			if err := lp.Flush(fr); err != nil {
				log.Printf("efferent: fleet: write: %v", err)
				return
			}
		}
	}
}
