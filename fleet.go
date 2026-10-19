// fleet.go is the fleet-health panel's pure discovery and per-session
// state -- no hardware, nothing in this file touches the Launchpad. Phase
// is read from the transcript's stop_reason rather than file growth (a
// finished turn ends with end_turn; growth alone cannot tell "thinking"
// from "done"), subagent sidecars count toward their parent session, and
// pad positions mirror the tmux cockpit's pane order.
//
// sessionPhase is deliberately not named phase/phaseWorking: machine_breath.go
// already has its own turn-phase concept (agentPhaseWorking, agentState.Phase,
// sourced from agenthook's single-session hook state -- a different,
// two-valued, globally-scoped signal). This one is per-session,
// three-valued, and derived straight from each transcript's tail. Same
// English word, different mechanism; distinct names keep that honest.
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---- Transcript tail state machine ----

type sessionPhase int

const (
	sessionPhaseUnknown sessionPhase = iota
	sessionPhaseWorking
	sessionPhaseAwaiting
)

func (p sessionPhase) String() string {
	switch p {
	case sessionPhaseWorking:
		return "working"
	case sessionPhaseAwaiting:
		return "waiting"
	}
	return "unknown"
}

type rec struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		StopReason string          `json:"stop_reason"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type    string `json:"type"`
	IsError bool   `json:"is_error"`
}

const tailBytes = 256 * 1024

// tstate is one transcript's tail-derived state -- the parent session file
// or one of its subagent sidecars.
type tstate struct {
	path          string
	sub           bool // a subagent sidecar, whose final report is a text record with no end_turn
	size          int64
	phase         sessionPhase
	awaitingSince time.Time // from the end_turn record's own timestamp
	errStreak     int
	lastErrAt     time.Time
	lastGrowth    time.Time
}

// scanTail reads the last tailBytes of the transcript (never the whole
// file -- some are 1GB+) and derives phase, waiting-since, and the current
// consecutive-error streak from the message records in it. Meta records
// (system, attachment, pr-link, ...) are ignored: they're appended after a
// turn ends and would read as "busy" if counted.
func (ts *tstate) scanTail() {
	f, err := os.Open(ts.path)
	if err != nil {
		return
	}
	defer f.Close()
	start := ts.size - tailBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, ts.size-start)
	n, _ := f.ReadAt(buf, start)
	lines := bytes.Split(buf[:n], []byte("\n"))
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // first line is a partial record
	}

	var lastType, lastStop string
	var lastTS, lastErr time.Time
	var lastText bool
	streak, found := 0, false
	for _, ln := range lines {
		if len(ln) == 0 {
			continue
		}
		var r rec
		if json.Unmarshal(ln, &r) != nil {
			continue
		}
		switch r.Type {
		case "assistant":
			found = true
			lastType, lastStop = "assistant", r.Message.StopReason
			lastTS, _ = time.Parse(time.RFC3339Nano, r.Timestamp)
			lastText = false
			var blocks []block
			if json.Unmarshal(r.Message.Content, &blocks) == nil {
				for _, b := range blocks {
					if b.Type == "text" {
						lastText = true
					}
				}
			}
		case "user":
			found = true
			lastType = "user"
			lastTS, _ = time.Parse(time.RFC3339Nano, r.Timestamp)
			var blocks []block
			if json.Unmarshal(r.Message.Content, &blocks) != nil {
				streak = 0 // a plain string prompt: new turn
				continue
			}
			hasResult, hasErr := false, false
			for _, b := range blocks {
				if b.Type == "tool_result" {
					hasResult = true
					if b.IsError {
						hasErr = true
					}
				}
			}
			if hasResult && hasErr {
				streak++
				lastErr = lastTS
			} else {
				streak = 0
			}
		}
	}
	if !found {
		return
	}
	// A parent's turn ends with end_turn. A subagent's final report is an
	// assistant text record whose stop_reason is usually null, sometimes
	// end_turn -- so for sidecars, a trailing text record that isn't
	// mid-tool_use counts as done.
	done := lastStop == "end_turn" || (ts.sub && lastText && lastStop != "tool_use")
	if lastType == "assistant" && done {
		ts.phase = sessionPhaseAwaiting
		ts.awaitingSince = lastTS
	} else {
		ts.phase = sessionPhaseWorking
	}
	ts.errStreak, ts.lastErrAt = streak, lastErr
}

// poll stats the transcript, rescans the tail if it grew, and returns the
// byte growth (feeds the core gauge).
func (ts *tstate) poll(now time.Time) int64 {
	fi, err := os.Stat(ts.path)
	if err != nil {
		return 0
	}
	grew := fi.Size() - ts.size
	if grew > 0 {
		ts.size = fi.Size()
		// mtime, not now: on first sight every historical file "grows"
		// from 0, and seeding lastGrowth with now would make finished
		// subagent sidecars look live for a few minutes.
		ts.lastGrowth = fi.ModTime()
		ts.scanTail()
		return grew
	}
	if grew < 0 {
		ts.size = fi.Size()
	}
	return 0
}

// ---- Sessions ----

// subStale bounds how long a subagent transcript whose tail says "working"
// keeps its parent session amber without writing anything -- a killed
// subagent would otherwise pin the session forever.
const subStale = 3 * time.Minute

type sessionSlot struct {
	key        string // ~/.claude/projects/<key>
	main       *tstate
	subs       map[string]*tstate
	row, col   int
	cockpitIdx int // -1 when not a cockpit pane
	foreground bool

	phase         sessionPhase // effective: parent, or any live subagent
	awaitingSince time.Time
	confirmAt     time.Time // working->waiting edge seen during this run
	errStreak     int
	lastErrAt     time.Time
	subsWorking   int
	gone          bool
	breathPhase   float64
}

// homeProjectKeyPrefix is the projects-directory key prefix shared by every
// project under $HOME/Documents, derived from the real home directory so
// short() labels are trimmed for whoever runs this.
func homeProjectKeyPrefix() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return projectKey(filepath.Join(home, "Documents") + "/")
}

func (s *sessionSlot) short() string {
	k := strings.TrimPrefix(s.key, homeProjectKeyPrefix())
	if len(k) > 22 {
		k = k[:22]
	}
	return k
}

// addSubs registers any subagent sidecars not yet tracked.
func (s *sessionSlot) addSubs(paths []string) {
	for _, p := range paths {
		if _, ok := s.subs[p]; !ok {
			s.subs[p] = &tstate{path: p, sub: true}
		}
	}
}

func (s *sessionSlot) poll(now time.Time) int64 {
	grew := s.main.poll(now)
	for _, sub := range s.subs {
		grew += sub.poll(now)
	}
	prev := s.phase
	s.phase, s.awaitingSince = s.main.phase, s.main.awaitingSince
	s.errStreak, s.lastErrAt = s.main.errStreak, s.main.lastErrAt
	s.subsWorking = 0
	for _, sub := range s.subs {
		if sub.phase == sessionPhaseWorking && now.Sub(sub.lastGrowth) < subStale {
			s.subsWorking++
			s.phase = sessionPhaseWorking
			if sub.errStreak > s.errStreak {
				s.errStreak, s.lastErrAt = sub.errStreak, sub.lastErrAt
			}
		}
	}
	if prev == sessionPhaseWorking && s.phase == sessionPhaseAwaiting {
		s.confirmAt = now
	}
	return grew
}

// activeSession is one discovered project: its newest parent transcript
// plus that session's subagent sidecars.
type activeSession struct {
	transcript string
	subs       []string
}

// subagentTranscripts lists <dir>/<session-id>/subagents/*.jsonl for a
// parent transcript <dir>/<session-id>.jsonl, touched since cutoff (a
// long-lived session accumulates 100+ finished sidecars; only recent ones
// can be live), plus the newest mtime among them.
func subagentTranscripts(transcript string, cutoff time.Time) ([]string, time.Time) {
	id := strings.TrimSuffix(filepath.Base(transcript), ".jsonl")
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(transcript), id, "subagents", "*.jsonl"))
	var out []string
	var newest time.Time
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil || fi.ModTime().Before(cutoff) {
			continue
		}
		out = append(out, m)
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	return out, newest
}

// fleetActiveWindow bounds how recently a project's transcript must have
// been touched to count as an active session -- was the demo's -window flag.
const fleetActiveWindow = 30 * time.Minute

// claudeProjectsRoot returns ~/.claude/projects, where Claude Code writes
// one directory per project with one JSONL transcript per session.
func claudeProjectsRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// discoverActive returns key -> the project's newest session (by parent or
// subagent mtime) touched within window.
func discoverActive(root string, window time.Duration) map[string]activeSession {
	cutoff := time.Now().Add(-window)
	out := map[string]activeSession{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, e.Name(), "*.jsonl"))
		var best activeSession
		var bestMod time.Time
		for _, m := range matches {
			fi, err := os.Stat(m)
			if err != nil {
				continue
			}
			mod := fi.ModTime()
			subs, subMod := subagentTranscripts(m, cutoff)
			if subMod.After(mod) {
				mod = subMod
			}
			if mod.Before(cutoff) {
				continue
			}
			if best.transcript == "" || mod.After(bestMod) {
				best, bestMod = activeSession{transcript: m, subs: subs}, mod
			}
		}
		if best.transcript != "" {
			out[e.Name()] = best
		}
	}
	return out
}

// projectKey mirrors Claude Code's ~/.claude/projects directory naming as
// as observed: every non-alphanumeric byte of the cwd becomes '-'
// (/home/x/work/my.project -> -home-x-work-my-project).
func projectKey(path string) string {
	var b strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

type cockpit struct {
	order      []string // keys, in tmux pane order
	foreground string
	paneID     map[string]string // key -> tmux pane id (%N), for jumping to a session
	windowID   map[string]string // key -> tmux window id (@N)
}

// readCockpit asks tmux for the panes of the cc-cockpit session (the
// saturday cockpit layout). Absent cockpit
// (no tmux, or no cc-cockpit session) soft-fails to an empty order and no
// foreground -- every session then lands on the "elsewhere" rows, and this
// runs on a 10s tick forever as a background service, so it must never
// treat a missing cockpit as fatal.
func readCockpit() cockpit {
	c := cockpit{paneID: map[string]string{}, windowID: map[string]string{}}
	out, err := exec.Command("tmux", "list-panes", "-t", "cc-cockpit", "-F", "#{pane_active} #{pane_id} #{window_id} #{pane_current_path}").Output()
	if err != nil {
		return c
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, " ", 4)
		if len(parts) != 4 {
			continue
		}
		k := projectKey(parts[3])
		c.order = append(c.order, k)
		c.paneID[k] = parts[1]
		c.windowID[k] = parts[2]
		if parts[0] == "1" {
			c.foreground = k
		}
	}
	return c
}

// ---- Fleet layout ----

type fleet struct {
	slots []*sessionSlot
	byKey map[string]*sessionSlot
	next  int // next free pad index for sessions discovered mid-run
}

func newSlot(key string, a activeSession, cockpitIdx int, foreground bool) *sessionSlot {
	s := &sessionSlot{key: key, main: &tstate{path: a.transcript}, subs: map[string]*tstate{}, cockpitIdx: cockpitIdx, foreground: foreground}
	s.addSubs(a.subs)
	return s
}

// newFleet assigns pads: row 1 = cockpit panes in pane order (a physical
// mirror of the tmux layout), rows 2+ = everything else alphabetically.
// Positions are stable for the life of the run; departed sessions keep
// their pad dark rather than shifting everyone down.
func newFleet(active map[string]activeSession, ck cockpit) *fleet {
	fl := &fleet{byKey: map[string]*sessionSlot{}}
	seen := map[string]bool{}
	var ordered []*sessionSlot
	for i, k := range ck.order {
		if a, ok := active[k]; ok && !seen[k] {
			seen[k] = true
			ordered = append(ordered, newSlot(k, a, i, k == ck.foreground))
		}
	}
	var rest []string
	for k := range active {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	// Cockpit sessions fill row 1 alone; the rest start on row 2 so the two
	// populations read as two groups (Gestalt proximity), even if row 1 is
	// short.
	pad := 0
	for _, s := range ordered {
		fl.place(s, pad)
		pad++
	}
	if len(ordered) > 0 && pad%8 != 0 {
		pad = ((pad / 8) + 1) * 8
	}
	for _, k := range rest {
		fl.place(newSlot(k, active[k], -1, false), pad)
		pad++
	}
	fl.next = pad
	return fl
}

// legendPadRow/legendPadCol/overviewPadRow/overviewPadCol reserve two
// bottom-right grid pads instead of sessions: legend-jump
// (fleet_panel.go's jumpToLegendPane) and cockpit-overview
// (jumpToCockpitOverview, mirrors saturday-cockpit's own Alt+0). Placement
// is ergonomics, not a protocol constraint: the most frequent inputs sit
// at the bottom right, and overview sits immediately left of legend so the
// two form one tight cluster.
//
// *Index is each cell's linear placement index (row-major, row 1 first);
// reservedPads must stay sorted by index ascending, since place's skip
// loop below depends on processing them in that order -- each skip can
// push a candidate into the NEXT reserved cell when they're adjacent
// (index 6 and 7 here), and only an ascending walk handles that correctly.
const (
	overviewPadRow   = 1
	overviewPadCol   = 7
	overviewPadIndex = (overviewPadRow-1)*8 + (overviewPadCol - 1)

	legendPadRow   = 1
	legendPadCol   = 8
	legendPadIndex = (legendPadRow-1)*8 + (legendPadCol - 1)
)

// coreClusterRowLo/coreClusterRowHi/coreClusterCol0 reserve an 8-cell,
// 2-row-by-4-col block at the grid's top-left (rows 7-8, cols 1-4) for the
// fleet-wide gauge. Row 7 is the "bottom" of this 2-row cluster and row 8
// the "top", so corePad's fill order runs bottom-to-top like a level meter.
const (
	coreClusterRowLo = 7
	coreClusterRowHi = 8
	coreClusterCol0  = 1
)

// corePad maps a gauge level (1-8) onto its reserved grid cell -- levels
// 1-4 fill row 7 left to right, 5-8 fill row 8 left to right. Shared by
// reservedPads below (so the cells can never be placed under a session)
// and fleet_panel.go's renderCore (so the gauge lights the same cells).
func corePad(level int) (row, col int) {
	if level <= 4 {
		return coreClusterRowLo, coreClusterCol0 + level - 1
	}
	return coreClusterRowHi, coreClusterCol0 + level - 5
}

func corePadEntry(level int) struct{ row, col, index int } {
	r, c := corePad(level)
	return struct{ row, col, index int }{r, c, (r-1)*8 + (c - 1)}
}

var reservedPads = []struct{ row, col, index int }{
	{overviewPadRow, overviewPadCol, overviewPadIndex},
	{legendPadRow, legendPadCol, legendPadIndex},
	corePadEntry(1), corePadEntry(2), corePadEntry(3), corePadEntry(4),
	corePadEntry(5), corePadEntry(6), corePadEntry(7), corePadEntry(8),
}

// place skips every reserved cell (see reservedPads) by remapping the
// linear counter past each one it reaches or passes, in ascending order --
// so no session is ever placed there, and every session at or after a
// reservation shifts down by one cell for each reservation it clears.
// Ten reservations (2 legend/overview + 8 gauge) means 54 usable pads,
// not 64; row 1 (the cockpit's own row) included.
func (fl *fleet) place(s *sessionSlot, pad int) {
	for _, r := range reservedPads {
		if pad >= r.index {
			pad++
		}
	}
	if pad >= 64 {
		return
	}
	s.row, s.col = pad/8+1, pad%8+1
	s.breathPhase = float64((pad*7)%11) / 11.0 // de-synchronize idle breathing per pad
	fl.slots = append(fl.slots, s)
	fl.byKey[s.key] = s
}

// refresh re-discovers sessions: new ones get the next pad and a mini
// spool-up (returned so the caller can animate it); ones that left the
// window are marked gone; new subagent sidecars are picked up.
func (fl *fleet) refresh(active map[string]activeSession, ck cockpit) (arrived []*sessionSlot) {
	for _, s := range fl.slots {
		a, ok := active[s.key]
		s.gone = !ok
		s.foreground = s.key == ck.foreground
		if ok && a.transcript == s.main.path {
			s.addSubs(a.subs)
		}
	}
	var keys []string
	for k := range active {
		if _, ok := fl.byKey[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if fl.next >= 64 {
			break
		}
		s := newSlot(k, active[k], -1, k == ck.foreground)
		fl.place(s, fl.next)
		fl.next++
		arrived = append(arrived, s)
	}
	return arrived
}

// at returns the session currently occupying (row, col), or nil -- an
// empty or departed pad included.
func (fl *fleet) at(row, col int) *sessionSlot {
	for _, s := range fl.slots {
		if !s.gone && s.row == row && s.col == col {
			return s
		}
	}
	return nil
}

// jumpToSession switches the cockpit's active window/pane to the session
// at (row, col). Re-reads tmux fresh rather than trusting a cached
// pane id from the last refresh cycle -- panes can move between a refresh
// and a press, and a press is rare enough that one more `tmux list-panes`
// costs nothing.
func jumpToSession(fl *fleet, row, col int) {
	s := fl.at(row, col)
	if s == nil {
		log.Printf("efferent: fleet: press pad(%d,%d): no session there", row, col)
		return
	}
	ck := readCockpit()
	pane, ok := ck.paneID[s.key]
	if !ok {
		log.Printf("efferent: fleet: press %s: not in the cockpit, nothing to jump to", s.short())
		return
	}
	if win := ck.windowID[s.key]; win != "" {
		if err := exec.Command("tmux", "select-window", "-t", win).Run(); err != nil {
			log.Printf("efferent: fleet: press %s: select-window %s: %v", s.short(), win, err)
		}
	}
	if err := exec.Command("tmux", "select-pane", "-t", pane).Run(); err != nil {
		log.Printf("efferent: fleet: press %s: select-pane %s: %v", s.short(), pane, err)
		return
	}
	if err := exec.Command("tmux", "resize-pane", "-Z", "-t", pane).Run(); err != nil {
		log.Printf("efferent: fleet: press %s: resize-pane -Z %s: %v", s.short(), pane, err)
		return
	}
	log.Printf("efferent: fleet: press %s -> jumped to pane %s", s.short(), pane)
}

// legendPaneTitle must match the --title given to `saturday-cockpit add`
// when the legend TUI pane (scripts/fleet-legend-tui) was set up. There's
// no other link between the two; if the pane is re-added under a different
// title, this constant needs to change with it.
const legendPaneTitle = "fleet-legend"

// jumpToLegendPane switches the cockpit's active pane to the fleet-legend
// TUI, zoomed -- the reserved legend pad's (legendPadRow, legendPadCol,
// bottom-right on the grid) one action: press-a-pad-to-jump-there, for
// the one pane that isn't a session. select-pane on an already-active pane is a no-op and
// resize-pane -Z toggles, so pressing again while already there un-zooms
// -- same behavior as saturday-cockpit's own Alt+N.
func jumpToLegendPane() {
	out, err := exec.Command("tmux", "list-panes", "-t", "cc-cockpit", "-f",
		"#{==:#{@cockpit_title},"+legendPaneTitle+"}", "-F", "#{pane_id}").Output()
	pane := strings.TrimSpace(string(out))
	if err != nil || pane == "" {
		log.Printf("efferent: fleet: legend-pad press: no %s pane found in the cockpit", legendPaneTitle)
		return
	}
	if err := exec.Command("tmux", "select-pane", "-t", pane).Run(); err != nil {
		log.Printf("efferent: fleet: legend-pad press: select-pane %s: %v", pane, err)
		return
	}
	if err := exec.Command("tmux", "resize-pane", "-Z", "-t", pane).Run(); err != nil {
		log.Printf("efferent: fleet: legend-pad press: resize-pane -Z %s: %v", pane, err)
		return
	}
	log.Printf("efferent: fleet: legend-pad press -> jumped to %s pane %s", legendPaneTitle, pane)
}

// jumpToCockpitOverview mirrors saturday-cockpit's own Alt+0 binding
// (bind_hotkeys in saturday-cockpit): un-zoom the currently-zoomed pane if
// any, then re-tile. Implemented as plain sequential steps rather than
// saturday-cockpit's single if-shell one-liner -- same net effect, easier
// to get right from Go than replicating its nested-format-string
// escaping. "tiled" is saturday-cockpit's documented default layout
// (COCKPIT_TILE); there's no way to query the actual live value from
// outside the script that started it, so this diverges if that was ever
// overridden.
func jumpToCockpitOverview() {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", "cc-cockpit", "#{window_zoomed_flag}").Output()
	if err != nil {
		log.Printf("efferent: fleet: overview-pad press: checking zoom state: %v", err)
		return
	}
	if strings.TrimSpace(string(out)) == "1" {
		if err := exec.Command("tmux", "resize-pane", "-Z", "-t", "cc-cockpit").Run(); err != nil {
			log.Printf("efferent: fleet: overview-pad press: un-zoom: %v", err)
		}
	}
	if err := exec.Command("tmux", "select-layout", "-t", "cc-cockpit", "tiled").Run(); err != nil {
		log.Printf("efferent: fleet: overview-pad press: select-layout: %v", err)
		return
	}
	log.Printf("efferent: fleet: overview-pad press -> re-tiled the cockpit")
}

// ---- Rendering: color math ----

type rgb [3]byte

var (
	colorTest      = rgb{50, 0, 0}    // Steel Battalion: red = TESTING
	colorConfirmed = rgb{0, 20, 70}   // Steel Battalion: blue = OK
	colorWorking   = rgb{100, 46, 0}  // amber: energy / spooled up
	colorFresh     = rgb{10, 95, 20}  // green: just finished, waiting on you
	colorCold      = rgb{0, 22, 44}   // the original cyan hum: waited a long time
	colorStuck     = rgb{110, 8, 8}   // red: >=3 consecutive errored tool results
	colorTick      = rgb{127, 24, 24} // one errored tool result, 300ms
	colorHere      = rgb{34, 34, 40}  // steady white: the pane you're sitting in
	colorFlash     = rgb{80, 127, 80} // working->waiting confirm flash
	colorCore      = rgb{110, 44, 0}  // core column: fleet-wide power draw
	colorPilot     = rgb{0, 12, 26}   // core column when nothing is working
	colorUnknown   = rgb{8, 8, 12}    // transcript tail unparseable
	colorLegend    = rgb{0, 40, 50}   // legend pad: dim teal, deliberately distinct from the pacer's dim blue
	colorOverview  = rgb{40, 0, 50}   // overview pad: dim purple, distinct from legend's teal and the pacer's blue
)

func scaled(c rgb, s float64) rgb {
	return rgb{byte(float64(c[0]) * s), byte(float64(c[1]) * s), byte(float64(c[2]) * s)}
}

func lerp(a, b rgb, f float64) rgb {
	f = math.Max(0, math.Min(1, f))
	return rgb{
		byte(float64(a[0]) + (float64(b[0])-float64(a[0]))*f),
		byte(float64(a[1]) + (float64(b[1])-float64(a[1]))*f),
		byte(float64(a[2]) + (float64(b[2])-float64(a[2]))*f),
	}
}

// breathe oscillates a 0..1 scale over period, offset by phase (0..1 of a
// cycle), so a lit pad has a pulse rather than a flat static color.
func breathe(t, period, phase, min, max float64) float64 {
	s := math.Sin(2 * math.Pi * (t/period + phase))
	return min + (max-min)*(0.5+0.5*s)
}

// coolDown is how long a waiting-on-you pad takes to fade green -> cyan.
const coolDown = 10 * time.Minute

// color is this slot's current pad color: steady white if it's the pane
// you're sitting in, a confirm flash on the working->waiting edge, a red
// tick on a single tool error, sustained red if stuck (>=3 in a row),
// amber breathing while working, green cooling to cyan while waiting.
func (s *sessionSlot) color(now time.Time, t float64) rgb {
	switch {
	case s.gone:
		return rgb{}
	case s.foreground:
		return colorHere
	case now.Sub(s.confirmAt) < 250*time.Millisecond:
		return colorFlash
	case s.errStreak > 0 && now.Sub(s.lastErrAt) < 300*time.Millisecond:
		return colorTick
	case s.errStreak >= 3:
		return scaled(colorStuck, breathe(t, 0.6, s.breathPhase, 0.6, 1.0))
	case s.phase == sessionPhaseWorking:
		return scaled(colorWorking, breathe(t, 0.9, s.breathPhase, 0.55, 1.0))
	case s.phase == sessionPhaseAwaiting:
		age := math.Max(0, math.Min(1, now.Sub(s.awaitingSince).Seconds()/coolDown.Seconds()))
		base := lerp(colorFresh, colorCold, age)
		period := 1.6 + 1.6*age
		lo, hi := 0.6-0.25*age, 1.0-0.15*age
		return scaled(base, breathe(t, period, s.breathPhase, lo, hi))
	}
	return colorUnknown
}
