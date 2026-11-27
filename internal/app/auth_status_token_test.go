// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type statusTokenCaller struct {
	authCoverageCaller
	calls int
	err   error
	body  string
}

func (c *statusTokenCaller) CallToolWithToken(ctx context.Context, token, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	if token != "status-token-canary" || product != "contact" || tool != "get_current_user_profile" {
		return nil, errors.New("wrong token query")
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.body}}}, c.err
}
func TestCrossPlatformCoverageAuthStatusExplicitToken(t *testing.T) {
	for _, tc := range []struct {
		name, body, verification string
		err                      error
		readonly, dryrun, want   bool
		calls                    int
	}{
		{name: "valid", body: `{"success":true,"result":[{"orgEmployeeModel":{"corpId":"corp","userId":"user"}}]}`, verification: "valid", want: true, calls: 1},
		{name: "invalid", err: apperrors.NewAuth("status-token-canary", apperrors.WithReason("http_401")), verification: "invalid", calls: 1},
		{name: "network", err: context.DeadlineExceeded, verification: "unknown", calls: 1},
		{name: "forbidden", err: apperrors.NewAuth("forbidden", apperrors.WithReason("http_403")), verification: "unknown", calls: 1},
		{name: "malformed", body: `{}`, verification: "unknown", calls: 1},
		{name: "invalid_json", body: `not json`, verification: "unknown", calls: 1},
		{name: "business_error", body: `{"dingOpenErrcode":1,"result":[{"orgEmployeeModel":{"corpId":"corp","userId":"user"}}]}`, verification: "unknown", calls: 1},
		{name: "business_failure", body: `{"success":false,"result":[{"orgEmployeeModel":{"corpId":"corp","userId":"user"}}]}`, verification: "unknown", calls: 1},
		{name: "readonly", readonly: true, verification: "not_checked"},
		{name: "dryrun", dryrun: true, verification: "not_checked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			testseam.Swap(t, &authOAuthStatus, func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) {
				t.Fatal("read persisted credentials")
				return nil, nil
			})
			testseam.Swap(t, &authOAuthAccessToken, func(*authpkg.OAuthProvider, context.Context) (string, error) {
				t.Fatal("refreshed persisted credentials")
				return "", nil
			})
			caller := &statusTokenCaller{body: tc.body, err: tc.err}
			root := &cobra.Command{Use: "dws"}
			root.PersistentFlags().String("token", "", "")
			root.PersistentFlags().String("format", "json", "")
			root.PersistentFlags().Bool("dry-run", false, "")
			root.AddCommand(buildAuthCommand(caller))
			args := []string{"--token", "status-token-canary", "auth", "status", "--profile", "nonexistent-profile"}
			if tc.readonly {
				args = append(args, "--readonly")
			}
			if tc.dryrun {
				args = append(args, "--dry-run")
			}
			root.SetArgs(args)
			var out bytes.Buffer
			root.SetOut(&out)
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err := json.Unmarshal(out.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response["authenticated"] != tc.want || response["token_source"] != "explicit" || response["verification"] != tc.verification {
				t.Fatalf("unexpected status: %s", out.String())
			}
			if caller.calls != tc.calls {
				t.Fatalf("calls=%d want %d", caller.calls, tc.calls)
			}
			if strings.Contains(out.String(), "status-token-canary") {
				t.Fatal("credential leaked")
			}
		})
	}
}

func TestCrossPlatformCoverageAuthStatusTokenIdentity(t *testing.T) {
	for _, result := range []*edition.ToolResult{
		nil,
		{},
		{Content: []edition.ContentBlock{{Type: "image", Text: `{"result":[{"orgEmployeeModel":{"corpId":"corp","userId":"user"}}]}`}}},
		{Content: []edition.ContentBlock{{Type: "text", Text: `{"result":[{"orgEmployeeModel":{"userId":"user"}}]}`}}},
	} {
		if authStatusTokenIdentityValid(result) {
			t.Fatal("accepted missing or non-text identity")
		}
	}
}

func TestCrossPlatformCoverageAuthStatusTokenTextOutput(t *testing.T) {
	t.Setenv("DWS_NO_UPDATE_CHECK", "1")
	for _, failOutput := range []bool{false, true} {
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().String("token", "", "")
		root.PersistentFlags().String("format", "table", "")
		root.AddCommand(newAuthStatusCommand())
		root.SetArgs([]string{"--token", "status-token-canary", "status", "--readonly"})
		var out bytes.Buffer
		root.SetOut(&out)
		writeErr := errors.New("status output failed")
		if failOutput {
			root.SetOut(appFailWriter{err: writeErr})
		}
		err := corecmd.ExecuteForTest(root)
		if failOutput {
			if !errors.Is(err, writeErr) {
				t.Fatalf("output error not preserved: %v", err)
			}
			continue
		}
		if err != nil || !strings.Contains(out.String(), "验证: not_checked") || strings.Contains(out.String(), "status-token-canary") {
			t.Fatalf("unexpected text status: %q, err=%v", out.String(), err)
		}
	}
}

func TestCrossPlatformCoverageAuthStatusExplicitTokenRealRoot(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		t.Fatal("resolved stored token")
		return AccessTokenSnapshot{}, nil
	})
	testseam.Swap(t, &runnerGetCachedRuntimeToken, func(context.Context) (string, error) { t.Fatal("prefetched stored token"); return "", nil })
	testseam.Swap(t, &authOAuthStatus, func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) { t.Fatal("read local OAuth"); return nil, nil })
	calls := 0
	testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, endpoint, tool string, _ map[string]any) (transport.ToolCallResult, error) {
		calls++
		if client.AuthToken != "status-token-canary" || tool != "get_current_user_profile" {
			t.Fatal("wrong runtime token or tool")
		}
		return transport.ToolCallResult{Content: map[string]any{"success": true, "result": []any{map[string]any{"orgEmployeeModel": map[string]any{"corpId": "corp", "userId": "user"}}}}}, nil
	})
	root := NewRootCommand(t.Context())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--token", "status-token-canary", "auth", "status", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if response["authenticated"] != true || calls != 1 {
		t.Fatalf("calls=%d status=%s", calls, out.String())
	}
}
