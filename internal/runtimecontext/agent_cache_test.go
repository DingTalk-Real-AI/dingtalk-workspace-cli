// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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
			{PID: 4242, PPID: 1001, StartTime: 123456789, Comm: "host"},
		}, true
	})
	t.Setenv(disableAgentCacheEnv, "")

	stored := make(chan struct{}, 4)
	testseam.Swap(t, &agentCacheStore, func(key string, snapshot AgentSnapshot) {
		storeCachedAgentSnapshot(key, snapshot)
		stored <- struct{}{}
	})
	waitStored := func() {
		t.Helper()
		select {
		case <-stored:
		case <-time.After(time.Second):
			t.Fatal("async cache store did not complete")
		}
	}

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

	first := newCachingProvider().resolve()
	if first.State != StateReady || first.AgentID != 7 || calls.Load() != 1 {
		t.Fatalf("first = %+v calls=%d", first, calls.Load())
	}
	waitStored()

	second := newCachingProvider().resolve()
	if second.State != StateReady || second.AgentID != 7 || second.OS != "mac" || string(second.JSON()) != string(raw) || calls.Load() != 1 {
		t.Fatalf("second = %+v calls=%d", second, calls.Load())
	}

	t.Setenv(disableAgentCacheEnv, "1")
	bypass := newCachingProvider().resolve()
	if bypass.State != StateReady || calls.Load() != 2 {
		t.Fatalf("bypass = %+v calls=%d", bypass, calls.Load())
	}
	t.Setenv(disableAgentCacheEnv, "")

	now = now.Add(agentCacheTTL + time.Second)
	expired := newCachingProvider().resolve()
	if expired.State != StateReady || calls.Load() != 3 {
		t.Fatalf("expired = %+v calls=%d", expired, calls.Load())
	}
	waitStored()
}

func TestCrossPlatformCoverageAgentCacheWriteDoesNotBlockAndLateResultIsNotCached(t *testing.T) {
	t.Run("blocked cache write still delivers native result", func(t *testing.T) {
		cacheDir := t.TempDir()
		testseam.Swap(t, &userCacheDir, func() (string, error) { return cacheDir, nil })
		testseam.Swap(t, &agentCacheAncestry, func() ([]ancestorProcess, bool) {
			return []ancestorProcess{{PID: 5151, PPID: 1, StartTime: 111, Comm: "host"}}, true
		})
		t.Setenv(disableAgentCacheEnv, "")

		started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
		testseam.Swap(t, &agentCacheStore, func(string, AgentSnapshot) {
			close(started)
			defer close(finished)
			<-release
		})
		t.Cleanup(func() {
			close(release)
			<-finished
		})

		p := &agentProvider{
			locate:   successfulLocator,
			timeout:  80 * time.Millisecond,
			useCache: true,
			collect: func(string, string) ([]byte, int32, error) {
				return []byte(`{"agent":7,"match_tier":"high","os":"mac","execution_mode":"local"}`), 0, nil
			},
		}
		got := p.resolve()
		if got.State != StateReady || got.AgentID != 7 {
			t.Fatalf("blocked cache write broke recognition: %+v", got)
		}
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("expected async cache store to start")
		}
	})

	t.Run("late native result after timeout is not persisted", func(t *testing.T) {
		cacheDir := t.TempDir()
		testseam.Swap(t, &userCacheDir, func() (string, error) { return cacheDir, nil })
		testseam.Swap(t, &agentCacheAncestry, func() ([]ancestorProcess, bool) {
			return []ancestorProcess{{PID: 6161, PPID: 1, StartTime: 222, Comm: "host"}}, true
		})
		t.Setenv(disableAgentCacheEnv, "")

		var storeCalls atomic.Int32
		testseam.Swap(t, &agentCacheStore, func(key string, snapshot AgentSnapshot) {
			storeCalls.Add(1)
			storeCachedAgentSnapshot(key, snapshot)
		})

		release, completed := make(chan struct{}), make(chan struct{})
		p := &agentProvider{
			locate:   successfulLocator,
			timeout:  10 * time.Millisecond,
			useCache: true,
			collect: func(string, string) ([]byte, int32, error) {
				defer close(completed)
				<-release
				return []byte(`{"agent":7,"match_tier":"high","os":"mac","execution_mode":"local"}`), 0, nil
			},
		}

		got := p.resolve()
		if got.State != StateTimeout || got.ErrorCategory != "collection_timeout" || got.AgentID != 0 {
			t.Fatalf("expected timeout snapshot, got %+v", got)
		}
		close(release)
		<-completed

		if storeCalls.Load() != 0 {
			t.Fatalf("late native result triggered %d cache stores", storeCalls.Load())
		}
		key, ok := computeAgentCacheKey()
		if !ok {
			t.Fatal("expected cache key")
		}
		if _, hit := loadCachedAgentSnapshot(key); hit {
			t.Fatal("late native result was persisted to cache")
		}
	})
}

