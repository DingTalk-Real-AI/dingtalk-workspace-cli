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
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	eventlock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	// AttemptStateFileName stores retry suppression state for personal
	// subscription creation. It intentionally does not share the successful
	// subscription run-state file.
	AttemptStateFileName = "personal_subscription_attempts.json"
	// AttemptStateLockFileName serializes cross-process attempt-state changes.
	AttemptStateLockFileName = "personal_subscription_attempts.lock"

	attemptStateVersion = 1

	attemptLockWaitTimeout = 5 * time.Second
	attemptLockRetryDelay  = 25 * time.Millisecond
	attemptFailureReset    = 24 * time.Hour
	attemptTerminalHold    = time.Hour
	attemptMaxFieldLength  = 256
)

var (
	// ErrAttemptClaimStale means another process replaced at least one claimed
	// fingerprint. Completion is rejected as a unit so an old process cannot
	// overwrite newer state.
	ErrAttemptClaimStale = errors.New("personal event: subscription attempt claim is stale")
)

// AttemptState is the persisted lifecycle state of one subscription attempt.
type AttemptState string

const (
	AttemptStateInFlight     AttemptState = "in_flight"
	AttemptStateCooldown     AttemptState = "cooldown"
	AttemptStateTerminalHold AttemptState = "terminal_hold"
)

// Retryability preserves the distinction between an explicit server decision
// and an error for which the server did not provide retry guidance.
type Retryability string

const (
	RetryabilityUnknown      Retryability = "unknown"
	RetryabilityRetryable    Retryability = "retryable"
	RetryabilityNonRetryable Retryability = "non_retryable"
)

// Value converts retryability to a bool while preserving whether it is known.
func (r Retryability) Value() (value bool, known bool) {
	switch r {
	case RetryabilityRetryable:
		return true, true
	case RetryabilityNonRetryable:
		return false, true
	default:
		return false, false
	}
}

// AttemptSpec identifies one normalized subscription request. Fingerprint must
// come from Fingerprint; only the digest and the non-sensitive event key are
// persisted.
type AttemptSpec struct {
	Fingerprint string
	EventKey    string
}

// AttemptClaim is the ownership token returned by Claim. Callers must finish
// it with CompleteSuccess, CompleteFailure, or Release.
type AttemptClaim struct {
	AttemptID    string
	Fingerprints []string

	previous map[string]attemptPrevious
}

type attemptPrevious struct {
	record  attemptRecord
	present bool
}

// AttemptBlockedError reports a local suppression decision. It never includes
// the raw endpoint, idempotency key, profile selector, rule parameter, or
// filter.
type AttemptBlockedError struct {
	Fingerprint   string
	EventKey      string
	State         AttemptState
	Retryability  Retryability
	RetryAfter    time.Duration
	NextAllowedAt time.Time
	FailureCount  int
	ErrorCode     string
	TraceID       string
}

func (e *AttemptBlockedError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"personal event: subscription attempt %s until %s",
		e.State,
		e.NextAllowedAt.UTC().Format(time.RFC3339),
	)
}

// AttemptFailure describes the one failed item in a claimed batch.
type AttemptFailure struct {
	Fingerprint  string
	Retryability Retryability
	RetryAfter   time.Duration
	ErrorCode    string
	TraceID      string
}

// AttemptHold is the persisted suppression decision after CompleteFailure.
type AttemptHold struct {
	Fingerprint   string
	State         AttemptState
	Retryability  Retryability
	RetryAfter    time.Duration
	NextAllowedAt time.Time
	FailureCount  int
}

// AttemptStoreOption customizes an AttemptStore.
type AttemptStoreOption func(*AttemptStore)

// WithAttemptClock installs a clock, primarily for deterministic tests.
func WithAttemptClock(now func() time.Time) AttemptStoreOption {
	return func(store *AttemptStore) {
		if now != nil {
			store.now = now
		}
	}
}

// WithAttemptIDGenerator installs an attempt-ID generator. IDs are persisted
// only as CAS tokens and must not contain request data.
func WithAttemptIDGenerator(generate func() (string, error)) AttemptStoreOption {
	return func(store *AttemptStore) {
		if generate != nil {
			store.newID = generate
		}
	}
}

