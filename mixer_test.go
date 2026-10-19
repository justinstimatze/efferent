package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// constChannel renders the same fixed per-motor map every tick -- enough
// to test blend()'s combine/gain/mute/solo logic without any real
// channel's own timing behavior in the way. Implements vestChannel;
// ignores dial entirely, since none of blend()'s own tests exercise
// placement math (that's familiars_breath_test.go's job).
type constChannel struct {
	name  string
	frame map[int]byte
}

func (c constChannel) Name() string { return c.name }
func (c constChannel) RenderVest(now time.Time, dial [3]float64) map[int]byte {
	return c.frame
}

func unmutedControl(gain float64) channelControl {
	return channelControl{Gain: gain, Mute: false}
}

func TestBlendSingleChannelPassesThroughGained(t *testing.T) {
	channels := [8]mixerChannel{0: constChannel{"a", map[int]byte{10: 4}}}
	var controls [8]channelControl
	controls[0] = unmutedControl(0.5)
	out := blend(time.Now(), channels, controls, 1.0, &ditherState{})
	if got := out[10]; got != 2 {
		t.Errorf("motor 10 = %v, want 2 (4 * 0.5 gain)", got)
	}
}

func TestBlendTwoChannelsCombineViaRootSumSquareNotSumOrMax(t *testing.T) {
	// Israr & Poupyrev's validated model (haptic-mixing-prior-art.md
	// Thread A1): sqrt(3^2 + 4^2) = 5 -- the classic 3-4-5 triple, chosen
	// so sum (7), max (4), and RSS (5) are all different and unambiguous.
	channels := [8]mixerChannel{
		0: constChannel{"a", map[int]byte{10: 3}},
		1: constChannel{"b", map[int]byte{10: 4}},
	}
	var controls [8]channelControl
	controls[0] = unmutedControl(1.0)
	controls[1] = unmutedControl(1.0)
	out := blend(time.Now(), channels, controls, 1.0, &ditherState{})
	if got := out[10]; got != 5 {
		t.Errorf("motor 10 = %v, want 5 (root-sum-square of 3 and 4), not sum (7) or max (4)", got)
	}
}

func TestBlendClampsToComfortCeiling(t *testing.T) {
	channels := [8]mixerChannel{
		0: constChannel{"a", map[int]byte{10: 15}},
		1: constChannel{"b", map[int]byte{10: 15}},
	}
	var controls [8]channelControl
	controls[0] = unmutedControl(1.0)
	controls[1] = unmutedControl(1.0)
	out := blend(time.Now(), channels, controls, 1.0, &ditherState{})
	if got := out[10]; got != mixerComfortCeiling {
		t.Errorf("motor 10 = %v, want clamped to the ceiling %v", got, byte(mixerComfortCeiling))
	}
}

func TestBlendMutedChannelContributesNothing(t *testing.T) {
	channels := [8]mixerChannel{0: constChannel{"a", map[int]byte{10: 4}}}
	var controls [8]channelControl
	controls[0] = channelControl{Gain: 1.0, Mute: true}
	out := blend(time.Now(), channels, controls, 1.0, &ditherState{})
	if _, ok := out[10]; ok {
		t.Errorf("muted channel contributed to the frame: %+v", out)
	}
}

func TestBlendSoloSilencesUnsoloedChannels(t *testing.T) {
	channels := [8]mixerChannel{
		0: constChannel{"a", map[int]byte{10: 4}},
		1: constChannel{"b", map[int]byte{20: 4}},
	}
	var controls [8]channelControl
	controls[0] = unmutedControl(1.0)
	controls[1] = unmutedControl(1.0)
	controls[1].Solo = true
	out := blend(time.Now(), channels, controls, 1.0, &ditherState{})
	if _, ok := out[10]; ok {
		t.Errorf("channel 0 (not soloed) contributed while channel 1 is soloed: %+v", out)
	}
	if got := out[20]; got != 4 {
		t.Errorf("soloed channel 1's motor 20 = %v, want 4", got)
	}
}

func TestBlendSoloOnMutedChannelDoesNotSilenceOthers(t *testing.T) {
	// A muted-and-soloed channel can't contribute either way -- soloing it
	// must not still trigger solo-exclusivity and silence channel 1, which
	// was already playing normally.
	channels := [8]mixerChannel{
		0: constChannel{"a", map[int]byte{10: 4}},
		1: constChannel{"b", map[int]byte{20: 4}},
	}
	var controls [8]channelControl
	controls[0] = channelControl{Gain: 1.0, Mute: true, Solo: true}
	controls[1] = unmutedControl(1.0)
	out := blend(time.Now(), channels, controls, 1.0, &ditherState{})
	if _, ok := out[10]; ok {
		t.Errorf("muted channel 0 contributed despite being muted: %+v", out)
	}
	if got := out[20]; got != 4 {
		t.Errorf("channel 1 = %v, want 4 -- a muted solo shouldn't silence an unrelated unmuted channel", got)
	}
}

