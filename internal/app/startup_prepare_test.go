// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestRuntimeRunnerResolvesSingleProfileOnceAndDryRunNotAtAll(t *testing.T) {
	previousProfile := authpkg.RuntimeProfile()
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })
	authpkg.SetRuntimeProfile("alpha")

	resolveCalls := 0
	testseam.Swap(t, &runnerResolveProfile, func(_ string, selector string) (*authpkg.Profile, error) {
		resolveCalls++
		if selector != "alpha" {
			t.Fatalf("profile selector = %q, want alpha", selector)
		}
		return &authpkg.Profile{CorpID: "corp-a", UserID: "user-a"}, nil
	})
	runner := &runtimeRunner{fallback: executor.EchoRunner{}}
	invocation := executor.Invocation{CanonicalProduct: "calendar", Tool: "list_events"}
	if _, err := runner.Run(context.Background(), invocation); err != nil {
		t.Fatal(err)
	}
	if resolveCalls != 1 {
		t.Fatalf("single-profile resolution calls = %d, want 1", resolveCalls)
	}

	resolveCalls = 0
	invocation.DryRun = true
	if _, err := runner.Run(context.Background(), invocation); err != nil {
		t.Fatal(err)
	}
	if resolveCalls != 0 {
		t.Fatalf("dry-run resolved profiles %d times, want 0", resolveCalls)
	}
}