// AttemptStore coordinates personal subscription creation across CLI
// processes sharing one identity work directory.
type AttemptStore struct {
	workDir string
	now     func() time.Time
	newID   func() (string, error)

	readFile   func(string) ([]byte, error)
	mkdirAll   func(string, os.FileMode) error
	tryAcquire func(string) (*eventlock.File, error)
	remove     func(string) error
	marshal    func(any, string, string) ([]byte, error)
	writeFile  func(string, []byte, os.FileMode) error
	chmod      func(string, os.FileMode) error
	rename     func(string, string) error
	sleep      func(time.Duration)
}

// NewAttemptStore creates a fail-closed persistent attempt guard rooted in the
// identity-specific personal event work directory.
func NewAttemptStore(workDir string, options ...AttemptStoreOption) *AttemptStore {
	store := &AttemptStore{
		workDir:    strings.TrimSpace(workDir),
		now:        time.Now,
		newID:      newAttemptID,
		readFile:   os.ReadFile,
		mkdirAll:   os.MkdirAll,
		tryAcquire: eventlock.TryAcquire,
		remove:     os.Remove,
		marshal:    json.MarshalIndent,
		writeFile:  os.WriteFile,
		chmod:      os.Chmod,
		rename:     os.Rename,
		sleep:      time.Sleep,
	}
	for _, option := range options {
		if option != nil {
			option(store)
		}
	}
	return store
}

