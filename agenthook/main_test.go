package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNextAgentStateUserPromptSubmit(t *testing.T) {
	got := nextAgentState(AgentState{}, hookInput{HookEventName: "UserPromptSubmit"}, "t1")
	if got.Phase != phaseWorking {
		t.Errorf("Phase = %q, want %q", got.Phase, phaseWorking)
	}
	if got.UpdatedAt != "t1" {
		t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, "t1")
	}
}

func TestNextAgentStateStop(t *testing.T) {
	prev := AgentState{Phase: phaseWorking}
	got := nextAgentState(prev, hookInput{HookEventName: "Stop"}, "t2")
	if got.Phase != phaseAwaitingUser {
		t.Errorf("Phase = %q, want %q", got.Phase, phaseAwaitingUser)
	}
}

func TestNextAgentStatePostToolUse(t *testing.T) {
	prev := AgentState{Phase: phaseAwaitingUser, ConsecutiveToolErrors: 3}
	got := nextAgentState(prev, hookInput{HookEventName: "PostToolUse", ToolName: "Bash"}, "t3")
	if got.Phase != phaseWorking {
		t.Errorf("Phase = %q, want %q", got.Phase, phaseWorking)
	}
	if got.ConsecutiveToolErrors != 0 {
		t.Errorf("ConsecutiveToolErrors = %d, want 0", got.ConsecutiveToolErrors)
	}
	if got.LastTool != "Bash" {
		t.Errorf("LastTool = %q, want %q", got.LastTool, "Bash")
	}
}

func TestNextAgentStatePostToolUseFailure(t *testing.T) {
	prev := AgentState{ConsecutiveToolErrors: 1}
	in := hookInput{HookEventName: "PostToolUseFailure", ToolName: "Edit", Error: "boom"}
	got := nextAgentState(prev, in, "t4")
	if got.Phase != phaseWorking {
		t.Errorf("Phase = %q, want %q", got.Phase, phaseWorking)
	}
	if got.ConsecutiveToolErrors != 2 {
		t.Errorf("ConsecutiveToolErrors = %d, want 2", got.ConsecutiveToolErrors)
	}
	if got.LastError != "boom" || got.LastErrorAt != "t4" {
		t.Errorf("LastError/LastErrorAt = %q/%q, want %q/%q", got.LastError, got.LastErrorAt, "boom", "t4")
	}
}

// PostToolUseFailure caused by an interrupted tool call is not a real tool
// error -- it must still mark the agent as working, but must not count
// against ConsecutiveToolErrors.
func TestNextAgentStatePostToolUseFailureInterrupt(t *testing.T) {
	prev := AgentState{ConsecutiveToolErrors: 1}
	in := hookInput{HookEventName: "PostToolUseFailure", ToolName: "Bash", IsInterrupt: true, Error: "interrupted"}
	got := nextAgentState(prev, in, "t5")
	if got.Phase != phaseWorking {
		t.Errorf("Phase = %q, want %q", got.Phase, phaseWorking)
	}
	if got.ConsecutiveToolErrors != 1 {
		t.Errorf("ConsecutiveToolErrors = %d, want unchanged at 1", got.ConsecutiveToolErrors)
	}
	if got.LastError != "" {
		t.Errorf("LastError = %q, want unset on an interrupt", got.LastError)
	}
}

func TestNextAgentStateUnrecognizedEventIsNoOp(t *testing.T) {
	prev := AgentState{Phase: phaseWorking, ConsecutiveToolErrors: 2}
	got := nextAgentState(prev, hookInput{HookEventName: "Notification"}, "t6")
	if got.Phase != phaseWorking {
		t.Errorf("Phase = %q, want unchanged at %q", got.Phase, phaseWorking)
	}
	if got.ConsecutiveToolErrors != 2 {
		t.Errorf("ConsecutiveToolErrors = %d, want unchanged at 2", got.ConsecutiveToolErrors)
	}
	if got.UpdatedAt != "t6" {
		t.Errorf("UpdatedAt = %q, want %q (should still advance)", got.UpdatedAt, "t6")
	}
}

// UserPromptSubmit/Stop payloads carry no tool_name. An earlier version of
// this hook overwrote LastTool unconditionally, which would blank the one
// "what actually happened last" field on every turn boundary.
func TestNextAgentStatePreservesLastToolAcrossTurnBoundary(t *testing.T) {
	prev := AgentState{LastTool: "Read"}
	got := nextAgentState(prev, hookInput{HookEventName: "UserPromptSubmit"}, "t7")
	if got.LastTool != "Read" {
		t.Errorf("LastTool = %q, want preserved %q", got.LastTool, "Read")
	}
}

func TestTruncateErrorCapsLengthOnRuneBoundary(t *testing.T) {
	long := strings.Repeat("é", maxErrorLen) // 2 bytes per rune
	got := truncateError(long)
	if len(got) > maxErrorLen {
		t.Errorf("len = %d, want <= %d", len(got), maxErrorLen)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncateError split a UTF-8 sequence: %q", got)
	}
	if short := "boom"; truncateError(short) != short {
		t.Errorf("short error was altered")
	}
}
