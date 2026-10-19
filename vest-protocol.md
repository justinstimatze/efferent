# bHaptics TactSuit X40: reverse-engineered BLE protocol

bHaptics ships no Linux SDK and no public BLE protocol documentation. Its own
Player app is Windows-only and does not run under Wine. Everything below was
found by connecting over BLE and testing on one unit. Treat it as a working
hypothesis backed by a handful of experiments, not a verified spec.

## Connecting

- Power on: hold the back button. The LED flashes yellow through three
  stages, then turns solid blue once advertising. The vest powers itself off
  after about 3 minutes if nothing connects. There is no such timeout once
  connected.
- No pairing or bonding is required. `bluetoothctl connect <mac>` (or bleak's
  `BleakClient(mac).connect()`) works directly. `Paired: no, Bonded: no,
  Connected: yes` is the normal working state.
- Advertised name: `TactSuitX40`. The MAC is randomized (BlueZ reports
  `(random)`), so re-scan after every power-on and do not hardcode the
  address. `vest.go` scans by name on every (re)connect.
- Use `bleak` or `tinygo.org/x/bluetooth`, not `bluetoothctl`'s own `write`
  command. bluetoothctl failed every write to the command characteristic
  with `org.bluez.Error.NotSupported`, while the identical write through
  bleak's `write_gatt_char(..., response=False)` succeeded. This looks like
  a client-side bug in that bluetoothctl build.
- If the device is already connected through `bluetoothctl`, bleak's
  discovery-based connect can fail with `BleakDeviceNotFoundError`, because a
  connected peripheral stops advertising. Run `bluetoothctl disconnect <mac>`
  first.

## GATT layout

An nRF52-based device. Services:

- `00001800-...` Generic Access and `00001801-...` Generic Attribute:
  standard, unused.
- `0000fe59-...` Nordic Secure DFU: firmware update, not touched.
- `6e400001-b5a3-f393-e0a9-e50e24dcca9e`: the command and telemetry service.
  It uses the Nordic UART Service UUID base, but bHaptics added a dozen custom
  characteristics and did not keep the UART role convention. The
  characteristic bluetoothctl labels "Nordic UART TX" (`6e400002`) is the
  write channel here (`read`, `write-without-response`), and the one labeled
  "RX" (`6e400003`) is read-only. Go by the flags, not the names.

Characteristics under `6e400001`, with observed flags:

| UUID (`6e4000xx-...`) | Flags | Role |
|---|---|---|
| `02` | read, write-without-response | motor command (confirmed) |
| `03` | read | unknown |
| `05` | read, write | unknown |
| `07` | read | device-info block |
| `08` | read, notify | battery level |
| `0a` | read, write | unknown |
| `0b` | read, notify | unknown, likely telemetry |
| `0c` | read, write | unknown |
| `0d` | read, write | calibration or addressing table (guess) |
| `0e` | read, write | calibration or addressing table (guess) |
| `0f` | read, notify | unknown |
| `11` | read, write, notify | likely motor-state readback (unconfirmed) |

Only `02` is used by efferent. Findings from reading the others once, most to
least confident:

- `08` is battery level: a single byte, `0x64` (100) when full.
- `07` is a 20-byte device-info block. Bytes 4-9 equal the unit's own BLE MAC
  address. The rest is presumably firmware, protocol and model version
  fields, not decoded.
- `11` is 20 zero bytes, the same length as the command frame. If it is a
  motor-state readback, reading it after a buzz should mirror the active
  pattern. Untested.
- `0d` and `0e` hold structured, repeating byte-triples (a reordered set of
  the same values in `0e`), consistent with per-motor calibration or
  addressing.
- `0f` reads all `0xFF` (30 bytes), likely an unset sentinel.
- `03`, `05`, `0a` and `0c` gave no clean interpretation from a single read.

No characteristic notified spontaneously during a 4 s listen with the vest
idle.

## Motor command frame

A 20-byte write-without-response to `6e400002`. One nibble per motor, 40
motors, high nibble first: motor `N` lives in byte `N // 2`, in the high
nibble if `N` is even and the low nibble if `N` is odd. The nibble is
intensity, `0x0` (off) to `0xF` (max).

```
byte:    [0]   [1]   [2]  ...  [19]
nibble: 0  1  2  3  4  5  ... 38 39
```

Twenty zero bytes turn every motor off. Every frame encodes the full 40-motor
state, so a write turns off any motor it does not mention. Anything that
writes the vest has to account for that (see "Concurrent writers" below).

