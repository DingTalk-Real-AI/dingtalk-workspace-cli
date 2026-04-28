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

package pat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// fakeToolCaller captures the toolArgs passed to CallTool so tests can
// assert how the two-tier --agentCode / DINGTALK_DWS_AGENTCODE / error
// resolver feeds into the outgoing MCP argv.
type fakeToolCaller struct {
	mu       sync.Mutex
	dryRun   bool
	gotTool  string
	gotArgs  map[string]any
	callN    int
	resultOK bool
}

func (f *fakeToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callN++
	f.gotTool = toolName
	// defensive copy — RunE / runApply may mutate the map after return
	f.gotArgs = make(map[string]any, len(args))
	for k, v := range args {
		f.gotArgs[k] = v
	}
	// Empty success payload keeps handleToolResult / emitApplyResult happy
	// without triggering PAT classification in errors.ClassifyMCPResponseText.
	if f.resultOK {
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"data":{}}`}}}, nil
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"data":{"authRequestId":"req-ok"}}`}}}, nil
}

func (f *fakeToolCaller) Format() string { return "json" }
func (f *fakeToolCaller) DryRun() bool   { return f.dryRun }

type recordedToolCall struct {
	tool string
	args map[string]any
}

type fallbackToolCaller struct {
	calls []recordedToolCall
}

func (f *fallbackToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	f.calls = append(f.calls, recordedToolCall{tool: toolName, args: copied})
	if len(f.calls) == 1 {
		return &edition.ToolResult{}, nil
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"data":{"authRequestId":"req-ok"}}`}}}, nil
}

func (f *fallbackToolCaller) Format() string { return "json" }
func (f *fallbackToolCaller) DryRun() bool   { return false }

type fallbackErrorToolCaller struct {
	calls []recordedToolCall
}

func (f *fallbackErrorToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	f.calls = append(f.calls, recordedToolCall{tool: toolName, args: copied})
	if len(f.calls) == 1 {
		return nil, errors.New("pat chmod failed: business error: PARAM_ERROR - 未找到指定工具")
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"data":{"authRequestId":"req-ok"}}`}}}, nil
}

func (f *fallbackErrorToolCaller) Format() string { return "json" }
func (f *fallbackErrorToolCaller) DryRun() bool   { return false }

type fallbackSchemaMismatchToolCaller struct {
	calls []recordedToolCall
}

func (f *fallbackSchemaMismatchToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	f.calls = append(f.calls, recordedToolCall{tool: toolName, args: copied})
	if len(f.calls) == 1 {
		return nil, apperrors.NewAPI("business error: success=false",
			apperrors.WithReason("business_error"),
			apperrors.WithServerDiag(apperrors.ServerDiagnostics{
				ServerErrorCode: "PARAM_ERROR",
				TechnicalDetail: `input schema validation failed: unknown field "scopes"; missing required field "scope"`,
			}),
		)
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":true,"data":{"authRequestId":"req-ok"}}`}}}, nil
}

func (f *fallbackSchemaMismatchToolCaller) Format() string { return "json" }
func (f *fallbackSchemaMismatchToolCaller) DryRun() bool   { return false }

type fallbackPermissionDeniedToolCaller struct {
	calls []recordedToolCall
}

func (f *fallbackPermissionDeniedToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	f.calls = append(f.calls, recordedToolCall{tool: toolName, args: copied})
	return nil, apperrors.NewAPI("business error: success=false",
		apperrors.WithReason("business_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			ServerErrorCode: "PAT_MEDIUM_RISK_NO_PERMISSION",
			TechnicalDetail: "permission denied for scope chat.message:send",
		}),
	)
}

func (f *fallbackPermissionDeniedToolCaller) Format() string { return "json" }
func (f *fallbackPermissionDeniedToolCaller) DryRun() bool   { return false }

type fallbackPATErrorToolCaller struct {
	calls []recordedToolCall
}

func (f *fallbackPATErrorToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	f.calls = append(f.calls, recordedToolCall{tool: toolName, args: copied})
	return nil, &apperrors.PATError{RawJSON: `{"success":false,"code":"PAT_SCOPE_AUTH_REQUIRED","data":{"missingScope":"mail:send"}}`}
}

func (f *fallbackPATErrorToolCaller) Format() string { return "json" }
func (f *fallbackPATErrorToolCaller) DryRun() bool   { return false }