func TestBlendMasterScalesFinalOutput(t *testing.T) {
	channels := [8]mixerChannel{0: constChannel{"a", map[int]byte{10: 4}}}
	var controls [8]channelControl
	controls[0] = unmutedControl(1.0)
	out := blend(time.Now(), channels, controls, 0.5, &ditherState{})
	if got := out[10]; got != 2 {
		t.Errorf("motor 10 = %v, want 2 (4 * master 0.5)", got)
	}
}

func TestBlendUnassignedChannelSlotIsSkipped(t *testing.T) {
	var channels [8]mixerChannel
	var controls [8]channelControl
	out := blend(time.Now(), channels, controls, 1.0, &ditherState{})
	if len(out) != 0 {
		t.Errorf("blend with no assigned channels = %+v, want empty", out)
	}
}

// constHueChannel is constChannel's hueChannel counterpart -- a fixed
// (delta, push, transitionDS) every call, enough to test blendHue's own
// gain/mute/solo logic without any real channel's timing behavior in the
// way.
type constHueChannel struct {
	name  string
	delta int
	push  bool
	ds    int
}

func (c constHueChannel) Name() string { return c.name }
func (c constHueChannel) RenderHue(now time.Time) (int, bool, int) {
	return c.delta, c.push, c.ds
}

func TestBlendHueAppliesGainAndMaster(t *testing.T) {
	channels := [8]mixerChannel{0: constHueChannel{"h", 20, true, 5}}
	var controls [8]channelControl
	controls[0] = unmutedControl(0.5)
	delta, push, ds := blendHue(time.Now(), channels, controls, 1.0, false)
	if !push {
		t.Fatal("want push=true when the channel reports one")
	}
	if delta != 10 {
		t.Errorf("delta = %d, want 10 (20 * gain 0.5 * master 1.0)", delta)
	}
	if ds != 5 {
		t.Errorf("transitionDS = %d, want the channel's own 5", ds)
	}
}

func TestBlendHueMutedChannelContributesNothingAndDoesNotPush(t *testing.T) {
	channels := [8]mixerChannel{0: constHueChannel{"h", 20, true, 5}}
	var controls [8]channelControl
	controls[0] = channelControl{Gain: 1.0, Mute: true}
	delta, push, _ := blendHue(time.Now(), channels, controls, 1.0, false)
	if push {
		t.Error("a muted hue channel must not push")
	}
	if delta != 0 {
		t.Errorf("delta = %d, want 0 -- a muted channel's RenderHue should never even be called", delta)
	}
}

func TestBlendHueChannelReportingNoPushYieldsNoPush(t *testing.T) {
	channels := [8]mixerChannel{0: constHueChannel{"h", 20, false, 0}}
	var controls [8]channelControl
	controls[0] = unmutedControl(1.0)
	_, push, _ := blendHue(time.Now(), channels, controls, 1.0, false)
	if push {
		t.Error("want push=false when no channel reports a fresh target this tick")
	}
}

// TestComputeAnySoloIsSharedAcrossVestAndHueChannels guards mixer.go's own
// design claim: solo is mixer-wide, not per-target. A soloed vest channel
// must silence an unmuted, unsoloed Hue channel just as it would silence
// an unsoloed vest channel.
func TestComputeAnySoloIsSharedAcrossVestAndHueChannels(t *testing.T) {
	channels := [8]mixerChannel{
		0: constChannel{"v", map[int]byte{10: 4}},
		1: constHueChannel{"h", 20, true, 5},
	}
	var controls [8]channelControl
	controls[0] = unmutedControl(1.0)
	controls[0].Solo = true
	controls[1] = unmutedControl(1.0)

	anySolo := computeAnySolo(channels, controls)
	if !anySolo {
		t.Fatal("want anySolo=true")
	}
	if delta, push, _ := blendHue(time.Now(), channels, controls, 1.0, anySolo); push || delta != 0 {
		t.Errorf("hue channel (not soloed) contributed while the vest channel is soloed: delta=%d push=%v", delta, push)
	}
}

