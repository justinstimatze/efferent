// negotiation.go wires the Launchpad's top control row as a negotiation
// panel where every pad is an answer or a dial, never a readout: a hold
// toggle, a four-pad self-report row, and a two-pad turn-rating pair
// (CC91-97). CC98 stays dark, reserved.
//
// Nothing reads the files these buttons write yet; they only record.
// Consuming the hold file and turn ratings elsewhere, and silencing other
// signals while a hold is active, are not built.
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	holdCC   = 91
	calmCC   = 92
	okCC     = 93
	frayedCC = 94
	doneCC   = 95
	landedCC = 96
	missedCC = 97
)

// negotiationFlashDur is how long a one-shot press (self-report,
// turn-rating) stays brightened before settling back to its idle color --
// visual confirmation the press registered, mirroring the fleet panel's
// own brief spool-up flash for a newly arrived session (fleet_panel.go).
const negotiationFlashDur = 300 * time.Millisecond

// Idle colors are starting values, untuned like the other palette values
// in this codebase. Hold is neutral (a pause control, not a feeling); the
// self-report row loosely follows a red/amber/green severity family;
// turn-rating is a plain positive/negative pair.
var (
	colorHoldIdle   = rgb{20, 20, 20}
	colorHoldActive = rgb{90, 90, 90}
	colorCalm       = rgb{0, 30, 40}
	colorOK         = rgb{0, 40, 0}
	colorFrayed     = rgb{50, 25, 0}
	colorDone       = rgb{25, 25, 25}
	colorLanded     = rgb{0, 45, 0}
	colorMissed     = rgb{45, 0, 0}
)