type fallbackPATContractErrorToolCaller struct {
	calls []recordedToolCall
}

func (f *fallbackPATContractErrorToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	f.calls = append(f.calls, recordedToolCall{tool: toolName, args: copied})
	return nil, apperrors.NewAPI("business error: success=false",
		apperrors.WithReason("business_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			ServerErrorCode: "PAT_SCOPE_AUTH_REQUIRED",
			TechnicalDetail: `missingScope mail:send`,
		}),
	)
}

func (f *fallbackPATContractErrorToolCaller) Format() string { return "json" }
func (f *fallbackPATContractErrorToolCaller) DryRun() bool   { return false }

func stringSliceArgEqual(got any, want []string) bool {
	gotSlice, ok := got.([]string)
	if !ok || len(gotSlice) != len(want) {
		return false
	}
	for i := range want {
		if gotSlice[i] != want[i] {
			return false
		}
	}
	return true
}

// buildChmod returns a freshly constructed chmod cobra.Command wired to
// fake. Using the factory (instead of a package-level var) keeps every
// subtest hermetic and matches the upstream shared-state fix in PR #129.
func buildChmod(t *testing.T, fake *fakeToolCaller) *cobra.Command {
	t.Helper()
	return newChmodCommand(fake)
}

// ---------------------------------------------------------------------------
// T1 · Agent-code env fallback tests
// ---------------------------------------------------------------------------

// TestChmod_agentCode_env_fallback verifies that when --agentCode is
// omitted but DINGTALK_DWS_AGENTCODE is exported, the resolver picks
// the env value up and forwards it verbatim in the MCP argv.
func TestChmod_agentCode_env_fallback(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)

	// grant-type=once → no session-id needed; keeps the test hermetic.
	_ = cmd.Flags().Set("grant-type", "once")
	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v (must not report flag missing)", err)
	}

	if got := fake.gotArgs["agentCode"]; got != "qoderwork" {
		t.Fatalf("agentCode in argv = %v, want %q (env fallback)", got, "qoderwork")
	}
	if got := fake.gotArgs["scopes"]; !stringSliceArgEqual(got, []string{"aitable.record:read"}) {
		t.Fatalf("scopes in argv = %#v, want %#v", got, []string{"aitable.record:read"})
	}
	if _, ok := fake.gotArgs["scope"]; ok {
		t.Fatalf("unexpected legacy singular scope arg in argv: %#v", fake.gotArgs)
	}
}

func TestCallPATToolWithLegacyFallback_emptyCanonicalResultDoesNotRetryLegacyAlias(t *testing.T) {
	fake := &fallbackToolCaller{}
	canonicalArgs := map[string]any{
		"agentCode": "qoderwork",
		"scopes":    []string{"aitable.record:read"},
		"grantType": "permanent",
	}
	legacyArgs := map[string]any{
		"agentCode": "qoderwork",
		"scope":     []string{"aitable.record:read"},
		"grantType": "permanent",
	}

	result, err := callPATToolWithLegacyFallback(context.Background(), fake, "pat", patGrantToolName, patGrantToolNameLegacyAlias, canonicalArgs, legacyArgs)
	if err != nil {
		t.Fatalf("callPATToolWithLegacyFallback error = %v", err)
	}
	if !isEmptyToolResult(result) {
		t.Fatalf("expected original empty canonical result, got %#v", result)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool call count = %d, want 1", len(fake.calls))
	}
	if fake.calls[0].tool != patGrantToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patGrantToolName)
	}
	if _, ok := fake.calls[0].args["scopes"]; !ok {
		t.Fatalf("canonical args missing scopes: %#v", fake.calls[0].args)
	}
	if _, ok := fake.calls[0].args["scope"]; ok {
		t.Fatalf("canonical args should not use legacy scope: %#v", fake.calls[0].args)
	}
}

func TestChmod_emptyCanonicalResultReturnsError(t *testing.T) {
	fake := &fallbackToolCaller{}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("agentCode", "qoderwork")
	_ = cmd.Flags().Set("grant-type", "permanent")

	err := cmd.RunE(cmd, []string{"aitable.record:read"})
	if err == nil {
		t.Fatal("chmod RunE error = nil, want empty PAT authorization result")
	}
	if !strings.Contains(err.Error(), "empty PAT authorization result") {
		t.Fatalf("chmod RunE error = %q, want empty PAT authorization result", err.Error())
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool call count = %d, want 1", len(fake.calls))
	}
	if fake.calls[0].tool != patGrantToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patGrantToolName)
	}
}

