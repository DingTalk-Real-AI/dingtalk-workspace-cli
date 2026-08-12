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

package personal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventlock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
)

type attemptTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newAttemptTestClock() *attemptTestClock {
	return &attemptTestClock{
		now: time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC),
	}
}

func (c *attemptTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *attemptTestClock) Advance(delta time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delta)
}

func attemptTestFingerprint(label string) string {
	return Fingerprint("https://mcp.example.test/dws", "idem-"+label, "profile-"+label)
}

func newAttemptTestStore(t *testing.T, clock *attemptTestClock) *AttemptStore {
	t.Helper()
	var sequence atomic.Int64
	return NewAttemptStore(
		t.TempDir(),
		WithAttemptClock(clock.Now),
		WithAttemptIDGenerator(func() (string, error) {
			return fmt.Sprintf("attempt-%d", sequence.Add(1)), nil
		}),
	)
}

func TestCrossPlatformCoverageAttemptFingerprintIsCanonicalScopedAndOpaque(t *testing.T) {
	base := Fingerprint(" HTTPS://MCP.Example.Test/dws/ ", " idem-1 ")
	if base == "" || len(base) != 64 {
		t.Fatalf("Fingerprint() = %q", base)
	}
	if got := Fingerprint("https://mcp.example.test/dws", "idem-1", ""); got != base {
		t.Fatalf("empty profile changed fingerprint: %q != %q", got, base)
	}
	if got := Fingerprint("https://mcp.example.test/dws", "idem-1", "profile-a"); got == base {
		t.Fatal("profile selector did not scope fingerprint")
	}
	if left, right := Fingerprint("https://mcp.example.test/dws", "idem-1", "profile-a"),
		Fingerprint("https://mcp.example.test/dws", "idem-1", "profile-b"); left == right {
		t.Fatal("different profile selectors produced the same fingerprint")
	}
	if Fingerprint("", "idem") != "" || Fingerprint("https://mcp.example.test", "") != "" {
		t.Fatal("blank required fingerprint input should return empty")
	}
	if strings.Contains(base, "idem") || strings.Contains(base, "mcp.example") {
		t.Fatalf("fingerprint leaked source material: %q", base)
	}
}

func TestCrossPlatformCoverageRetryabilityValuePreservesUnknown(t *testing.T) {
	if value, known := RetryabilityRetryable.Value(); !known || !value {
		t.Fatalf("retryable Value() = %v, %v", value, known)
	}
	if value, known := RetryabilityNonRetryable.Value(); !known || value {
		t.Fatalf("non-retryable Value() = %v, %v", value, known)
	}
	if _, known := RetryabilityUnknown.Value(); known {
		t.Fatal("unknown retryability became known")
	}
	if _, known := Retryability("invalid").Value(); known {
		t.Fatal("invalid retryability became known")
	}
	var nilBlocked *AttemptBlockedError
	if nilBlocked.Error() != "" {
		t.Fatalf("nil blocked error = %q", nilBlocked.Error())
	}
	blocked := &AttemptBlockedError{
		State:         AttemptStateCooldown,
		NextAllowedAt: time.Date(2026, 7, 30, 10, 1, 0, 0, time.UTC),
	}
	if message := blocked.Error(); !strings.Contains(message, "cooldown") ||
		!strings.Contains(message, "2026-07-30T10:01:00Z") {
		t.Fatalf("blocked error = %q", message)
	}
}

