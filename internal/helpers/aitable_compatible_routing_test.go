// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableCompatibleRouteCall struct {
	product string
	tool    string
}

type aitableCompatibleRouteCaller struct {
	tools        map[string]map[string]bool
	resolveErr   error
	dryRun       bool
	resolveCalls [][]string
	calls        []aitableCompatibleRouteCall
}

func (c *aitableCompatibleRouteCaller) ResolveToolProduct(_ context.Context, products []string, tool string) (string, error) {
	c.resolveCalls = append(c.resolveCalls, append([]string(nil), products...))
	if c.resolveErr != nil {
		return "", c.resolveErr
	}
	for _, product := range products {
		if c.tools[product][tool] {
			return product, nil
		}
	}
	return "", errors.New("tool not found")
}

func (c *aitableCompatibleRouteCaller) CallTool(_ context.Context, product, tool string, _ map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableCompatibleRouteCall{product: product, tool: tool})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (c *aitableCompatibleRouteCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, product, tool, args)
}

func (*aitableCompatibleRouteCaller) Format() string { return "json" }
func (c *aitableCompatibleRouteCaller) DryRun() bool { return c.dryRun }
func (*aitableCompatibleRouteCaller) Fields() string { return "" }
func (*aitableCompatibleRouteCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageAitableCompatibleRouteSplitAndUnified(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tools       map[string]map[string]bool
		wantProduct string
		writeTool   string
	}{
		{
			name: "legacy split service",
			tools: map[string]map[string]bool{
				"aitable-helper": {"create_role": true},
				"aitable":        {},
			},
			wantProduct: "aitable-helper",
			writeTool:   "create_role",
		},
		{
			name: "unified service",
			tools: map[string]map[string]bool{
				"aitable-helper": {},
				"aitable":        {"create_role": true},
			},
			wantProduct: "aitable",
			writeTool:   "create_role",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableCompatibleRouteCaller{tools: tc.tools}
			InitDepsForTest(t, caller)
			if _, err := callMCPToolReturnTextOnServer(t.Context(), "aitable-helper", tc.writeTool, map[string]any{"baseId": "b"}); err != nil {
				t.Fatalf("compatible route: %v", err)
			}
			if !reflect.DeepEqual(caller.resolveCalls, [][]string{{"aitable", "aitable-helper"}}) {
				t.Fatalf("capability candidates = %#v", caller.resolveCalls)
			}
			wantCalls := []aitableCompatibleRouteCall{{product: tc.wantProduct, tool: tc.writeTool}}
			if !reflect.DeepEqual(caller.calls, wantCalls) {
				t.Fatalf("tools/call history = %#v, want exactly one call %#v", caller.calls, wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableCompatibleRouteFailsBeforeWrite(t *testing.T) {
	wantErr := errors.New("capability discovery failed")
	caller := &aitableCompatibleRouteCaller{resolveErr: wantErr}
	InitDepsForTest(t, caller)
	if _, err := callMCPToolReturnTextOnServer(t.Context(), "aitable-helper", "create_role", nil); !errors.Is(err, wantErr) {
		t.Fatalf("route error = %v, want %v", err, wantErr)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("capability failure reached write tools/call: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAitableCompatibleDryRunReadUsesUnifiedRoute(t *testing.T) {
	caller := &aitableCompatibleRouteCaller{
		dryRun: true,
		tools: map[string]map[string]bool{
			"aitable-helper": {},
			"aitable":        {"list_roles": true},
		},
	}
	InitDepsForTest(t, caller)
	if _, err := callMCPReadToolReturnTextOnServer(t.Context(), "aitable-helper", "list_roles", nil); err != nil {
		t.Fatalf("dry-run compatible read: %v", err)
	}
	wantCalls := []aitableCompatibleRouteCall{{product: "aitable", tool: "list_roles"}}
	if !reflect.DeepEqual(caller.calls, wantCalls) {
		t.Fatalf("dry-run read history = %#v, want %#v", caller.calls, wantCalls)
	}
}

type aitableLegacyRouteCaller struct{}

func (*aitableLegacyRouteCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return &edition.ToolResult{}, nil
}
func (*aitableLegacyRouteCaller) Format() string { return "json" }
func (*aitableLegacyRouteCaller) DryRun() bool   { return false }
func (*aitableLegacyRouteCaller) Fields() string { return "" }
func (*aitableLegacyRouteCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageAitableCompatibleRouteBoundaries(t *testing.T) {
	caller := &aitableCompatibleRouteCaller{}
	InitDepsForTest(t, caller)
	if got, err := resolveCompatibleToolServer(t.Context(), "aitable", "list_roles"); err != nil || got != "aitable" || len(caller.resolveCalls) != 0 {
		t.Fatalf("ordinary server route = (%q, %v), resolver calls=%#v", got, err, caller.resolveCalls)
	}

	InitDepsForTest(t, &aitableLegacyRouteCaller{})
	if got, err := resolveCompatibleToolServer(t.Context(), "aitable-helper", "list_roles"); err != nil || got != "aitable-helper" {
		t.Fatalf("legacy caller route = (%q, %v)", got, err)
	}

	testseam.Swap(t, &deps, nil)
	if got, err := resolveCompatibleToolServer(t.Context(), "aitable-helper", "list_roles"); err != nil || got != "aitable-helper" {
		t.Fatalf("uninitialized caller route = (%q, %v)", got, err)
	}
}
