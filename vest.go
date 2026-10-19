// Driver for the bHaptics TactSuit X40, reverse-engineered — see
// vest-protocol.md for how these values were found. No official Linux SDK
// exists; this talks BLE GATT directly via tinygo.org/x/bluetooth (BlueZ
// D-Bus on Linux, the same WriteValue call bleak used during discovery).
package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	vestName       = "TactSuitX40"
	vestSvcUUID    = "6e400001-b5a3-f393-e0a9-e50e24dcca9e"
	vestCmdUUID    = "6e400002-b5a3-f393-e0a9-e50e24dcca9e"
	vestKeepalive  = 30 * time.Second
	vestScanWindow = 20 * time.Second

	// vestHardCeiling is the highest per-motor intensity (of 0x0-0xF) any
	// write may carry, enforced at the last step before the wire. Routine
	// signals stay far below it (see mixerComfortCeiling); this only bounds
	// a bug or a future caller. 0xF is mildly uncomfortable to wear
	// (vest-protocol.md).
	vestHardCeiling byte = 0xA
)

// Front-left/back-left/back-right/front-right, 10 motors each, top to
// bottom -- see vest-protocol.md's motor sweep. Front carries agent-state
// events, back carries physio/environment state, per prior-art.md's
// design-implications section.
var vestBack = append(motorRange(10, 19), motorRange(20, 29)...)

func motorRange(lo, hi int) []int {
	r := make([]int, 0, hi-lo+1)
	for m := lo; m <= hi; m++ {
		r = append(r, m)
	}
	return r
}

// gattWriter is the one BLE call *Vest ever makes on cmdChar. Narrowed to
// an interface (bluetooth.DeviceCharacteristic already satisfies it, no
// call-site change needed) so the locking behavior around it -- writeMu,
// Buzz holding it across a whole on/sleep/off burst -- is unit-testable
// with a fake, without a real vest.
type gattWriter interface {
	WriteWithoutResponse(p []byte) (int, error)
}

type Vest struct {
	adapter *bluetooth.Adapter
	device  bluetooth.Device
	cmdChar gattWriter
	stop    chan struct{}

	// writeMu serializes every motor write, held for the duration of one
	// logical operation, not just one frame. That distinction is load-
	// bearing: mixerLoop (mixer.go) writes a fresh full-state frame every
	// tick (20Hz while any channel is active) and applyVest's fatigue
	// alert (vest_loop.go) is a single on/sleep(500ms)/off Buzz -- if Buzz
	// only held the lock per-writeStep (as it used to), the mixer's own
	// tick could land in the 500ms gap between Buzz's "on" and "off"
	// writes and silently overwrite the alert (vestFrame always encodes
	// the FULL 40-motor state, so any write zeroes every motor it doesn't
	// itself claim); Buzz and mixerLoop share vestBack as their zone. Buzz
	// takes writeMu once for its whole burst so the mixer's tick blocks and
	// resumes afterward instead of racing it. Deliberately NOT applied to
	// Ramp (below): Ramp runs for minutes, and holding writeMu that long
	// would starve the mixer. Ramp has no call sites; if revived, it should
	// become a mixerChannel and go through the blend pipeline.
	writeMu sync.Mutex

	// lastVizWrite throttles recordVizFrame -- guarded by writeMu since
	// it's only ever touched from inside writeFrameLocked.
	lastVizWrite time.Time

	// lastWrite is the last time any frame actually went out over the bus,
	// guarded by writeMu the same way -- keepalive checks this instead of
	// blindly pinging on a fixed tick, since its whole purpose (keep GATT
	// traffic flowing, reveal a firmware idle-disconnect) is already served
	// by whatever else is writing.
	lastWrite time.Time

	// ceiling overrides vestHardCeiling when nonzero (the hardware smoke
	// tests raise it to sweep the full range). Guarded by writeMu.
	ceiling byte

	// silenced is set by silence() at shutdown; after it, writeFrameLocked
	// sends nothing, so a late mixer tick cannot re-energize a motor after
	// the final all-off frame. Guarded by writeMu.
	silenced bool
}

// silence writes one all-off frame and refuses every later write. Called
// on SIGINT/SIGTERM so a stopped daemon does not leave a motor running.
func (v *Vest) silence() {
	v.writeMu.Lock()
	defer v.writeMu.Unlock()
	v.silenced = true
	_, _ = v.cmdChar.WriteWithoutResponse(vestOffFrame)
}

// vestVizMinInterval caps how often writeFrame records a snapshot for
// scripts/vest-viz. Familiar's Breath alone calls writeFrame at 20Hz while
// active; nothing reading a live debug view needs faster than this, and
// writing state to disk at 20Hz for the life of the daemon doesn't earn
// its keep.
const vestVizMinInterval = 100 * time.Millisecond

type vestVizFrame struct {
	Timestamp string   `json:"timestamp"`
	Build     string   `json:"build"`
	Motors    [40]byte `json:"motors"`
}

