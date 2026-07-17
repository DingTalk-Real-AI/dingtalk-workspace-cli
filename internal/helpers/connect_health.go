// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// connect_health is the "does the bot actually work" side of connect
// observability. The daemon (connect_daemon.go) answers "is a supervisor
// process alive"; the single-instance lock (connect_lock.go) answers "is there
// exactly one connector". Neither answers the question a developer actually
// asks — "is my connection live and receiving?" — which is the gap raised in
// the 0701 review: a foreground `connect` prints logs and looks busy, but if
// the Stream silently dropped there is no way to tell a working-but-idle
// connector from a dead one.
//
// The connector (runStreamConnector, foreground OR daemon worker) writes a
// heartbeat file recording its pid, when it last connected, and when it last
// received/answered a message. `connect status` reads it and derives a
// healthy/degraded/down verdict, and `--json` exposes the raw signal so an
// external supervisor (launchd / systemd / pm2 / cron watchdog) can decide
// whether to restart without guessing from the process table.
//
// Scope note (honest limitation, documented in the PR): with the Stream SDK's
// WithAutoReconnect(true) and no exposed disconnect callback, we cannot cheaply
// detect a silently-deaf connection beyond "process alive + connected at least
// once". lastPushAgoSec is surfaced as data, NOT as a degraded trigger, because
// an idle bot legitimately receives nothing for hours. The single-instance lock
// already rules out the duplicate-connection failure mode, so "alive +
// connected" is a strong healthy signal in practice.

const (
	connectHeartbeatFile   = "heartbeat.json"
	connectHeartbeatFlush  = 15 * time.Second
	connectHealthStalePush = 5 * time.Minute // informational threshold only
)

// connectHeartbeat is the JSON persisted by a running connector. All times are
// unix seconds; a zero value means "never happened".
type connectHeartbeat struct {
	Pid           int    `json:"pid"`
	Channel       string `json:"channel,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	StartUnix     int64  `json:"startUnix"`
	ConnectedUnix int64  `json:"connectedUnix,omitempty"`
	LastPushUnix  int64  `json:"lastPushUnix,omitempty"`
	LastReplyUnix int64  `json:"lastReplyUnix,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	LastErrorUnix int64  `json:"lastErrorUnix,omitempty"`
	UpdatedUnix   int64  `json:"updatedUnix"`
}

// connectHealth is the in-memory writer owned by a connector. Events update it
// in memory (cheap, called on the message hot path); a background ticker bumps
// UpdatedUnix and flushes on every tick — the periodic write IS the liveness
// proof consumed by the staleness check in deriveConnectHealth, so an idle
// connector must keep writing or it would be misreported as down (pid reuse).
type connectHealth struct {
	dir string
	mu  sync.Mutex
	hb  connectHeartbeat

	flushedUnix int64
}

// newConnectHealth builds a health writer filed under connect/<dirKey>/. Returns
// nil when no identity is available (dirKey empty) so every call site can treat
// health as best-effort — a nil *connectHealth's methods are all no-ops.
func newConnectHealth(clientID, channel string) *connectHealth {
	dirKey := daemonDirKey(clientID, "")
	if dirKey == "" {
		return nil
	}
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return nil
	}
	now := time.Now().Unix()
	return &connectHealth{
		dir: dir,
		hb: connectHeartbeat{
			Pid:         os.Getpid(),
			Channel:     channel,
			ClientID:    clientID,
			StartUnix:   now,
			UpdatedUnix: now,
		},
	}
}

func (h *connectHealth) touch(mutate func(*connectHeartbeat)) {
	if h == nil {
		return
	}
	h.mu.Lock()
	mutate(&h.hb)
	h.hb.UpdatedUnix = time.Now().Unix()
	h.mu.Unlock()
}

// onConnected records a successful Stream connect. Called after cli.Start.
func (h *connectHealth) onConnected() {
	h.touch(func(hb *connectHeartbeat) { hb.ConnectedUnix = time.Now().Unix() })
}

