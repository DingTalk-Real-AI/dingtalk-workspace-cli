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
	"sync"
	"testing"
	"time"
)

// countingProvider hands out a fresh code per call and records how often it was
// asked, so cache behaviour can be asserted.
type countingProvider struct {
	mu    sync.Mutex
	calls int
	code  string
	err   error
}

func (p *countingProvider) AuthCode(context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return "", p.err
	}
	if p.code != "" {
		return p.code, nil
	}
	return "code-" + string(rune('a'+p.calls-1)), nil
}

// callCount reports the number of upstream fetches.
func (p *countingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestAuthCodeFuncAdaptsFunction(t *testing.T) {
	provider := AuthCodeFunc(func(context.Context) (string, error) { return "abc", nil })
	code, err := provider.AuthCode(context.Background())
	if err != nil || code != "abc" {
		t.Fatalf("AuthCode() = %q, %v; want abc, nil", code, err)
	}
}

func TestStaticAuthCodeReturnsCode(t *testing.T) {
	code, err := StaticAuthCode("fixed").AuthCode(context.Background())
	if err != nil || code != "fixed" {
		t.Fatalf("AuthCode() = %q, %v; want fixed, nil", code, err)
	}
}

func TestStaticAuthCodeRejectsEmptyCode(t *testing.T) {
	_, err := StaticAuthCode("").AuthCode(context.Background())
	if !errors.Is(err, ErrNoAuthCode) {
		t.Fatalf("AuthCode() = %v, want ErrNoAuthCode", err)
	}
}

func TestCachedAuthCodeReusesCodeWithinTTL(t *testing.T) {
	provider := &countingProvider{code: "same"}
	cache := NewCachedAuthCode(provider, time.Minute)

	for i := 0; i < 5; i++ {
		code, err := cache.AuthCode(context.Background())
		if err != nil {
			t.Fatalf("AuthCode() #%d = %v", i+1, err)
		}
		if code != "same" {
			t.Fatalf("AuthCode() #%d = %q, want same", i+1, code)
		}
	}
	if got := provider.callCount(); got != 1 {
		t.Fatalf("upstream called %d times, want 1 (the code must be cached)", got)
	}
}

func TestCachedAuthCodeRefetchesAfterTTL(t *testing.T) {
	provider := &countingProvider{}
	cache := NewCachedAuthCode(provider, time.Minute)

	now := time.Now()
	cache.now = func() time.Time { return now }

	first, err := cache.AuthCode(context.Background())
	if err != nil {
		t.Fatalf("first AuthCode() = %v", err)
	}

	// Move past the TTL. The DingTalk code expires server-side, so a stale
	// one must not be reused.
	now = now.Add(time.Minute + time.Second)

	second, err := cache.AuthCode(context.Background())
	if err != nil {
		t.Fatalf("second AuthCode() = %v", err)
	}
	if first == second {
		t.Fatalf("AuthCode() returned the same code %q after the TTL expired", first)
	}
	if got := provider.callCount(); got != 2 {
		t.Fatalf("upstream called %d times, want 2", got)
	}
}

func TestCachedAuthCodeDefaultTTLIsUnderServerWindow(t *testing.T) {
	// Portal vendorAuthCode expiresIn is 120s and the code is one-shot.
	// The unconsumed-cache window must stay under that server lifetime.
	if DefaultAuthCodeTTL >= 120*time.Second {
		t.Fatalf("DefaultAuthCodeTTL = %v, want less than the 120s portal expiresIn", DefaultAuthCodeTTL)
	}
	cache := NewCachedAuthCode(&countingProvider{}, 0)
	if cache.ttl != DefaultAuthCodeTTL {
		t.Fatalf("ttl = %v, want DefaultAuthCodeTTL %v", cache.ttl, DefaultAuthCodeTTL)
	}
}

func TestCachedAuthCodeNegativeTTLFallsBackToDefault(t *testing.T) {
	cache := NewCachedAuthCode(&countingProvider{}, -time.Second)
	if cache.ttl != DefaultAuthCodeTTL {
		t.Fatalf("ttl = %v, want DefaultAuthCodeTTL %v", cache.ttl, DefaultAuthCodeTTL)
	}
}

func TestCachedAuthCodePropagatesUpstreamError(t *testing.T) {
	wantErr := errors.New("token service down")
	cache := NewCachedAuthCode(&countingProvider{err: wantErr}, time.Minute)

	_, err := cache.AuthCode(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("AuthCode() = %v, want it to wrap %v", err, wantErr)
	}
}

func TestCachedAuthCodeDoesNotCacheFailures(t *testing.T) {
	provider := &countingProvider{err: errors.New("transient")}
	cache := NewCachedAuthCode(provider, time.Minute)

	if _, err := cache.AuthCode(context.Background()); err == nil {
		t.Fatal("AuthCode() = nil error, want failure")
	}

	provider.mu.Lock()
	provider.err = nil
	provider.code = "recovered"
	provider.mu.Unlock()

	code, err := cache.AuthCode(context.Background())
	if err != nil {
		t.Fatalf("AuthCode() after recovery = %v", err)
	}
	if code != "recovered" {
		t.Fatalf("AuthCode() = %q, want recovered (a failure must not be cached)", code)
	}
}

func TestCachedAuthCodeRejectsEmptyUpstreamCode(t *testing.T) {
	// A provider that reports success with no code is a bug upstream; the
	// cache must surface it instead of caching an unusable value.
	cache := NewCachedAuthCode(AuthCodeFunc(func(context.Context) (string, error) {
		return "", nil
	}), time.Minute)

	if _, err := cache.AuthCode(context.Background()); !errors.Is(err, ErrNoAuthCode) {
		t.Fatalf("AuthCode() = %v, want ErrNoAuthCode", err)
	}
}

func TestCachedAuthCodeInvalidateForcesRefetch(t *testing.T) {
	provider := &countingProvider{}
	cache := NewCachedAuthCode(provider, time.Hour)

	if _, err := cache.AuthCode(context.Background()); err != nil {
		t.Fatalf("first AuthCode() = %v", err)
	}
	cache.Invalidate()
	if _, err := cache.AuthCode(context.Background()); err != nil {
		t.Fatalf("second AuthCode() = %v", err)
	}
	if got := provider.callCount(); got != 2 {
		t.Fatalf("upstream called %d times, want 2 after Invalidate", got)
	}
}

func TestCachedAuthCodeWithoutProviderReportsMissingProvider(t *testing.T) {
	cache := NewCachedAuthCode(nil, time.Minute)
	if _, err := cache.AuthCode(context.Background()); !errors.Is(err, ErrNoAuthCodeProvider) {
		t.Fatalf("AuthCode() = %v, want ErrNoAuthCodeProvider", err)
	}
}

func TestCachedAuthCodeIsSafeForConcurrentUse(t *testing.T) {
	// The backend may ask for a code from a CGO callback while another
	// operation is in flight, so concurrent access must not race.
	provider := &countingProvider{code: "shared"}
	cache := NewCachedAuthCode(provider, time.Hour)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, err := cache.AuthCode(context.Background()); err != nil || code != "shared" {
				t.Errorf("AuthCode() = %q, %v; want shared, nil", code, err)
			}
		}()
	}
	wg.Wait()

	if got := provider.callCount(); got != 1 {
		t.Fatalf("upstream called %d times, want 1", got)
	}
}
