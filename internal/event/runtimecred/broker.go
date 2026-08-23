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

// Package runtimecred provides an in-memory credential broker for event bus
// processes. Runtime credentials are never persisted by this package.
package runtimecred

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// DefaultMaxTokenBytes bounds credentials accepted over the local IPC
// channel. Access tokens are normally only a few KiB; the larger limit leaves
// ample room for future token formats while avoiding accidental large secret
// allocations.
const DefaultMaxTokenBytes = 64 << 10

var (
	ErrEmptyToken              = errors.New("runtime credential: token is empty")
	ErrTokenTooLarge           = errors.New("runtime credential: token exceeds size limit")
	ErrCredentialUnavailable   = errors.New("runtime credential: no credential resolver is available")
	ErrLocalRefreshUnavailable = errors.New("runtime credential: no local refresh callback is available")
	ErrRuntimeTokenRejected    = &RuntimeTokenRejectedError{}
)

// RuntimeTokenRejectedError means the currently installed runtime token was
// rejected and no newer runtime token is available. It deliberately carries
// no token or server response data so it is safe to surface to users and logs.
type RuntimeTokenRejectedError struct{}

func (*RuntimeTokenRejectedError) Error() string {
	return "event runtime token was rejected; retry with a fresh host credential"
}

func (*RuntimeTokenRejectedError) Is(target error) bool {
	_, ok := target.(*RuntimeTokenRejectedError)
	return ok
}

// GenerationConflictError reports a failed compare-and-swap update.
type GenerationConflictError struct {
	Expected uint64
	Actual   uint64
}

func (e *GenerationConflictError) Error() string {
	return fmt.Sprintf("runtime credential: generation conflict (expected %d, actual %d)", e.Expected, e.Actual)
}

// ResolveFunc resolves the existing local OAuth credential when no runtime
// credential has been installed.
type ResolveFunc func(context.Context) (string, error)

// RefreshFunc refreshes a rejected local OAuth credential. It is never called
// after a runtime credential has been installed.
type RefreshFunc func(context.Context, string) (string, error)

type Config struct {
	LocalResolve ResolveFunc
	LocalRefresh RefreshFunc
	RequireSeed  bool
	// RequireActivation keeps the first installed runtime credential pending
	// until Activate is called. Detached buses use it to register the consumer
	// before ticket acquisition can emit or fail.
	RequireActivation bool
	MaxTokenBytes     int
}

// Broker holds at most one runtime credential. All state, including the
// credential generation, is process-local and concurrency-safe.
type Broker struct {
	localResolve      ResolveFunc
	localRefresh      RefreshFunc
	requireSeed       bool
	requireActivation bool
	maxBytes          int

	mu         sync.Mutex
	token      string
	generation uint64
	active     bool
	changed    chan struct{}
}

func New(cfg Config) *Broker {
	maxBytes := cfg.MaxTokenBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxTokenBytes
	}
	return &Broker{
		localResolve:      cfg.LocalResolve,
		localRefresh:      cfg.LocalRefresh,
		requireSeed:       cfg.RequireSeed,
		requireActivation: cfg.RequireActivation,
		maxBytes:          maxBytes,
		active:            !cfg.RequireActivation,
		changed:           make(chan struct{}),
	}
}

// Generation returns the current runtime credential generation. Generation 0
// means that no runtime credential has been installed yet.
func (b *Broker) Generation() uint64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.generation
}

