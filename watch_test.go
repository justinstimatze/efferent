package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const watchTestTimeout = 2 * time.Second

func TestWatcherFileCreate(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher(dir, "state.json")
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Events():
	case <-time.After(watchTestTimeout):
		t.Fatal("no event within timeout after creating state.json")
	}
}

func TestWatcherIgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher(dir, "state.json")
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := os.WriteFile(filepath.Join(dir, "unrelated.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Events():
		t.Fatal("got an event for a file that doesn't match the watched filename")
	case <-time.After(3 * watchCoalesceWindow):
	}
}

// TestWatcherAtomicRename mirrors afferent's real write pattern -- write to
// a temp file, then os.Rename it into place. This is the single most
// load-bearing assumption behind watching a directory instead of a file
// path (previously verified only by a manual throwaway fsnotify script);
// pinning it here makes it a real, automated check instead of a memory.
func TestWatcherAtomicRename(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher(dir, "state.json")
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	tmp := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(tmp, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Events():
	case <-time.After(watchTestTimeout):
		t.Fatal("no event within timeout after an atomic rename onto state.json")
	}
}

func TestWatcherCoalescesBurst(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher(dir, "state.json")
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	path := filepath.Join(dir, "state.json")
	const writes = 5
	for i := 0; i < writes; i++ {
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	got := 0
	deadline := time.After(watchTestTimeout)
drain:
	for {
		select {
		case <-w.Events():
			got++
		case <-deadline:
			break drain
		}
	}

	if got == 0 {
		t.Fatal("burst of writes produced no events at all")
	}
	if got >= writes {
		t.Errorf("burst of %d rapid writes produced %d events, want meaningfully fewer -- coalescing should collapse a burst within one window, not fire once per write", writes, got)
	}
}