// Fingerprint hashes an endpoint, the existing subscription idempotency key,
// and optional profile selectors into a stable non-reversible store key.
// Passing the active selector ensures two profiles resolving to the same
// corp/user can recover independently after a profile change.
func Fingerprint(endpoint, idempotencyKey string, profileSelector ...string) string {
	endpoint = normalizeAttemptEndpoint(endpoint)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if endpoint == "" || idempotencyKey == "" {
		return ""
	}
	h := sha256.New()
	writeFingerprintPart(h, "dws-personal-subscription-attempt-v1")
	writeFingerprintPart(h, endpoint)
	writeFingerprintPart(h, idempotencyKey)
	for _, selector := range profileSelector {
		if selector = strings.TrimSpace(selector); selector != "" {
			writeFingerprintPart(h, selector)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

type fingerprintWriter interface {
	Write([]byte) (int, error)
}

func writeFingerprintPart(w fingerprintWriter, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = w.Write([]byte(value))
}

func normalizeAttemptEndpoint(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

// Claim atomically reserves every fingerprint in specs. If any fingerprint is
// still suppressed, none are reserved and an AttemptBlockedError is returned.
func (s *AttemptStore) Claim(specs []AttemptSpec, lease time.Duration) (*AttemptClaim, error) {
	normalized, err := normalizeAttemptSpecs(specs)
	if err != nil {
		return nil, err
	}
	if lease <= 0 {
		return nil, errors.New("personal event: subscription attempt lease must be positive")
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	attemptID, err := s.newID()
	if err != nil {
		return nil, fmt.Errorf("personal event: generate subscription attempt id: %w", err)
	}
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" || len(attemptID) > attemptMaxFieldLength {
		return nil, errors.New("personal event: generated subscription attempt id is invalid")
	}

	now := s.now().UTC()
	claim := &AttemptClaim{
		AttemptID:    attemptID,
		Fingerprints: make([]string, 0, len(normalized)),
		previous:     make(map[string]attemptPrevious, len(normalized)),
	}
	var blocked *AttemptBlockedError
	err = s.withLock(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		pruneAttemptRecords(state, now)

		for _, spec := range normalized {
			record, ok := state.Records[spec.Fingerprint]
			if !ok {
				continue
			}
			if candidate := blockedAttempt(record, now); candidate != nil {
				blocked = candidate
				return nil
			}
		}
		for _, spec := range normalized {
			previous, present := state.Records[spec.Fingerprint]
			claim.previous[spec.Fingerprint] = attemptPrevious{
				record:  previous,
				present: present,
			}
			claim.Fingerprints = append(claim.Fingerprints, spec.Fingerprint)
			record := previous
			record.Fingerprint = spec.Fingerprint
			record.EventKey = spec.EventKey
			record.State = AttemptStateInFlight
			record.AttemptID = attemptID
			record.LeaseUntil = now.Add(lease)
			record.NextAllowedAt = time.Time{}
			record.Retryability = ""
			record.ErrorCode = ""
			record.TraceID = ""
			state.Records[spec.Fingerprint] = record
		}
		return s.write(state)
	})
	if err != nil {
		return nil, err
	}
	if blocked != nil {
		return nil, blocked
	}
	return claim, nil
}

// CompleteSuccess clears failure history for every item in a successful
// claimed batch.
func (s *AttemptStore) CompleteSuccess(claim *AttemptClaim) error {
	if err := validateAttemptClaim(claim); err != nil {
		return err
	}
	if err := s.validate(); err != nil {
		return err
	}
	return s.withLock(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		if !claimOwnsAll(state, claim) {
			return ErrAttemptClaimStale
		}
		for _, fingerprint := range claim.Fingerprints {
			delete(state.Records, fingerprint)
		}
		return s.write(state)
	})
}

// CompleteFailure records the failed item, clears successfully completed
// items, and restores every unexecuted item to its exact pre-claim state.
func (s *AttemptStore) CompleteFailure(
	claim *AttemptClaim,
	succeeded []string,
	failure AttemptFailure,
) (AttemptHold, error) {
	var zero AttemptHold
	if err := validateAttemptClaim(claim); err != nil {
		return zero, err
	}
	failure.Fingerprint = strings.TrimSpace(failure.Fingerprint)
	if !containsFingerprint(claim.Fingerprints, failure.Fingerprint) {
		return zero, errors.New("personal event: failed fingerprint is not part of the attempt claim")
	}
	if !validRetryability(failure.Retryability) {
		return zero, fmt.Errorf("personal event: invalid retryability %q", failure.Retryability)
	}
	succeededSet, err := normalizeSucceededFingerprints(claim, succeeded, failure.Fingerprint)
	if err != nil {
		return zero, err
	}
	if err := s.validate(); err != nil {
		return zero, err
	}

	now := s.now().UTC()
	var hold AttemptHold
	err = s.withLock(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		if !claimOwnsAll(state, claim) {
			return ErrAttemptClaimStale
		}

		for _, fingerprint := range claim.Fingerprints {
			switch {
			case fingerprint == failure.Fingerprint:
				previous := claim.previous[fingerprint]
				record := previous.record
				count := record.FailureCount
				if record.LastFailureAt.IsZero() || now.Sub(record.LastFailureAt) >= attemptFailureReset {
					count = 0
				}
				count++
				delay, stateName := attemptFailureDelay(fingerprint, count, failure)
				record.Fingerprint = fingerprint
				record.EventKey = state.Records[fingerprint].EventKey
				record.State = stateName
				record.AttemptID = ""
				record.LeaseUntil = time.Time{}
				record.FailureCount = count
				record.LastFailureAt = now
				record.NextAllowedAt = now.Add(delay)
				record.Retryability = failure.Retryability
				record.ErrorCode = boundedAttemptField(failure.ErrorCode)
				record.TraceID = boundedAttemptField(failure.TraceID)
				state.Records[fingerprint] = record
				hold = AttemptHold{
					Fingerprint:   fingerprint,
					State:         stateName,
					Retryability:  failure.Retryability,
					RetryAfter:    delay,
					NextAllowedAt: record.NextAllowedAt,
					FailureCount:  count,
				}
			case succeededSet[fingerprint]:
				delete(state.Records, fingerprint)
			default:
				restoreAttemptPrevious(state, fingerprint, claim.previous[fingerprint])
			}
		}
		return s.write(state)
	})
	if err != nil {
		return zero, err
	}
	return hold, nil
}

// Release restores every claimed item without recording a remote failure. It
// is intended for local failures before a subscription request is sent.
func (s *AttemptStore) Release(claim *AttemptClaim) error {
	if err := validateAttemptClaim(claim); err != nil {
		return err
	}
	if err := s.validate(); err != nil {
		return err
	}
	return s.withLock(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		if !claimOwnsAll(state, claim) {
			return ErrAttemptClaimStale
		}
		for _, fingerprint := range claim.Fingerprints {
			restoreAttemptPrevious(state, fingerprint, claim.previous[fingerprint])
		}
		return s.write(state)
	})
}

