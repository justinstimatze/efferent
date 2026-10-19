// efferent closes afferent's sensor loop physically: it reads afferent's
// on-disk state snapshot (the same integration surface tint already uses)
// and drives Hue, a haptic vest, and a Launchpad from it. Ambient, coarse,
// edge-triggered -- informs, never commands, same restraint as tint's own
// design.
//
// CO2 is deliberately not one of those signals: a color mirror was
// unreadable against Machine's Breath's brightness rhythm, and a vest pulse
// on a smooth threshold (not a real edge) was attention-getting for little
// information (see signal-vocabulary.md). "The air is bad, open a window"
// is better said in words than felt or seen, so efferent has no channel
// for it.
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const requestTimeout = 4 * time.Second

// targetLights are Hue light IDs on the paired bridge -- specific to the
// author's room. Change them to the IDs of your own lights (list them with
// GET /api/<username>/lights on the bridge).
var targetLights = []string{"1", "10"}

type hueBridge struct {
	BridgeID string `json:"bridge_id"`
	BridgeIP string `json:"bridge_ip"`
	Username string `json:"username"`
}

type afferentState struct {
	Timestamp string   `json:"timestamp"`
	CO2PPM    int      `json:"co2_ppm"`
	CO2Bucket string   `json:"co2_bucket"`
	Factors   []string `json:"factors"`
	HRBPM     float64  `json:"hr_bpm"`
}

type lightState struct {
	CT  int // mireds (153-500); > 0 means native color-temperature mode instead of hue/sat
	Hue int
	Sat int
	Bri int
}

// normalCT/normalBri are the fixed soft-white baseline Machine's Breath
// breathes around.
const (
	normalCT  = 370 // ~2700K soft white
	normalBri = 200
)

