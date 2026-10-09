// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"io"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageScopedTokenNeverReauthorizesSupervisor(t *testing.T) {
	if token, ok := scopedAuthToken(nil); ok || token != "" {
		t.Fatal("nil context has token")
	}
	for _, scenario := range []string{"preflight", "transport-scope", "edition-pat", "structured-pat", "business-scope", "second-classifier"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			old := edition.Get()
			t.Cleanup(func() { edition.Override(old) })
			edition.Override(&edition.Hooks{})
			pat := &apperrors.PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}
			testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
				if scenario == "preflight" {
					return pat
				}
				return nil
			})
			testseam.Swap(t, &runnerHandlePatAuthCheck, func(context.Context, *runtimeRunner, executor.Invocation, *apperrors.PATError, string, io.Writer) (executor.Result, error) {
				t.Error("scoped token requested supervisor authorization")
				return executor.Result{}, nil
			})
			testseam.Swap(t, &runnerRetryWithPatAuthRetry, func(context.Context, executor.Runner, executor.Invocation, *PatScopeError, string, io.Writer) (executor.Result, error) {
				t.Error("scoped token retried under another identity")
				return executor.Result{}, nil
			})
			testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
				switch scenario {
				case "transport-scope":
					return transport.ToolCallResult{}, errors.New("missing_scope mail:read")
				case "business-scope":
					return transport.ToolCallResult{IsError: true, Blocks: []transport.ContentBlock{{Text: "missing_scope mail:read"}}, Content: map[string]any{}}, nil
				case "second-classifier":
					return transport.ToolCallResult{IsError: true, Content: map[string]any{"success": false}}, nil
				default:
					return transport.ToolCallResult{Content: map[string]any{"code": "PAT_NO_PERMISSION", "data": map[string]any{"flowId": "fixture"}}}, nil
				}
			})
			if scenario == "edition-pat" {
				edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error { return pat }})
			}
			if scenario == "second-classifier" {
				calls := 0
				edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error {
					calls++
					if calls == 1 {
						return nil
					}
					return &authretry.AuthRefreshRequired{Cause: errors.New("expired employee token")}
				}})
			}
			r := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{Token: "supervisor"}}
			ctx := context.WithValue(context.Background(), scopedAuthTokenKey, "employee")
			if _, err := r.executeInvocation(ctx, "https://fixture.test", executor.Invocation{CanonicalProduct: "product", Tool: "tool"}); err == nil {
				t.Fatal("authorization failure accepted")
			}
		})
	}
}