func TestCallPATToolWithLegacyFallback_toolNotFoundRetriesLegacyAlias(t *testing.T) {
	fake := &fallbackErrorToolCaller{}
	canonicalArgs := map[string]any{
		"agentCode": "qoderwork",
		"scopes":    []string{"aitable.record:read"},
		"grantType": "permanent",
	}
	legacyArgs := map[string]any{
		"agentCode": "qoderwork",
		"scope":     []string{"aitable.record:read"},
		"grantType": "permanent",
	}

	result, err := callPATToolWithLegacyFallback(context.Background(), fake, "pat", patGrantToolName, patGrantToolNameLegacyAlias, canonicalArgs, legacyArgs)
	if err != nil {
		t.Fatalf("callPATToolWithLegacyFallback error = %v", err)
	}
	if isEmptyToolResult(result) {
		t.Fatalf("fallback result is empty: %#v", result)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool call count = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].tool != patGrantToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patGrantToolName)
	}
	if fake.calls[1].tool != patGrantToolNameLegacyAlias {
		t.Fatalf("fallback tool = %q, want %q", fake.calls[1].tool, patGrantToolNameLegacyAlias)
	}
	if _, ok := fake.calls[1].args["scope"]; !ok {
		t.Fatalf("legacy args missing scope: %#v", fake.calls[1].args)
	}
	if _, ok := fake.calls[1].args["scopes"]; ok {
		t.Fatalf("legacy args should not use canonical scopes: %#v", fake.calls[1].args)
	}
}

func TestCallPATToolWithLegacyFallback_schemaMismatchRetriesLegacyAlias(t *testing.T) {
	fake := &fallbackSchemaMismatchToolCaller{}
	canonicalArgs := map[string]any{
		"agentCode": "qoderwork",
		"scopes":    []string{"aitable.record:read"},
		"grantType": "permanent",
	}
	legacyArgs := map[string]any{
		"agentCode": "qoderwork",
		"scope":     []string{"aitable.record:read"},
		"grantType": "permanent",
	}

	result, err := callPATToolWithLegacyFallback(context.Background(), fake, "pat", patGrantToolName, patGrantToolNameLegacyAlias, canonicalArgs, legacyArgs)
	if err != nil {
		t.Fatalf("callPATToolWithLegacyFallback error = %v", err)
	}
	if isEmptyToolResult(result) {
		t.Fatalf("fallback result is empty: %#v", result)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool call count = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].tool != patGrantToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patGrantToolName)
	}
	if fake.calls[1].tool != patGrantToolNameLegacyAlias {
		t.Fatalf("fallback tool = %q, want %q", fake.calls[1].tool, patGrantToolNameLegacyAlias)
	}
	if _, ok := fake.calls[1].args["scope"]; !ok {
		t.Fatalf("legacy args missing scope: %#v", fake.calls[1].args)
	}
}