// recordVizFrame snapshots exactly what's on the wire -- vestFrame's own
// "unlisted motors are off" semantics, not just active's sparse keys, so
// scripts/vest-viz shows the real state of a motor no current caller
// mentions. Each snapshot carries the build string, so a live view can tell
// a fresh build's output from stale state left over from before a restart.
func (v *Vest) recordVizFrame(active map[int]byte) {
	now := time.Now()
	if now.Sub(v.lastVizWrite) < vestVizMinInterval {
		return
	}
	v.lastVizWrite = now
	var frame vestVizFrame
	frame.Timestamp = now.UTC().Format(time.RFC3339Nano)
	frame.Build = buildInfo
	for m, intensity := range active {
		if m >= 0 && m < len(frame.Motors) {
			frame.Motors[m] = intensity
		}
	}
	writeJSON(statePath("efferent", "vest_frame.json"), frame)
}

// motorMap expands a uniform-intensity motor list into a per-motor map --
// the shape writeFrame/writeFrameLocked actually take.
func motorMap(motors []int, intensity byte) map[int]byte {
	active := make(map[int]byte, len(motors))
	for _, m := range motors {
		active[m] = intensity
	}
	return active
}

// writeStep sends one full-state frame: all of motors at intensity, every
// other motor off. A thin convenience over writeFrame for the common case
// of one uniform intensity across a motor list.
func (v *Vest) writeStep(motors []int, intensity byte) error {
	return v.writeFrame(motorMap(motors, intensity))
}

// writeStepLocked is writeStep for a caller (Buzz) that already holds
// writeMu across a multi-frame burst.
func (v *Vest) writeStepLocked(motors []int, intensity byte) error {
	return v.writeFrameLocked(motorMap(motors, intensity))
}

// writeFrame sends one full-state frame from an arbitrary per-motor
// intensity map -- unlisted motors are off. Exposed directly (not just
// through writeStep) for a caller that needs different intensities on
// different motors in the same frame, e.g. the mixer's blended output
// (mixer.go) or Familiar's Breath's own spatial fill -- vestFrame already
// accepts an arbitrary map, so this needed no protocol-level change, just
// a name for the write path that doesn't force a single uniform intensity
// first. Locks writeMu for exactly this one frame; see writeFrameLocked
// for the multi-frame case (Buzz), which needs the lock held across the
// whole burst instead.
func (v *Vest) writeFrame(active map[int]byte) error {
	v.writeMu.Lock()
	defer v.writeMu.Unlock()
	return v.writeFrameLocked(active)
}

// writeFrameLocked is writeFrame's body, callable directly by a caller
// that already holds writeMu across a multi-frame operation -- the only
// thing that ever touches cmdChar. See writeMu's doc comment for why this
// distinction (per-frame lock vs. per-operation lock) matters.
func (v *Vest) writeFrameLocked(active map[int]byte) error {
	if v.silenced {
		return nil
	}
	ceiling := vestHardCeiling
	if v.ceiling != 0 {
		ceiling = v.ceiling
	}
	capped := make(map[int]byte, len(active))
	for m, intensity := range active {
		capped[m] = min(intensity, ceiling)
	}
	_, err := v.cmdChar.WriteWithoutResponse(vestFrame(capped))
	if err == nil {
		v.lastWrite = time.Now()
		v.recordVizFrame(active)
	}
	return err
}

// sinceLastWrite reports how long it's been since any frame actually went
// out over the bus, guarded by writeMu the same way lastWrite itself is.
// A zero lastWrite (nothing written yet since connecting) reports a
// duration well past vestKeepalive, so a fresh connection still pings once.
func (v *Vest) sinceLastWrite() time.Duration {
	v.writeMu.Lock()
	defer v.writeMu.Unlock()
	if v.lastWrite.IsZero() {
		return vestKeepalive + time.Second
	}
	return time.Since(v.lastWrite)
}

// vestFrame packs motor intensities (0x0-0xF each) into the 20-byte
// nibble-packed command frame: motor N lives in byte N/2, high nibble if
// N is even, low nibble if N is odd. Unset motors default to 0 (off).
func vestFrame(active map[int]byte) []byte {
	b := make([]byte, 20)
	for motor, intensity := range active {
		byteI, hi := motor/2, motor%2 == 0
		if hi {
			b[byteI] |= intensity << 4
		} else {
			b[byteI] |= intensity & 0x0F
		}
	}
	return b
}

var vestOffFrame = make([]byte, 20)

