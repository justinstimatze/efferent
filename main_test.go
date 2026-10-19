package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeAfferentFixture(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "afferent"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "afferent", "state.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReadAfferentState(t *testing.T) {
	writeAfferentFixture(t, `{"timestamp":"2026-09-06T12:00:00Z","co2_ppm":1101,"co2_bucket":"approaching","factors":["co2_approaching"],"hr_bpm":63.5}`)

	got, err := readAfferentState()
	if err != nil {
		t.Fatalf("readAfferentState: %v", err)
	}
	want := afferentState{
		Timestamp: "2026-09-06T12:00:00Z",
		CO2PPM:    1101,
		CO2Bucket: "approaching",
		Factors:   []string{"co2_approaching"},
		HRBPM:     63.5,
	}
	if got.Timestamp != want.Timestamp || got.CO2PPM != want.CO2PPM ||
		got.CO2Bucket != want.CO2Bucket || got.HRBPM != want.HRBPM ||
		!slices.Equal(got.Factors, want.Factors) {
		t.Errorf("readAfferentState() = %+v, want %+v", got, want)
	}
}

func TestReadAfferentStateMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if _, err := readAfferentState(); err == nil {
		t.Error("readAfferentState() with no state file: want error, got nil")
	}
}

func writeAgentStateFixture(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "efferent"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "efferent", "agent_state.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReadAgentState(t *testing.T) {
	writeAgentStateFixture(t, `{"phase":"working","updated_at":"2026-09-07T12:00:00Z"}`)

	got, err := readAgentState()
	if err != nil {
		t.Fatalf("readAgentState: %v", err)
	}
	if got.Phase != "working" {
		t.Errorf("readAgentState().Phase = %q, want %q", got.Phase, "working")
	}
}

func TestReadAgentStateMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if _, err := readAgentState(); err == nil {
		t.Error("readAgentState() with no state file: want error, got nil")
	}
}