// controlDescription documents one top-control-row button for the legend
// TUI -- generated from this file's own constants rather than hand-written
// elsewhere, so the legend can't drift out of sync with what these buttons
// actually do.
type controlDescription struct {
	CC          int    `json:"cc"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// negotiationLegendEntries lists every top-control-row button in physical
// left-to-right order (CC91-98), including the one still dark and
// unassigned -- a legend that silently omits a button is worse than one
// that says "reserved."
func negotiationLegendEntries() []controlDescription {
	return []controlDescription{
		{holdCC, "HOLD", "press to toggle a hold -- the agent should finish its current step and not start another. Writes hold.json; no consumer reads it yet. Dim gray idle, bright white while held."},
		{calmCC, "CALM", "self-report: things feel calm. Appends {timestamp, label, afferent snapshot} to labels.jsonl. Dim teal, flashes brighter cyan on press."},
		{okCC, "OK", "self-report: things feel ok. Appends to labels.jsonl. Dim green, flashes brighter green on press."},
		{frayedCC, "FRAYED", "self-report: things feel frayed. Appends to labels.jsonl. Dim amber, flashes brighter amber on press."},
		{doneCC, "DONE", "self-report: this stretch of work feels done. Appends to labels.jsonl. Dim gray, flashes brighter gray on press."},
		{landedCC, "LANDED", "turn rating: the agent's last response landed. Appends {timestamp, rating} to turn_ratings.jsonl. Dim green, flashes brighter green on press."},
		{missedCC, "MISSED", "turn rating: the agent's last response missed. Appends to turn_ratings.jsonl. Dim red, flashes brighter red on press."},
		{98, "----", "reserved, dark -- no assignment."},
	}
}

// negotiationLighter is the one Launchpad capability this panel needs --
// an interface so it is testable without real MIDI hardware.
type negotiationLighter interface {
	LightIndex(index, r, g, b byte) error
}

func light(lp negotiationLighter, cc int, c rgb) {
	if err := lp.LightIndex(byte(cc), c[0], c[1], c[2]); err != nil {
		log.Printf("efferent: negotiation: lighting CC%d: %v", cc, err)
	}
}

// flash briefly brightens a one-shot pad to confirm a press registered,
// then settles it back to idle -- run in its own goroutine so it doesn't
// block the event loop from noticing the next press.
func flash(lp negotiationLighter, cc int, active, idle rgb) {
	light(lp, cc, active)
	time.Sleep(negotiationFlashDur)
	light(lp, cc, idle)
}

type holdState struct {
	Active bool   `json:"active"`
	Since  string `json:"since,omitempty"`
}

func writeHoldState(active bool) {
	s := holdState{Active: active}
	if active {
		s.Since = time.Now().UTC().Format(time.RFC3339)
	}
	writeJSON(statePath("efferent", "hold.json"), s)
}

type selfReportEntry struct {
	Timestamp string         `json:"timestamp"`
	Label     string         `json:"label"`
	Afferent  *afferentState `json:"afferent,omitempty"`
}

func appendSelfReport(label string) {
	entry := selfReportEntry{Timestamp: time.Now().UTC().Format(time.RFC3339), Label: label}
	if state, err := readAfferentState(); err == nil {
		entry.Afferent = &state
	}
	appendJSONLine(statePath("efferent", "labels.jsonl"), entry)
}

type turnRatingEntry struct {
	Timestamp string `json:"timestamp"`
	Rating    string `json:"rating"`
}

func appendTurnRating(rating string) {
	appendJSONLine(statePath("efferent", "turn_ratings.jsonl"), turnRatingEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Rating:    rating,
	})
}

// ensureStateDir creates path's parent directory (0700) if it doesn't exist
// yet, e.g. on a fresh install with the Launchpad connected before Hue is
// ever paired.
func ensureStateDir(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		log.Printf("efferent: negotiation: creating %s: %v", filepath.Dir(path), err)
	}
}

// writeJSON writes via a temp file plus rename, not a direct write --
// os.WriteFile truncates before it writes, so a concurrent reader polling
// the same path (scripts/vest-viz polls vest_frame.json several times a
// second) can catch a truncated or partial file. Rename is atomic on the same
// filesystem, so a reader always sees either the old complete file or the
// new one, never a torn one.
func writeJSON(path string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("efferent: negotiation: marshaling %s: %v", path, err)
		return
	}
	ensureStateDir(path)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		log.Printf("efferent: negotiation: writing %s: %v", path, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("efferent: negotiation: renaming %s: %v", path, err)
	}
}

func appendJSONLine(path string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("efferent: negotiation: marshaling %s: %v", path, err)
		return
	}
	ensureStateDir(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("efferent: negotiation: opening %s: %v", path, err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		log.Printf("efferent: negotiation: writing %s: %v", path, err)
	}
}

// negotiationLoop owns the top control row single-threaded -- only this
// goroutine ever reads or writes holdActive, so it needs no lock. events
// is normally the control half of demuxLaunchpadEvents; taking it as a
// parameter (not calling Listen() internally) is what makes this testable
// without real hardware, the same shape launchpadLoop's pacer handler used.
func negotiationLoop(lp negotiationLighter, events <-chan LaunchpadEvent) {
	light(lp, holdCC, colorHoldIdle)
	light(lp, calmCC, colorCalm)
	light(lp, okCC, colorOK)
	light(lp, frayedCC, colorFrayed)
	light(lp, doneCC, colorDone)
	light(lp, landedCC, colorLanded)
	light(lp, missedCC, colorMissed)

	holdActive := false

	for ev := range events {
		if ev.Type != ControlDown {
			continue
		}
		log.Printf("efferent: negotiation event: %+v", ev) // low-frequency (human button presses), cheap to keep permanently
		switch int(ev.Control) {
		case holdCC:
			holdActive = !holdActive
			writeHoldState(holdActive)
			if holdActive {
				light(lp, holdCC, colorHoldActive)
				log.Printf("efferent: negotiation: hold engaged")
			} else {
				light(lp, holdCC, colorHoldIdle)
				log.Printf("efferent: negotiation: hold released")
			}
		case calmCC:
			appendSelfReport("calm")
			go flash(lp, calmCC, rgb{0, 90, 120}, colorCalm)
		case okCC:
			appendSelfReport("ok")
			go flash(lp, okCC, rgb{0, 120, 0}, colorOK)
		case frayedCC:
			appendSelfReport("frayed")
			go flash(lp, frayedCC, rgb{120, 60, 0}, colorFrayed)
		case doneCC:
			appendSelfReport("done")
			go flash(lp, doneCC, rgb{80, 80, 80}, colorDone)
		case landedCC:
			appendTurnRating("landed")
			go flash(lp, landedCC, rgb{0, 120, 0}, colorLanded)
		case missedCC:
			appendTurnRating("missed")
			go flash(lp, missedCC, rgb{120, 0, 0}, colorMissed)
		}
	}
	log.Printf("efferent: negotiation panel disconnected")
}