func (s *AttemptStore) validate() error {
	if s == nil {
		return errors.New("personal event: nil subscription attempt store")
	}
	if s.workDir == "" {
		return errors.New("personal event: subscription attempt work directory is required")
	}
	if s.now == nil || s.newID == nil || s.readFile == nil || s.mkdirAll == nil ||
		s.tryAcquire == nil || s.remove == nil || s.marshal == nil ||
		s.writeFile == nil || s.chmod == nil || s.rename == nil || s.sleep == nil {
		return errors.New("personal event: subscription attempt store is not initialized")
	}
	return nil
}

func normalizeAttemptSpecs(specs []AttemptSpec) ([]AttemptSpec, error) {
	if len(specs) == 0 {
		return nil, errors.New("personal event: at least one subscription attempt is required")
	}
	out := make([]AttemptSpec, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		spec.Fingerprint = strings.TrimSpace(spec.Fingerprint)
		spec.EventKey = strings.TrimSpace(spec.EventKey)
		if !validAttemptFingerprint(spec.Fingerprint) {
			return nil, errors.New("personal event: subscription attempt fingerprint is invalid")
		}
		if len(spec.EventKey) > attemptMaxFieldLength {
			return nil, errors.New("personal event: subscription attempt event key is too long")
		}
		if _, ok := seen[spec.Fingerprint]; ok {
			continue
		}
		seen[spec.Fingerprint] = struct{}{}
		out = append(out, spec)
	}
	return out, nil
}

func normalizeSucceededFingerprints(
	claim *AttemptClaim,
	succeeded []string,
	failed string,
) (map[string]bool, error) {
	out := make(map[string]bool, len(succeeded))
	for _, fingerprint := range succeeded {
		fingerprint = strings.TrimSpace(fingerprint)
		if fingerprint == failed {
			return nil, errors.New("personal event: failed fingerprint cannot also be successful")
		}
		if !containsFingerprint(claim.Fingerprints, fingerprint) {
			return nil, errors.New("personal event: successful fingerprint is not part of the attempt claim")
		}
		out[fingerprint] = true
	}
	return out, nil
}

func validateAttemptClaim(claim *AttemptClaim) error {
	if claim == nil || strings.TrimSpace(claim.AttemptID) == "" ||
		len(claim.Fingerprints) == 0 || claim.previous == nil {
		return errors.New("personal event: invalid subscription attempt claim")
	}
	for _, fingerprint := range claim.Fingerprints {
		if !validAttemptFingerprint(fingerprint) {
			return errors.New("personal event: subscription attempt claim contains an invalid fingerprint")
		}
		if _, ok := claim.previous[fingerprint]; !ok {
			return errors.New("personal event: subscription attempt claim is incomplete")
		}
	}
	return nil
}

func containsFingerprint(fingerprints []string, target string) bool {
	for _, fingerprint := range fingerprints {
		if fingerprint == target {
			return true
		}
	}
	return false
}

func validAttemptFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validRetryability(value Retryability) bool {
	switch value {
	case RetryabilityUnknown, RetryabilityRetryable, RetryabilityNonRetryable:
		return true
	default:
		return false
	}
}

func attemptFailureDelay(
	fingerprint string,
	count int,
	failure AttemptFailure,
) (time.Duration, AttemptState) {
	if failure.Retryability == RetryabilityNonRetryable {
		return attemptTerminalHold, AttemptStateTerminalHold
	}
	base := attemptBackoff(count)
	delay := base + deterministicAttemptJitter(fingerprint, count, base)
	if failure.RetryAfter > delay {
		delay = failure.RetryAfter
	}
	return delay, AttemptStateCooldown
}

func attemptBackoff(count int) time.Duration {
	switch count {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	case 3:
		return 10 * time.Minute
	default:
		return 30 * time.Minute
	}
}

func deterministicAttemptJitter(fingerprint string, count int, base time.Duration) time.Duration {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", fingerprint, count)))
	// 0..2000 basis points, inclusive (0%..20%).
	basisPoints := int(binary.BigEndian.Uint16(sum[:2])) % 2001
	return time.Duration(int64(base) * int64(basisPoints) / 10000)
}

func boundedAttemptField(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > attemptMaxFieldLength {
		return value[:attemptMaxFieldLength]
	}
	return value
}

func restoreAttemptPrevious(state *attemptStateFile, fingerprint string, previous attemptPrevious) {
	if previous.present {
		state.Records[fingerprint] = previous.record
		return
	}
	delete(state.Records, fingerprint)
}

