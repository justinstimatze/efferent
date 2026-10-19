package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- Fixture builders for real transcript record shapes ----

func assistantToolUse(ts string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"stop_reason":"tool_use","content":[{"type":"tool_use"}]}}`, ts)
}

func assistantEndTurn(ts string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}`, ts)
}

// assistantTextNullStop mirrors a subagent's final report: an assistant
// text record with stop_reason null (verified live on five of six finished
// sidecars; the sixth had end_turn instead).
func assistantTextNullStop(ts string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"stop_reason":null,"content":[{"type":"text","text":"report"}]}}`, ts)
}

func userToolResult(ts string, isErr bool) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","is_error":%v}]}}`, ts, isErr)
}

func userPrompt(ts string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":"hello"}}`, ts)
}

func metaRecord(ts string) string {
	return fmt.Sprintf(`{"type":"system","timestamp":%q}`, ts)
}

// writeTranscript writes lines (already-built JSONL records) to a fresh
// file under t.TempDir() and returns its path.
func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---- scanTail (via poll, its real entry point) ----

func TestTstatePollPhaseFromTail(t *testing.T) {
	const t0, t1, t2 = "2026-09-07T10:00:00Z", "2026-09-07T10:00:05Z", "2026-09-07T10:00:10Z"

	cases := []struct {
		name      string
		sub       bool
		lines     []string
		wantPhase sessionPhase
		wantErr   int
	}{
		{
			name:      "mid tool call: last line is a tool_result, not an assistant turn end",
			lines:     []string{assistantToolUse(t0), userToolResult(t1, false)},
			wantPhase: sessionPhaseWorking,
		},
		{
			name:      "clean end_turn: awaiting the user",
			lines:     []string{assistantToolUse(t0), userToolResult(t1, false), assistantEndTurn(t2)},
			wantPhase: sessionPhaseAwaiting,
		},
		{
			name:      "one errored tool result, still mid-turn",
			lines:     []string{assistantToolUse(t0), userToolResult(t1, true)},
			wantPhase: sessionPhaseWorking,
			wantErr:   1,
		},
		{
			name: "three consecutive tool errors: streak 3",
			lines: []string{
				assistantToolUse(t0), userToolResult(t1, true),
				assistantToolUse(t1), userToolResult(t2, true),
				assistantToolUse(t2), userToolResult(t2, true),
			},
			wantPhase: sessionPhaseWorking,
			wantErr:   3,
		},
		{
			name: "a fresh user prompt resets the error streak",
			lines: []string{
				assistantToolUse(t0), userToolResult(t1, true),
				userPrompt(t2),
			},
			wantPhase: sessionPhaseWorking,
			wantErr:   0,
		},
		{
			name:      "subagent sidecar: final text report with stop_reason null counts as done",
			sub:       true,
			lines:     []string{assistantToolUse(t0), assistantTextNullStop(t1)},
			wantPhase: sessionPhaseAwaiting,
		},
		{
			name:      "subagent sidecar: still mid tool_use, not done",
			sub:       true,
			lines:     []string{assistantToolUse(t0)},
			wantPhase: sessionPhaseWorking,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := &tstate{path: writeTranscript(t, c.lines...), sub: c.sub}
			ts.poll(time.Now())
			if ts.phase != c.wantPhase {
				t.Errorf("phase = %s, want %s", ts.phase, c.wantPhase)
			}
			if ts.errStreak != c.wantErr {
				t.Errorf("errStreak = %d, want %d", ts.errStreak, c.wantErr)
			}
		})
	}
}

func TestTstatePollIgnoresMetaOnlyTail(t *testing.T) {
	ts := &tstate{path: writeTranscript(t, metaRecord("2026-09-07T10:00:00Z"))}
	ts.poll(time.Now())
	if ts.phase != sessionPhaseUnknown {
		t.Errorf("phase = %s, want unchanged sessionPhaseUnknown (no assistant/user record in tail)", ts.phase)
	}
}