// Update atomically installs token when expectedGeneration matches the
// current generation. Reinstalling the same token is idempotent, including
// when another concurrent writer already installed it.
func (b *Broker) Update(expectedGeneration uint64, token string) (uint64, error) {
	if b == nil {
		return 0, ErrCredentialUnavailable
	}
	normalized, err := b.validate(token)
	if err != nil {
		return b.Generation(), err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.token == normalized {
		return b.generation, nil
	}
	if expectedGeneration != b.generation {
		return b.generation, &GenerationConflictError{Expected: expectedGeneration, Actual: b.generation}
	}
	b.token = normalized
	b.generation++
	if b.active {
		b.signalChangedLocked()
	}
	return b.generation, nil
}

// Activate publishes a pending first runtime credential after the bus has
// registered the initiating consumer. It is an idempotent generation-checked
// no-op for brokers that do not require activation.
func (b *Broker) Activate(expectedGeneration uint64) (uint64, error) {
	if b == nil {
		return 0, ErrCredentialUnavailable
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if expectedGeneration != b.generation {
		return b.generation, &GenerationConflictError{Expected: expectedGeneration, Actual: b.generation}
	}
	if b.token == "" {
		return b.generation, ErrCredentialUnavailable
	}
	if b.active {
		return b.generation, nil
	}
	b.active = true
	b.signalChangedLocked()
	return b.generation, nil
}

// Resolve returns the runtime credential when installed. In RequireSeed mode
// it waits until Update installs one; otherwise it preserves the existing
// local resolver behavior until a runtime credential arrives.
func (b *Broker) Resolve(ctx context.Context) (string, error) {
	if b == nil {
		return "", ErrCredentialUnavailable
	}
	for {
		token, wait, requireSeed, active := b.snapshot()
		if token != "" && active {
			return token, nil
		}
		if requireSeed || (token != "" && !active) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-wait:
				continue
			}
		}
		if b.localResolve == nil {
			return "", ErrCredentialUnavailable
		}
		local, err := b.localResolve(ctx)
		if err != nil {
			return "", err
		}
		if runtime, _, _, runtimeActive := b.snapshot(); runtime != "" && runtimeActive {
			return runtime, nil
		}
		return validateResolved(local)
	}
}

// RefreshRejected returns a newer runtime token if one was installed after
// rejectedToken was used. If the installed runtime token itself was rejected,
// it returns RuntimeTokenRejectedError and never invokes local OAuth refresh.
func (b *Broker) RefreshRejected(ctx context.Context, rejectedToken string) (string, error) {
	if b == nil {
		return "", ErrCredentialUnavailable
	}
	for {
		token, wait, requireSeed, active := b.snapshot()
		if token != "" && active {
			if token != rejectedToken {
				return token, nil
			}
			return "", &RuntimeTokenRejectedError{}
		}
		if requireSeed || (token != "" && !active) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-wait:
				continue
			}
		}
		if b.localRefresh == nil {
			return "", ErrLocalRefreshUnavailable
		}
		refreshed, err := b.localRefresh(ctx, rejectedToken)
		if err != nil {
			return "", err
		}
		if runtime, _, _, runtimeActive := b.snapshot(); runtime != "" && runtimeActive {
			if runtime != rejectedToken {
				return runtime, nil
			}
			return "", &RuntimeTokenRejectedError{}
		}
		return validateResolved(refreshed)
	}
}

// ClassifyRejectedAfterRetry is called when a token returned by
// RefreshRejected was itself rejected. When a still newer runtime credential
// is already installed, superseded is true so the source may reconnect and
// resolve that generation without a second in-attempt retry. When the rejected
// token is still current, the fixed typed rejection is returned. With no
// runtime credential installed it preserves local OAuth behavior by returning
// (false, nil).
func (b *Broker) ClassifyRejectedAfterRetry(rejectedToken string) (superseded bool, err error) {
	if b == nil {
		return false, ErrCredentialUnavailable
	}
	token, _, _, active := b.snapshot()
	if token == "" || !active {
		return false, nil
	}
	if token != strings.TrimSpace(rejectedToken) {
		return true, nil
	}
	return false, &RuntimeTokenRejectedError{}
}

func (b *Broker) snapshot() (string, <-chan struct{}, bool, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.token, b.changed, b.requireSeed, b.active
}

func (b *Broker) signalChangedLocked() {
	close(b.changed)
	b.changed = make(chan struct{})
}

func (b *Broker) validate(token string) (string, error) {
	normalized := strings.TrimSpace(token)
	if normalized == "" {
		return "", ErrEmptyToken
	}
	if len(normalized) > b.maxBytes {
		return "", ErrTokenTooLarge
	}
	return normalized, nil
}

func validateResolved(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrCredentialUnavailable
	}
	return token, nil
}