// ConnectVest scans for the suit (must already be powered on, blue LED)
// and connects. Unbonded, so BlueZ forgets the device once disconnected --
// re-scan from scratch on every reconnect, never cache the address.
func ConnectVest() (*Vest, error) {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("enable adapter: %w", err)
	}

	found := make(chan bluetooth.ScanResult, 1)
	go func() {
		time.Sleep(vestScanWindow)
		adapter.StopScan()
	}()
	err := adapter.Scan(func(a *bluetooth.Adapter, res bluetooth.ScanResult) {
		if res.LocalName() == vestName {
			a.StopScan()
			select {
			case found <- res:
			default:
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	var res bluetooth.ScanResult
	select {
	case res = <-found:
	case <-time.After(vestScanWindow + time.Second):
		return nil, fmt.Errorf("no %s advertising within %s (powered on? blue LED?)", vestName, vestScanWindow)
	}

	device, err := adapter.Connect(res.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	svcUUID, _ := bluetooth.ParseUUID(vestSvcUUID)
	svcs, err := device.DiscoverServices([]bluetooth.UUID{svcUUID})
	if err != nil || len(svcs) == 0 {
		return nil, fmt.Errorf("discover command service: %w", err)
	}
	charUUID, _ := bluetooth.ParseUUID(vestCmdUUID)
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{charUUID})
	if err != nil || len(chars) == 0 {
		return nil, fmt.Errorf("discover command characteristic: %w", err)
	}

	v := &Vest{adapter: adapter, device: device, cmdChar: chars[0], stop: make(chan struct{})}
	go v.keepalive()
	return v, nil
}

// keepalive writes the all-zero (motors-off) frame on a fixed interval,
// but only when nothing else has written more recently -- its purpose is
// to keep GATT traffic flowing and reveal (rather than silently eat) any
// firmware idle-disconnect the vest might have (undocumented and untested,
// see vest-protocol.md), and that purpose is already served whenever the
// mixer (or anything else) is already producing bus traffic every tick.
// Skipping in that case also means it can never itself race a Buzz or a
// mixer write -- it only ever fires during a genuine quiet period.
func (v *Vest) keepalive() {
	t := time.NewTicker(vestKeepalive)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if v.sinceLastWrite() < vestKeepalive {
				continue
			}
			if err := v.writeStep(nil, 0); err != nil {
				fmt.Printf("vest: keepalive write failed: %v\n", err)
			}
		case <-v.stop:
			return
		}
	}
}

// Buzz drives a set of motors at the given intensity (0x0-0xF) for the
// given duration, then turns them off. Holds writeMu for the whole
// on/sleep/off sequence -- see writeMu's own doc comment for why: without
// it, a concurrent mixer tick can land in the sleep gap and stomp the
// alert, or get stomped by Buzz's own final "off" write.
func (v *Vest) Buzz(motors []int, intensity byte, dur time.Duration) error {
	v.writeMu.Lock()
	defer v.writeMu.Unlock()
	start := time.Now()
	if err := v.writeStepLocked(motors, intensity); err != nil {
		return fmt.Errorf("buzz on: %w", err)
	}
	log.Printf("vest: write-without-response call took %s", time.Since(start))
	time.Sleep(dur)
	if err := v.writeStepLocked(nil, 0); err != nil {
		return fmt.Errorf("buzz off: %w", err)
	}
	return nil
}

// rampSteps returns the intensity sequence for one full breathe-in/
// breathe-out cycle: a linear rise from 0 to peak over `steps` ticks, then
// a linear fall back to 0 over `steps` ticks. Pure and hardware-independent
// so the shape itself is unit-testable without a real vest.
func rampSteps(peak byte, steps int) []byte {
	seq := make([]byte, 0, steps*2)
	for i := 0; i <= steps; i++ {
		seq = append(seq, byte(int(peak)*i/steps))
	}
	for i := steps - 1; i >= 0; i-- {
		seq = append(seq, byte(int(peak)*i/steps))
	}
	return seq
}

const rampStepsPerCycle = 20

// Ramp drives motors through a smooth triangular intensity ramp -- the
// breathing pacer's entrainment rhythm, sustained over minutes rather than
// a single discrete pulse (see signal-vocabulary.md's vest primitive
// alphabet: sharp pulse = alert, smooth ramp = entrainment). Runs for
// cycles repetitions of cycleDur each, or stops early if stop fires.
func (v *Vest) Ramp(motors []int, peak byte, cycleDur time.Duration, cycles int, stop <-chan struct{}) error {
	seq := rampSteps(peak, rampStepsPerCycle)
	stepDur := cycleDur / time.Duration(len(seq))
	for c := 0; c < cycles; c++ {
		for _, intensity := range seq {
			if err := v.writeStep(motors, intensity); err != nil {
				return fmt.Errorf("ramp write: %w", err)
			}
			select {
			case <-stop:
				_ = v.writeStep(nil, 0)
				return nil
			case <-time.After(stepDur):
			}
		}
	}
	return v.writeStep(nil, 0)
}

func (v *Vest) Close() error {
	close(v.stop)
	return v.device.Disconnect()
}

// Service re-discovers the vest's main command/telemetry service, for
// exploration code that needs characteristics beyond the command channel
// Connect already discovered. See vest-protocol.md's "Open questions".
func (v *Vest) Service() (bluetooth.DeviceService, error) {
	svcUUID, _ := bluetooth.ParseUUID(vestSvcUUID)
	svcs, err := v.device.DiscoverServices([]bluetooth.UUID{svcUUID})
	if err != nil || len(svcs) == 0 {
		return bluetooth.DeviceService{}, fmt.Errorf("discover service: %w", err)
	}
	return svcs[0], nil
}