var httpClient = &http.Client{
	Timeout: requestTimeout,
	Transport: &http.Transport{
		// The Hue bridge's certificate is issued for its bridge ID, not its LAN
		// IP, and is signed by a private CA, so normal verification fails.
		// Verification is skipped; the bridge's identity is instead confirmed
		// by comparing config.bridgeid to the paired ID (resolveBridge).
		// Consequence: the application key is sent over TLS that a LAN
		// attacker could intercept.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

func statePath(app, file string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(base, app, file)
}

func loadHueBridge() (hueBridge, error) {
	var b hueBridge
	data, err := os.ReadFile(statePath("efferent", "hue_bridge.json"))
	if err != nil {
		return b, fmt.Errorf("reading hue_bridge.json: %w", err)
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("parsing hue_bridge.json: %w", err)
	}
	return b, nil
}

func saveHueBridge(b hueBridge) error {
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	path := statePath("efferent", "hue_bridge.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return nil
}

// bridgeConfigID confirms the bridge at ip actually is the one we paired
// with, not just some bridge -- necessary because the IP is DHCP-assigned
// and drifts (see README.md).
func bridgeConfigID(ip, username string) (string, error) {
	url := fmt.Sprintf("https://%s/api/%s/config", ip, username)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var cfg struct {
		BridgeID string `json:"bridgeid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", err
	}
	return cfg.BridgeID, nil
}

// rediscoverIP falls back to Philips' cloud discovery endpoint
// (discovery.meethue.com), which reveals this network's public IP to
// Philips. Only used when the cached IP no longer answers or answers as the
// wrong bridge; the steady-state path is fully local.
func rediscoverIP(wantID string) (string, error) {
	req, _ := http.NewRequest("GET", "https://discovery.meethue.com/", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var candidates []struct {
		ID string `json:"id"`
		IP string `json:"internalipaddress"`
	}
	if err := json.Unmarshal(body, &candidates); err != nil {
		return "", err
	}
	for _, c := range candidates {
		if strings.EqualFold(c.ID, wantID) {
			return c.IP, nil
		}
	}
	return "", fmt.Errorf("bridge id %s not found in cloud discovery response", wantID)
}

// resolveBridge returns a verified-reachable IP for the paired bridge,
// re-resolving and updating the cache if the cached IP has drifted.
func resolveBridge(b *hueBridge) error {
	if id, err := bridgeConfigID(b.BridgeIP, b.Username); err == nil && strings.EqualFold(id, b.BridgeID) {
		return nil // cached IP still good
	}
	log.Printf("cached bridge IP %s unreachable or stale, re-resolving via cloud discovery", b.BridgeIP)
	ip, err := rediscoverIP(b.BridgeID)
	if err != nil {
		return fmt.Errorf("re-resolving bridge: %w", err)
	}
	id, err := bridgeConfigID(ip, b.Username)
	if err != nil || !strings.EqualFold(id, b.BridgeID) {
		return fmt.Errorf("re-resolved IP %s did not confirm as bridge %s", ip, b.BridgeID)
	}
	log.Printf("bridge moved: %s -> %s", b.BridgeIP, ip)
	b.BridgeIP = ip
	return saveHueBridge(*b)
}

func readAfferentState() (afferentState, error) {
	var s afferentState
	data, err := os.ReadFile(statePath("afferent", "state.json"))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	return s, nil
}

// agentState is efferent's own minimal mirror of agenthook's AgentState
// schema (agenthook/main.go), trimmed to the field Machine's Breath reads.
// Phase's zero value ("") is treated as "awaiting_user" (idle rhythm) by
// every reader: it is what a freshly booted machine, or one the hook has
// never fired on, looks like -- not an error state.
type agentState struct {
	Phase string `json:"phase"`
}

// readAgentState reads agenthook's agent_state.json -- same zero-value/
// error-on-missing-or-corrupt shape as readAfferentState, never panics.
func readAgentState() (agentState, error) {
	var s agentState
	data, err := os.ReadFile(statePath("efferent", "agent_state.json"))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	return s, nil
}

// pushLight sets one light's color/brightness -- ct mode when ls.CT is set,
// hue/sat mode otherwise. transitionDS is Hue's transitiontime unit
// (deciseconds); Machine's Breath is the only caller and passes a value
// matched to its current breath half-cycle, so the bridge does the smooth
// interpolation instead of efferent stepping brightness by hand.
func pushLight(bridge hueBridge, lightID string, ls lightState, transitionDS int) error {
	body := map[string]any{
		"on":             true,
		"bri":            ls.Bri,
		"transitiontime": transitionDS,
	}
	if ls.CT > 0 {
		body["ct"] = ls.CT
	} else {
		body["hue"] = ls.Hue
		body["sat"] = ls.Sat
	}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("https://%s/api/%s/lights/%s/state", bridge.BridgeIP, bridge.Username, lightID)
	req, _ := http.NewRequest("PUT", url, strings.NewReader(string(payload)))
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	log.Printf("efferent: hue PUT to light %s took %s (bridge round-trip, before any transitiontime fade)", lightID, time.Since(start))
	return nil
}

// setupHue does the one-time bridge pairing/resolution. Soft-fails
// (ok=false, logged) rather than killing the process on a missing/
// unreachable bridge, so a Hue outage never takes the vest side down with
// it. retryable distinguishes the two failure shapes: an unpaired bridge
// (no hue_bridge.json) is a permanent config state until re-paired,
// so it's not worth polling forever, but a paired bridge that's merely
// unreachable at this exact moment (network hiccup, bridge mid-reboot) is
// -- the caller retries only the latter.
func setupHue() (bridge hueBridge, ok, retryable bool) {
	b, err := loadHueBridge()
	if err != nil {
		log.Printf("efferent: hue disabled (%v; pair with the bridge first)", err)
		return hueBridge{}, false, false
	}
	if err := resolveBridge(&b); err != nil {
		log.Printf("efferent: hue bridge unreachable (%v), will retry with backoff", err)
		return hueBridge{}, false, true
	}
	log.Printf("efferent: bridge %s confirmed at %s, driving lights %v", b.BridgeID, b.BridgeIP, targetLights)
	return b, true, false
}

// hueHandle is the small mutex-guarded handle for Hue's current connection
// state -- mirrors vestHandle in shape and reason: the main dispatch loop
// and Machine's Breath's independent breathing loop (dispatch.go) both need
// to read the current bridge/availability without racing on it or running
// two separate, driftable "is Hue up" retry loops for the same bridge.
type hueHandle struct {
	mu        sync.Mutex
	bridge    hueBridge
	ok        bool
	retryable bool
}

func (h *hueHandle) set(bridge hueBridge, ok, retryable bool) {
	h.mu.Lock()
	h.bridge, h.ok, h.retryable = bridge, ok, retryable
	h.mu.Unlock()
}

// get returns the current bridge and whether it's usable right now.
func (h *hueHandle) get() (hueBridge, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bridge, h.ok
}

// needsRetry reports whether Hue is down in a way worth retrying (paired
// but momentarily unreachable, not simply never paired).
func (h *hueHandle) needsRetry() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.ok && h.retryable
}

// buildInfo is overridden at build time via -ldflags (see Makefile: every
// build/install target stamps it with the git commit and build timestamp).
// Logged once at startup so "is the running binary current" is answered by
// `journalctl`.
var buildInfo = "dev (unstamped build)"

func main() {
	log.Printf("efferent: starting (build %s)", buildInfo)
	runDispatch()
}
