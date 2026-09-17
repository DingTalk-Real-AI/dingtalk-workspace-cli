// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
)

func TestCrossPlatformCoverageAitableCapabilityRouteSelection(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tools       map[string][]transport.ToolDescriptor
		missing     map[string]bool
		failures    map[string]error
		wantProduct string
		wantChecked []string
	}{
		{
			name: "legacy split service",
			tools: map[string][]transport.ToolDescriptor{
				"aitable-helper": {{Name: "create_role"}},
				"aitable":        {},
			},
			missing:     map[string]bool{"aitable": true},
			wantProduct: "aitable-helper",
			wantChecked: []string{"aitable", "aitable-helper"},
		},
		{
			name: "unified service",
			tools: map[string][]transport.ToolDescriptor{
				"aitable": {{Name: "create_role"}},
			},
			wantProduct: "aitable",
			wantChecked: []string{"aitable"},
		},
		{
			name: "legacy fallback after unified discovery failure",
			tools: map[string][]transport.ToolDescriptor{
				"aitable-helper": {{Name: "create_role"}},
			},
			failures:    map[string]error{"aitable": errors.New("unified discovery unavailable")},
			wantProduct: "aitable-helper",
			wantChecked: []string{"aitable", "aitable-helper"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var checked []string
			testseam.Swap(t, &runnerListProductTools, func(_ *runtimeRunner, _ context.Context, product, tool, _ string) ([]transport.ToolDescriptor, error) {
				checked = append(checked, product)
				if tool != "create_role" {
					t.Fatalf("discovered tool = %q", tool)
				}
				if tc.missing[product] {
					return nil, endpointNotResolvedError(product, tool, "test fixture")
				}
				if err := tc.failures[product]; err != nil {
					return nil, err
				}
				return tc.tools[product], nil
			})
			got, err := (&runtimeRunner{}).ResolveToolProduct(t.Context(), []string{"aitable", "aitable-helper"}, "create_role")
			if err != nil || got != tc.wantProduct {
				t.Fatalf("ResolveToolProduct() = (%q, %v), want (%q, nil)", got, err, tc.wantProduct)
			}
			if !reflect.DeepEqual(checked, tc.wantChecked) {
				t.Fatalf("checked products = %#v, want %#v", checked, tc.wantChecked)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableCapabilityRouteFailsClosed(t *testing.T) {
	wantErr := errors.New("tools/list unavailable")
	testseam.Swap(t, &runnerListProductTools, func(_ *runtimeRunner, _ context.Context, _, _, _ string) ([]transport.ToolDescriptor, error) {
		return nil, wantErr
	})
	_, err := (&runtimeRunner{}).ResolveToolProduct(t.Context(), []string{"aitable-helper", "aitable"}, "create_role")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryDiscovery {
		t.Fatalf("discovery error = %#v, want discovery category", err)
	}

	testseam.Swap(t, &runnerListProductTools, func(_ *runtimeRunner, _ context.Context, _, _, _ string) ([]transport.ToolDescriptor, error) {
		return nil, nil
	})
	if _, err := (&runtimeRunner{}).ResolveToolProduct(t.Context(), []string{"", "aitable-helper", "aitable"}, "create_role"); err == nil || !strings.Contains(err.Error(), "not exposed") {
		t.Fatalf("absent tool error = %v", err)
	}
}

func TestCrossPlatformCoverageAitableCapabilityRouteValidationAndMultiProfile(t *testing.T) {
	runner := &runtimeRunner{}
	var nilContext context.Context
	if _, err := runner.ResolveToolProduct(nilContext, nil, " "); err == nil {
		t.Fatal("blank tool name succeeded")
	}
	runner.globalFlags = &GlobalFlags{Mock: true}
	if _, err := runner.ResolveToolProduct(t.Context(), nil, "create_role"); err == nil {
		t.Fatal("mock route without candidates succeeded")
	}
	if got, err := runner.ResolveToolProduct(t.Context(), []string{"aitable-helper"}, "create_role"); err != nil || got != "aitable-helper" {
		t.Fatalf("mock route = (%q, %v)", got, err)
	}
	runner.globalFlags = nil

	wantProfilesErr := errors.New("invalid profile list")
	testseam.Swap(t, &runnerResolveMultiProfileSelections, func(string, string) ([]multiProfileSelection, bool, error) {
		return nil, false, wantProfilesErr
	})
	if _, err := runner.ResolveToolProduct(t.Context(), []string{"aitable"}, "create_role"); err == nil || !strings.Contains(err.Error(), wantProfilesErr.Error()) {
		t.Fatalf("profile resolution error = %v", err)
	}
}

func TestCrossPlatformCoverageAitableCapabilityRouteUsesOneMultiProfileIdentity(t *testing.T) {
	previousProfile := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile("one,two")
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })

	testseam.Swap(t, &runnerResolveMultiProfileSelections, func(string, string) ([]multiProfileSelection, bool, error) {
		return []multiProfileSelection{{Selector: "one", Profile: authpkg.Profile{CorpID: "corp", UserID: "user"}}}, true, nil
	})
	var profileDuringDiscovery string
	testseam.Swap(t, &runnerListProductTools, func(_ *runtimeRunner, _ context.Context, _, _, profile string) ([]transport.ToolDescriptor, error) {
		profileDuringDiscovery = profile
		return []transport.ToolDescriptor{{Name: "create_role"}}, nil
	})
	got, err := (&runtimeRunner{}).ResolveToolProduct(t.Context(), []string{"aitable-helper"}, "create_role")
	if err != nil || got != "aitable-helper" {
		t.Fatalf("multi-profile route = (%q, %v)", got, err)
	}
	if profileDuringDiscovery != "corp:user" || authpkg.RuntimeProfile() != "one,two" {
		t.Fatalf("profiles during/after discovery = %q/%q", profileDuringDiscovery, authpkg.RuntimeProfile())
	}
}

func TestCrossPlatformCoverageAitableCapabilityRouteRejectsEmptyMultiProfile(t *testing.T) {
	testseam.Swap(t, &runnerResolveMultiProfileSelections, func(string, string) ([]multiProfileSelection, bool, error) {
		return nil, true, nil
	})
	if _, err := (&runtimeRunner{}).ResolveToolProduct(t.Context(), []string{"aitable"}, "create_role"); err == nil {
		t.Fatal("empty multi-profile selection succeeded")
	}
}

func TestCrossPlatformCoverageListAitableProductTools(t *testing.T) {
	if _, err := (*runtimeRunner)(nil).listProductTools(t.Context(), "aitable", "list_roles", ""); err == nil {
		t.Fatal("nil runtime runner succeeded")
	}
	if _, err := (&runtimeRunner{transport: transport.NewClient(nil)}).listProductTools(t.Context(), "missing-product", "list_roles", ""); err == nil {
		t.Fatal("missing endpoint succeeded")
	}

	SetDynamicServers([]mcptypes.ServerDescriptor{{
		Endpoint: "https://mcp-gw.dingtalk.com/server/aitable-helper-test",
		CLI:      mcptypes.CLIOverlay{ID: "aitable-helper"},
	}})
	t.Cleanup(func() { SetDynamicServers(nil) })
	testseam.Swap(t, &runnerResolveAuthSnapshotForProfile, func(*runtimeRunner, context.Context, string) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "test-token", LoginRegion: authpkg.LoginRegionDefault, LoginRegionKnown: true}, nil
	})

	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Host != "mcp-gw.dingtalk.com" || req.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("tools/list request = %s headers=%v", req.URL.String(), req.Header)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"list_roles","title":"List roles","description":"","inputSchema":{"type":"object"}}]}}`,
			)),
			Request: req,
		}, nil
	})}
	runner := &runtimeRunner{transport: transport.NewClient(httpClient)}
	tools, err := runner.listProductTools(t.Context(), "aitable-helper", "list_roles", "")
	if err != nil || len(tools) != 1 || tools[0].Name != "list_roles" || requests != 1 {
		t.Fatalf("listProductTools() = (%#v, %v), requests=%d", tools, err, requests)
	}
	errorRunner := &runtimeRunner{transport: transport.NewClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("tools/list transport failed")
	})})}
	if _, err := errorRunner.listProductTools(t.Context(), "aitable-helper", "list_roles", ""); err == nil {
		t.Fatal("tools/list transport failure succeeded")
	}

	testseam.Swap(t, &runnerResolveAuthSnapshotForProfile, func(*runtimeRunner, context.Context, string) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{}, errors.New("token unavailable")
	})
	if _, err := runner.listProductTools(t.Context(), "aitable-helper", "list_roles", ""); err == nil {
		t.Fatal("token resolution failure succeeded")
	}
}

func TestCrossPlatformCoverageListAitableProductToolsIsolatesConcurrentProfiles(t *testing.T) {
	previousProfile := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile("process-profile-must-not-change")
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })

	SetDynamicServers([]mcptypes.ServerDescriptor{{
		Endpoint: "https://mcp-gw.dingtalk.com/server/aitable-profile-isolation-test",
		CLI:      mcptypes.CLIOverlay{ID: "aitable-helper"},
	}})
	t.Cleanup(func() { SetDynamicServers(nil) })

	testseam.Swap(t, &runnerResolveAuthSnapshotForProfile, func(_ *runtimeRunner, _ context.Context, profile string) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "token-for-" + profile}, nil
	})

	var seenMu sync.Mutex
	seenTokens := make(map[string]int)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		seenMu.Lock()
		seenTokens[token]++
		seenMu.Unlock()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"list_roles","inputSchema":{"type":"object"}}]}}`,
			)),
			Request: req,
		}, nil
	})}
	runner := &runtimeRunner{transport: transport.NewClient(client)}

	profiles := []string{"corp-a:user-a", "corp-b:user-b"}
	errs := make(chan error, len(profiles))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, profile := range profiles {
		profile := profile
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tools, err := runner.listProductTools(t.Context(), "aitable-helper", "list_roles", profile)
			if err == nil && (len(tools) != 1 || tools[0].Name != "list_roles") {
				err = fmt.Errorf("unexpected tools for %s: %#v", profile, tools)
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	wantTokens := map[string]int{
		"token-for-corp-a:user-a": 1,
		"token-for-corp-b:user-b": 1,
	}
	if !reflect.DeepEqual(seenTokens, wantTokens) {
		t.Fatalf("concurrent discovery tokens = %#v, want %#v", seenTokens, wantTokens)
	}
	if got := authpkg.RuntimeProfile(); got != "process-profile-must-not-change" {
		t.Fatalf("runtime profile after concurrent discovery = %q", got)
	}
}

type compatibleRouteTestRunner struct {
	resolved string
	err      error
}

func (*compatibleRouteTestRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	return executor.Result{}, nil
}

func (r *compatibleRouteTestRunner) ResolveToolProduct(context.Context, []string, string) (string, error) {
	return r.resolved, r.err
}

func TestCrossPlatformCoverageAitableToolCallerRouteDecorators(t *testing.T) {
	fallback := &toolCallerAdapter{runner: executor.EchoRunner{}}
	if _, err := fallback.ResolveToolProduct(t.Context(), nil, "create_role"); err == nil {
		t.Fatal("adapter fallback accepted no candidates")
	}
	if got, err := fallback.ResolveToolProduct(t.Context(), []string{"aitable-helper", "aitable"}, "create_role"); err != nil || got != "aitable-helper" {
		t.Fatalf("adapter fallback route = (%q, %v)", got, err)
	}

	wantErr := errors.New("resolver failed")
	delegating := &toolCallerAdapter{runner: &compatibleRouteTestRunner{resolved: "aitable", err: wantErr}}
	if got, err := delegating.ResolveToolProduct(t.Context(), []string{"aitable-helper", "aitable"}, "create_role"); got != "aitable" || !errors.Is(err, wantErr) {
		t.Fatalf("adapter delegated route = (%q, %v)", got, err)
	}

	plainRecording := recordingToolCaller{inner: nonReadToolCaller{}}
	if _, err := plainRecording.ResolveToolProduct(t.Context(), nil, "create_role"); err == nil {
		t.Fatal("recording fallback accepted no candidates")
	}
	if got, err := plainRecording.ResolveToolProduct(t.Context(), []string{"aitable-helper"}, "create_role"); err != nil || got != "aitable-helper" {
		t.Fatalf("recording fallback route = (%q, %v)", got, err)
	}
	wrapped := recordingToolCaller{inner: delegating}
	if got, err := wrapped.ResolveToolProduct(t.Context(), []string{"aitable-helper", "aitable"}, "create_role"); got != "aitable" || !errors.Is(err, wantErr) {
		t.Fatalf("recording delegated route = (%q, %v)", got, err)
	}
}