An independent Linux reverse-engineer (Orion Moonclaw, moonclaw.eu,
2026-03-08) documented the same nibble-packed 20-byte "Vest frame" for the
bHaptics TactSuit Air. The format appears to carry across the product line,
with only the motor-index-to-position mapping changing.

## Motor index to physical position (hypothesis)

A full 0-39 sweep, each motor buzzed about 0.9 s in index order, felt like
one continuous path: front-left, down the back-left, across to back-right,
down the front-right. One loop around the torso, not a mirrored left/right
pattern. Interpreted as four columns of 10 motors:

| Motor indices | Panel | Direction felt |
|---|---|---|
| 0-9 | front-left | top to bottom |
| 10-19 | back-left | top to bottom |
| 20-29 | back-right | top to bottom |
| 30-39 | front-right | top to bottom |

This is consistent with wiring the vest laid flat: front-left and back-left
sit next to each other at the left seam, back-right and front-right at the
right seam, and one wiring run across the flattened garment produces this
order.

No gaps or repeats were felt across the sweep. That is evidence against the
motor-duplication quirk reported for the Air: all 40 indices appear to
address 40 distinct physical motors. Buzzing the first and last motor of each
column (0/9, 10/19, 20/29, 30/39) felt like the corners of the panel,
consistent with the column hypothesis.

Not verified: exact boundaries within each column (rows 2-9), and whether
"top to bottom" is strictly monotonic within a column.

## Intensity and comfort

Motor 0 stepped through `0x1, 0x3, 0x5, 0x7, 0x9, 0xB, 0xD, 0xF` felt like a
graduated ramp, not on/off. `0xF` is mildly uncomfortable to wear, so
nothing routine should approach it.

A single test tap at each level shows whether a value is uncomfortable. It
says nothing about whether the value is calm enough to live with for hours. Routine signals worn
through a full session read as too strong at `0x8`-`0xA`, and even a
near-subliminal tap at `0x3` was startling in a recurring pattern. efferent
therefore keeps routine intensities at or below `0x5` (`mixerComfortCeiling`
and `vestIntensity`), and `vest.go` clamps every write to `0xA`
(`vestHardCeiling`). These values were tuned by feel and are not a confirmed
floor.

Placement matters as much as intensity. Firing both front shoulder columns
together (20 motors, `0xA`, 500 ms) felt harsher and more alarm-like than the
back channel at the same intensity. Chest placement may read as more alarming
than back placement, or 20 simultaneous motors may simply be a lot more
sensation than a single-motor test. New front-channel signals should start
with fewer motors, one column and a shorter duration.

## Concurrent writers

Two writers can target the same motors: the mixer (every 50 ms while any
channel is active) and the fatigue alert (`Buzz`, an on/sleep/off sequence
lasting 500 ms). Because each frame replaces the whole vest state, a mixer
tick landing between a buzz's "on" and "off" writes would silently truncate
the alert. `Vest.Buzz` therefore holds `writeMu` for its whole burst, and the
mixer's tick waits. The keepalive (an all-zero ping every 30 s) skips itself
whenever anything else has written more recently.

## Sub-nibble intensity by dithering

Intensity has 16 levels, which is coarse for a fader that should taper
smoothly. At low gain, a signal whose scaled value stays below 1.0 truncates
to zero on every tick, so a fader sits at "nothing" until it jumps to
"something". `blend()` in `mixer.go` fixes this with delta-sigma dithering:
a per-motor accumulator adds the true target each tick, emits the whole-number
part, and keeps the remainder, so a target of 0.3 comes out as one pulse in
about three ticks.

Whether this reads as smoother than separate taps depends on the ERM motor's
mechanical low-pass, because the eccentric mass has to physically accelerate
and decelerate (Precision Microdrives application note AB-004, "Understanding
ERM Vibration Motor Characteristics"). The figures for it, roughly 10-20 Hz
with 30-40 ms rise and decay, are recalled and not checked against a
readable source. In use, a fader eased down on a real MIDImix tapers
smoothly. That is a felt report with no measurement behind it, and it does not show
20 Hz is the best rate.

## Open questions

- Does write-with-response work as a fallback? The flags suggest
  write-without-response is the only supported mode.
- What does the vest do if the host disappears mid-frame? It is unknown
  whether the firmware stops motors on disconnect. efferent writes an
  all-off frame on SIGINT and SIGTERM, but a crash or SIGKILL cannot.
