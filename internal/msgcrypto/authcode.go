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

package msgcrypto

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultAuthCodeTTL is the unconsumed-cache window for a freshly minted
// vendor authCode. The portal issues codes with expiresIn=120s and they are
// one-shot, so the default stays under that server window. Prefer not wrapping
// PortalAuthCode in CachedAuthCode: mint in goProxy and discard after the key
// request.
const DefaultAuthCodeTTL = 90 * time.Second

// ErrNoAuthCode means the provider returned an empty code without an error.
var ErrNoAuthCode = errors.New("msgcrypto: auth code provider returned an empty code")

// AuthCodeProvider yields a DingTalk 免登 authCode for key-server
// authentication. DWS does not mint the code itself, so integrations inject an
// implementation. The backend calls this only from the vendor goProxy
// callback, never on every encrypt or decrypt.
//
// Implementations must be safe for concurrent use; the backend may call this
// from a CGO callback while an encrypt or decrypt call is in flight.
type AuthCodeProvider interface {
	AuthCode(ctx context.Context) (string, error)
}

// CorpAuthCodeProvider mints a code for a specific organization. PortalAuthCode
// implements this so goProxy can pass the C library's corpID. Domain and
// redirectURI are never part of this call.
type CorpAuthCodeProvider interface {
	AuthCodeProvider
	AuthCodeForCorp(ctx context.Context, corpID string) (string, error)
}

// AuthCodeFunc adapts a function to AuthCodeProvider.
type AuthCodeFunc func(ctx context.Context) (string, error)

// AuthCode calls f.
func (f AuthCodeFunc) AuthCode(ctx context.Context) (string, error) { return f(ctx) }

// StaticAuthCode returns a provider that always yields code. It is meant for
// tests and manual integration runs; a static code stops working once the
// server-side five-minute window closes.
func StaticAuthCode(code string) AuthCodeProvider {
	return AuthCodeFunc(func(context.Context) (string, error) {
		if code == "" {
			return "", ErrNoAuthCode
		}
		return code, nil
	})
}

// CachedAuthCode memoises an AuthCodeProvider for a TTL so a burst of key
// requests does not trigger one upstream call each.
type CachedAuthCode struct {
	provider AuthCodeProvider
	ttl      time.Duration
	now      func() time.Time

	mu        sync.Mutex
	code      string
	expiresAt time.Time
}

// NewCachedAuthCode wraps provider with a TTL cache. A ttl of zero or less
// selects DefaultAuthCodeTTL.
func NewCachedAuthCode(provider AuthCodeProvider, ttl time.Duration) *CachedAuthCode {
	if ttl <= 0 {
		ttl = DefaultAuthCodeTTL
	}
	return &CachedAuthCode{provider: provider, ttl: ttl, now: time.Now}
}

// AuthCode returns the cached code when it is still fresh, otherwise fetches a
// new one. A failed fetch leaves no stale value behind.
func (c *CachedAuthCode) AuthCode(ctx context.Context) (string, error) {
	if c.provider == nil {
		return "", ErrNoAuthCodeProvider
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.code != "" && c.now().Before(c.expiresAt) {
		return c.code, nil
	}

	code, err := c.provider.AuthCode(ctx)
	if err != nil {
		c.code, c.expiresAt = "", time.Time{}
		return "", fmt.Errorf("msgcrypto: fetch auth code: %w", err)
	}
	if code == "" {
		c.code, c.expiresAt = "", time.Time{}
		return "", ErrNoAuthCode
	}

	c.code = code
	c.expiresAt = c.now().Add(c.ttl)
	return code, nil
}

// Invalidate drops the cached code so the next AuthCode call refetches. The
// backend calls this after the key server rejects a code.
func (c *CachedAuthCode) Invalidate() {
	c.mu.Lock()
	c.code, c.expiresAt = "", time.Time{}
	c.mu.Unlock()
}