// TestTstatePollSeedsLastGrowthFromMTime guards against seeding lastGrowth
// with time.Now() on first sight of a transcript, which would make
// already-finished historical subagent sidecars look "live" for a few
// minutes. It must seed from the file's own mtime.
func TestTstatePollSeedsLastGrowthFromMTime(t *testing.T) {
	path := writeTranscript(t, assistantEndTurn("2026-09-07T10:00:00Z"))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	ts := &tstate{path: path}
	ts.poll(time.Now())

	if !ts.lastGrowth.Equal(old) {
		t.Errorf("lastGrowth = %s, want the file's mtime %s (not time.Now())", ts.lastGrowth, old)
	}
}

// ---- sessionSlot: subagent-aware effective phase, confirm-flash edge ----

func TestSessionSlotPollSubagentKeepsParentWorking(t *testing.T) {
	s := &sessionSlot{
		main: &tstate{path: writeTranscript(t, assistantEndTurn("2026-09-07T10:00:00Z"))},
		subs: map[string]*tstate{},
	}
	subPath := writeTranscript(t, assistantToolUse("2026-09-07T10:00:00Z"))
	s.subs[subPath] = &tstate{path: subPath, sub: true}

	s.poll(time.Now())

	if s.phase != sessionPhaseWorking {
		t.Errorf("phase = %s, want working -- a live subagent should override the parent's own end_turn", s.phase)
	}
	if s.subsWorking != 1 {
		t.Errorf("subsWorking = %d, want 1", s.subsWorking)
	}
}

func TestSessionSlotPollStaleSubagentDoesNotOverride(t *testing.T) {
	s := &sessionSlot{
		main: &tstate{path: writeTranscript(t, assistantEndTurn("2026-09-07T10:00:00Z"))},
		subs: map[string]*tstate{},
	}
	subPath := writeTranscript(t, assistantToolUse("2026-09-07T10:00:00Z"))
	s.subs[subPath] = &tstate{
		path:       subPath,
		sub:        true,
		phase:      sessionPhaseWorking,
		lastGrowth: time.Now().Add(-subStale - time.Minute), // older than subStale: a killed subagent
	}
	// Give the sub its real size so poll() sees no further growth and
	// trusts the pre-seeded stale phase/lastGrowth above.
	fi, err := os.Stat(subPath)
	if err != nil {
		t.Fatal(err)
	}
	s.subs[subPath].size = fi.Size()

	s.poll(time.Now())

	if s.phase != sessionPhaseAwaiting {
		t.Errorf("phase = %s, want awaiting -- a stale (subStale-old) subagent must not pin the parent working", s.phase)
	}
	if s.subsWorking != 0 {
		t.Errorf("subsWorking = %d, want 0", s.subsWorking)
	}
}