func TestCrossPlatformCoverageAttemptStoreClaimFailureBackoffAndRetryAfter(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("one")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}

	claim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claim.AttemptID != "attempt-1" || len(claim.Fingerprints) != 1 {
		t.Fatalf("claim = %#v", claim)
	}
	if _, err := store.Claim([]AttemptSpec{spec}, time.Minute); err == nil {
		t.Fatal("second in-flight claim succeeded")
	} else {
		var blocked *AttemptBlockedError
		if !errors.As(err, &blocked) || blocked.State != AttemptStateInFlight ||
			blocked.Retryability != RetryabilityUnknown || blocked.RetryAfter != time.Minute {
			t.Fatalf("in-flight error = %#v, %v", blocked, err)
		}
	}

	hold, err := store.CompleteFailure(claim, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityRetryable,
		ErrorCode:    "TEMPORARY",
		TraceID:      "trace-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if hold.State != AttemptStateCooldown || hold.FailureCount != 1 ||
		hold.RetryAfter < 30*time.Second || hold.RetryAfter > 36*time.Second {
		t.Fatalf("first hold = %#v", hold)
	}
	if _, err := store.Claim([]AttemptSpec{spec}, time.Minute); err == nil {
		t.Fatal("cooldown claim succeeded")
	} else {
		var blocked *AttemptBlockedError
		if !errors.As(err, &blocked) || blocked.State != AttemptStateCooldown ||
			blocked.Retryability != RetryabilityRetryable ||
			blocked.ErrorCode != "TEMPORARY" || blocked.TraceID != "trace-1" {
			t.Fatalf("cooldown error = %#v, %v", blocked, err)
		}
	}

	clock.Advance(hold.RetryAfter + time.Second)
	second, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	secondHold, err := store.CompleteFailure(second, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityUnknown,
		RetryAfter:   5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondHold.FailureCount != 2 || secondHold.RetryAfter != 5*time.Minute ||
		secondHold.Retryability != RetryabilityUnknown {
		t.Fatalf("second hold = %#v", secondHold)
	}
}

func TestCrossPlatformCoverageAttemptStoreBackoffSequenceAndTwentyFourHourReset(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("backoff")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}
	wantBases := []time.Duration{
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		30 * time.Minute,
		30 * time.Minute,
	}
	for index, base := range wantBases {
		claim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
		if err != nil {
			t.Fatalf("claim %d: %v", index+1, err)
		}
		hold, err := store.CompleteFailure(claim, nil, AttemptFailure{
			Fingerprint:  fingerprint,
			Retryability: RetryabilityRetryable,
		})
		if err != nil {
			t.Fatalf("failure %d: %v", index+1, err)
		}
		if hold.FailureCount != index+1 ||
			hold.RetryAfter < base ||
			hold.RetryAfter > base+base/5 {
			t.Fatalf("hold %d = %#v, base %s", index+1, hold, base)
		}
		if got := deterministicAttemptJitter(fingerprint, index+1, base); got != hold.RetryAfter-base {
			t.Fatalf("jitter %d = %s, hold delta %s", index+1, got, hold.RetryAfter-base)
		}
		clock.Advance(hold.RetryAfter + time.Second)
	}

	clock.Advance(attemptFailureReset + time.Second)
	claim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hold, err := store.CompleteFailure(claim, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityRetryable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hold.FailureCount != 1 || hold.RetryAfter < 30*time.Second || hold.RetryAfter > 36*time.Second {
		t.Fatalf("post-reset hold = %#v", hold)
	}
}

func TestCrossPlatformCoverageAttemptStoreLongRetryAfterSurvivesFailureCountResetWindow(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("long-retry-after")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}

	claim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hold, err := store.CompleteFailure(claim, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityRetryable,
		RetryAfter:   48 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hold.RetryAfter != 48*time.Hour {
		t.Fatalf("hold Retry-After = %s, want 48h", hold.RetryAfter)
	}

	clock.Advance(24*time.Hour + time.Second)
	if _, err := store.Claim([]AttemptSpec{spec}, time.Minute); err == nil {
		t.Fatal("active 48h Retry-After was pruned at the 24h count reset boundary")
	} else {
		var blocked *AttemptBlockedError
		if !errors.As(err, &blocked) || blocked.NextAllowedAt != hold.NextAllowedAt {
			t.Fatalf("long Retry-After block = %#v, %v", blocked, err)
		}
	}

	clock.Advance(24 * time.Hour)
	recovered, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.CompleteFailure(recovered, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityRetryable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.FailureCount != 1 {
		t.Fatalf("failure count after an inactive 48h hold = %d, want 1", next.FailureCount)
	}
}

func TestCrossPlatformCoverageAttemptStoreExpiredInFlightResetsOldFailureHistory(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("expired-inflight-reset")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}

	first, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	firstHold, err := store.CompleteFailure(first, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityRetryable,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(firstHold.RetryAfter + time.Second)
	crashed, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a process dying after Claim. The record remains in_flight with
	// the old failure timestamp until a later process prunes it.
	clock.Advance(attemptFailureReset + time.Second)
	recovered, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.AttemptID == crashed.AttemptID {
		t.Fatal("expired in-flight claim was not replaced")
	}
	hold, err := store.CompleteFailure(recovered, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityRetryable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hold.FailureCount != 1 {
		t.Fatalf("failure count after expired in-flight reset = %d, want 1", hold.FailureCount)
	}
}

func TestCrossPlatformCoverageAttemptStoreTerminalHoldAndSuccessClear(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("terminal")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventInChat}
	claim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hold, err := store.CompleteFailure(claim, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityNonRetryable,
		ErrorCode:    "GROUP_NOT_BELONG_TO_ORG",
	})
	if err != nil {
		t.Fatal(err)
	}
	if hold.State != AttemptStateTerminalHold || hold.RetryAfter != time.Hour {
		t.Fatalf("terminal hold = %#v", hold)
	}
	if _, err := store.Claim([]AttemptSpec{spec}, time.Minute); err == nil {
		t.Fatal("terminal hold did not block")
	} else {
		var blocked *AttemptBlockedError
		if !errors.As(err, &blocked) || blocked.State != AttemptStateTerminalHold {
			t.Fatalf("terminal block = %#v, %v", blocked, err)
		}
		if retryable, known := blocked.Retryability.Value(); !known || retryable {
			t.Fatalf("terminal retryability = %v, %v", retryable, known)
		}
	}
	clock.Advance(time.Hour + time.Second)
	recovery, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSuccess(recovery); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.workDir, AttemptStateFileName)); !os.IsNotExist(err) {
		t.Fatalf("state file after success = %v, want missing", err)
	}
	if delay, state := attemptFailureDelay(fingerprint, 1, AttemptFailure{
		Retryability: RetryabilityNonRetryable,
		RetryAfter:   2 * time.Hour,
	}); delay != time.Hour || state != AttemptStateTerminalHold {
		t.Fatalf("terminal Retry-After = %s, %s", delay, state)
	}
}