func claimOwnsAll(state *attemptStateFile, claim *AttemptClaim) bool {
	for _, fingerprint := range claim.Fingerprints {
		record, ok := state.Records[fingerprint]
		if !ok || record.State != AttemptStateInFlight || record.AttemptID != claim.AttemptID {
			return false
		}
	}
	return true
}

func blockedAttempt(record attemptRecord, now time.Time) *AttemptBlockedError {
	var next time.Time
	retryability := record.Retryability
	switch record.State {
	case AttemptStateInFlight:
		if !record.LeaseUntil.After(now) {
			return nil
		}
		next = record.LeaseUntil
		retryability = RetryabilityUnknown
	case AttemptStateCooldown, AttemptStateTerminalHold:
		if !record.NextAllowedAt.After(now) {
			return nil
		}
		next = record.NextAllowedAt
	default:
		return nil
	}
	return &AttemptBlockedError{
		Fingerprint:   record.Fingerprint,
		EventKey:      record.EventKey,
		State:         record.State,
		Retryability:  retryability,
		RetryAfter:    next.Sub(now),
		NextAllowedAt: next,
		FailureCount:  record.FailureCount,
		ErrorCode:     record.ErrorCode,
		TraceID:       record.TraceID,
	}
}

type attemptStateFile struct {
	Version int                      `json:"version"`
	Records map[string]attemptRecord `json:"records"`
}

type attemptRecord struct {
	Fingerprint   string       `json:"fingerprint"`
	EventKey      string       `json:"event_key,omitempty"`
	State         AttemptState `json:"state"`
	AttemptID     string       `json:"attempt_id,omitempty"`
	LeaseUntil    time.Time    `json:"lease_until,omitempty"`
	FailureCount  int          `json:"failure_count,omitempty"`
	LastFailureAt time.Time    `json:"last_failure_at,omitempty"`
	NextAllowedAt time.Time    `json:"next_allowed_at,omitempty"`
	Retryability  Retryability `json:"retryability,omitempty"`
	ErrorCode     string       `json:"error_code,omitempty"`
	TraceID       string       `json:"trace_id,omitempty"`
}

func (s *AttemptStore) load() (*attemptStateFile, error) {
	path := filepath.Join(s.workDir, AttemptStateFileName)
	data, err := s.readFile(path)
	if os.IsNotExist(err) {
		return newAttemptStateFile(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("personal event: read subscription attempt state: %w", err)
	}
	var state attemptStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("personal event: decode subscription attempt state: %w", err)
	}
	if state.Version != attemptStateVersion {
		return nil, fmt.Errorf(
			"personal event: unsupported subscription attempt state version %d",
			state.Version,
		)
	}
	if state.Records == nil {
		return nil, errors.New("personal event: subscription attempt state has no records map")
	}
	for fingerprint, record := range state.Records {
		if err := validateAttemptRecord(fingerprint, record); err != nil {
			return nil, err
		}
	}
	return &state, nil
}

func newAttemptStateFile() *attemptStateFile {
	return &attemptStateFile{
		Version: attemptStateVersion,
		Records: make(map[string]attemptRecord),
	}
}

func validateAttemptRecord(fingerprint string, record attemptRecord) error {
	if !validAttemptFingerprint(fingerprint) || fingerprint != record.Fingerprint {
		return errors.New("personal event: subscription attempt state contains an invalid fingerprint")
	}
	if len(record.EventKey) > attemptMaxFieldLength ||
		len(record.AttemptID) > attemptMaxFieldLength ||
		len(record.ErrorCode) > attemptMaxFieldLength ||
		len(record.TraceID) > attemptMaxFieldLength {
		return errors.New("personal event: subscription attempt state contains an oversized field")
	}
	if record.FailureCount < 0 {
		return errors.New("personal event: subscription attempt state contains a negative failure count")
	}
	if record.Retryability != "" && !validRetryability(record.Retryability) {
		return errors.New("personal event: subscription attempt state contains invalid retryability")
	}
	switch record.State {
	case AttemptStateInFlight:
		if record.AttemptID == "" || record.LeaseUntil.IsZero() {
			return errors.New("personal event: in-flight subscription attempt state is incomplete")
		}
	case AttemptStateCooldown:
		if record.NextAllowedAt.IsZero() ||
			(record.Retryability != RetryabilityUnknown &&
				record.Retryability != RetryabilityRetryable) {
			return errors.New("personal event: cooldown subscription attempt state is invalid")
		}
	case AttemptStateTerminalHold:
		if record.NextAllowedAt.IsZero() ||
			record.Retryability != RetryabilityNonRetryable {
			return errors.New("personal event: terminal subscription attempt state is invalid")
		}
	default:
		return errors.New("personal event: subscription attempt state contains an invalid state")
	}
	return nil
}