// onPush records an inbound message accepted for forwarding.
func (h *connectHealth) onPush() {
	h.touch(func(hb *connectHeartbeat) { hb.LastPushUnix = time.Now().Unix() })
}

// onReply records a reply successfully produced by the agent.
func (h *connectHealth) onReply() {
	h.touch(func(hb *connectHeartbeat) { hb.LastReplyUnix = time.Now().Unix() })
}

// onError records the most recent forward/delivery error.
func (h *connectHealth) onError(err error) {
	if err == nil {
		return
	}
	h.touch(func(hb *connectHeartbeat) {
		hb.LastError = truncateRunes(err.Error(), 300)
		hb.LastErrorUnix = time.Now().Unix()
	})
}

// start writes an initial heartbeat and launches the flush ticker. It stops and
// removes the heartbeat file when ctx is cancelled, so a graceful shutdown
// leaves no stale "healthy" file behind.
func (h *connectHealth) start(ctx context.Context) {
	if h == nil {
		return
	}
	_ = h.flush()
	go func() {
		t := time.NewTicker(connectHeartbeatFlush)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				h.remove()
				return
			case <-t.C:
				// Each tick is a liveness proof: advance UpdatedUnix even when
				// nothing else changed, otherwise an idle connector's heartbeat
				// goes stale and deriveConnectHealth misreports it as down.
				h.touch(func(*connectHeartbeat) {})
				_ = h.flush()
			}
		}
	}()
}

// flush persists the heartbeat only when it changed since the last write.
func (h *connectHealth) flush() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	if h.hb.UpdatedUnix == h.flushedUnix {
		h.mu.Unlock()
		return nil
	}
	snapshot := h.hb
	h.mu.Unlock()

	data, _ := json.MarshalIndent(snapshot, "", "  ")
	path := connectHeartbeatPath(h.dir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, config.FilePerm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	h.mu.Lock()
	h.flushedUnix = snapshot.UpdatedUnix
	h.mu.Unlock()
	return nil
}

func (h *connectHealth) remove() {
	if h == nil {
		return
	}
	_ = os.Remove(connectHeartbeatPath(h.dir))
}

func connectHeartbeatPath(dir string) string {
	return dir + string(os.PathSeparator) + connectHeartbeatFile
}

// readConnectHeartbeat loads the heartbeat file. Missing file yields (nil, nil)
// so callers distinguish "no connector ran" from an I/O error.
func readConnectHeartbeat(dir string) (*connectHeartbeat, error) {
	data, err := os.ReadFile(connectHeartbeatPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hb connectHeartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil, err
	}
	return &hb, nil
}

// Health states, ordered worst-to-best for reporting.
const (
	healthNotRunning = "not_running"
	healthDown       = "down"
	healthDegraded   = "degraded"
	healthHealthy    = "healthy"
)

