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
	"strings"
	"sync"
	"testing"
	"time"
)

type corpCountingProvider struct {
	mu    sync.Mutex
	calls []string
	code  string
	err   error
}

func (p *corpCountingProvider) AuthCode(context.Context) (string, error) {
	return p.AuthCodeForCorp(context.Background(), "")
}

func (p *corpCountingProvider) AuthCodeForCorp(_ context.Context, corpID string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, corpID)
	if p.err != nil {
		return "", p.err
	}
	if p.code != "" {
		return p.code, nil
	}
	return "code-for-" + corpID, nil
}

func TestCrossPlatformCoverageMintAuthCodeForProxyUsesCorpProvider(t *testing.T) {
	inner := &corpCountingProvider{}
	code, err := mintAuthCodeForProxy(inner, "sso.anhei.test", "ding_corp", "https://sso.anhei.test/login")
	if err != nil || code != "code-for-ding_corp" {
		t.Fatalf("mint = %q, %v; want code-for-ding_corp, nil", code, err)
	}
	if len(inner.calls) != 1 || inner.calls[0] != "ding_corp" {
		t.Fatalf("corpIDs = %v, want [ding_corp]", inner.calls)
	}
}

func TestCrossPlatformCoverageMintAuthCodeForProxyInvalidatesUnconsumedCache(t *testing.T) {
	inner := &countingProvider{code: "once"}
	cache := NewCachedAuthCode(inner, time.Hour)

	if _, err := cache.AuthCode(context.Background()); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if got := inner.callCount(); got != 1 {
		t.Fatalf("seed fetches = %d, want 1", got)
	}

	code, err := mintAuthCodeForProxy(cache, "sso.anhei.test", "ding_corp", "https://sso.anhei.test/login")
	if err != nil || code != "once" {
		t.Fatalf("mint = %q, %v; want once, nil", code, err)
	}

	// Cache was invalidated after spend; next mint hits upstream again.
	if _, err := mintAuthCodeForProxy(cache, "sso.anhei.test", "ding_corp", "sso.anhei.test"); err != nil {
		t.Fatalf("second mint: %v", err)
	}
	if got := inner.callCount(); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 (seed reused once, then refetch)", got)
	}
}

func TestCrossPlatformCoverageMintAuthCodeForProxyRejectsDomainMismatchWithoutFetching(t *testing.T) {
	inner := &corpCountingProvider{code: "once"}
	_, err := mintAuthCodeForProxy(inner, "sso.anhei.test", "ding_corp", "evil.example.test")
	if !errors.Is(err, ErrRedirectHostMismatch) {
		t.Fatalf("mint = %v, want ErrRedirectHostMismatch", err)
	}
	if got := len(inner.calls); got != 0 {
		t.Fatalf("upstream called %d times on domain mismatch, want 0", got)
	}
}

func TestCrossPlatformCoverageMintAuthCodeForProxyRequiresProvider(t *testing.T) {
	if _, err := mintAuthCodeForProxy(nil, "", "ding_corp", ""); !errors.Is(err, ErrNoAuthCodeProvider) {
		t.Fatalf("mint = %v, want ErrNoAuthCodeProvider", err)
	}
}

func TestCrossPlatformCoverageMintAuthCodeForProxyInvalidatesOnFailure(t *testing.T) {
	inner := &countingProvider{err: errors.New("boom")}
	cache := NewCachedAuthCode(inner, time.Hour)
	if _, err := mintAuthCodeForProxy(cache, "", "ding_corp", ""); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("provider error = %v", err)
	}
	inner.mu.Lock()
	inner.err = nil
	inner.code = ""
	inner.mu.Unlock()
	if code, err := mintAuthCodeForProxy(cache, "", "ding_corp", ""); err != nil || code == "" {
		t.Fatalf("post-error mint = %q, %v", code, err)
	}

	empty := &corpCountingProvider{code: ""}
	if _, err := mintAuthCodeForProxy(AuthCodeFunc(func(context.Context) (string, error) {
		return "", nil
	}), "", "ding_corp", ""); !errors.Is(err, ErrNoAuthCode) {
		t.Fatalf("empty provider err = %v", err)
	}
	if code, err := mintAuthCodeForProxy(empty, "", "ding_corp", ""); err != nil || code != "code-for-ding_corp" {
		t.Fatalf("corp provider sanity = %q, %v", code, err)
	}
}