// TestBlendDitherAveragesSubTruncationTargetOverTime guards against a
// target that never reaches 1.0 on its own -- raw peak 3 * gain 0.1 = 0.3
// -- truncating to 0 every single tick, no matter how long the channel
// runs. With dithering it must average out to roughly the right value over
// many ticks instead.
func TestBlendDitherAveragesSubTruncationTargetOverTime(t *testing.T) {
	channels := [8]mixerChannel{0: constChannel{"a", map[int]byte{10: 3}}}
	var controls [8]channelControl
	controls[0] = unmutedControl(0.1) // 3 * 0.1 = 0.3, always truncated to 0 without dithering

	ds := &ditherState{}
	const ticks = 100
	sum := 0
	for i := 0; i < ticks; i++ {
		out := blend(time.Now(), channels, controls, 1.0, ds)
		sum += int(out[10])
	}

	// Delta-sigma's error is bounded to less than one unit: the emitted
	// sum over N ticks at a constant target t lands in (N*t - 1, N*t] --
	// 100 * 0.3 = 30, so 29 or 30. Not 0, which is what plain byte()
	// truncation gave forever.
	if sum < 29 || sum > 30 {
		t.Errorf("sum of out[10] over %d ticks at target 0.3 = %d, want 29 or 30 (bounded delta-sigma error), and definitely not 0", ticks, sum)
	}
}

// TestBlendDitherResetsAccumulatorWhenMotorGoesUntouched guards against a
// channel's leftover fractional debt surviving past the tick it stopped
// touching a motor: without the reset, a later, unrelated channel's very
// first (tiny) value on the same motor could cash in a pulse it never
// earned, purely from a stranger's stale accumulator state.
func TestBlendDitherResetsAccumulatorWhenMotorGoesUntouched(t *testing.T) {
	ds := &ditherState{}
	ds.accum[10] = 0.99 // one tick away from emitting, as if a prior channel had just been driving it

	// A tick where nothing touches motor 10 at all.
	var channels [8]mixerChannel
	var controls [8]channelControl
	blend(time.Now(), channels, controls, 1.0, ds)
	if ds.accum[10] != 0 {
		t.Fatalf("accum[10] = %v after an untouched tick, want reset to 0", ds.accum[10])
	}

	// A different channel's small value on the same motor right after.
	channels[0] = constChannel{"b", map[int]byte{10: 1}}
	controls[0] = unmutedControl(0.02) // 1 * 0.02 = 0.02
	out := blend(time.Now(), channels, controls, 1.0, ds)
	if got := out[10]; got != 0 {
		t.Errorf("motor 10 = %v right after reset, want 0 -- the old channel's leftover debt must not have survived", got)
	}
}

func TestMixerControlDefaultsMutedWithGainUp(t *testing.T) {
	mc := newMixerControl()
	channels, master := mc.snapshot()
	if master != 1.0 {
		t.Errorf("master = %v, want 1.0", master)
	}
	for i, c := range channels {
		if !c.Mute {
			t.Errorf("channel %d: Mute = false, want true (silent until opted in)", i)
		}
		if c.Gain != 1.0 {
			t.Errorf("channel %d: Gain = %v, want 1.0", i, c.Gain)
		}
	}
}

func TestMixerControlSetGetMute(t *testing.T) {
	mc := newMixerControl()
	if !mc.getMute(2) {
		t.Fatal("channel 2 should start muted")
	}
	mc.setMute(2, false)
	if mc.getMute(2) {
		t.Error("channel 2 should be unmuted after setMute(2, false)")
	}
}

func TestFrameIsActiveEmptyMapIsInactive(t *testing.T) {
	if frameIsActive(map[int]byte{}) {
		t.Error("empty frame should be inactive")
	}
}

func TestFrameIsActiveAllZeroValuesIsInactive(t *testing.T) {
	// familiarBreathFrame's own convention: a full map including explicit
	// zeros at the trough. Must read as inactive exactly like an empty map
	// would, or a channel using this convention pins the vest at 20Hz
	// writes forever.
	if frameIsActive(map[int]byte{10: 0, 11: 0, 12: 0}) {
		t.Error("a frame with only zero values should be inactive")
	}
}

func TestFrameIsActiveAnyNonzeroValueIsActive(t *testing.T) {
	if !frameIsActive(map[int]byte{10: 0, 11: 1}) {
		t.Error("a frame with a nonzero value should be active")
	}
}

// resetMixerMoveThrottle clears mixerMoveThrottle's cross-test state --
// it's a package-level var shared by every test in this binary, so a
// preceding test's recent call for the same (channel, control) key would
// otherwise spuriously throttle this one depending on execution speed.
func resetMixerMoveThrottle(t *testing.T) {
	t.Helper()
	mixerMoveThrottle = newMoveThrottle()
}