func TestCrossPlatformCoverageAttemptStorePartialFailureClearsSuccessAndReleasesUnexecuted(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprintA := attemptTestFingerprint("batch-a")
	fingerprintB := attemptTestFingerprint("batch-b")
	fingerprintC := attemptTestFingerprint("batch-c")
	specA := AttemptSpec{Fingerprint: fingerprintA, EventKey: EventMention}
	specB := AttemptSpec{Fingerprint: fingerprintB, EventKey: EventSingleChat}
	specC := AttemptSpec{Fingerprint: fingerprintC, EventKey: EventInChat}

	seed, err := store.Claim([]AttemptSpec{specA}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	seedHold, err := store.CompleteFailure(seed, nil, AttemptFailure{
		Fingerprint:  fingerprintA,
		Retryability: RetryabilityRetryable,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(seedHold.RetryAfter + time.Second)

	claim, err := store.Claim([]AttemptSpec{specA, specB, specC}, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hold, err := store.CompleteFailure(claim, []string{fingerprintA}, AttemptFailure{
		Fingerprint:  fingerprintB,
		Retryability: RetryabilityUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hold.Fingerprint != fingerprintB || hold.State != AttemptStateCooldown {
		t.Fatalf("batch hold = %#v", hold)
	}
	state, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Records) != 1 {
		t.Fatalf("records = %#v, want only failed item", state.Records)
	}
	if _, ok := state.Records[fingerprintB]; !ok {
		t.Fatalf("failed fingerprint missing: %#v", state.Records)
	}
	if _, ok := state.Records[fingerprintA]; ok {
		t.Fatal("successful fingerprint retained failure state")
	}
	if _, ok := state.Records[fingerprintC]; ok {
		t.Fatal("unexecuted new fingerprint was not released")
	}
}

func TestCrossPlatformCoverageAttemptStoreReleaseRestoresPreviousState(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprintA := attemptTestFingerprint("release-a")
	fingerprintB := attemptTestFingerprint("release-b")
	specA := AttemptSpec{Fingerprint: fingerprintA, EventKey: EventMention}
	specB := AttemptSpec{Fingerprint: fingerprintB, EventKey: EventSingleChat}

	seed, err := store.Claim([]AttemptSpec{specA}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	seedHold, err := store.CompleteFailure(seed, nil, AttemptFailure{
		Fingerprint:  fingerprintA,
		Retryability: RetryabilityUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(seedHold.RetryAfter + time.Second)
	before, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	previousA := before.Records[fingerprintA]

	claim, err := store.Claim([]AttemptSpec{specA, specB}, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Release(claim); err != nil {
		t.Fatal(err)
	}
	after, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Records) != 1 || after.Records[fingerprintA] != previousA {
		t.Fatalf("release state = %#v, want prior A %#v", after.Records, previousA)
	}
}

func TestCrossPlatformCoverageAttemptStoreCASRejectsOldCompletion(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("cas")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}
	oldClaim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute + time.Second)
	newClaim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSuccess(oldClaim); !errors.Is(err, ErrAttemptClaimStale) {
		t.Fatalf("old completion error = %v, want stale", err)
	}
	if err := store.CompleteSuccess(newClaim); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAttemptStoreConcurrentClaimsOnlyOneWins(t *testing.T) {
	workDir := t.TempDir()
	fingerprint := attemptTestFingerprint("concurrent")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}
	const count = 100
	var wg sync.WaitGroup
	claims := make(chan *AttemptClaim, count)
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := NewAttemptStore(workDir).Claim([]AttemptSpec{spec}, time.Minute)
			if err != nil {
				errs <- err
				return
			}
			claims <- claim
		}()
	}
	wg.Wait()
	close(claims)
	close(errs)

	var winners []*AttemptClaim
	for claim := range claims {
		winners = append(winners, claim)
	}
	if len(winners) != 1 {
		t.Fatalf("winning claims = %d, want 1", len(winners))
	}
	for err := range errs {
		var blocked *AttemptBlockedError
		if !errors.As(err, &blocked) || blocked.State != AttemptStateInFlight {
			t.Fatalf("concurrent claim error = %v", err)
		}
	}
	if err := NewAttemptStore(workDir).CompleteSuccess(winners[0]); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAttemptStoreBatchClaimIsAtomicWhenOneItemBlocked(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprintA := attemptTestFingerprint("atomic-a")
	fingerprintB := attemptTestFingerprint("atomic-b")
	specA := AttemptSpec{Fingerprint: fingerprintA, EventKey: EventMention}
	specB := AttemptSpec{Fingerprint: fingerprintB, EventKey: EventSingleChat}
	first, err := store.Claim([]AttemptSpec{specA}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim([]AttemptSpec{specB, specA}, time.Minute); err == nil {
		t.Fatal("batch with blocked item succeeded")
	}
	state, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Records) != 1 {
		t.Fatalf("atomic blocked claim wrote partial state: %#v", state.Records)
	}
	if _, ok := state.Records[fingerprintB]; ok {
		t.Fatal("unblocked batch item was partially reserved")
	}
	if err := store.CompleteSuccess(first); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAttemptStorePersistsNoRawFingerprintInputs(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	const (
		endpoint = "https://sensitive.example.test/dws"
		idem     = "idem-sensitive-user-and-group"
		profile  = "private-profile-name"
	)
	fingerprint := Fingerprint(endpoint, idem, profile)
	claim, err := store.Claim([]AttemptSpec{{
		Fingerprint: fingerprint,
		EventKey:    EventMention,
	}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.workDir, AttemptStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{endpoint, idem, profile} {
		if strings.Contains(string(data), raw) {
			t.Fatalf("state leaked %q: %s", raw, data)
		}
	}
	if err := store.CompleteSuccess(claim); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAttemptStoreFailsClosedOnCorruptStateAndWriteFailure(t *testing.T) {
	t.Run("corrupt state", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		if err := os.WriteFile(
			filepath.Join(store.workDir, AttemptStateFileName),
			[]byte(`{"version":1,"records":`),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("corrupt"),
			EventKey:    EventMention,
		}}, time.Minute); err == nil || !strings.Contains(err.Error(), "decode subscription attempt state") {
			t.Fatalf("corrupt state error = %v", err)
		}
	})

	t.Run("unknown version", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		if err := os.WriteFile(
			filepath.Join(store.workDir, AttemptStateFileName),
			[]byte(`{"version":99,"records":{}}`),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("version"),
			EventKey:    EventMention,
		}}, time.Minute); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("version error = %v", err)
		}
	})

	t.Run("write failure", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		store.writeFile = func(string, []byte, os.FileMode) error {
			return errors.New("disk full")
		}
		if _, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("write"),
			EventKey:    EventMention,
		}}, time.Minute); err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("write error = %v", err)
		}
	})

	t.Run("failure write keeps in-flight reservation", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		fingerprint := attemptTestFingerprint("failure-write")
		spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}
		claim, err := store.Claim([]AttemptSpec{spec}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		originalWrite := store.writeFile
		store.writeFile = func(string, []byte, os.FileMode) error {
			return errors.New("disk full")
		}
		if _, err := store.CompleteFailure(claim, nil, AttemptFailure{
			Fingerprint:  fingerprint,
			Retryability: RetryabilityUnknown,
		}); err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("failure persistence error = %v", err)
		}
		store.writeFile = originalWrite
		if _, err := store.Claim([]AttemptSpec{spec}, time.Minute); err == nil {
			t.Fatal("failed persistence silently released in-flight reservation")
		} else {
			var blocked *AttemptBlockedError
			if !errors.As(err, &blocked) || blocked.State != AttemptStateInFlight {
				t.Fatalf("post-write-failure block = %#v, %v", blocked, err)
			}
		}
	})
}