func TestCallPATToolWithLegacyFallback_permissionDeniedDoesNotRetryLegacyAlias(t *testing.T) {
	fake := &fallbackPermissionDeniedToolCaller{}
	canonicalArgs := map[string]any{
		"agentCode": "qoderwork",
		"scopes":    []string{"chat.message:send"},
		"grantType": "once",
	}
	legacyArgs := map[string]any{
		"agentCode": "qoderwork",
		"scope":     []string{"chat.message:send"},
		"grantType": "once",
	}

	_, err := callPATToolWithLegacyFallback(context.Background(), fake, "pat", patGrantToolName, patGrantToolNameLegacyAlias, canonicalArgs, legacyArgs)
	if err == nil {
		t.Fatal("callPATToolWithLegacyFallback error = nil, want original permission denial")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if typed.ServerDiag.ServerErrorCode != "PAT_MEDIUM_RISK_NO_PERMISSION" {
		t.Fatalf("ServerErrorCode = %q, want PAT_MEDIUM_RISK_NO_PERMISSION", typed.ServerDiag.ServerErrorCode)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool call count = %d, want 1", len(fake.calls))
	}
}

func TestCallPATToolWithLegacyFallback_patErrorDoesNotRetryLegacyAlias(t *testing.T) {
	fake := &fallbackPATErrorToolCaller{}
	canonicalArgs := map[string]any{
		"agentCode": "qoderwork",
		"scopes":    []string{"mail:send"},
		"grantType": "once",
	}
	legacyArgs := map[string]any{
		"agentCode": "qoderwork",
		"scope":     []string{"mail:send"},
		"grantType": "once",
	}

	_, err := callPATToolWithLegacyFallback(context.Background(), fake, "pat", patGrantToolName, patGrantToolNameLegacyAlias, canonicalArgs, legacyArgs)
	if err == nil {
		t.Fatal("callPATToolWithLegacyFallback error = nil, want PATError")
	}
	if !apperrors.IsPATError(err) {
		t.Fatalf("expected PATError, got %T", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool call count = %d, want 1", len(fake.calls))
	}
}

func TestCallPATToolWithLegacyFallback_patContractErrorDoesNotRetryLegacyAlias(t *testing.T) {
	fake := &fallbackPATContractErrorToolCaller{}
	canonicalArgs := map[string]any{
		"agentCode": "qoderwork",
		"scopes":    []string{"mail:send"},
		"grantType": "once",
	}
	legacyArgs := map[string]any{
		"agentCode": "qoderwork",
		"scope":     []string{"mail:send"},
		"grantType": "once",
	}

	_, err := callPATToolWithLegacyFallback(context.Background(), fake, "pat", patGrantToolName, patGrantToolNameLegacyAlias, canonicalArgs, legacyArgs)
	if err == nil {
		t.Fatal("callPATToolWithLegacyFallback error = nil, want original PAT contract error")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if typed.ServerDiag.ServerErrorCode != "PAT_SCOPE_AUTH_REQUIRED" {
		t.Fatalf("ServerErrorCode = %q, want PAT_SCOPE_AUTH_REQUIRED", typed.ServerDiag.ServerErrorCode)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool call count = %d, want 1", len(fake.calls))
	}
}

func TestIsToolNotRegisteredError_ChineseGatewayMessage(t *testing.T) {
	err := errors.New("pat chmod failed: business error: PARAM_ERROR - 未找到指定工具")
	if !isToolNotRegisteredError(err) {
		t.Fatalf("isToolNotRegisteredError(%q) = false, want true", err.Error())
	}
}

func TestIsToolNotRegisteredError_ChineseGatewayDiagnostics(t *testing.T) {
	err := apperrors.NewAPI("business error: success=false",
		apperrors.WithReason("business_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			ServerErrorCode: "PARAM_ERROR",
			TechnicalDetail: "Tool metadata API error: PARAM_ERROR - 未找到指定工具",
		}),
	)
	if !isToolNotRegisteredError(err) {
		t.Fatalf("isToolNotRegisteredError(%q) = false, want true", err.Error())
	}
}

func TestHandleToolResult_emptyResultReturnsError(t *testing.T) {
	err := handleToolResult(&edition.ToolResult{})
	if err == nil {
		t.Fatal("handleToolResult error = nil, want empty PAT authorization result error")
	}
	if !strings.Contains(err.Error(), "empty PAT authorization result") {
		t.Fatalf("handleToolResult error = %q, want empty PAT authorization result", err.Error())
	}
}

// TestChmod_agentCode_env_invalid verifies that a malformed
// DINGTALK_DWS_AGENTCODE value (whitespace, shell metacharacters) is
// rejected by the regex gate in validateAgentCode before any MCP call
// is attempted.
func TestChmod_agentCode_env_invalid(t *testing.T) {
	t.Setenv(agentCodeEnv, "bad value with space!")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)
	_ = cmd.Flags().Set("grant-type", "once")

	err := cmd.RunE(cmd, []string{"aitable.record:read"})
	if err == nil {
		t.Fatalf("expected validateAgentCode error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid agentCode") {
		t.Fatalf("error = %q, want to mention 'invalid agentCode'", err.Error())
	}
	if !strings.Contains(err.Error(), agentCodeEnv) {
		t.Fatalf("error = %q, want to attribute to %s env", err.Error(), agentCodeEnv)
	}
	if fake.callN != 0 {
		t.Fatalf("CallTool was invoked %d times; validator must short-circuit before MCP", fake.callN)
	}
}

// TestChmod_agentCode_flag_wins_over_env verifies the Priority-1 contract
// of resolveAgentCode: when both the flag and the env are set, the flag
// wins and env is silently ignored (no warning needed because the flag is
// the explicit, scripted intent).
func TestChmod_agentCode_flag_wins_over_env(t *testing.T) {
	t.Setenv(agentCodeEnv, "envval")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)

	_ = cmd.Flags().Set("grant-type", "once")
	_ = cmd.Flags().Set("agentCode", "flagval")

	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if got := fake.gotArgs["agentCode"]; got != "flagval" {
		t.Fatalf("agentCode in argv = %v, want %q (flag must win over env)", got, "flagval")
	}
}

// TestChmod_agentCode_legacy_env_not_recognized is a reverse-guard: after
// the SSOT hard-removal of the DWS_AGENTCODE alias, exporting only the
// legacy env MUST NOT satisfy the --agentCode requirement. The command
// is expected to fail with an error that explicitly names the canonical
// DINGTALK_DWS_AGENTCODE env, and MUST NOT mention DWS_AGENTCODE as a
// usable fallback. No MCP call is permitted.
func TestChmod_agentCode_legacy_env_not_recognized(t *testing.T) {
	t.Setenv(agentCodeEnv, "")
	t.Setenv("DWS_AGENTCODE", "legacyval")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)
	_ = cmd.Flags().Set("grant-type", "once")

	err := cmd.RunE(cmd, []string{"aitable.record:read"})
	if err == nil {
		t.Fatalf("expected hard error when only legacy DWS_AGENTCODE is set, got nil")
	}
	if !strings.Contains(err.Error(), "DINGTALK_DWS_AGENTCODE") {
		t.Fatalf("error = %q, want to name canonical DINGTALK_DWS_AGENTCODE env", err.Error())
	}
	// Defensive: the canonical env naturally contains the substring
	// "DWS_AGENTCODE" as part of "DINGTALK_DWS_AGENTCODE"; the above
	// assertion plus the absence check below precisely guard against
	// advertising the legacy alias as usable.
	hint := strings.ReplaceAll(err.Error(), "DINGTALK_DWS_AGENTCODE", "")
	if strings.Contains(hint, "DWS_AGENTCODE") {
		t.Fatalf("error = %q must not advertise DWS_AGENTCODE as usable", err.Error())
	}
	if fake.callN != 0 {
		t.Fatalf("CallTool was invoked %d times; legacy env must not satisfy --agentCode", fake.callN)
	}
}

