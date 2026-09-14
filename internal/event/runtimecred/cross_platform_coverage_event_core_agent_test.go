package runtimecred

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCrossPlatformCoverageEventCoreBrokerEdges(t *testing.T) {
	rejected := &RuntimeTokenRejectedError{}
	if rejected.Error() == "" || !errors.Is(rejected, ErrRuntimeTokenRejected) || rejected.Is(errors.New("other")) {
		t.Fatal("runtime rejection error contract failed")
	}
	conflict := &GenerationConflictError{Expected: 2, Actual: 3}
	if conflict.Error() == "" {
		t.Fatal("generation conflict error is empty")
	}

	var nilBroker *Broker
	if nilBroker.Generation() != 0 {
		t.Fatal("nil broker generation is non-zero")
	}
	if _, err := nilBroker.Update(0, "token"); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("nil Update error = %v", err)
	}
	if _, err := nilBroker.Activate(0); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("nil Activate error = %v", err)
	}
	if _, err := nilBroker.Resolve(context.Background()); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("nil Resolve error = %v", err)
	}
	if _, err := nilBroker.RefreshRejected(context.Background(), "token"); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("nil RefreshRejected error = %v", err)
	}
	if superseded, err := nilBroker.ClassifyRejectedAfterRetry("token"); superseded || !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("nil classification = %v, %v", superseded, err)
	}

	b := New(Config{})
	if _, err := b.Activate(0); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("empty Activate error = %v", err)
	}
	if _, err := b.Update(0, "token"); err != nil {
		t.Fatal(err)
	}
	if generation, err := b.Activate(1); err != nil || generation != 1 {
		t.Fatalf("active Activate = %d, %v", generation, err)
	}

	if _, err := New(Config{}).Resolve(context.Background()); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("missing local resolver error = %v", err)
	}
	wantResolveErr := errors.New("resolve failed")
	if _, err := New(Config{LocalResolve: func(context.Context) (string, error) {
		return "", wantResolveErr
	}}).Resolve(context.Background()); !errors.Is(err, wantResolveErr) {
		t.Fatalf("local resolve error = %v", err)
	}
	if _, err := New(Config{LocalResolve: func(context.Context) (string, error) {
		return "   ", nil
	}}).Resolve(context.Background()); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("empty resolved credential error = %v", err)
	}
	var racingResolve *Broker
	racingResolve = New(Config{LocalResolve: func(context.Context) (string, error) {
		_, err := racingResolve.Update(0, "runtime-wins")
		return "local", err
	}})
	if token, err := racingResolve.Resolve(context.Background()); err != nil || token != "runtime-wins" {
		t.Fatalf("runtime precedence after resolve = %q, %v", token, err)
	}

	seed := New(Config{RequireSeed: true})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := seed.Resolve(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("seed cancellation = %v", err)
	}

	if _, err := New(Config{}).RefreshRejected(context.Background(), "local"); !errors.Is(err, ErrLocalRefreshUnavailable) {
		t.Fatalf("missing local refresh error = %v", err)
	}
	wantRefreshErr := errors.New("refresh failed")
	if _, err := New(Config{LocalRefresh: func(context.Context, string) (string, error) {
		return "", wantRefreshErr
	}}).RefreshRejected(context.Background(), "local"); !errors.Is(err, wantRefreshErr) {
		t.Fatalf("local refresh error = %v", err)
	}
	if _, err := New(Config{LocalRefresh: func(context.Context, string) (string, error) {
		return " ", nil
	}}).RefreshRejected(context.Background(), "local"); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("empty refreshed credential error = %v", err)
	}

	var racingRefresh *Broker
	racingRefresh = New(Config{LocalRefresh: func(context.Context, string) (string, error) {
		_, err := racingRefresh.Update(0, "runtime-new")
		return "local-new", err
	}})
	if token, err := racingRefresh.RefreshRejected(context.Background(), "runtime-old"); err != nil || token != "runtime-new" {
		t.Fatalf("runtime precedence after refresh = %q, %v", token, err)
	}
	var sameRefresh *Broker
	sameRefresh = New(Config{LocalRefresh: func(context.Context, string) (string, error) {
		_, err := sameRefresh.Update(0, "same")
		return "local-new", err
	}})
	if _, err := sameRefresh.RefreshRejected(context.Background(), "same"); !errors.Is(err, ErrRuntimeTokenRejected) {
		t.Fatalf("same runtime after refresh error = %v", err)
	}

	pending := New(Config{RequireSeed: true, RequireActivation: true})
	generation, err := pending.Update(0, "pending")
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, waitCancel := context.WithCancel(context.Background())
	waitCancel()
	if _, err := pending.RefreshRejected(waitCtx, "pending"); !errors.Is(err, context.Canceled) {
		t.Fatalf("pending refresh cancellation = %v", err)
	}
	refreshDone := make(chan error, 1)
	go func() {
		_, err := pending.RefreshRejected(context.Background(), "pending")
		refreshDone <- err
	}()
	// Let RefreshRejected take the pending credential's changed-channel path
	// before activation publishes it.
	time.Sleep(10 * time.Millisecond)
	if _, err := pending.Activate(generation); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-refreshDone:
		if !errors.Is(err, ErrRuntimeTokenRejected) {
			t.Fatalf("activated pending refresh error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending refresh did not resume")
	}

	inactive := New(Config{RequireActivation: true})
	if _, err := inactive.Update(0, "inactive"); err != nil {
		t.Fatal(err)
	}
	if superseded, err := inactive.ClassifyRejectedAfterRetry("other"); superseded || err != nil {
		t.Fatalf("inactive classification = %v, %v", superseded, err)
	}
}