func TestCrossPlatformCoverageAttemptStoreFailsClosedOnInjectedIOErrors(t *testing.T) {
	testClaim := func(store *AttemptStore, label string) error {
		_, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint(label),
			EventKey:    EventMention,
		}}, time.Minute)
		return err
	}

	t.Run("read", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		store.readFile = func(string) ([]byte, error) {
			return nil, errors.New("read denied")
		}
		if err := testClaim(store, "read-error"); err == nil || !strings.Contains(err.Error(), "read denied") {
			t.Fatalf("read error = %v", err)
		}
	})

	t.Run("mkdir", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		store.mkdirAll = func(string, os.FileMode) error {
			return errors.New("mkdir denied")
		}
		if err := testClaim(store, "mkdir-error"); err == nil || !strings.Contains(err.Error(), "mkdir denied") {
			t.Fatalf("mkdir error = %v", err)
		}
	})

	t.Run("acquire", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		store.tryAcquire = func(string) (*eventlock.File, error) {
			return nil, errors.New("lock denied")
		}
		if err := testClaim(store, "lock-error"); err == nil || !strings.Contains(err.Error(), "lock denied") {
			t.Fatalf("lock error = %v", err)
		}
	})

	t.Run("marshal", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		store.marshal = func(any, string, string) ([]byte, error) {
			return nil, errors.New("marshal denied")
		}
		if err := testClaim(store, "marshal-error"); err == nil || !strings.Contains(err.Error(), "marshal denied") {
			t.Fatalf("marshal error = %v", err)
		}
	})

	t.Run("chmod", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		store.chmod = func(string, os.FileMode) error {
			return errors.New("chmod denied")
		}
		if err := testClaim(store, "chmod-error"); err == nil ||
			!strings.Contains(err.Error(), "chmod denied") {
			t.Fatalf("chmod error = %v", err)
		}
	})

	t.Run("rename and temporary cleanup", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		store.rename = func(string, string) error {
			return errors.New("rename denied")
		}
		if err := testClaim(store, "rename-error"); err == nil || !strings.Contains(err.Error(), "rename denied") {
			t.Fatalf("rename error = %v", err)
		}
		tmp := filepath.Join(store.workDir, AttemptStateFileName) + ".tmp"
		if _, err := os.Stat(tmp); !os.IsNotExist(err) {
			t.Fatalf("temporary file after rename error = %v", err)
		}
	})

	t.Run("remove", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		claim, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("remove-error"),
			EventKey:    EventMention,
		}}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		store.remove = func(string) error {
			return errors.New("remove denied")
		}
		if err := store.CompleteSuccess(claim); err == nil || !strings.Contains(err.Error(), "remove denied") {
			t.Fatalf("remove error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageAttemptStoreTightensStateAndLockPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	workDir := t.TempDir()
	statePath := filepath.Join(workDir, AttemptStateFileName)
	lockPath := filepath.Join(workDir, AttemptStateLockFileName)
	tmpPath := statePath + ".tmp"
	for _, path := range []string{lockPath, tmpPath} {
		if err := os.WriteFile(path, []byte("stale"), 0o666); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o666); err != nil {
			t.Fatal(err)
		}
	}

	store := NewAttemptStore(workDir)
	claim, err := store.Claim([]AttemptSpec{{
		Fingerprint: attemptTestFingerprint("permissions"),
		EventKey:    EventMention,
	}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Release(claim) }()

	for _, path := range []string{statePath, lockPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", filepath.Base(path), got)
		}
	}
}

