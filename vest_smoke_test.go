package main

import (
	"os"
	"testing"
	"time"

	"tinygo.org/x/bluetooth"
)

// Requires the vest powered on (blue LED) and VEST_LIVE=1 -- this drives
// real hardware and needs a human to confirm the sensation, so it's opt-in
// and skipped by default rather than part of `make test`.
func TestVestSmoke(t *testing.T) {
	if os.Getenv("VEST_LIVE") != "1" {
		t.Skip("set VEST_LIVE=1 to run against real hardware")
	}
	v, err := ConnectVest()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer v.Close()
	v.ceiling = 0xF // the smoke tests sweep the full range

	t.Log("buzzing motors 0 and 1 together (same byte, high+low nibble) for 1.5s")
	if err := v.Buzz([]int{0, 1}, 0xF, 1500*time.Millisecond); err != nil {
		t.Fatalf("buzz: %v", err)
	}
}

// Steps motor 0 alone through the nibble intensity range so a human can
// describe whether it scales perceptibly or behaves closer to on/off.
func TestVestIntensityCurve(t *testing.T) {
	if os.Getenv("VEST_LIVE") != "1" {
		t.Skip("set VEST_LIVE=1 to run against real hardware")
	}
	v, err := ConnectVest()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer v.Close()
	v.ceiling = 0xF // the smoke tests sweep the full range

	levels := []byte{0x1, 0x3, 0x5, 0x7, 0x9, 0xB, 0xD, 0xF}
	for _, level := range levels {
		t.Logf("motor 0 at intensity 0x%X", level)
		if err := v.Buzz([]int{0}, level, time.Second); err != nil {
			t.Fatalf("buzz at 0x%X: %v", level, err)
		}
		time.Sleep(700 * time.Millisecond)
	}
}

// unknownCharUUIDs are the six vendor-specific characteristics under the
// command service that aren't the confirmed motor channel (0x02) -- see
// vest-protocol.md's characteristic table.
var unknownCharUUIDs = []string{
	"6e400003-b5a3-f393-e0a9-e50e24dcca9e", // read only
	"6e400005-b5a3-f393-e0a9-e50e24dcca9e", // read, write
	"6e400007-b5a3-f393-e0a9-e50e24dcca9e", // read only
	"6e400008-b5a3-f393-e0a9-e50e24dcca9e", // read, notify
	"6e40000a-b5a3-f393-e0a9-e50e24dcca9e", // read, write
	"6e40000b-b5a3-f393-e0a9-e50e24dcca9e", // read, notify
	"6e40000c-b5a3-f393-e0a9-e50e24dcca9e", // read, write
	"6e40000d-b5a3-f393-e0a9-e50e24dcca9e", // read, write
	"6e40000e-b5a3-f393-e0a9-e50e24dcca9e", // read, write
	"6e40000f-b5a3-f393-e0a9-e50e24dcca9e", // read, notify
	"6e400011-b5a3-f393-e0a9-e50e24dcca9e", // read, write, notify
}

var notifyCharUUIDs = map[string]bool{
	"6e400008-b5a3-f393-e0a9-e50e24dcca9e": true,
	"6e40000b-b5a3-f393-e0a9-e50e24dcca9e": true,
	"6e40000f-b5a3-f393-e0a9-e50e24dcca9e": true,
	"6e400011-b5a3-f393-e0a9-e50e24dcca9e": true,
}

// One connected session covering everything queued: read every unknown
// characteristic's current value, listen briefly on the notify-capable
// ones for anything spontaneous (battery ticks, status pings), then run
// the four-column endpoint check (first vs. last motor per column) so a
// human only has to sit through one power-on for all of it.
func TestVestExplore(t *testing.T) {
	if os.Getenv("VEST_LIVE") != "1" {
		t.Skip("set VEST_LIVE=1 to run against real hardware")
	}
	v, err := ConnectVest()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer v.Close()
	v.ceiling = 0xF // the smoke tests sweep the full range

	svc, err := v.Service()
	if err != nil {
		t.Fatalf("service: %v", err)
	}

	t.Log("--- reading unknown characteristics ---")
	buf := make([]byte, 512) // GATT max attribute value length; a too-small buffer panics on this backend rather than truncating
	for _, u := range unknownCharUUIDs {
		uuid, _ := bluetooth.ParseUUID(u)
		chars, err := svc.DiscoverCharacteristics([]bluetooth.UUID{uuid})
		if err != nil || len(chars) == 0 {
			t.Logf("%s: discover failed: %v", u, err)
			continue
		}
		n, err := chars[0].Read(buf)
		if err != nil {
			t.Logf("%s: read failed: %v", u, err)
			continue
		}
		t.Logf("%s: % x", u, buf[:n])
	}

	t.Log("--- listening on notify characteristics for 4s ---")
	for u := range notifyCharUUIDs {
		uuid, _ := bluetooth.ParseUUID(u)
		chars, err := svc.DiscoverCharacteristics([]bluetooth.UUID{uuid})
		if err != nil || len(chars) == 0 {
			continue
		}
		uCopy := u
		err = chars[0].EnableNotifications(func(data []byte) {
			t.Logf("%s notified: % x", uCopy, data)
		})
		if err != nil {
			t.Logf("%s: enable notifications failed: %v", u, err)
		}
	}
	time.Sleep(4 * time.Second)

	t.Log("--- column endpoint check (first vs. last motor per column) ---")
	columns := []struct {
		name        string
		first, last int
	}{
		{"front-left", 0, 9},
		{"back-left", 10, 19},
		{"back-right", 20, 29},
		{"front-right", 30, 39},
	}
	for _, c := range columns {
		t.Logf("%s: motor %d (expected top)", c.name, c.first)
		must2(t, v.Buzz([]int{c.first}, 0xF, 800*time.Millisecond))
		time.Sleep(500 * time.Millisecond)
		t.Logf("%s: motor %d (expected bottom)", c.name, c.last)
		must2(t, v.Buzz([]int{c.last}, 0xF, 800*time.Millisecond))
		time.Sleep(700 * time.Millisecond)
	}
}

func must2(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("buzz: %v", err)
	}
}