func TestSessionSlotPollSetsConfirmAtOnWorkingToAwaitingEdge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(assistantToolUse("2026-09-07T10:00:00Z")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &sessionSlot{main: &tstate{path: path}, subs: map[string]*tstate{}}

	s.poll(time.Now())
	if s.phase != sessionPhaseWorking {
		t.Fatalf("setup: phase = %s, want working", s.phase)
	}
	if !s.confirmAt.IsZero() {
		t.Fatalf("setup: confirmAt should still be zero before any edge")
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(assistantEndTurn("2026-09-07T10:00:05Z") + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	now := time.Now()
	s.poll(now)
	if s.phase != sessionPhaseAwaiting {
		t.Fatalf("phase = %s, want awaiting after end_turn appended", s.phase)
	}
	if !s.confirmAt.Equal(now) {
		t.Errorf("confirmAt = %s, want %s (the working->awaiting edge)", s.confirmAt, now)
	}
}

// ---- Discovery ----

func TestDiscoverActiveFindsSessionsAndSubagentsWithinWindow(t *testing.T) {
	root := t.TempDir()
	now := time.Now()

	proj1 := filepath.Join(root, "proj1")
	if err := os.MkdirAll(filepath.Join(proj1, "sess1", "subagents"), 0700); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(proj1, "sess1.jsonl")
	if err := os.WriteFile(transcript, []byte(assistantEndTurn("2026-09-07T10:00:00Z")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	recentSub := filepath.Join(proj1, "sess1", "subagents", "agent-a.jsonl")
	if err := os.WriteFile(recentSub, []byte(assistantToolUse("2026-09-07T10:00:00Z")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	staleSub := filepath.Join(proj1, "sess1", "subagents", "agent-b.jsonl")
	if err := os.WriteFile(staleSub, []byte(assistantEndTurn("2026-09-07T09:00:00Z")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	staleTime := now.Add(-time.Hour)
	if err := os.Chtimes(staleSub, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	proj2 := filepath.Join(root, "proj2")
	if err := os.MkdirAll(proj2, 0700); err != nil {
		t.Fatal(err)
	}
	outOfWindow := filepath.Join(proj2, "sess2.jsonl")
	if err := os.WriteFile(outOfWindow, []byte(assistantEndTurn("2026-09-07T08:00:00Z")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(outOfWindow, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	active := discoverActive(root, 30*time.Minute)

	got, ok := active["proj1"]
	if !ok {
		t.Fatalf("active = %v, want proj1 present", active)
	}
	if got.transcript != transcript {
		t.Errorf("proj1 transcript = %s, want %s", got.transcript, transcript)
	}
	if len(got.subs) != 1 || got.subs[0] != recentSub {
		t.Errorf("proj1 subs = %v, want just %s (agent-b is older than the window)", got.subs, recentSub)
	}
	if _, ok := active["proj2"]; ok {
		t.Errorf("active = %v, want proj2 absent (its only transcript is outside the window)", active)
	}
}

func TestProjectKeyMirrorsClaudeCodeDirectoryNaming(t *testing.T) {
	got := projectKey("/home/x/work/my.project")
	want := "-home-x-work-my-project"
	if got != want {
		t.Errorf("projectKey = %q, want %q", got, want)
	}
}

// TestFleetPlaceSkipsReservedPads exercises the boundary that matters:
// enough cockpit sessions to reach past both reserved bottom-right pads
// (linear indices 6 and 7 -- overview and legend, adjacent), neither of
// which may ever hold a session, and every session at or after them must
// shift by two cells (one per reservation cleared), not collide or shift
// by only one.
func TestFleetPlaceSkipsReservedPads(t *testing.T) {
	active := map[string]activeSession{}
	ck := cockpit{paneID: map[string]string{}, windowID: map[string]string{}}
	for i := 0; i < 8; i++ {
		key := fmt.Sprintf("proj%d", i)
		active[key] = activeSession{transcript: "/dummy/" + key + ".jsonl"}
		ck.order = append(ck.order, key)
	}

	fl := newFleet(active, ck)

	if len(fl.slots) != 8 {
		t.Fatalf("got %d slots, want 8", len(fl.slots))
	}
	for _, s := range fl.slots {
		if s.row == overviewPadRow && s.col == overviewPadCol {
			t.Fatalf("session %s placed on the reserved overview pad (%d,%d)", s.key, overviewPadRow, overviewPadCol)
		}
		if s.row == legendPadRow && s.col == legendPadCol {
			t.Fatalf("session %s placed on the reserved legend pad (%d,%d)", s.key, legendPadRow, legendPadCol)
		}
	}
	// Sessions 0-5 (linear indices 0-5) are unaffected: row 1, cols 1-6.
	// Session 6 (linear index 6, which the overview pad would otherwise
	// occupy) shifts past BOTH reservations to row 2, col 1. Session 7
	// (linear index 7, the legend pad's own slot) shifts to row 2, col 2.
	seventh, eighth := fl.slots[6], fl.slots[7]
	if seventh.row != 2 || seventh.col != 1 {
		t.Errorf("7th session placed at (%d,%d), want (2,1)", seventh.row, seventh.col)
	}
	if eighth.row != 2 || eighth.col != 2 {
		t.Errorf("8th session placed at (%d,%d), want (2,2)", eighth.row, eighth.col)
	}
}