func pruneAttemptRecords(state *attemptStateFile, now time.Time) {
	for fingerprint, record := range state.Records {
		switch {
		case !record.LastFailureAt.IsZero() &&
			!now.Before(record.LastFailureAt) &&
			now.Sub(record.LastFailureAt) >= attemptFailureReset &&
			((record.State == AttemptStateInFlight && !record.LeaseUntil.After(now)) ||
				((record.State == AttemptStateCooldown ||
					record.State == AttemptStateTerminalHold) &&
					!record.NextAllowedAt.After(now))):
			delete(state.Records, fingerprint)
		case record.State == AttemptStateInFlight &&
			record.LastFailureAt.IsZero() &&
			!record.LeaseUntil.IsZero() &&
			!now.Before(record.LeaseUntil) &&
			now.Sub(record.LeaseUntil) >= attemptFailureReset:
			delete(state.Records, fingerprint)
		}
	}
}

func (s *AttemptStore) write(state *attemptStateFile) error {
	if state == nil {
		return errors.New("personal event: nil subscription attempt state")
	}
	if err := s.mkdirAll(s.workDir, config.DirPerm); err != nil {
		return fmt.Errorf("personal event: create subscription attempt directory: %w", err)
	}
	path := filepath.Join(s.workDir, AttemptStateFileName)
	if len(state.Records) == 0 {
		if err := s.remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("personal event: remove empty subscription attempt state: %w", err)
		}
		return nil
	}

	keys := make([]string, 0, len(state.Records))
	for fingerprint := range state.Records {
		keys = append(keys, fingerprint)
	}
	sort.Strings(keys)
	ordered := make(map[string]attemptRecord, len(keys))
	for _, fingerprint := range keys {
		ordered[fingerprint] = state.Records[fingerprint]
	}
	payload := attemptStateFile{Version: attemptStateVersion, Records: ordered}
	data, err := s.marshal(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("personal event: encode subscription attempt state: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := s.writeFile(tmp, data, config.FilePerm); err != nil {
		return fmt.Errorf("personal event: write subscription attempt state: %w", err)
	}
	if err := s.chmod(tmp, config.FilePerm); err != nil {
		_ = s.remove(tmp)
		return fmt.Errorf("personal event: secure subscription attempt state: %w", err)
	}
	if err := s.rename(tmp, path); err != nil {
		_ = s.remove(tmp)
		return fmt.Errorf("personal event: replace subscription attempt state: %w", err)
	}
	return nil
}

func (s *AttemptStore) withLock(fn func() error) error {
	if err := s.mkdirAll(s.workDir, config.DirPerm); err != nil {
		return fmt.Errorf("personal event: create subscription attempt directory: %w", err)
	}
	lockPath := filepath.Join(s.workDir, AttemptStateLockFileName)
	deadline := s.now().Add(attemptLockWaitTimeout)
	for {
		held, err := s.tryAcquire(lockPath)
		if err == nil {
			if err := s.chmod(lockPath, config.FilePerm); err != nil {
				_ = held.Close()
				return fmt.Errorf("personal event: secure subscription attempt lock: %w", err)
			}
			defer held.Close()
			return fn()
		}
		if !errors.Is(err, eventlock.ErrBusy) {
			return fmt.Errorf("personal event: acquire subscription attempt lock: %w", err)
		}
		remaining := deadline.Sub(s.now())
		if remaining <= 0 {
			return fmt.Errorf(
				"personal event: timed out waiting for subscription attempt lock after %s",
				attemptLockWaitTimeout,
			)
		}
		delay := attemptLockRetryDelay
		if remaining < delay {
			delay = remaining
		}
		s.sleep(delay)
	}
}

func newAttemptID() (string, error) {
	return rand.Text(), nil
}
