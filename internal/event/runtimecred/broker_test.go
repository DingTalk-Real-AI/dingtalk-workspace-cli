// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecred

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCrossPlatformCoverageBrokerLocalFallbackAndRuntimePrecedence(t *testing.T) {
	var localCalls atomic.Int32
	b := New(Config{LocalResolve: func(context.Context) (string, error) {
		localCalls.Add(1)
		return "local", nil
	}})
	if got, err := b.Resolve(context.Background()); err != nil || got != "local" {
		t.Fatalf("local Resolve = %q, %v", got, err)
	}
	gen, err := b.Update(0, " runtime ")
	if err != nil || gen != 1 {
		t.Fatalf("Update = %d, %v", gen, err)
	}
	if got, err := b.Resolve(context.Background()); err != nil || got != "runtime" {
		t.Fatalf("runtime Resolve = %q, %v", got, err)
	}
	if localCalls.Load() != 1 {
		t.Fatalf("local resolver calls = %d", localCalls.Load())
	}
}

func TestCrossPlatformCoverageBrokerRequireSeedWaitsAndCancels(t *testing.T) {
	b := New(Config{RequireSeed: true})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := b.Resolve(ctx); done <- err }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v", err)
	}

	resolved := make(chan string, 1)
	go func() {
		token, _ := b.Resolve(context.Background())
		resolved <- token
	}()
	select {
	case <-resolved:
		t.Fatal("Resolve returned before seed")
	case <-time.After(20 * time.Millisecond):
	}
	if _, err := b.Update(0, "seed"); err != nil {
		t.Fatal(err)
	}
	if got := <-resolved; got != "seed" {
		t.Fatalf("Resolve = %q", got)
	}
}

func TestCrossPlatformCoverageBrokerUpdateCASIdempotenceAndConcurrentConflict(t *testing.T) {
	b := New(Config{})
	if gen, err := b.Update(0, "same"); err != nil || gen != 1 {
		t.Fatalf("first update = %d, %v", gen, err)
	}
	if gen, err := b.Update(0, "same"); err != nil || gen != 1 {
		t.Fatalf("idempotent stale update = %d, %v", gen, err)
	}
	if _, err := b.Update(0, "different"); err == nil {
		t.Fatal("expected generation conflict")
	} else {
		var conflict *GenerationConflictError
		if !errors.As(err, &conflict) || conflict.Actual != 1 {
			t.Fatalf("conflict = %#v, %v", conflict, err)
		}
	}

	b = New(Config{})
	start := make(chan struct{})
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wg sync.WaitGroup
	for _, token := range []string{"a", "b"} {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			<-start
			_, err := b.Update(0, token)
			if err == nil {
				successes.Add(1)
				return
			}
			var conflict *GenerationConflictError
			if errors.As(err, &conflict) {
				conflicts.Add(1)
			}
		}(token)
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestCrossPlatformCoverageBrokerRefreshRejectedRuntimeNeverFallsBack(t *testing.T) {
	var refreshCalls atomic.Int32
	b := New(Config{LocalRefresh: func(context.Context, string) (string, error) {
		refreshCalls.Add(1)
		return "local-new", nil
	}})
	if _, err := b.Update(0, "runtime-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.RefreshRejected(context.Background(), "runtime-a"); !errors.Is(err, ErrRuntimeTokenRejected) {
		t.Fatalf("same token refresh error = %v", err)
	}
	if refreshCalls.Load() != 0 {
		t.Fatalf("local refresh called %d times", refreshCalls.Load())
	}
	if _, err := b.Update(1, "runtime-b"); err != nil {
		t.Fatal(err)
	}
	if got, err := b.RefreshRejected(context.Background(), "runtime-a"); err != nil || got != "runtime-b" {
		t.Fatalf("rotated refresh = %q, %v", got, err)
	}
	if superseded, err := b.ClassifyRejectedAfterRetry("runtime-b"); superseded || !errors.Is(err, ErrRuntimeTokenRejected) {
		t.Fatalf("current retry rejection = superseded %v, error %v", superseded, err)
	}
	if _, err := b.Update(2, "runtime-c"); err != nil {
		t.Fatal(err)
	}
	if superseded, err := b.ClassifyRejectedAfterRetry("runtime-b"); !superseded || err != nil {
		t.Fatalf("newer generation classification = superseded %v, error %v", superseded, err)
	}
	localOnly := New(Config{})
	if superseded, err := localOnly.ClassifyRejectedAfterRetry("local"); superseded || err != nil {
		t.Fatalf("local-only classification = superseded %v, error %v", superseded, err)
	}
}

func TestCrossPlatformCoverageBrokerDefersSeedUntilActivation(t *testing.T) {
	b := New(Config{RequireSeed: true, RequireActivation: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolved := make(chan string, 1)
	errs := make(chan error, 1)
	go func() {
		token, err := b.Resolve(ctx)
		if err != nil {
			errs <- err
			return
		}
		resolved <- token
	}()

	generation, err := b.Update(0, "runtime-a")
	if err != nil || generation != 1 {
		t.Fatalf("Update() = generation %d, err %v", generation, err)
	}
	select {
	case token := <-resolved:
		t.Fatalf("Resolve() returned before activation: %q", token)
	case err := <-errs:
		t.Fatalf("Resolve() failed before activation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	if _, err := b.Activate(0); err == nil {
		t.Fatal("Activate() with stale generation unexpectedly succeeded")
	}
	if activeGeneration, err := b.Activate(generation); err != nil || activeGeneration != generation {
		t.Fatalf("Activate() = generation %d, err %v", activeGeneration, err)
	}
	select {
	case token := <-resolved:
		if token != "runtime-a" {
			t.Fatalf("Resolve() token = %q", token)
		}
	case err := <-errs:
		t.Fatalf("Resolve() failed after activation: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Resolve() remained blocked after activation")
	}
}

func TestCrossPlatformCoverageBrokerRejectsInvalidTokensWithoutEcho(t *testing.T) {
	b := New(Config{MaxTokenBytes: 4})
	for _, token := range []string{"   ", "secret-token"} {
		_, err := b.Update(0, token)
		if err == nil {
			t.Fatalf("Update(%q) succeeded", token)
		}
		if strings.Contains(err.Error(), token) {
			t.Fatal("validation error contained rejected token")
		}
	}
}
