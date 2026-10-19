// watch.go is a small directory watcher: a coalesced events channel over
// fsnotify, watching a directory (not a file path) so an atomic write-then-rename -- afferent's
// own write pattern for state.json -- keeps firing indefinitely instead of
// dying after the first rename swaps the watched inode out from under a
// file-path watch.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchCoalesceWindow = 100 * time.Millisecond

// Watcher watches one directory for Create/Write events on one specific
// filename, coalescing bursts into a single buffered signal. Both ops are
// required: afferent's atomic-rename write surfaces as Create on the
// destination name and never Write, while agenthook's direct os.WriteFile
// surfaces as Create on the first write and Write on every write after --
// a Write-only filter would silently never see afferent's updates at all.
type Watcher struct {
	dir      string
	filename string
	closed   chan struct{}
	wg       sync.WaitGroup
	events   chan struct{}
}

// NewWatcher creates dir if missing (so efferent can start before the
// producer ever has) and starts watching it in the background. It blocks
// until the underlying fsnotify watch is actually established (or has
// failed) -- without this handshake, a write landing between NewWatcher
// returning and the background goroutine finishing fsnotify.Add(dir) would
// be silently lost, and a setup failure would leave a "successful" Watcher
// that never fires again.
func NewWatcher(dir, filename string) (*Watcher, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating watch dir %s: %w", dir, err)
	}
	w := &Watcher{
		dir:      dir,
		filename: filename,
		closed:   make(chan struct{}),
		events:   make(chan struct{}, 1), // buffer one signal for coalescing
	}
	ready := make(chan error, 1)
	w.wg.Add(1)
	go w.watch(ready)
	if err := <-ready; err != nil {
		w.wg.Wait() // watch() already returned on this path; just reap it
		return nil, err
	}
	return w, nil
}

// Events fires (coalesced) whenever filename may have changed inside dir.
func (w *Watcher) Events() <-chan struct{} { return w.events }

func (w *Watcher) Close() error {
	select {
	case <-w.closed:
		return fmt.Errorf("watcher already closed")
	default:
	}
	close(w.closed)
	w.wg.Wait()
	return nil
}

func (w *Watcher) watch(ready chan<- error) {
	defer w.wg.Done()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		ready <- fmt.Errorf("watcher init failed for %s: %w", w.dir, err)
		return
	}
	defer func() { _ = watcher.Close() }()

	if err := watcher.Add(w.dir); err != nil {
		ready <- fmt.Errorf("watcher failed to add dir %s: %w", w.dir, err)
		return
	}
	ready <- nil

	coalesce := time.NewTicker(watchCoalesceWindow)
	defer coalesce.Stop()

	pending := false
	for {
		select {
		case <-w.closed:
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 &&
				filepath.Base(event.Name) == w.filename {
				pending = true
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("efferent: watcher error on %s: %v", w.dir, err)
		case <-coalesce.C:
			if pending {
				pending = false
				select {
				case w.events <- struct{}{}:
				default:
				}
			}
		}
	}
}
