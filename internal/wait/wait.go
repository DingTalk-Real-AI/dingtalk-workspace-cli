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

// Package wait is the framework terminal-state wait engine behind the
// reviewed contract.WaitSpec capability. It owns polling cadence, status
// extraction, and status→outcome mapping; it knows nothing about Cobra, MCP,
// or any product backend. How one poll executes is supplied by the leaf's
// WaitPoll hook (corecmd), so "poll = an existing read command" stays a leaf
// decision rather than a framework assumption.
package wait

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// DefaultPollInterval is the cadence between polls when LoopSpec.Interval is
// zero. The first poll runs immediately so an already-terminal resource does
// not pay a sleep tax.
const DefaultPollInterval = 2 * time.Second

// MaxPollInterval caps the exponential backoff growth between polls so a long
// wait cannot degenerate into effectively-blind polling.
const MaxPollInterval = 30 * time.Second

// PollDoc is one decoded poll response document (typically the unified-output
// envelope data of the poll command).
type PollDoc map[string]any

// Poller executes one poll. Returning an error fails the wait phase; the
// engine never retries a poller error because read commands failing is a real
// failure, not a "not yet" signal.
type Poller func(ctx context.Context) (PollDoc, error)

// LoopSpec is the runtime-resolved projection of contract.WaitSpec plus the
// caller-provided timeout.
type LoopSpec struct {
	StatusQuery string
	Terminal    map[string]contract.ResultOutcome
	Pending     []string
	Timeout     time.Duration
	Interval    time.Duration
}

// Outcome is the closed result of a wait loop. TimedOut reports deadline
// exhaustion (Outcome is then pending — an accepted-but-not-terminal state is
// not a process failure per the exit-code contract); Status is the last
// observed status value.
type Outcome struct {
	Status   string
	Outcome  contract.ResultOutcome
	Attempts int
	TimedOut bool
}

// ErrUnknownStatus reports a status value that is neither declared terminal
// nor declared pending. Unknown fails closed: mapping it to pending could
// hide a real state change until timeout, mapping it to success is worse.
type ErrUnknownStatus struct {
	Status string
	Query  string
}

func (e *ErrUnknownStatus) Error() string {
	return fmt.Sprintf("wait: status %q (from %q) is neither terminal nor pending", e.Status, e.Query)
}

// Run polls poller until a declared terminal status, deadline exhaustion, or
// a poller error. The first poll is immediate; subsequent polls back off
// exponentially (×1.5) from Interval, capped at MaxPollInterval. Deadline
// exhaustion anywhere — before a poll, during a poll (a context-aware poller
// returns ctx.Err()), or during the wait between polls — always closes as
// timed-out pending with the last observed status, never as a poll failure.
func Run(ctx context.Context, spec LoopSpec, poll Poller) (Outcome, error) {
	if spec.Interval <= 0 {
		spec.Interval = DefaultPollInterval
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	pending := make(map[string]bool, len(spec.Pending))
	for _, value := range spec.Pending {
		pending[value] = true
	}
	timedOut := func(status string, attempts int) Outcome {
		return Outcome{Status: status, Outcome: contract.ResultOutcomePending, Attempts: attempts, TimedOut: true}
	}
	interval := spec.Interval
	attempts := 0
	lastStatus := ""
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return timedOut(lastStatus, attempts), nil
			}
			return Outcome{Attempts: attempts}, err
		}
		doc, err := poll(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				if errors.Is(ctxErr, context.DeadlineExceeded) {
					return timedOut(lastStatus, attempts), nil
				}
				return Outcome{Attempts: attempts}, ctxErr
			}
			return Outcome{Attempts: attempts}, fmt.Errorf("wait: poll failed: %w", err)
		}
		attempts++
		status, ok := ExtractStatus(doc, spec.StatusQuery)
		if !ok {
			return Outcome{Attempts: attempts}, fmt.Errorf(
				"wait: status query %q not found in poll result", spec.StatusQuery)
		}
		lastStatus = status
		if outcome, ok := spec.Terminal[status]; ok {
			return Outcome{Status: status, Outcome: outcome, Attempts: attempts}, nil
		}
		if !pending[status] {
			return Outcome{Status: status, Attempts: attempts}, &ErrUnknownStatus{Status: status, Query: spec.StatusQuery}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return timedOut(status, attempts), nil
			}
			return Outcome{Status: status, Attempts: attempts}, ctx.Err()
		case <-timer.C:
		}
		interval = nextInterval(interval)
	}
}