func TestCrossPlatformCoverageAgentCacheEdgeCasesAndAncestryWalker(t *testing.T) {
	// Default ancestry closure exercises collectProcessAncestry on the live platform.
	if chain, ok := agentCacheAncestry(); !ok || len(chain) == 0 {
		t.Fatalf("agentCacheAncestry returned ok=%v len=%d", ok, len(chain))
	}

	// walkProcessAncestry edge branches: invalid start, lookup failure, cycle, and root stop.
	if _, ok := walkProcessAncestry(1, 4, 2, func(int) (ancestorProcess, bool) { return ancestorProcess{}, true }); ok {
		t.Fatal("accepted pid below minPID")
	}
	if _, ok := walkProcessAncestry(10, 0, 2, func(int) (ancestorProcess, bool) { return ancestorProcess{}, true }); ok {
		t.Fatal("accepted zero maxDepth")
	}
	if _, ok := walkProcessAncestry(10, 4, 2, func(int) (ancestorProcess, bool) { return ancestorProcess{}, false }); ok {
		t.Fatal("accepted failed lookup")
	}
	cyclic, ok := walkProcessAncestry(10, 6, 2, func(pid int) (ancestorProcess, bool) {
		if pid == 10 {
			return ancestorProcess{PID: 10, PPID: 20}, true
		}
		return ancestorProcess{PID: 20, PPID: 10}, true
	})
	if !ok || len(cyclic) != 2 {
		t.Fatalf("cyclic walk = %+v, %v", cyclic, ok)
	}
	rooted, ok := walkProcessAncestry(10, 6, 2, func(pid int) (ancestorProcess, bool) {
		return ancestorProcess{PID: pid, PPID: 1}, true
	})
	if !ok || len(rooted) != 1 {
		t.Fatalf("rooted walk = %+v, %v", rooted, ok)
	}

	// computeAgentCacheKey with empty ancestry.
	t.Setenv(disableAgentCacheEnv, "")
	testseam.Swap(t, &agentCacheAncestry, func() ([]ancestorProcess, bool) { return nil, false })
	if _, ok := computeAgentCacheKey(); ok {
		t.Fatal("empty ancestry produced key")
	}

	// agentCacheFilePath with empty key or failing userCacheDir.
	if _, ok := agentCacheFilePath(""); ok {
		t.Fatal("empty key produced path")
	}
	testseam.Swap(t, &userCacheDir, func() (string, error) { return "", errors.New("no cache") })
	if _, ok := agentCacheFilePath("abc"); ok {
		t.Fatal("missing cache dir produced path")
	}
	if _, ok := loadCachedAgentSnapshot("abc"); ok {
		t.Fatal("loaded with missing cache dir")
	}
	storeCachedAgentSnapshot("abc", AgentSnapshot{State: StateReady, raw: []byte(`{"agent":1,"match_tier":"high"}`)})

	// Cache directory setup for read/write failure branches.
	cacheDir := t.TempDir()
	now := time.Unix(1_795_700_000, 0)
	testseam.Swap(t, &userCacheDir, func() (string, error) { return cacheDir, nil })
	testseam.Swap(t, &agentCacheNow, func() time.Time { return now })

	key := "testkey"
	path, ok := agentCacheFilePath(key)
	if !ok {
		t.Fatal("expected valid path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	// Failed snapshot is not stored.
	storeCachedAgentSnapshot(key, AgentSnapshot{State: StateError, ErrorCode: 1004})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("error snapshot should not be stored")
	}

	// ReadFile failure branch.
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &agentCacheReadFile, func(string) ([]byte, error) { return nil, errors.New("read failed") })
	if _, ok := loadCachedAgentSnapshot(key); ok {
		t.Fatal("loaded despite read error")
	}
	testseam.Swap(t, &agentCacheReadFile, os.ReadFile)

	// Malformed JSON branch.
	if err := os.WriteFile(path, []byte(`not-json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadCachedAgentSnapshot(key); ok {
		t.Fatal("loaded malformed json")
	}

	// Version/key mismatch branch.
	if err := os.WriteFile(path, []byte(`{"version":999,"payload_version":"old","key":"other"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadCachedAgentSnapshot(key); ok {
		t.Fatal("loaded mismatched metadata")
	}

	// Invalid embedded raw agent JSON branch.
	badRaw := `{"version":1,"payload_version":"` + PayloadVersion + `","key":"testkey","created_at_unix":1795700000,"error_code":0,"raw":{"match_tier":"none"}}`
	if err := os.WriteFile(path, []byte(badRaw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadCachedAgentSnapshot(key); ok {
		t.Fatal("loaded invalid agent payload")
	}

	// Oversized snapshot skips write.
	storeCachedAgentSnapshot(key, AgentSnapshot{
		State: StateReady,
		raw:   bytes.Repeat([]byte("a"), agentCacheMaxFileBytes+1),
	})

	// WriteFile failure branch.
	validSnap := AgentSnapshot{State: StateReady, raw: []byte(`{"agent":7,"match_tier":"high"}`)}
	testseam.Swap(t, &agentCacheWriteFile, func(string, []byte, os.FileMode) error { return errors.New("disk full") })
	storeCachedAgentSnapshot(key, validSnap)
	testseam.Swap(t, &agentCacheWriteFile, os.WriteFile)

	// Rename failure branch.
	testseam.Swap(t, &agentCacheRenameFile, func(string, string) error { return errors.New("rename failed") })
	storeCachedAgentSnapshot(key, validSnap)
	testseam.Swap(t, &agentCacheRenameFile, os.Rename)

	// MkdirAll failure branch (cache root is a regular file).
	fileAsDir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &userCacheDir, func() (string, error) { return fileAsDir, nil })
	storeCachedAgentSnapshot(key, validSnap)
}
