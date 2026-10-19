package main

import (
	"os"
	"testing"
	"time"
)

// Requires the Launchpad X connected via USB -- this drives real hardware
// and needs a human to confirm the pad actually lit, so it's opt-in and
// skipped by default rather than part of `make test`.
func TestLaunchpadSmoke(t *testing.T) {
	if os.Getenv("LAUNCHPAD_LIVE") != "1" {
		t.Skip("set LAUNCHPAD_LIVE=1 to run against real hardware")
	}
	lp, err := ConnectLaunchpad()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer lp.Close()

	t.Log("lighting bottom-left pad (row 1, col 1) red for 3s")
	if err := lp.LightPad(1, 1, 127, 0, 0); err != nil {
		t.Fatalf("light pad: %v", err)
	}
	time.Sleep(3 * time.Second)

	t.Log("turning the surface off")
	if err := lp.Off(); err != nil {
		t.Fatalf("off: %v", err)
	}
}