func nextInterval(current time.Duration) time.Duration {
	next := current * 3 / 2
	if next > MaxPollInterval {
		next = MaxPollInterval
	}
	return next
}

// ExtractStatus resolves a dotted status query against a poll document. Each
// segment walks one map level; array indexes are not supported because wait
// targets a single resource. Numeric segments are stringified, so a document
// decoded with json.Number keys still resolves.
func ExtractStatus(doc PollDoc, query string) (string, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", false
	}
	// PollDoc is a defined type, so its dynamic type does not satisfy a
	// map[string]any assertion — convert once at the boundary; nested values
	// from JSON decoding are plain maps.
	var current any = map[string]any(doc)
	for _, segment := range strings.Split(query, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return "", false
		}
		node, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		value, ok := node[segment]
		if !ok {
			return "", false
		}
		current = value
	}
	switch value := current.(type) {
	case string:
		return value, true
	case fmt.Stringer:
		return value.String(), true
	case bool:
		return strconv.FormatBool(value), true
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	default:
		return "", false
	}
}

// IsUnknownStatus reports whether err is the closed fail-on-unknown error.
func IsUnknownStatus(err error) bool {
	var unknown *ErrUnknownStatus
	return errors.As(err, &unknown)
}

// EventStream is the leaf-owned push subscription consumed by the event
// phase (the WaitEvents hook in corecmd). Recv delivers the next decoded
// event document; it returns an error or io.EOF-style termination when the
// stream ends — the engine treats non-terminal termination as a stream
// failure the caller (auto mode) may fall back from.
type EventStream interface {
	Recv(ctx context.Context) (PollDoc, error)
}

// EventLoopSpec is the event-phase projection of contract.WaitSpec.
type EventLoopSpec struct {
	StatusQuery string
	MatchField  string
	Terminal    map[string]contract.ResultOutcome
	Pending     []string
	Timeout     time.Duration
}

// ErrEventStreamEnded reports a stream that terminated before a terminal
// status. Auto mode uses it to fall back to polling; strict event mode
// surfaces it as a wait failure.
var ErrEventStreamEnded = errors.New("wait: event stream ended before a terminal status")

// RunEvent consumes stream until a correlated event reaches a declared
// terminal status, the deadline exhausts, or the stream ends. Events whose
// MatchField value does not equal resource are ignored (other resources on
// the same channel); a correlated event with an unknown status fails closed
// exactly like a poll would.
func RunEvent(ctx context.Context, spec EventLoopSpec, resource string, stream EventStream) (Outcome, error) {
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	pending := make(map[string]bool, len(spec.Pending))
	for _, value := range spec.Pending {
		pending[value] = true
	}
	attempts := 0
	lastStatus := ""
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return Outcome{Status: lastStatus, Outcome: contract.ResultOutcomePending, Attempts: attempts, TimedOut: true}, nil
			}
			return Outcome{Status: lastStatus, Attempts: attempts}, err
		}
		doc, err := stream.Recv(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				if errors.Is(ctxErr, context.DeadlineExceeded) {
					return Outcome{Status: lastStatus, Outcome: contract.ResultOutcomePending, Attempts: attempts, TimedOut: true}, nil
				}
				return Outcome{Status: lastStatus, Attempts: attempts}, ctxErr
			}
			// Wrap with ErrEventStreamEnded so auto mode can fall back to
			// polling while correlated-status failures (unknown status,
			// missing status query) stay non-recoverable. The outcome keeps
			// the last correlated status: auto's fallback poll carries it
			// so a timeout before its first completed poll still reports
			// the state the events already observed.
			return Outcome{Status: lastStatus, Attempts: attempts}, fmt.Errorf("%w: %v", ErrEventStreamEnded, err)
		}
		attempts++
		correlated, ok := ExtractStatus(doc, spec.MatchField)
		if !ok || correlated != resource {
			continue
		}
		status, ok := ExtractStatus(doc, spec.StatusQuery)
		if !ok {
			return Outcome{Attempts: attempts}, fmt.Errorf(
				"wait: status query %q not found in event document", spec.StatusQuery)
		}
		lastStatus = status
		if outcome, ok := spec.Terminal[status]; ok {
			return Outcome{Status: status, Outcome: outcome, Attempts: attempts}, nil
		}
		if !pending[status] {
			return Outcome{Status: status, Attempts: attempts}, &ErrUnknownStatus{Status: status, Query: spec.StatusQuery}
		}
	}
}