// TestLogMixerMoveWritesRealJSONTypes guards against a regression back to
// mixerMoveEntry.Value being pre-stringified (fmt.Sprintf) rather than a
// real JSON number/bool -- the latter is what lets mixer-viz's jq read
// .value directly instead of parsing a quoted string.
func TestLogMixerMoveWritesRealJSONTypes(t *testing.T) {
	setStateDir(t)
	resetMixerMoveThrottle(t)
	path := statePath("efferent", "mixer_moves.jsonl")

	logMixerMove(0, "gain", 0.5)
	logMixerMoveBool(0, "mute", true)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading mixer_moves.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), data)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &raw); err != nil {
		t.Fatalf("unmarshaling gain entry: %v", err)
	}
	if got := string(raw["value"]); got != "0.5" {
		t.Errorf("gain entry value = %s, want the bare JSON number 0.5, not a quoted string", got)
	}

	if err := json.Unmarshal([]byte(lines[1]), &raw); err != nil {
		t.Fatalf("unmarshaling mute entry: %v", err)
	}
	if got := string(raw["value"]); got != "true" {
		t.Errorf("mute entry value = %s, want the bare JSON boolean true, not a quoted string", got)
	}
}

// TestLogMixerMoveThrottlesRepeatedCallsForSameKey guards against the
// logMixerMove appending one line per raw MIDI CC message, which would
// turn one fader sweep (100+ messages) into 100+ file opens.
func TestLogMixerMoveThrottlesRepeatedCallsForSameKey(t *testing.T) {
	setStateDir(t)
	resetMixerMoveThrottle(t)
	path := statePath("efferent", "mixer_moves.jsonl")

	logMixerMove(0, "gain", 0.1)
	logMixerMove(0, "gain", 0.2) // same key, within the throttle window -- dropped
	logMixerMove(1, "gain", 0.9) // different channel -- its own key, not throttled by channel 0's

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading mixer_moves.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (channel 0's first move, channel 1's move): %q", len(lines), data)
	}
}

func TestMoveThrottleAllowsAgainAfterIntervalElapses(t *testing.T) {
	th := newMoveThrottle()
	key := moveKey{0, "gain"}
	base := time.Now()
	if !th.allow(key, 10*time.Millisecond, base) {
		t.Fatal("first call for a fresh key should be allowed")
	}
	if th.allow(key, 10*time.Millisecond, base.Add(5*time.Millisecond)) {
		t.Error("a call within the interval should be throttled")
	}
	if !th.allow(key, 10*time.Millisecond, base.Add(11*time.Millisecond)) {
		t.Error("a call past the interval should be allowed again")
	}
}

func TestRecordMixerStateVizWritesChannelNamesAndControls(t *testing.T) {
	setStateDir(t)
	lastMixerStateViz = time.Time{}

	channels := [8]mixerChannel{0: constChannel{"fleet-breath", nil}}
	var controls [8]channelControl
	controls[0] = channelControl{Gain: 0.75, Mute: false, Solo: false, Dial: [3]float64{0.1, 0.2, 0.3}}
	recordMixerStateViz(channels, controls, 0.9)

	data, err := os.ReadFile(statePath("efferent", "mixer_state.json"))
	if err != nil {
		t.Fatalf("reading mixer_state.json: %v", err)
	}
	var viz mixerStateViz
	if err := json.Unmarshal(data, &viz); err != nil {
		t.Fatalf("unmarshaling mixer_state.json: %v", err)
	}
	if viz.Channels[0].Name != "fleet-breath" {
		t.Errorf("channel 0 name = %q, want %q", viz.Channels[0].Name, "fleet-breath")
	}
	if viz.Channels[0].Gain != 0.75 {
		t.Errorf("channel 0 gain = %v, want 0.75", viz.Channels[0].Gain)
	}
	if viz.Channels[0].Dial != [3]float64{0.1, 0.2, 0.3} {
		t.Errorf("channel 0 dial = %v, want [0.1 0.2 0.3]", viz.Channels[0].Dial)
	}
	if viz.Channels[1].Name != "" {
		t.Errorf("channel 1 (unassigned) name = %q, want empty", viz.Channels[1].Name)
	}
	if viz.Master != 0.9 {
		t.Errorf("master = %v, want 0.9", viz.Master)
	}
}

func TestRecordMixerStateVizThrottles(t *testing.T) {
	setStateDir(t)
	lastMixerStateViz = time.Time{}

	var channels [8]mixerChannel
	var controls [8]channelControl
	recordMixerStateViz(channels, controls, 1.0)
	firstWrite := lastMixerStateViz

	recordMixerStateViz(channels, controls, 0.5) // immediately after -- within the throttle window
	if lastMixerStateViz != firstWrite {
		t.Error("recordMixerStateViz wrote again within the throttle interval")
	}
}
