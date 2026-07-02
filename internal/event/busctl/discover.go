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

package busctl

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// SpawnFunc abstracts the fork-bus operation so tests can inject a fake
// instead of execing a real binary. Production callers pass busctl.Spawn.
type SpawnFunc func(SpawnConfig) (pid int, err error)

// DiscoverConfig describes one discover attempt. WorkDir holds bus.lock and
// usually (on Unix) bus.sock — see dwsevent.IPCEndpoint for the short-path
// fallback when WorkDir is too deep; the caller must mkdir it with
// pkg/config.DirPerm beforehand.
type DiscoverConfig struct {
	WorkDir     string
	IPCEndpoint string
	ClientID    string
	// Spawn is the fork-bus callback. Default busctl.Spawn.
	Spawn SpawnFunc
	// SpawnExtraArgs is forwarded to Spawn (for tests).
	SpawnExtraArgs []string
	// DialBackoff: initial sleep between retry dials when another process
	// is spawning. Doubled each attempt up to DialMaxBackoff.
	DialBackoff time.Duration
	// DialMaxBackoff caps backoff.
	DialMaxBackoff time.Duration
	// DialDeadline caps total wall-clock time spent discovering.
	DialDeadline time.Duration
}

// Default knobs. Conservative — dial is local and cheap, so retry is cheap.
const (
	defaultDialBackoff    = 25 * time.Millisecond
	defaultDialMaxBackoff = 250 * time.Millisecond
	defaultDialDeadline   = 5 * time.Second
)

// Discover returns a connected net.Conn to the bus for cfg.ClientID. If the
// bus is not running, Discover forks a new one and waits for it to come up.
//
// Race-free three-step (plan §12 P3):
//
//  1. try dial IPC endpoint → success → done
//  2. failed → call Spawn (fork _bus); Spawn blocks until ready pipe says
//     'R' (or returns ErrSpawnFailed if another process won the race and
//     our bus startup hit ErrBusy on the lock)
//  3. dial again — should succeed; retry with backoff up to DialDeadline
//     in case Spawn succeeded but socket bind has tiny latency
//
// On concurrent Discover by N processes: only one Spawn succeeds (the
// others get ErrBusy via the bus daemon's own lock acquisition). Losers
// fall through to the retry-dial loop in step 3 and connect to the bus
// the winner brought up.
func Discover(cfg DiscoverConfig) (net.Conn, error) {
	if cfg.WorkDir == "" {
		return nil, errors.New("busctl: WorkDir is required")
	}
	if cfg.IPCEndpoint == "" {
		return nil, errors.New("busctl: IPCEndpoint is required")
	}
	if cfg.ClientID == "" {
		return nil, errors.New("busctl: ClientID is required")
	}
	if cfg.Spawn == nil {
		cfg.Spawn = Spawn
	}
	if cfg.DialBackoff == 0 {
		cfg.DialBackoff = defaultDialBackoff
	}
	if cfg.DialMaxBackoff == 0 {
		cfg.DialMaxBackoff = defaultDialMaxBackoff
	}
	if cfg.DialDeadline == 0 {
		cfg.DialDeadline = defaultDialDeadline
	}

	// Step 1: try dial.
	if conn, err := transport.Dial(cfg.IPCEndpoint); err == nil {
		return conn, nil
	}

	// Step 2: ensure WorkDir, then spawn _bus.
	if err := os.MkdirAll(cfg.WorkDir, config.DirPerm); err != nil {
		return nil, fmt.Errorf("busctl: mkdir workdir: %w", err)
	}
	_, spawnErr := cfg.Spawn(SpawnConfig{
		ClientID:  cfg.ClientID,
		ExtraArgs: cfg.SpawnExtraArgs,
	})
	if spawnErr != nil && !errors.Is(spawnErr, ErrSpawnFailed) {
		// Hard error (couldn't even exec the child). Stop here — no bus
		// will come up.
		return nil, fmt.Errorf("busctl: spawn bus: %w", spawnErr)
	}
	// spawnErr == ErrSpawnFailed → child reported it can't start, most
	// commonly because another process already holds the lock. Either way
	// we fall through to retry-dial — if someone else's bus IS up we'll
	// connect to it.

	// Step 3: retry dial until DialDeadline.
	deadline := time.Now().Add(cfg.DialDeadline)
	backoff := cfg.DialBackoff
	var lastDialErr error
	for time.Now().Before(deadline) {
		conn, err := transport.Dial(cfg.IPCEndpoint)
		if err == nil {
			return conn, nil
		}
		lastDialErr = err
		time.Sleep(backoff)
		backoff *= 2
		if backoff > cfg.DialMaxBackoff {
			backoff = cfg.DialMaxBackoff
		}
	}
	return nil, fmt.Errorf("busctl: discover deadline exceeded; last dial error: %w (spawn error: %v)", lastDialErr, spawnErr)
}

// LockPath returns the canonical bus.lock path for the given working dir.
// Centralised so consume / status / stop all agree on the location.
func LockPath(workDir string) string {
	return filepath.Join(workDir, "bus.lock")
}

// MetaPath returns the canonical bus.meta path.
func MetaPath(workDir string) string {
	return filepath.Join(workDir, "bus.meta")
}
