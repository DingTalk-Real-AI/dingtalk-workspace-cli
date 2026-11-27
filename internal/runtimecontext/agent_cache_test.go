// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAgentCacheRoundTripAndExpiry(t *testing.T) {
	cacheDir := t.TempDir()
	now := time.Unix(1_795_700_000, 0)
	testseam.Swap(t, &userCacheDir, func() (string, error) { return cacheDir, nil })
	testseam.Swap(t, &agentCacheNow, func() time.Time { return now })
	testseam.Swap(t, &agentCacheAncestry, func() ([]ancestorProcess, bool) {
		return []ancestorProcess{
			{PID: 4242, PPID: 1001, StartTime: 123456789, Comm: "codex"},
		}, true
	})
	t.Setenv(disableAgentCacheEnv, "")

	raw := []byte(`{"agent":7,"match_tier":"high","os":"mac","execution_mode":"local"}`)
	var calls atomic.Int32
	newCachingProvider := func() *agentProvider {
		return &agentProvider{
			locate:   successfulLocator,
			timeout:  time.Second,
			useCache: true,
			collect: func(string, string) ([]byte, int32, error) {
				calls.Add(1)
				return raw, 0, nil
			},
		}
	}

	// First call misses cache, collects natively, and populates cache.
	first := newCachingProvider().resolve()
	if first.State != StateReady || first.AgentID != 7 || calls.Load() != 1 {
		t.Fatalf("first = %+v calls=%d", first, calls.Load())
	}

	// Second call hits cache and avoids native collection completely.
	second := newCachingProvider().resolve()
	if second.State != StateReady || second.AgentID != 7 || second.OS != "mac" || string(second.JSON()) != string(raw) || calls.Load() != 1 {
		t.Fatalf("second = %+v calls=%d", second, calls.Load())
	}

	// Explicit env opt-out bypasses cache.
	t.Setenv(disableAgentCacheEnv, "1")
	bypass := newCachingProvider().resolve()
	if bypass.State != StateReady || calls.Load() != 2 {
		t.Fatalf("bypass = %+v calls=%d", bypass, calls.Load())
	}
	t.Setenv(disableAgentCacheEnv, "")

	// Advancing past TTL invalidates the cached entry.
	now = now.Add(agentCacheTTL + time.Second)
	expired := newCachingProvider().resolve()
	if expired.State != StateReady || calls.Load() != 3 {
		t.Fatalf("expired = %+v calls=%d", expired, calls.Load())
	}
}

func TestCrossPlatformCoverageAgentCacheSkipsFailuresAndCollectsAncestry(t *testing.T) {
	cacheDir := t.TempDir()
	testseam.Swap(t, &userCacheDir, func() (string, error) { return cacheDir, nil })
	testseam.Swap(t, &agentCacheAncestry, func() ([]ancestorProcess, bool) {
		return []ancestorProcess{{PID: 9999, PPID: 1, StartTime: 555, Comm: "zsh"}}, true
	})
	t.Setenv(disableAgentCacheEnv, "")

	var calls atomic.Int32
	for range 2 {
		p := &agentProvider{
			locate:   successfulLocator,
			timeout:  time.Second,
			useCache: true,
			collect: func(string, string) ([]byte, int32, error) {
				calls.Add(1)
				return nil, 1004, nil
			},
		}
		if got := p.resolve(); got.State != StateError {
			t.Fatalf("got = %+v", got)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("failed result was cached: calls=%d", calls.Load())
	}

	if chain, ok := collectProcessAncestry(agentCacheMaxDepth); !ok || len(chain) == 0 {
		t.Fatalf("collectProcessAncestry returned ok=%v len=%d", ok, len(chain))
	}
}