func TestCrossPlatformCoverageAttemptStoreRejectsInvalidPersistedRecords(t *testing.T) {
	fingerprint := attemptTestFingerprint("invalid-record")
	longValue := strings.Repeat("x", attemptMaxFieldLength+1)
	tests := []struct {
		name string
		body string
	}{
		{"nil records", `{"version":1,"records":null}`},
		{"key mismatch", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","state":"cooldown","next_allowed_at":"2026-07-30T11:00:00Z","retryability":"unknown"}}}`,
			fingerprint,
			attemptTestFingerprint("other"),
		)},
		{"oversized field", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","event_key":"%s","state":"cooldown","next_allowed_at":"2026-07-30T11:00:00Z","retryability":"unknown"}}}`,
			fingerprint,
			fingerprint,
			longValue,
		)},
		{"negative failures", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","state":"cooldown","failure_count":-1,"next_allowed_at":"2026-07-30T11:00:00Z","retryability":"unknown"}}}`,
			fingerprint,
			fingerprint,
		)},
		{"invalid retryability", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","state":"cooldown","next_allowed_at":"2026-07-30T11:00:00Z","retryability":"sometimes"}}}`,
			fingerprint,
			fingerprint,
		)},
		{"inflight incomplete", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","state":"in_flight"}}}`,
			fingerprint,
			fingerprint,
		)},
		{"cooldown nonretryable", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","state":"cooldown","next_allowed_at":"2026-07-30T11:00:00Z","retryability":"non_retryable"}}}`,
			fingerprint,
			fingerprint,
		)},
		{"terminal retryable", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","state":"terminal_hold","next_allowed_at":"2026-07-30T11:00:00Z","retryability":"retryable"}}}`,
			fingerprint,
			fingerprint,
		)},
		{"invalid state", fmt.Sprintf(
			`{"version":1,"records":{"%s":{"fingerprint":"%s","state":"open"}}}`,
			fingerprint,
			fingerprint,
		)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(workDir, AttemptStateFileName),
				[]byte(test.body),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			store := NewAttemptStore(workDir)
			if _, err := store.Claim([]AttemptSpec{{
				Fingerprint: attemptTestFingerprint("new"),
				EventKey:    EventMention,
			}}, time.Minute); err == nil {
				t.Fatalf("invalid persisted record was accepted: %s", test.body)
			}
		})
	}
}

