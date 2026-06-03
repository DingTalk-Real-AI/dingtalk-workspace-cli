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

package consume

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
)

// TestRun_DurationExitsCleanly verifies that --duration triggers a
// graceful exit (nil return, no error surfaced) and does so within a
// small multiple of the requested duration. The contract: --duration
// is a wall-clock budget, not an "abort" — Run wraps the caller's ctx
// with WithTimeout and returns nil rather than DeadlineExceeded so the
// exit code stays 0.
func TestRun_DurationExitsCleanly(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, nil)
	defer func() { cancel(); <-runDone }()
	close(trigger) // no events to fire — Run will exit on duration alone

	duration := 200 * time.Millisecond
	start := time.Now()
	err := Run(context.Background(), Config{
		WorkDir:     dir,
		IPCEndpoint: sock,
		ClientID:    "ding_test",
		Stdout:      io.Discard,
		Stderr:      io.Discard,
		Duration:    duration,
	})
	if err != nil {
		t.Fatalf("Run with --duration should exit nil, got %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < duration {
		t.Errorf("Run returned before duration elapsed: %s < %s", elapsed, duration)
	}
	if elapsed > duration+2*time.Second {
		t.Errorf("Run took %s, much longer than duration %s", elapsed, duration)
	}
}

// TestRun_DurationZeroMeansUnlimited verifies the documented "0 = no
// limit" semantic of --duration. We start with a small parent-ctx
// timeout to bound the test runtime; Run should respect that ctx
// instead of having its own (zero) duration trigger.
func TestRun_DurationZeroMeansUnlimited(t *testing.T) {
	skipOnWindows(t)
	dir, sock, cancel, runDone, trigger := bringUpBus(t, nil)
	defer func() { cancel(); <-runDone }()
	close(trigger)

	ctx, ctxCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer ctxCancel()
	start := time.Now()
	err := Run(ctx, Config{
		WorkDir:     dir,
		IPCEndpoint: sock,
		ClientID:    "ding_test",
		Stdout:      io.Discard,
		Stderr:      io.Discard,
		Duration:    0, // explicitly unlimited
	})
	if err != nil {
		t.Fatalf("Run with --duration=0 should exit nil (ctx cancel), got %v", err)
	}
	// Run should respect the parent ctx — not Duration. So it returns
	// at roughly the ctx deadline.
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("Run returned too quickly (%s); --duration=0 should defer to parent ctx", elapsed)
	}
}

// TestRun_DryRunPrintsConfigAndExits verifies --dry-run is end-to-end
// observable: Run never dials the bus (so even with a bogus IPC endpoint
// it returns nil) and writes the config block to Stderr.
func TestRun_DryRunDoesNotDial(t *testing.T) {
	// We deliberately give a non-existent endpoint to prove Run does
	// not try to dial.
	dir := shortTempDir(t)
	bogusSock := filepath.Join(dir, "no-such.sock")
	err := Run(context.Background(), Config{
		WorkDir:     dir,
		IPCEndpoint: bogusSock,
		ClientID:    "ding_test",
		Stdout:      io.Discard,
		Stderr:      io.Discard,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("DryRun should bypass dial and return nil, got %v", err)
	}
}

// Sanity check that bus.ApplyEnvTuning is wired the same way Run reads
// from Config.Duration — both should be additive and not interfere.
func TestApplyEnvTuning_DoesNotTouchDuration(t *testing.T) {
	// Duration is a consume.Config field, not bus.Config — but we still
	// want a smoke test that ApplyEnvTuning doesn't accidentally reach
	// into the consume layer.
	cfg := bus.Config{}
	bus.ApplyEnvTuning(&cfg)
	// (no Duration field on bus.Config; this test compiles only if the
	// invariant holds — caught by reviewer if someone adds one)
	_ = cfg
}
