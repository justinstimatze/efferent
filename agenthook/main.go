// agenthook is a PostToolUse/PostToolUseFailure/UserPromptSubmit/Stop hook
// for Claude Code that tracks the agent's turn phase and tool-error streak
// and writes them to a small state file ($XDG_STATE_HOME/efferent/
// agent_state.json, mode 0600) that efferent's main loop reads -- the
// "agent state" half of the fused signal from prior-art.md, mirroring how
// afferent exposes physio/environment state.
//
// Deterministic on purpose: a real tool error is an unambiguous signal.
// Semantic signals (verbose output, frustration) need judgment and are not
// attempted; a wrong deterministic proxy is worse than a narrower correct
// one.
//
// The PostToolUse/PostToolUseFailure payload fields used here were captured
// empirically; PostToolUseFailure is not documented
// (github.com/anthropics/claude-code/issues/19372) and third-party
// descriptions disagree on the field names. UserPromptSubmit and Stop are
// only consumed for hook_event_name.
//
// Turn phase is "working" while a turn is in progress and "awaiting_user"
// once Stop fires; it drives Machine's Breath's pulse rate
// (machine_breath.go). PostToolUse/PostToolUseFailure also mark "working",
// redundant with UserPromptSubmit normally but insurance against a missed
// one (e.g. a --resume'd session). Two states only, deliberately: a
// permission-prompt block is a harness anomaly, not a regular state worth
// its own rhythm.
//
// The event -> state transition lives in nextAgentState, kept pure and
// separate from main's I/O so it is unit-testable against fixture inputs.
//
// Privacy: the hook payload's error text can contain command output or
// paths, so LastError stores at most maxErrorLen bytes. The state file is
// readable only by its owner, and nothing here transmits it anywhere.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"
)

type hookInput struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	IsInterrupt   bool   `json:"is_interrupt"`
	Error         string `json:"error"`
}

type AgentState struct {
	Phase                 string `json:"phase"`
	ConsecutiveToolErrors int    `json:"consecutive_tool_errors"`
	LastTool              string `json:"last_tool"`
	LastError             string `json:"last_error,omitempty"`
	LastErrorAt           string `json:"last_error_at,omitempty"`
	UpdatedAt             string `json:"updated_at"`
}

// maxErrorLen caps how much of a tool's error text is kept on disk.
const maxErrorLen = 200

const (
	phaseWorking      = "working"
	phaseAwaitingUser = "awaiting_user"
)

// truncateError cuts s to at most maxErrorLen bytes without splitting a
// UTF-8 sequence.
func truncateError(s string) string {
	if len(s) <= maxErrorLen {
		return s
	}
	cut := maxErrorLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func statePath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(base, "efferent", "agent_state.json")
}

func loadState(path string) AgentState {
	var s AgentState
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s) // corrupt/partial state resets to zero value, never blocks
	return s
}

func saveState(path string, s AgentState) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(path, data, 0600)
}

// nextAgentState computes the next AgentState for one hook invocation from
// the previous on-disk state, the decoded hook payload, and a caller-
// supplied timestamp -- no I/O, no internal clock read, so the event ->
// phase/error-count transition table is unit-testable against fixture
// inputs (main_test.go) instead of only end-to-end.
func nextAgentState(prev AgentState, in hookInput, now string) AgentState {
	s := prev
	s.UpdatedAt = now
	if in.ToolName != "" { // UserPromptSubmit/Stop payloads carry no tool_name -- don't clobber the last real one with blank
		s.LastTool = in.ToolName
	}

	switch in.HookEventName {
	case "UserPromptSubmit":
		s.Phase = phaseWorking
	case "PostToolUse":
		s.Phase = phaseWorking
		s.ConsecutiveToolErrors = 0
	case "PostToolUseFailure":
		s.Phase = phaseWorking
		if !in.IsInterrupt {
			s.ConsecutiveToolErrors++
			s.LastError = truncateError(in.Error)
			s.LastErrorAt = now
		}
	case "Stop":
		s.Phase = phaseAwaitingUser
	}
	return s
}

func main() {
	var in hookInput
	_ = json.NewDecoder(os.Stdin).Decode(&in) // malformed stdin -> zero value, exit clean below

	path := statePath()
	now := time.Now().UTC().Format(time.RFC3339)
	s := nextAgentState(loadState(path), in, now)

	saveState(path, s)
	// Always exit 0 -- this hook only observes, never blocks or reports back
	// into the conversation.
}