func TestCrossPlatformCoverageAttemptStoreLockTimeoutAndIDFailureAreFailClosed(t *testing.T) {
	t.Run("lock timeout", func(t *testing.T) {
		var tick atomic.Int64
		base := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
		store := NewAttemptStore(
			t.TempDir(),
			WithAttemptClock(func() time.Time {
				return base.Add(time.Duration(tick.Add(1)) * 3 * time.Second)
			}),
			WithAttemptIDGenerator(func() (string, error) { return "attempt", nil }),
		)
		store.tryAcquire = func(string) (*eventlock.File, error) {
			return nil, eventlock.ErrBusy
		}
		store.sleep = func(time.Duration) {}
		if _, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("lock"),
			EventKey:    EventMention,
		}}, time.Minute); err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("lock timeout error = %v", err)
		}
	})

	t.Run("id failure", func(t *testing.T) {
		store := NewAttemptStore(
			t.TempDir(),
			WithAttemptIDGenerator(func() (string, error) {
				return "", errors.New("random source unavailable")
			}),
		)
		if _, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("id"),
			EventKey:    EventMention,
		}}, time.Minute); err == nil || !strings.Contains(err.Error(), "random source unavailable") {
			t.Fatalf("id error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageAttemptStoreValidationAndFailureCAS(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("validation")
	spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}
	if _, err := store.Claim(nil, time.Minute); err == nil {
		t.Fatal("empty claim succeeded")
	}
	if _, err := store.Claim([]AttemptSpec{{Fingerprint: "raw-id"}}, time.Minute); err == nil {
		t.Fatal("raw fingerprint claim succeeded")
	}
	if _, err := store.Claim([]AttemptSpec{spec}, 0); err == nil {
		t.Fatal("zero lease claim succeeded")
	}
	var nilStore *AttemptStore
	if _, err := nilStore.Claim([]AttemptSpec{spec}, time.Minute); err == nil {
		t.Fatal("nil store claim succeeded")
	}
	uninitialized := &AttemptStore{workDir: t.TempDir()}
	if _, err := uninitialized.Claim([]AttemptSpec{spec}, time.Minute); err == nil ||
		!strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("uninitialized store error = %v", err)
	}

	claim, err := store.Claim([]AttemptSpec{spec, spec}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claim.Fingerprints) != 1 {
		t.Fatalf("duplicate specs were not deduplicated: %#v", claim.Fingerprints)
	}
	if _, err := store.CompleteFailure(claim, []string{fingerprint}, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityRetryable,
	}); err == nil {
		t.Fatal("same successful and failed fingerprint was accepted")
	}
	if _, err := store.CompleteFailure(claim, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: Retryability("invalid"),
	}); err == nil {
		t.Fatal("invalid retryability was accepted")
	}
	if _, err := store.CompleteFailure(claim, nil, AttemptFailure{
		Fingerprint:  attemptTestFingerprint("not-in-claim"),
		Retryability: RetryabilityUnknown,
	}); err == nil {
		t.Fatal("out-of-claim failure was accepted")
	}
	if _, err := store.CompleteFailure(claim, []string{attemptTestFingerprint("not-in-claim")}, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityUnknown,
	}); err == nil {
		t.Fatal("out-of-claim success was accepted")
	}
	if err := store.Release(claim); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSuccess(nil); err == nil {
		t.Fatal("nil success claim was accepted")
	}
	if err := store.Release(nil); err == nil {
		t.Fatal("nil release claim was accepted")
	}
}