// ---------------------------------------------------------------------------
// validateAgentCode / resolveAgentCodeFromEnv unit tests
// ---------------------------------------------------------------------------

func TestValidateAgentCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"qoderwork", false},
		{"agt-abc123", false},
		{"Agt_Xyz-09", false},
		{"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false}, // 64 chars
		{"", true},
		{"bad value", true},
		{"bad!chars", true},
		{"中文不行", true},
		{"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789X", true}, // 65
	}
	for _, tc := range cases {
		err := validateAgentCode(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateAgentCode(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestResolveAgentCodeFromEnv(t *testing.T) {
	// Not parallel: mutates process env.

	// DINGTALK_DWS_AGENTCODE is honoured and trimmed.
	t.Setenv(agentCodeEnv, "  qoderwork  ")
	if code, src := resolveAgentCodeFromEnv(); code != "qoderwork" || src != agentCodeEnv {
		t.Errorf("resolveAgentCodeFromEnv() = (%q, %q), want (%q, %q)",
			code, src, "qoderwork", agentCodeEnv)
	}

	// Empty primary → ("", "").
	t.Setenv(agentCodeEnv, "")
	if code, src := resolveAgentCodeFromEnv(); code != "" || src != "" {
		t.Errorf("resolveAgentCodeFromEnv() = (%q, %q), want empty", code, src)
	}

	// Reverse-guard: legacy DWS_AGENTCODE MUST NOT be picked up when the
	// canonical env is unset — it was hard-removed as a legacy alias.
	t.Setenv(agentCodeEnv, "")
	t.Setenv("DWS_AGENTCODE", "legacy")
	if code, src := resolveAgentCodeFromEnv(); code != "" || src != "" {
		t.Errorf("resolveAgentCodeFromEnv() = (%q, %q), want empty — legacy DWS_AGENTCODE must be ignored",
			code, src)
	}
}
