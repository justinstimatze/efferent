# efferent

efferent is a Linux daemon that turns the activity of Claude Code coding-agent sessions into things you can feel and see at your desk: a vibrating vest on your back, two room lights that slowly brighten and dim, and a grid of lit pads showing which sessions are working. It can also react to body and room readings published by its sibling project [afferent](https://github.com/justinstimatze/afferent), which is the sensor side (efferent is the output side).

The idea is to let someone running several AI agents at once keep track of them without reading a terminal. Quiet, peripheral signals (a light's breathing rate, a slow wave of vibration) carry the coarse state, and a physical grid carries the detail when you go looking for it.

**Status: experimental, built for one person's desk.** It runs daily on one Linux machine with one set of hardware. Some device IDs and layout choices are specific to that setup, there is no installer, and none of the file formats or code is a stable interface. Read it, borrow from it, and expect to edit it.

## What you would experience

[Claude Code](https://claude.com/claude-code) is Anthropic's coding agent, which runs in a terminal. Each running copy is a session, and the author keeps many of them open at once in tmux panes (tmux is a terminal multiplexer; efferent expects a tmux session named `cc-cockpit` for this).

- **Launchpad (looked at and touched).** A Novation Launchpad X is a USB grid of 64 pads that can be lit any color. Each live session gets one pad, placed to mirror its tmux pane and colored by state: working, waiting for you, or stuck. A pad turns "stuck" (red) after three tool calls in a row have failed in that session. A cluster of pads in the corner works as a gauge of how much the whole fleet is doing. Pressing a session's pad jumps tmux to that pane and zooms it. A short staged animation plays when the daemon starts.
- **Launchpad control row (touched).** The top row of buttons records answers: a hold toggle, four self-report buttons (calm, ok, frayed, done), and a pair rating the agent's last reply as landed or missed. Presses are written to files. Nothing reads those files yet.
- **Hue lights (glanced at).** Two Philips Hue bulbs, called "Machine's Breath" here, brighten and dim around a fixed soft-white level. A breath takes about 3 seconds while the active agent is working and about 12 seconds while it waits for you. This channel starts muted.
- **Haptic vest (felt).** A bHaptics TactSuit X40 is a wearable vest with 40 small vibration motors. Two things drive it. A slow wave that rises up the back, called "fleet breath", speeds up and strengthens as more sessions are working (it saturates at four). It also starts muted. Separately, when afferent reports that the session has gone past a fatigue threshold, the vest gives one sharp pulse on the back, once per crossing.
- **MIDImix (turned by hand).** An Akai MIDImix is a USB mixing-console controller with faders, knobs and buttons. Here it works as a mixer for the vest and the lights: strip 1 is fleet breath on the vest, strip 2 is Machine's Breath on the lights. Each strip has a fader for gain, a mute button and three knobs, plus a master fader. For fleet breath the knobs set the vertical center of the wave, its left/right balance and how sharp its edge is. The Solo button is not wired up.

[`signal-vocabulary.md`](signal-vocabulary.md) explains why each signal looks and feels the way it does.

## Hardware

Everything is optional. Each device is searched for in the background and retried (every 90 seconds for the vest, Launchpad and MIDImix); a missing device is logged and the rest keeps running.

- bHaptics TactSuit X40 haptic vest, over Bluetooth Low Energy (no vendor SDK is used; see [`vest-protocol.md`](vest-protocol.md))
- Philips Hue bridge with at least one light, on the local network
- Novation Launchpad X
- Akai MIDImix
- A Linux machine with BlueZ (the Linux Bluetooth stack) and ALSA (the Linux sound and MIDI layer; the two controllers are driven through its raw MIDI devices, found via `/proc/asound/cards`)
- tmux, with a session named `cc-cockpit`, for the Launchpad pad layout and jump-to-session
- afferent, for the fatigue pulse

## Build and run

Requires Linux and Go 1.25 or newer (`go.mod`).

```bash
make build     # ./efferent and ./agenthook/agenthook
make install   # builds both into ~/go/bin
make run       # go run .
go test ./...  # tests
```

To run as a systemd user service (the daemon needs your user D-Bus session to reach BlueZ):

```bash
make service-install   # installs the unit from deploy/systemd/ and enables it
make service-start
make service-status
make service-logs
make service-restart   # reinstall and restart
make service-stop
make service-uninstall
loginctl enable-linger $USER   # once, if it should keep running after you log out
```

`service-install` enables the unit but does not start it; run `make service-start` to start it now. The unit rebuilds the binary from the checkout on every start. The binary logs its build stamp once at startup, so `make service-logs` shows which build is running.

There is also a small diagnostic, `cmd/lpdiag`, for Launchpad connection problems: `go run ./cmd/lpdiag <alsa-card-index>`, with the card index from `/proc/asound/cards`.

Three read-only terminal views live in `scripts/`. They read the state files the daemon writes and never touch a device. They need `jq`.

- `scripts/vest-viz`: the last 40-motor frame actually sent to the vest
- `scripts/mixer-viz`: the state of every MIDImix strip and the master fader (`--once` draws one frame)
- `scripts/fleet-legend-tui`: a terminal copy of the Launchpad grid (`--once`, `--describe`; `?` toggles a legend, `q` quits)

## Configuration

There is no config file. Device wiring is in the Go source (for example `targetLights` in `main.go` and the MIDImix control numbers in `midimix.go`), so adapting this to other hardware means editing code.

State lives in `$XDG_STATE_HOME/efferent/` (default `~/.local/state/efferent/`), created with mode 0700, files with 0600.

**Hue.** Pair with your bridge using Philips' application-key procedure, then write `hue_bridge.json` in that directory:

```json
{ "bridge_id": "<bridge id>", "bridge_ip": "<last known ip>", "username": "<application key>" }
```

Set `targetLights` in `main.go` to the IDs of your own lights; the default is `"1"` and `"10"`. Without `hue_bridge.json`, Hue is disabled and the log says so.

**afferent.** efferent watches afferent's `state.json` at `$XDG_STATE_HOME/afferent/state.json` (fsnotify, with a 60-second fallback check). Without it, the fatigue pulse never fires.

**agenthook.** `agenthook` is a Claude Code hook: a small program Claude Code runs on events such as a prompt being submitted or a turn ending. It records the agent's turn phase and consecutive tool-error count in `agent_state.json`, which drives Machine's Breath. Build it with `make build` or `make install`, then register it in your Claude Code settings with your own path:

```json
{
  "hooks": {
    "UserPromptSubmit":   [{ "matcher": "", "hooks": [{ "type": "command", "command": "/path/to/agenthook" }] }],
    "PostToolUse":        [{ "matcher": "", "hooks": [{ "type": "command", "command": "/path/to/agenthook" }] }],
    "PostToolUseFailure": [{ "matcher": "", "hooks": [{ "type": "command", "command": "/path/to/agenthook" }] }],
    "Stop":               [{ "matcher": "", "hooks": [{ "type": "command", "command": "/path/to/agenthook" }] }]
  }
}
```

The hook always exits 0 and never blocks or alters the conversation.

**Files the daemon writes** in the state directory: `vest_frame.json`, `mixer_state.json`, `fleet_legend.json` (the views above read these), `mixer_moves.jsonl`, `hold.json`, `labels.jsonl` and `turn_ratings.jsonl`.

## Privacy and safety

**What it reads**
- `afferent/state.json`: CO2 level, heart rate and a list of named factors. Only the fatigue factor is acted on.
- Claude Code transcripts under `~/.claude/projects/`. For each recently active session (touched in the last 30 minutes) it reads the last 256 KiB of the transcript and uses only record types, stop reasons, timestamps, tool-error flags and whether a block is text. It does not read message text.
- tmux pane IDs and working directories for the `cc-cockpit` session.
- `agenthook` stores the phase, the tool name, an error count and the last error message cut to 200 bytes. Error text can contain paths or command output, so treat `agent_state.json` as private.
- Self-report presses are stored with a snapshot of afferent's state in `labels.jsonl`.

**What leaves the machine.** The only network code is the Hue client in `main.go`. Normally it talks to the bridge on your LAN. If the saved bridge IP stops answering, it queries Philips' discovery service (`discovery.meethue.com`), which reveals your network's public IP to Philips. The bridge's certificate is signed by a private CA, so TLS verification is turned off and the bridge is identified by its ID instead. An attacker on your LAN could therefore intercept the application key.

**Vest intensity.** Each motor takes a level from 0x0 to 0xF. Routine signals are limited to 0x5 by the mixer, fleet breath peaks at 0x2 to 0x3, and every frame is clamped to 0xA before it is written. These limits were tuned by feel on one person and are not a medical or clinical threshold. 0xF is described in `vest-protocol.md` as mildly uncomfortable to wear.

**Shutdown and crashes.** On SIGINT or SIGTERM (Ctrl-C, `systemctl stop`) the daemon writes an all-off frame to the vest, waiting at most 2 seconds. SIGKILL or a crash cannot send that frame, and it is unknown whether the vest firmware stops its motors when the connection drops. The first time you run it, keep the vest's power button within reach.

## Further reading

- [`signal-vocabulary.md`](signal-vocabulary.md): the fixed set of signals per device, how they compose, and the design reasoning
- [`vest-protocol.md`](vest-protocol.md): the reverse-engineered Bluetooth protocol of the vest, motor layout and intensity findings, with open questions
- [`prior-art.md`](prior-art.md): literature behind the design, with citation status marked
- [`haptic-mixing-prior-art.md`](haptic-mixing-prior-art.md): research on how simultaneous vibration channels combine and on mixing-console interfaces

These documents cite research on touch, ambient displays and physiological computing as design context. Nothing in this repository has been tested for any health or wellness effect.

## License

MIT. See [`LICENSE`](LICENSE).

## Contact

justin@justinstimatze.com