// connectHealthReport is the derived, presentation-ready verdict combining the
// connector heartbeat with the (optional) supervising daemon.
type connectHealthReport struct {
	State          string `json:"state"`
	Pid            int    `json:"pid,omitempty"`
	Channel        string `json:"channel,omitempty"`
	ClientID       string `json:"clientId,omitempty"`
	AppName        string `json:"appName,omitempty"`
	UnifiedAppID   string `json:"unifiedAppId,omitempty"`
	UptimeSec      int64  `json:"uptimeSec,omitempty"`
	ConnectedAgo   int64  `json:"connectedAgoSec,omitempty"`
	LastPushAgoSec int64  `json:"lastPushAgoSec,omitempty"`
	LastReplyAgo   int64  `json:"lastReplyAgoSec,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	Supervised     bool   `json:"supervised"`
	Detail         string `json:"detail,omitempty"`
}

// deriveConnectHealth turns the raw heartbeat (and whether a daemon supervisor
// is alive) into a verdict. now is injected so the logic is unit-testable.
func deriveConnectHealth(hb *connectHeartbeat, supervised bool, now time.Time) connectHealthReport {
	nowUnix := now.Unix()
	if hb == nil {
		return connectHealthReport{State: healthNotRunning, Supervised: supervised,
			Detail: "no connector heartbeat found"}
	}
	r := connectHealthReport{
		Pid:        hb.Pid,
		Channel:    hb.Channel,
		ClientID:   hb.ClientID,
		Supervised: supervised,
		LastError:  hb.LastError,
	}
	if hb.StartUnix > 0 {
		r.UptimeSec = nowUnix - hb.StartUnix
	}
	if hb.ConnectedUnix > 0 {
		r.ConnectedAgo = nowUnix - hb.ConnectedUnix
	}
	if hb.LastPushUnix > 0 {
		r.LastPushAgoSec = nowUnix - hb.LastPushUnix
	}
	if hb.LastReplyUnix > 0 {
		r.LastReplyAgo = nowUnix - hb.LastReplyUnix
	}

	// Connector process gone: down. A supervisor (if any) will restart it.
	if hb.Pid <= 0 || !processAlive(hb.Pid) {
		r.State = healthDown
		if supervised {
			r.Detail = "connector process not alive; supervisor should restart it"
		} else {
			r.Detail = "connector process not alive"
		}
		return r
	}
	// Guard against pid reuse: a live pid whose heartbeat is stale (no flush
	// within 2× the flush interval) is not our connector.
	heartbeatStaleThreshold := int64((2 * connectHeartbeatFlush).Seconds())
	if hb.UpdatedUnix > 0 && (nowUnix-hb.UpdatedUnix) > heartbeatStaleThreshold {
		r.State = healthDown
		r.Detail = "heartbeat stale (pid may have been reused by another process)"
		return r
	}
	// Alive but never reached a connected state: still starting or failing to
	// establish the Stream.
	if hb.ConnectedUnix == 0 {
		r.State = healthDegraded
		r.Detail = "process alive but never established a Stream connection"
		return r
	}
	// Alive and connected, but the most recent event was an error with no
	// successful activity after it: degraded.
	lastOK := hb.ConnectedUnix
	if hb.LastReplyUnix > lastOK {
		lastOK = hb.LastReplyUnix
	}
	if hb.LastErrorUnix > lastOK {
		r.State = healthDegraded
		r.Detail = "last activity was an error after the last success"
		return r
	}
	r.State = healthHealthy
	return r
}

// connectBaseDir returns <configDir>/connect — the directory holding one
// subdirectory per connector (keyed by dirKey). Honours the test override.
func connectBaseDir() string {
	base := connectDaemonDirOverride
	if base == "" {
		base = config.DefaultConfigDir()
	}
	return filepath.Join(base, "connect")
}

// listConnectors enumerates every connector on this machine by scanning
// connect/<dirKey>/ and deriving each one's health from its heartbeat plus any
// supervising daemon. This is the multi-connector view behind `connect list` —
// the same signal `status` reports for one robot, over all of them. Directories
// with neither a heartbeat nor a daemon state are skipped (empty leftovers).
// Results are sorted by clientId for stable output. now is injected for tests.
var connectHealthReadDir = os.ReadDir

func listConnectors(now time.Time) ([]connectHealthReport, error) {
	ents, err := connectHealthReadDir(connectBaseDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []connectHealthReport
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(connectBaseDir(), e.Name())
		hb, herr := readConnectHeartbeat(dir)
		if herr != nil {
			continue // unreadable heartbeat: skip rather than fail the whole list
		}
		st, _ := readDaemonState(dir)
		if hb == nil && st == nil {
			continue // empty leftover dir
		}
		supervised := st != nil && st.Pid > 0 && processAlive(st.Pid)
		r := deriveConnectHealth(hb, supervised, now)
		if r.ClientID == "" {
			r.ClientID = e.Name() // fall back to the dir key when no heartbeat identity
		}
		if st != nil {
			r.UnifiedAppID = st.UnifiedAppID
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClientID < out[j].ClientID })
	return out, nil
}