func TestCrossPlatformCoverageAttemptStoreBoundsPersistedDiagnostics(t *testing.T) {
	clock := newAttemptTestClock()
	store := newAttemptTestStore(t, clock)
	fingerprint := attemptTestFingerprint("bounded")
	claim, err := store.Claim([]AttemptSpec{{
		Fingerprint: fingerprint,
		EventKey:    EventMention,
	}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	longValue := strings.Repeat("x", attemptMaxFieldLength+100)
	if _, err := store.CompleteFailure(claim, nil, AttemptFailure{
		Fingerprint:  fingerprint,
		Retryability: RetryabilityUnknown,
		ErrorCode:    longValue,
		TraceID:      longValue,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	record := state.Records[fingerprint]
	if len(record.ErrorCode) != attemptMaxFieldLength ||
		len(record.TraceID) != attemptMaxFieldLength {
		t.Fatalf("diagnostic lengths = %d, %d", len(record.ErrorCode), len(record.TraceID))
	}
}

func TestCrossPlatformCoverageAttemptStoreChangedCoverageEdges(t *testing.T) {
	t.Run("endpoint fallback", func(t *testing.T) {
		const raw = "https://example.test/%zz"
		if got := normalizeAttemptEndpoint(raw); got != raw {
			t.Fatalf("normalizeAttemptEndpoint(%q) = %q", raw, got)
		}
	})

	t.Run("invalid generated ids", func(t *testing.T) {
		for _, attemptID := range []string{" ", strings.Repeat("x", attemptMaxFieldLength+1)} {
			store := NewAttemptStore(
				t.TempDir(),
				WithAttemptIDGenerator(func() (string, error) { return attemptID, nil }),
			)
			if _, err := store.Claim([]AttemptSpec{{
				Fingerprint: attemptTestFingerprint("invalid-id"),
				EventKey:    EventMention,
			}}, time.Minute); err == nil || !strings.Contains(err.Error(), "attempt id is invalid") {
				t.Fatalf("attempt ID %q error = %v", attemptID, err)
			}
		}
	})

	t.Run("complete success validates store", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		claim, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("success-validate"),
			EventKey:    EventMention,
		}}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		store.now = nil
		if err := store.CompleteSuccess(claim); err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("CompleteSuccess validation error = %v", err)
		}
	})

	t.Run("complete success read failure", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		claim, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("success-read"),
			EventKey:    EventMention,
		}}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		store.readFile = func(string) ([]byte, error) {
			return nil, errors.New("success read denied")
		}
		if err := store.CompleteSuccess(claim); err == nil ||
			!strings.Contains(err.Error(), "success read denied") {
			t.Fatalf("CompleteSuccess read error = %v", err)
		}
	})

	t.Run("complete failure rejects invalid claim", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		if _, err := store.CompleteFailure(nil, nil, AttemptFailure{}); err == nil {
			t.Fatal("CompleteFailure accepted nil claim")
		}
	})

	t.Run("complete failure validates store", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		fingerprint := attemptTestFingerprint("failure-validate")
		claim, err := store.Claim([]AttemptSpec{{
			Fingerprint: fingerprint,
			EventKey:    EventMention,
		}}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		store.now = nil
		if _, err := store.CompleteFailure(claim, nil, AttemptFailure{
			Fingerprint:  fingerprint,
			Retryability: RetryabilityUnknown,
		}); err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("CompleteFailure validation error = %v", err)
		}
	})

	t.Run("complete failure read failure", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		fingerprint := attemptTestFingerprint("failure-read")
		claim, err := store.Claim([]AttemptSpec{{
			Fingerprint: fingerprint,
			EventKey:    EventMention,
		}}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		store.readFile = func(string) ([]byte, error) {
			return nil, errors.New("failure read denied")
		}
		if _, err := store.CompleteFailure(claim, nil, AttemptFailure{
			Fingerprint:  fingerprint,
			Retryability: RetryabilityUnknown,
		}); err == nil || !strings.Contains(err.Error(), "failure read denied") {
			t.Fatalf("CompleteFailure read error = %v", err)
		}
	})

	t.Run("complete failure stale claim", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		fingerprint := attemptTestFingerprint("failure-stale")
		spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}
		stale, err := store.Claim([]AttemptSpec{spec}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Minute + time.Second)
		current, err := store.Claim([]AttemptSpec{spec}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CompleteFailure(stale, nil, AttemptFailure{
			Fingerprint:  fingerprint,
			Retryability: RetryabilityUnknown,
		}); !errors.Is(err, ErrAttemptClaimStale) {
			t.Fatalf("CompleteFailure stale error = %v", err)
		}
		if err := store.CompleteSuccess(current); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("release validates store", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		claim, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("release-validate"),
			EventKey:    EventMention,
		}}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		store.now = nil
		if err := store.Release(claim); err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Release validation error = %v", err)
		}
	})

	t.Run("release read failure", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		claim, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("release-read"),
			EventKey:    EventMention,
		}}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		store.readFile = func(string) ([]byte, error) {
			return nil, errors.New("release read denied")
		}
		if err := store.Release(claim); err == nil ||
			!strings.Contains(err.Error(), "release read denied") {
			t.Fatalf("Release read error = %v", err)
		}
	})

	t.Run("release stale claim", func(t *testing.T) {
		clock := newAttemptTestClock()
		store := newAttemptTestStore(t, clock)
		fingerprint := attemptTestFingerprint("release-stale")
		spec := AttemptSpec{Fingerprint: fingerprint, EventKey: EventMention}
		stale, err := store.Claim([]AttemptSpec{spec}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Minute + time.Second)
		current, err := store.Claim([]AttemptSpec{spec}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Release(stale); !errors.Is(err, ErrAttemptClaimStale) {
			t.Fatalf("Release stale error = %v", err)
		}
		if err := store.CompleteSuccess(current); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("empty work directory", func(t *testing.T) {
		if err := NewAttemptStore(" ").validate(); err == nil ||
			!strings.Contains(err.Error(), "work directory is required") {
			t.Fatalf("empty work directory error = %v", err)
		}
	})

	t.Run("oversized event key", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		if _, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("long-event"),
			EventKey:    strings.Repeat("e", attemptMaxFieldLength+1),
		}}, time.Minute); err == nil || !strings.Contains(err.Error(), "event key is too long") {
			t.Fatalf("oversized event key error = %v", err)
		}
	})

	t.Run("claim shape validation", func(t *testing.T) {
		fingerprint := attemptTestFingerprint("claim-shape")
		if err := validateAttemptClaim(&AttemptClaim{
			AttemptID:    "attempt",
			Fingerprints: []string{"not-a-fingerprint"},
			previous:     map[string]attemptPrevious{"not-a-fingerprint": {}},
		}); err == nil || !strings.Contains(err.Error(), "invalid fingerprint") {
			t.Fatalf("invalid fingerprint claim error = %v", err)
		}
		if err := validateAttemptClaim(&AttemptClaim{
			AttemptID:    "attempt",
			Fingerprints: []string{fingerprint},
			previous:     map[string]attemptPrevious{},
		}); err == nil || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("incomplete claim error = %v", err)
		}
	})

	t.Run("unknown blocked state is ignored", func(t *testing.T) {
		if blocked := blockedAttempt(attemptRecord{State: AttemptState("unknown")}, time.Now()); blocked != nil {
			t.Fatalf("unknown state blocked = %#v", blocked)
		}
	})

	t.Run("old abandoned first claim is pruned", func(t *testing.T) {
		now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
		fingerprint := attemptTestFingerprint("prune-abandoned")
		state := newAttemptStateFile()
		state.Records[fingerprint] = attemptRecord{
			Fingerprint: fingerprint,
			State:       AttemptStateInFlight,
			AttemptID:   "abandoned",
			LeaseUntil:  now.Add(-attemptFailureReset - time.Second),
		}
		pruneAttemptRecords(state, now)
		if len(state.Records) != 0 {
			t.Fatalf("abandoned first claim not pruned: %#v", state.Records)
		}
	})

	t.Run("write validates state and directory", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		if err := store.write(nil); err == nil || !strings.Contains(err.Error(), "nil subscription attempt state") {
			t.Fatalf("nil state write error = %v", err)
		}
		store.mkdirAll = func(string, os.FileMode) error {
			return errors.New("write mkdir denied")
		}
		if err := store.write(newAttemptStateFile()); err == nil ||
			!strings.Contains(err.Error(), "write mkdir denied") {
			t.Fatalf("write mkdir error = %v", err)
		}
	})

	t.Run("state chmod failure cleans temporary file", func(t *testing.T) {
		store := NewAttemptStore(t.TempDir())
		store.chmod = func(path string, mode os.FileMode) error {
			if strings.HasSuffix(path, ".tmp") {
				return errors.New("state chmod denied")
			}
			return os.Chmod(path, mode)
		}
		if _, err := store.Claim([]AttemptSpec{{
			Fingerprint: attemptTestFingerprint("state-chmod"),
			EventKey:    EventMention,
		}}, time.Minute); err == nil || !strings.Contains(err.Error(), "state chmod denied") {
			t.Fatalf("state chmod error = %v", err)
		}
		tmp := filepath.Join(store.workDir, AttemptStateFileName) + ".tmp"
		if _, err := os.Stat(tmp); !os.IsNotExist(err) {
			t.Fatalf("temporary state after chmod error = %v", err)
		}
	})

	t.Run("lock wait clamps final sleep", func(t *testing.T) {
		workDir := t.TempDir()
		base := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
		var nowCalls int
		store := NewAttemptStore(workDir, WithAttemptClock(func() time.Time {
			nowCalls++
			if nowCalls == 1 {
				return base
			}
			return base.Add(attemptLockWaitTimeout - 10*time.Millisecond)
		}))
		var acquireCalls int
		store.tryAcquire = func(path string) (*eventlock.File, error) {
			acquireCalls++
			if acquireCalls == 1 {
				return nil, eventlock.ErrBusy
			}
			return eventlock.TryAcquire(path)
		}
		var slept time.Duration
		store.sleep = func(delay time.Duration) {
			slept = delay
		}
		if err := store.withLock(func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		if slept != 10*time.Millisecond {
			t.Fatalf("final lock sleep = %s, want 10ms", slept)
		}
	})

	t.Run("default attempt id", func(t *testing.T) {
		attemptID, err := newAttemptID()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(attemptID) == "" || len(attemptID) > attemptMaxFieldLength {
			t.Fatalf("newAttemptID() = %q", attemptID)
		}
	})
}
