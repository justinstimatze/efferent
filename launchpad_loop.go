// launchpad_loop.go owns the Launchpad's connect/reconnect lifecycle. The
// top control row belongs to negotiation.go's panel; the grid belongs to the
// fleet panel (fleet_panel.go).
package main

import (
	"log"
	"time"
)

// launchpadRetryInterval matches vestRetryInterval (vest_loop.go) -- same
// convention for optional, sometimes-absent hardware.
const launchpadRetryInterval = 90 * time.Second

// launchpadSupervisor owns the Launchpad's connect/reconnect lifecycle --
// mirrors vestLoop's shape (vest_loop.go): connect, run both consumer
// loops until the device disconnects, close, retry on a slow ticker.
// A disconnect after startup (or the device not being plugged in yet) is
// recoverable without a service restart, so the Launchpad can be unplugged
// for other MIDI use. ConnectLaunchpad is already bounded
// by launchpadConnectTimeout (launchpad.go), so calling it repeatedly here
// can't wedge this goroutine the way an unbounded retry against a stuck
// substream open could. Takes fwh (fleet_panel.go) only to hand it down to
// fleetLoop, which publishes the live working-session count through it for
// the fleet-breath mixer channel (familiars_breath.go) to read.
func launchpadSupervisor(fwh *fleetWorkingHandle) {
	for {
		lp, err := ConnectLaunchpad()
		if err != nil {
			log.Printf("efferent: launchpad not available (%v), retrying in %s", err, launchpadRetryInterval)
			time.Sleep(launchpadRetryInterval)
			continue
		}
		log.Printf("efferent: launchpad connected, negotiation panel and fleet panel booting")
		lp.Off() // fleetLoop's boot() (fleet_panel.go) draws the whole surface from a known-blank state
		// Listen() can only be called once per connection (see fleet_panel.go's
		// header) -- demux the one event stream into the negotiation panel's
		// control events and the fleet panel's grid events so both loops can
		// run as separate, independently-tested goroutines.
		control, grid := demuxLaunchpadEvents(lp.Listen())
		done := make(chan struct{}, 2)
		go func() { negotiationLoop(lp, control); done <- struct{}{} }()
		go func() { fleetLoop(lp, grid, fwh); done <- struct{}{} }()
		<-done // either loop noticing the closed event stream is enough to know the device is gone
		lp.Close()
		fwh.reset() // the fleet panel that was publishing this count just stopped -- see fleetWorkingHandle's own doc comment
		log.Printf("efferent: launchpad disconnected, reconnecting")
	}
}
