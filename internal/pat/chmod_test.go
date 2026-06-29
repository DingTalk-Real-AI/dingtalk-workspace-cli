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
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// fakeToolCaller captures the toolArgs passed to CallTool so tests can
// assert how the optional --agentCode / DINGTALK_DWS_AGENTCODE resolver feeds
// into the outgoing batch request.
type fakeToolCaller struct {
	mu                sync.Mutex
	dryRun            bool
	gotTool           string
	gotArgs           map[string]any
	gotAgentEnv       string
	gotSessionEnv     string
	gotDingSessionEnv string
	callN             int
	resultOK          bool
}

func (f *fakeToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callN++
	f.gotTool = toolName
	f.gotAgentEnv = os.Getenv(agentCodeEnv)
	f.gotSessionEnv = os.Getenv(sessionIDEnvDWS)
	f.gotDingSessionEnv = os.Getenv(sessionIDEnvDingtalk)
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
	tool           string
	args           map[string]any
	agentEnv       string
	sessionEnv     string
	dingSessionEnv string
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

type sequenceToolCaller struct {
	calls     []recordedToolCall
	responses []string
	errs      []error
	dryRun    bool
}

func (s *sequenceToolCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	s.calls = append(s.calls, recordedToolCall{tool: toolName, args: copied})
	s.calls[len(s.calls)-1].agentEnv = os.Getenv(agentCodeEnv)
	s.calls[len(s.calls)-1].sessionEnv = os.Getenv(sessionIDEnvDWS)
	s.calls[len(s.calls)-1].dingSessionEnv = os.Getenv(sessionIDEnvDingtalk)
	if len(s.errs) >= len(s.calls) && s.errs[len(s.calls)-1] != nil {
		return nil, s.errs[len(s.calls)-1]
	}
	response := `{"success":true,"data":{}}`
	if len(s.responses) >= len(s.calls) {
		response = s.responses[len(s.calls)-1]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}

func (s *sequenceToolCaller) Format() string { return "json" }
func (s *sequenceToolCaller) DryRun() bool   { return s.dryRun }

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

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error = %v", err)
	}
	os.Stdout = writePipe
	runErr := fn()
	_ = writePipe.Close()
	os.Stdout = oldStdout
	out, readErr := io.ReadAll(readPipe)
	_ = readPipe.Close()
	if readErr != nil {
		t.Fatalf("ReadAll stdout error = %v", readErr)
	}
	return string(out), runErr
}

// buildChmod returns a freshly constructed chmod cobra.Command wired to
// fake. Using the factory (instead of a package-level var) keeps every
// subtest hermetic and matches the upstream shared-state fix in PR #129.
func buildChmod(t *testing.T, fake *fakeToolCaller) *cobra.Command {
	t.Helper()
	return newChmodCommand(fake)
}

func attachRootYesFlag(t *testing.T, cmd *cobra.Command, yes bool) {
	t.Helper()
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("yes", false, "skip confirmation")
	root.AddCommand(cmd)
	if yes {
		if err := root.PersistentFlags().Set("yes", "true"); err != nil {
			t.Fatalf("set root --yes: %v", err)
		}
	}
}

func attachRootPATFlags(t *testing.T, cmd *cobra.Command, yes bool, formatChanged bool) {
	t.Helper()
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("yes", false, "skip confirmation")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("verbose", false, "")
	root.AddCommand(cmd)
	if yes {
		if err := root.PersistentFlags().Set("yes", "true"); err != nil {
			t.Fatalf("set root --yes: %v", err)
		}
	}
	if formatChanged {
		if err := root.PersistentFlags().Set("format", "json"); err != nil {
			t.Fatalf("set root --format: %v", err)
		}
	}
}

func setBatchYesForTest(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	attachRootYesFlag(t, cmd, true)
}

func TestRegisterCommands_OnlyExposesChmodForAuthorization(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	RegisterCommands(root, &fakeToolCaller{})

	patCmd, _, err := root.Find([]string{"pat"})
	if err != nil {
		t.Fatalf("pat command not found: %v", err)
	}
	children := map[string]bool{}
	for _, child := range patCmd.Commands() {
		children[child.Name()] = true
	}
	if !children["chmod"] {
		t.Fatal("pat chmod command not registered")
	}
	for _, unexpected := range []string{"grant-batch", "grant-recommend", "scopes", "check"} {
		if children[unexpected] {
			t.Fatalf("unexpected pat subcommand %q registered; authorization must stay on chmod", unexpected)
		}
	}
}

func TestPATHelpDocumentsBatchAuthorization(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	RegisterCommands(root, &fakeToolCaller{})

	patCmd, _, err := root.Find([]string{"pat"})
	if err != nil {
		t.Fatalf("pat command not found: %v", err)
	}
	var out strings.Builder
	patCmd.SetOut(&out)
	patCmd.SetErr(&out)
	if err := patCmd.Help(); err != nil {
		t.Fatalf("pat help error = %v", err)
	}
	patHelp := out.String()
	for _, want := range []string{
		"支持批量授权",
		"--products / --product",
		"--domains / --domain",
		"--recommend",
		"DINGTALK_DWS_AGENTCODE",
		"未传 agentCode 时由服务端默认兜底",
	} {
		if !strings.Contains(patHelp, want) {
			t.Fatalf("pat help missing %q\nhelp:\n%s", want, patHelp)
		}
	}

	chmodCmd, _, err := root.Find([]string{"pat", "chmod"})
	if err != nil {
		t.Fatalf("pat chmod command not found: %v", err)
	}
	out.Reset()
	chmodCmd.SetOut(&out)
	chmodCmd.SetErr(&out)
	if err := chmodCmd.Help(); err != nil {
		t.Fatalf("pat chmod help error = %v", err)
	}
	chmodHelp := out.String()
	for _, want := range []string{
		"批量授权:",
		"一次传多个 scope",
		"batch plan",
		"--dry-run 只返回授权计划",
		"执行批量授权必须显式",
		"由服务端默认兜底",
		"aitable.record:read aitable.record:write --grant-type permanent --yes",
		"dws pat chmod --products calendar,aitable",
		"dws pat chmod --recommend --grant-type session",
	} {
		if !strings.Contains(chmodHelp, want) {
			t.Fatalf("pat chmod help missing %q\nhelp:\n%s", want, chmodHelp)
		}
	}
}

func TestChmod_productsFlagPlansThenGrantsSelectedScopes(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"selectedScopes":["calendar.event:read","aitable.record:read"]}}`,
		`{"success":true,"data":{"grantedScopes":["calendar.event:read","aitable.record:read"]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")
	_ = cmd.Flags().Set("products", "calendar,aitable")
	setBatchYesForTest(t, cmd)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}

	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
	}
	if got := fake.calls[0].args["productCodes"]; !stringSliceArgEqual(got, []string{"calendar", "aitable"}) {
		t.Fatalf("productCodes = %#v, want calendar/aitable", got)
	}
	if got := fake.calls[0].args["recommend"]; got != false {
		t.Fatalf("recommend = %#v, want false", got)
	}
	if got := fake.calls[0].agentEnv; got != "qoderwork" {
		t.Fatalf("plan agent env = %q, want qoderwork", got)
	}
	if got := fake.calls[0].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("batch plan agentCode = %#v, want qoderwork", got)
	}
	if fake.calls[1].tool != patBatchGrantToolName {
		t.Fatalf("second tool = %q, want %q", fake.calls[1].tool, patBatchGrantToolName)
	}
	if got := fake.calls[1].args["scopes"]; !stringSliceArgEqual(got, []string{"calendar.event:read", "aitable.record:read"}) {
		t.Fatalf("grant scopes = %#v, want selected scopes", got)
	}
	if got := fake.calls[1].agentEnv; got != "qoderwork" {
		t.Fatalf("grant agent env = %q, want qoderwork", got)
	}
	if got := fake.calls[1].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("batch grant agentCode = %#v, want qoderwork", got)
	}
}

func TestChmod_productsFlagWithoutYesBlocksAfterPlan(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"selectedScopes":["calendar.event:read"]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")
	_ = cmd.Flags().Set("products", "calendar")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("chmod RunE error = nil, want batch --yes blocker")
	}
	if !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "batch PAT authorization blocked") {
		t.Fatalf("error = %q, want explicit batch --yes blocker", err.Error())
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool count = %d, want plan only", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
	}
}

func TestChmod_multipleScopesWithoutYesBlocksBeforeMCP(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "permanent")

	err := cmd.RunE(cmd, []string{"calendar.event:read", "aitable.record:write"})
	if err == nil {
		t.Fatal("chmod RunE error = nil, want batch --yes blocker")
	}
	if !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "batch PAT authorization blocked") {
		t.Fatalf("error = %q, want explicit batch --yes blocker", err.Error())
	}
	if len(fake.calls) != 0 {
		t.Fatalf("CallTool count = %d, want blocker before MCP", len(fake.calls))
	}
}

func TestChmod_productsSessionModePassesIdentityArgsAndCompatEnv(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"grantedScopes":["calendar.event:read"]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("products", "calendar")
	_ = cmd.Flags().Set("session-id", "session-123")
	setBatchYesForTest(t, cmd)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}

	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want 2", len(fake.calls))
	}
	if got := fake.calls[0].args["grantType"]; got != "session" {
		t.Fatalf("plan grantType = %#v, want session", got)
	}
	if got := fake.calls[0].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("plan agentCode = %#v, want qoderwork", got)
	}
	if got := fake.calls[0].args["sessionId"]; got != "session-123" {
		t.Fatalf("plan sessionId = %#v, want session-123", got)
	}
	if got := fake.calls[0].agentEnv; got != "qoderwork" {
		t.Fatalf("plan agent env = %q, want qoderwork", got)
	}
	if fake.calls[0].dingSessionEnv != "session-123" {
		t.Fatalf("plan %s env = %q, want session-123", sessionIDEnvDingtalk, fake.calls[0].dingSessionEnv)
	}
	if got := fake.calls[1].args["grantType"]; got != "session" {
		t.Fatalf("grant grantType = %#v, want session", got)
	}
	if got := fake.calls[1].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("grant agentCode = %#v, want qoderwork", got)
	}
	if got := fake.calls[1].args["sessionId"]; got != "session-123" {
		t.Fatalf("grant sessionId = %#v, want session-123", got)
	}
	if got := fake.calls[1].agentEnv; got != "qoderwork" {
		t.Fatalf("grant agent env = %q, want qoderwork", got)
	}
	if fake.calls[1].dingSessionEnv != "session-123" {
		t.Fatalf("grant %s env = %q, want session-123", sessionIDEnvDingtalk, fake.calls[1].dingSessionEnv)
	}
}

func TestChmod_singleScopeReturnsServerAgentCodeInSummary(t *testing.T) {
	t.Setenv(agentCodeEnv, "")
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"code":"OK","data":{"agentCode":"dingmbw5n9ktkkbbjv3g","grantType":"once","grantedScopes":["contact.user:get-self"]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")
	attachRootPATFlags(t, cmd, false, false)

	output, err := captureStdout(t, func() error {
		return cmd.RunE(cmd, []string{"contact.user:get-self"})
	})
	if err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool count = %d, want 1", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchGrantToolName {
		t.Fatalf("tool = %q, want %q", fake.calls[0].tool, patBatchGrantToolName)
	}
	if _, ok := fake.calls[0].args["agentCode"]; ok {
		t.Fatalf("agentCode arg must be omitted so PAT-core can default it: %#v", fake.calls[0].args)
	}
	if !strings.Contains(output, "agentCode: dingmbw5n9ktkkbbjv3g") {
		t.Fatalf("summary output missing server default agentCode:\n%s", output)
	}
}

func TestChmod_flagAgentCodeWinsAndReturnedAgentCodeMatches(t *testing.T) {
	t.Setenv(agentCodeEnv, "envshouldlose")
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"code":"OK","data":{"agentCode":"qoderwork","grantType":"once","grantedScopes":["chat.bot:search"]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")
	_ = cmd.Flags().Set("agentCode", "qoderwork")
	attachRootPATFlags(t, cmd, false, false)

	output, err := captureStdout(t, func() error {
		return cmd.RunE(cmd, []string{"chat.bot:search"})
	})
	if err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool count = %d, want 1", len(fake.calls))
	}
	if got := fake.calls[0].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("agentCode arg = %#v, want qoderwork", got)
	}
	if got := fake.calls[0].agentEnv; got != "qoderwork" {
		t.Fatalf("%s during CallTool = %q, want qoderwork", agentCodeEnv, got)
	}
	if !strings.Contains(output, "agentCode: qoderwork") {
		t.Fatalf("summary output missing qoderwork agentCode:\n%s", output)
	}
}

func TestChmod_batchEntryPointMatrixRequiresYesAndReturnsAgentCode(t *testing.T) {
	cases := []struct {
		name             string
		args             []string
		setFlags         func(*cobra.Command)
		wantPlanProducts []string
		wantRecommend    bool
		wantCallCount    int
	}{
		{
			name: "direct multi scope",
			args: []string{"calendar.event:list", "calendar.event:create"},
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("grant-type", "once")
			},
			wantCallCount: 1,
		},
		{
			name: "product repeated",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("grant-type", "once")
				_ = cmd.Flags().Set("product", "calendar")
				_ = cmd.Flags().Set("product", "aitable")
			},
			wantPlanProducts: []string{"calendar", "aitable"},
			wantCallCount:    2,
		},
		{
			name: "products comma list",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("grant-type", "once")
				_ = cmd.Flags().Set("products", "calendar,aitable")
			},
			wantPlanProducts: []string{"calendar", "aitable"},
			wantCallCount:    2,
		},
		{
			name: "domain repeated",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("grant-type", "once")
				_ = cmd.Flags().Set("domain", "calendar")
				_ = cmd.Flags().Set("domain", "chat")
			},
			wantPlanProducts: []string{"calendar", "chat"},
			wantCallCount:    2,
		},
		{
			name: "domains comma list",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("grant-type", "once")
				_ = cmd.Flags().Set("domains", "calendar,chat")
			},
			wantPlanProducts: []string{"calendar", "chat"},
			wantCallCount:    2,
		},
		{
			name: "recommend",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("grant-type", "once")
				_ = cmd.Flags().Set("recommend", "true")
			},
			wantRecommend: true,
			wantCallCount: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agentCodeEnv, "qoderwork")
			responses := []string{
				`{"success":true,"code":"OK","data":{"agentCode":"qoderwork","grantType":"once","grantedScopes":["calendar.event:list","calendar.event:create"]}}`,
			}
			if tc.wantCallCount == 2 {
				responses = []string{
					`{"success":true,"code":"OK","data":{"agentCode":"qoderwork","selectedScopes":["calendar.event:list","calendar.event:create"],"skippedScopes":[],"pendingScopes":[]}}`,
					`{"success":true,"code":"OK","data":{"agentCode":"qoderwork","grantType":"once","grantedScopes":["calendar.event:list","calendar.event:create"]}}`,
				}
			}
			fake := &sequenceToolCaller{responses: responses}
			cmd := newChmodCommand(fake)
			tc.setFlags(cmd)
			attachRootPATFlags(t, cmd, true, false)

			output, err := captureStdout(t, func() error {
				return cmd.RunE(cmd, tc.args)
			})
			if err != nil {
				t.Fatalf("chmod RunE error = %v", err)
			}
			if len(fake.calls) != tc.wantCallCount {
				t.Fatalf("CallTool count = %d, want %d", len(fake.calls), tc.wantCallCount)
			}
			if tc.wantCallCount == 1 {
				if fake.calls[0].tool != patBatchGrantToolName {
					t.Fatalf("tool = %q, want %q", fake.calls[0].tool, patBatchGrantToolName)
				}
				if got := fake.calls[0].args["scopes"]; !stringSliceArgEqual(got, tc.args) {
					t.Fatalf("grant scopes = %#v, want %#v", got, tc.args)
				}
			} else {
				if fake.calls[0].tool != patBatchPlanToolName {
					t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
				}
				if got := fake.calls[0].args["productCodes"]; !stringSliceArgEqual(got, tc.wantPlanProducts) {
					t.Fatalf("plan productCodes = %#v, want %#v", got, tc.wantPlanProducts)
				}
				if got := fake.calls[0].args["recommend"]; got != tc.wantRecommend {
					t.Fatalf("plan recommend = %#v, want %v", got, tc.wantRecommend)
				}
				if fake.calls[1].tool != patBatchGrantToolName {
					t.Fatalf("second tool = %q, want %q", fake.calls[1].tool, patBatchGrantToolName)
				}
				if got := fake.calls[1].args["scopes"]; !stringSliceArgEqual(got, []string{"calendar.event:list", "calendar.event:create"}) {
					t.Fatalf("grant scopes = %#v, want selected scopes", got)
				}
			}
			last := fake.calls[len(fake.calls)-1]
			if got := last.args["agentCode"]; got != "qoderwork" {
				t.Fatalf("grant agentCode = %#v, want qoderwork", got)
			}
			if !strings.Contains(output, "agentCode: qoderwork") {
				t.Fatalf("summary output missing qoderwork agentCode:\n%s", output)
			}
		})
	}
}

func TestChmod_batchPlanEntryPointsDryRunOnlyReturnPlanAgentCode(t *testing.T) {
	cases := []struct {
		name             string
		setFlags         func(*cobra.Command)
		wantPlanProducts []string
		wantRecommend    bool
	}{
		{
			name: "product",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("products", "calendar,aitable")
			},
			wantPlanProducts: []string{"calendar", "aitable"},
		},
		{
			name: "domain",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("domains", "calendar,chat")
			},
			wantPlanProducts: []string{"calendar", "chat"},
		},
		{
			name: "recommend",
			setFlags: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("recommend", "true")
			},
			wantRecommend: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agentCodeEnv, "qoderwork")
			fake := &sequenceToolCaller{
				dryRun: true,
				responses: []string{
					`{"success":true,"code":"OK","data":{"agentCode":"qoderwork","allGranted":false,"selectedScopes":["calendar.event:list"],"skippedScopes":[],"pendingScopes":[]}}`,
				},
			}
			cmd := newChmodCommand(fake)
			_ = cmd.Flags().Set("grant-type", "once")
			tc.setFlags(cmd)
			attachRootPATFlags(t, cmd, false, false)

			output, err := captureStdout(t, func() error {
				return cmd.RunE(cmd, nil)
			})
			if err != nil {
				t.Fatalf("chmod RunE error = %v", err)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("CallTool count = %d, want dry-run plan only", len(fake.calls))
			}
			if fake.calls[0].tool != patBatchPlanToolName {
				t.Fatalf("tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
			}
			if got := fake.calls[0].args["productCodes"]; !stringSliceArgEqual(got, tc.wantPlanProducts) {
				t.Fatalf("plan productCodes = %#v, want %#v", got, tc.wantPlanProducts)
			}
			if got := fake.calls[0].args["recommend"]; got != tc.wantRecommend {
				t.Fatalf("plan recommend = %#v, want %v", got, tc.wantRecommend)
			}
			if !strings.Contains(output, "agentCode: qoderwork") || !strings.Contains(output, "selected: 1") {
				t.Fatalf("dry-run summary missing plan agentCode/selection:\n%s", output)
			}
		})
	}
}

func TestChmod_grantTypeAndSessionParameterMatrix(t *testing.T) {
	cases := []struct {
		name          string
		grantType     string
		sessionFlag   string
		sessionEnv    string
		wantSessionID string
		wantErr       string
	}{
		{name: "once no session", grantType: "once"},
		{name: "permanent no session", grantType: "permanent"},
		{name: "session from flag", grantType: "session", sessionFlag: "flag-session", wantSessionID: "flag-session"},
		{name: "session from env", grantType: "session", sessionEnv: "env-session", wantSessionID: "env-session"},
		{name: "session missing rejected", grantType: "session", wantErr: "--session-id is required"},
		{name: "invalid grant type rejected", grantType: "invalid", wantErr: "invalid --grant-type"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agentCodeEnv, "qoderwork")
			if tc.sessionEnv != "" {
				t.Setenv(sessionIDEnvDWS, tc.sessionEnv)
			}
			fake := &sequenceToolCaller{responses: []string{
				`{"success":true,"code":"OK","data":{"agentCode":"qoderwork","grantedScopes":["aitable.record:read"]}}`,
			}}
			cmd := newChmodCommand(fake)
			_ = cmd.Flags().Set("grant-type", tc.grantType)
			if tc.sessionFlag != "" {
				_ = cmd.Flags().Set("session-id", tc.sessionFlag)
			}

			err := cmd.RunE(cmd, []string{"aitable.record:read"})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("chmod RunE error = %v, want containing %q", err, tc.wantErr)
				}
				if len(fake.calls) != 0 {
					t.Fatalf("CallTool count = %d, want validator to block before MCP", len(fake.calls))
				}
				return
			}
			if err != nil {
				t.Fatalf("chmod RunE error = %v", err)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("CallTool count = %d, want 1", len(fake.calls))
			}
			if got := fake.calls[0].args["grantType"]; got != tc.grantType {
				t.Fatalf("grantType arg = %#v, want %s", got, tc.grantType)
			}
			if tc.wantSessionID == "" {
				if _, ok := fake.calls[0].args["sessionId"]; ok {
					t.Fatalf("unexpected sessionId arg: %#v", fake.calls[0].args)
				}
				return
			}
			if got := fake.calls[0].args["sessionId"]; got != tc.wantSessionID {
				t.Fatalf("sessionId arg = %#v, want %s", got, tc.wantSessionID)
			}
			if got := fake.calls[0].dingSessionEnv; got != tc.wantSessionID {
				t.Fatalf("%s during CallTool = %q, want %s", sessionIDEnvDingtalk, got, tc.wantSessionID)
			}
		})
	}
}

func TestChmod_productsDryRunUsesSessionIDFromEnv(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	t.Setenv(sessionIDEnvDWS, "env-session-123")
	fake := &sequenceToolCaller{
		dryRun: true,
		responses: []string{
			`{"success":true,"data":{"selectedScopes":["calendar.event:read"]}}`,
		},
	}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("products", "calendar")

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}

	if len(fake.calls) != 1 {
		t.Fatalf("CallTool count = %d, want 1", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("plan tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
	}
	if got := fake.calls[0].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("plan agentCode = %#v, want qoderwork", got)
	}
	if got := fake.calls[0].args["sessionId"]; got != "env-session-123" {
		t.Fatalf("plan sessionId = %#v, want env-session-123", got)
	}
	if got := fake.calls[0].agentEnv; got != "qoderwork" {
		t.Fatalf("plan agent env = %q, want qoderwork", got)
	}
	if fake.calls[0].dingSessionEnv != "env-session-123" {
		t.Fatalf("plan %s env = %q, want env-session-123", sessionIDEnvDingtalk, fake.calls[0].dingSessionEnv)
	}
}

func TestChmod_batchPlanRetriesWithoutIdentityArgsForCompat(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{
		errs: []error{
			apperrors.NewAPI("PAT batch identity field 'agentCode' must be derived by gateway.",
				apperrors.WithReason("business_error"),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					ServerErrorCode: patForgedIdentityCode,
				}),
			),
			nil,
		},
		responses: []string{
			"",
			`{"success":true,"data":{"agentCode":"qoderwork","allGranted":true,"selectedScopes":[]}}`,
		},
	}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")
	_ = cmd.Flags().Set("products", "calendar")
	setBatchYesForTest(t, cmd)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName || fake.calls[1].tool != patBatchPlanToolName {
		t.Fatalf("tools = %q, %q; want repeated %q", fake.calls[0].tool, fake.calls[1].tool, patBatchPlanToolName)
	}
	if got := fake.calls[0].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("first plan agentCode = %#v, want qoderwork", got)
	}
	if _, ok := fake.calls[1].args["agentCode"]; ok {
		t.Fatalf("compat retry must omit agentCode arg: %#v", fake.calls[1].args)
	}
	if got := fake.calls[1].agentEnv; got != "qoderwork" {
		t.Fatalf("compat retry %s = %q, want qoderwork", agentCodeEnv, got)
	}
}

func TestChmod_batchGrantRetriesWithoutIdentityArgsForCompat(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{
		errs: []error{
			apperrors.NewAPI("PAT batch identity field 'agentCode' must be derived by gateway.",
				apperrors.WithReason("business_error"),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					ServerErrorCode: patForgedIdentityCode,
				}),
			),
			nil,
		},
		responses: []string{
			"",
			`{"success":true,"data":{"agentCode":"qoderwork","grantedScopes":["calendar.event:read"]}}`,
		},
	}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")

	if err := cmd.RunE(cmd, []string{"calendar.event:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchGrantToolName || fake.calls[1].tool != patBatchGrantToolName {
		t.Fatalf("tools = %q, %q; want repeated %q", fake.calls[0].tool, fake.calls[1].tool, patBatchGrantToolName)
	}
	if got := fake.calls[0].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("first grant agentCode = %#v, want qoderwork", got)
	}
	if _, ok := fake.calls[1].args["agentCode"]; ok {
		t.Fatalf("compat retry must omit agentCode arg: %#v", fake.calls[1].args)
	}
	if got := fake.calls[1].agentEnv; got != "qoderwork" {
		t.Fatalf("compat retry %s = %q, want qoderwork", agentCodeEnv, got)
	}
}

func TestChmod_batchGrantIdentityFallbackRejectsMismatchedAgentCode(t *testing.T) {
	t.Setenv(agentCodeEnv, "dinglqdkz3mmw2xwvend")
	fake := &sequenceToolCaller{
		errs: []error{
			apperrors.NewAPI("PAT batch identity field 'agentCode' must be derived by gateway.",
				apperrors.WithReason("business_error"),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					ServerErrorCode: patForgedIdentityCode,
				}),
			),
			nil,
		},
		responses: []string{
			"",
			`{"success":true,"result":{"agentCode":"dingmbw5n9ktkkbbjv3g","grantedScopes":[],"alreadyGrantedScopes":["chat.message:send"]}}`,
		},
	}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "permanent")

	err := cmd.RunE(cmd, []string{"chat.message:send"})
	if err == nil {
		t.Fatal("chmod RunE error = nil, want identity fallback agentCode mismatch")
	}
	if !strings.Contains(err.Error(), "identity fallback returned agentCode") ||
		!strings.Contains(err.Error(), "dingmbw5n9ktkkbbjv3g") ||
		!strings.Contains(err.Error(), "dinglqdkz3mmw2xwvend") {
		t.Fatalf("error = %q, want fallback mismatch details", err.Error())
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want 2", len(fake.calls))
	}
	if _, ok := fake.calls[1].args["agentCode"]; ok {
		t.Fatalf("compat retry should still omit agentCode arg, got %#v", fake.calls[1].args)
	}
	if got := fake.calls[1].agentEnv; got != "dinglqdkz3mmw2xwvend" {
		t.Fatalf("compat retry %s = %q, want requested agentCode", agentCodeEnv, got)
	}
}

func TestChmod_batchGrantIdentityFallbackRejectsMissingAgentCode(t *testing.T) {
	t.Setenv(agentCodeEnv, "dinglqdkz3mmw2xwvend")
	fake := &sequenceToolCaller{
		errs: []error{
			apperrors.NewAPI("PAT batch identity field 'agentCode' must be derived by gateway.",
				apperrors.WithReason("business_error"),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					ServerErrorCode: patForgedIdentityCode,
				}),
			),
			nil,
		},
		responses: []string{
			"",
			`{"success":true,"data":{"grantedScopes":["chat.message:send"]}}`,
		},
	}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "permanent")

	err := cmd.RunE(cmd, []string{"chat.message:send"})
	if err == nil {
		t.Fatal("chmod RunE error = nil, want unverifiable fallback error")
	}
	if !strings.Contains(err.Error(), "authorization target cannot be verified") {
		t.Fatalf("error = %q, want unverifiable fallback details", err.Error())
	}
}

func TestResolveSessionIDFromEnvMatchesHeaderPriority(t *testing.T) {
	t.Setenv(sessionIDEnvDingtalk, "ding-session")
	t.Setenv(sessionIDEnvDWS, "dws-session")
	t.Setenv(sessionIDEnvRewind, "rewind-session")

	if got := resolveSessionIDFromEnv(); got != "ding-session" {
		t.Fatalf("resolveSessionIDFromEnv() = %q, want DINGTALK_SESSION_ID", got)
	}

	t.Setenv(sessionIDEnvDingtalk, "")
	if got := resolveSessionIDFromEnv(); got != "dws-session" {
		t.Fatalf("resolveSessionIDFromEnv() = %q, want DWS_SESSION_ID", got)
	}

	t.Setenv(sessionIDEnvDWS, "")
	if got := resolveSessionIDFromEnv(); got != "rewind-session" {
		t.Fatalf("resolveSessionIDFromEnv() = %q, want REWIND_SESSION_ID", got)
	}
}

func TestChmod_sessionModeUsesDingtalkSessionEnv(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	t.Setenv(sessionIDEnvDingtalk, "ding-session-123")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)

	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}

	if got := fake.gotArgs["agentCode"]; got != "qoderwork" {
		t.Fatalf("agentCode arg = %#v, want qoderwork", got)
	}
	if got := fake.gotArgs["sessionId"]; got != "ding-session-123" {
		t.Fatalf("sessionId arg = %#v, want ding-session-123", got)
	}
	if fake.gotDingSessionEnv != "ding-session-123" {
		t.Fatalf("%s during CallTool = %q, want ding-session-123", sessionIDEnvDingtalk, fake.gotDingSessionEnv)
	}
	if fake.gotSessionEnv != "ding-session-123" {
		t.Fatalf("%s during CallTool = %q, want ding-session-123", sessionIDEnvDWS, fake.gotSessionEnv)
	}
}

func TestChmod_explicitSessionIDOverridesStaleDingtalkSessionEnv(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	t.Setenv(sessionIDEnvDingtalk, "stale-session")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)
	_ = cmd.Flags().Set("session-id", "flag-session")

	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}

	if got := fake.gotArgs["agentCode"]; got != "qoderwork" {
		t.Fatalf("agentCode arg = %#v, want qoderwork", got)
	}
	if got := fake.gotArgs["sessionId"]; got != "flag-session" {
		t.Fatalf("sessionId arg = %#v, want flag-session", got)
	}
	if fake.gotDingSessionEnv != "flag-session" {
		t.Fatalf("%s during CallTool = %q, want flag-session", sessionIDEnvDingtalk, fake.gotDingSessionEnv)
	}
	if fake.gotSessionEnv != "flag-session" {
		t.Fatalf("%s during CallTool = %q, want flag-session", sessionIDEnvDWS, fake.gotSessionEnv)
	}
}

func TestChmod_recommendFlagPlansThenGrantsWithoutPositionalScopes(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"selectedScopes":["recommended.scope:read"]}}`,
		`{"success":true,"data":{"grantedScopes":["recommended.scope:read"]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")
	_ = cmd.Flags().Set("recommend", "true")
	setBatchYesForTest(t, cmd)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}

	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
	}
	if got := fake.calls[0].args["recommend"]; got != true {
		t.Fatalf("recommend = %#v, want true", got)
	}
	if fake.calls[1].tool != patBatchGrantToolName {
		t.Fatalf("second tool = %q, want %q", fake.calls[1].tool, patBatchGrantToolName)
	}
}

func TestLoginRecommendAuthorizationSelectorReplansBySelectedProducts(t *testing.T) {
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历","displayName":"日程查看","operationSummary":"日历管理、日程创建"},{"scope":"doc.document:read","productCode":"doc","productName":"文档","displayName":"文档读取"}],"selectedScopes":["calendar.event:read","doc.document:read"]}}`,
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历","displayName":"日程查看"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"flowId":"flow-1","userCode":"ABCD-EFGH","uri":"https://example.com/auth"}}`,
	}}
	var selectorProducts []LoginRecommendProduct
	err := RunLoginRecommendAuthorizationWithOptions(context.Background(), fake, io.Discard, LoginRecommendOptions{
		ProductSelector: func(products []LoginRecommendProduct) ([]string, error) {
			selectorProducts = products
			return []string{"calendar"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunLoginRecommendAuthorizationWithOptions error = %v", err)
	}
	if len(selectorProducts) != 2 {
		t.Fatalf("selector products = %d, want 2", len(selectorProducts))
	}
	if selectorProducts[0].ProductCode != "calendar" || selectorProducts[0].ProductName != "日历" {
		t.Fatalf("first selector product = %+v, want calendar/日历", selectorProducts[0])
	}
	if selectorProducts[0].Summary != "日历管理、日程创建" {
		t.Fatalf("first selector summary = %q", selectorProducts[0].Summary)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("CallTool count = %d, want initial plan + selected plan + grant", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
	}
	if got := fake.calls[0].args["productCodes"]; !stringSliceArgEqual(got, nil) {
		t.Fatalf("initial plan productCodes = %#v, want empty", got)
	}
	if got := fake.calls[0].args["recommend"]; got != true {
		t.Fatalf("initial plan recommend = %#v, want true", got)
	}
	if got := fake.calls[1].args["productCodes"]; !stringSliceArgEqual(got, []string{"calendar"}) {
		t.Fatalf("selected plan productCodes = %#v, want calendar", got)
	}
	if got := fake.calls[1].args["recommend"]; got != true {
		t.Fatalf("selected plan recommend = %#v, want true", got)
	}
	if got := fake.calls[1].args["caller"]; got != patCallerAuthLoginRecommend {
		t.Fatalf("selected plan caller = %#v, want %q", got, patCallerAuthLoginRecommend)
	}
	if fake.calls[2].tool != patBatchGrantToolName {
		t.Fatalf("third tool = %q, want %q", fake.calls[2].tool, patBatchGrantToolName)
	}
	if got := fake.calls[2].args["scopes"]; !stringSliceArgEqual(got, []string{"calendar.event:read"}) {
		t.Fatalf("grant scopes = %#v, want selected calendar scope", got)
	}
}

func TestLoginRecommendAuthorizationWithoutSelectorKeepsSinglePlan(t *testing.T) {
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"flowId":"flow-1","userCode":"ABCD-EFGH","uri":"https://example.com/auth"}}`,
	}}
	if err := RunLoginRecommendAuthorizationWithOptions(context.Background(), fake, io.Discard, LoginRecommendOptions{}); err != nil {
		t.Fatalf("RunLoginRecommendAuthorizationWithOptions error = %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want one plan + grant", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
	}
	if got := fake.calls[0].args["productCodes"]; !stringSliceArgEqual(got, nil) {
		t.Fatalf("plan productCodes = %#v, want empty", got)
	}
	if fake.calls[1].tool != patBatchGrantToolName {
		t.Fatalf("second tool = %q, want %q", fake.calls[1].tool, patBatchGrantToolName)
	}
	if got := fake.calls[1].args["startFlow"]; got != true {
		t.Fatalf("startFlow = %#v, want true for unconfirmed recommend login", got)
	}
	if got := fake.calls[1].args["noWait"]; got != true {
		t.Fatalf("noWait = %#v, want true for unconfirmed recommend login", got)
	}
}

func TestLoginRecommendAuthorizationRecommendedAlreadyGrantedSkipsSelectorAndGrant(t *testing.T) {
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"allGranted":false,"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"}],"selectedScopes":[]}}`,
	}}
	var out bytes.Buffer
	err := RunLoginRecommendAuthorizationWithOptions(context.Background(), fake, &out, LoginRecommendOptions{
		ScopeMode: LoginRecommendScopeRecommended,
		ProductSelector: func([]LoginRecommendProduct) ([]string, error) {
			t.Fatal("already-granted recommended plan must not ask for product selection")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("RunLoginRecommendAuthorizationWithOptions error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool count = %d, want only initial recommend plan", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("tool sequence = %q, want only %q", fake.calls[0].tool, patBatchPlanToolName)
	}
	if strings.Contains(out.String(), "flowId") {
		t.Fatalf("output = %q, must not include authorization flow", out.String())
	}
	if !strings.Contains(out.String(), "推荐权限已全部授权或没有可授权项") {
		t.Fatalf("output = %q, want already-granted message", out.String())
	}
}

func TestLoginRecommendAuthorizationAllScopeModePlansAllProductScopes(t *testing.T) {
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"},{"scope":"calendar.event:write","productCode":"calendar","productName":"日历"}],"selectedScopes":["calendar.event:read","calendar.event:write"]}}`,
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"},{"scope":"calendar.event:write","productCode":"calendar","productName":"日历"}],"selectedScopes":["calendar.event:read","calendar.event:write"]}}`,
		`{"success":true,"data":{"flowId":"flow-1","userCode":"ABCD-EFGH","uri":"https://example.com/auth"}}`,
	}}
	err := RunLoginRecommendAuthorizationWithOptions(context.Background(), fake, io.Discard, LoginRecommendOptions{
		ScopeMode: LoginRecommendScopeAll,
		ProductSelector: func(products []LoginRecommendProduct) ([]string, error) {
			return []string{"calendar"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunLoginRecommendAuthorizationWithOptions error = %v", err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("CallTool count = %d, want initial plan + selected plan + grant", len(fake.calls))
	}
	if got := fake.calls[0].args["recommend"]; got != true {
		t.Fatalf("initial discovery plan recommend = %#v, want true before product selection", got)
	}
	if got := fake.calls[0].args["productCodes"]; !stringSliceArgEqual(got, nil) {
		t.Fatalf("initial discovery plan productCodes = %#v, want empty", got)
	}
	if got := fake.calls[1].args["recommend"]; got != false {
		t.Fatalf("selected plan recommend = %#v, want false for all scope mode", got)
	}
	if got := fake.calls[1].args["productCodes"]; !stringSliceArgEqual(got, []string{"calendar"}) {
		t.Fatalf("selected plan productCodes = %#v, want calendar", got)
	}
	if got := fake.calls[2].args["scopes"]; !stringSliceArgEqual(got, []string{"calendar.event:read", "calendar.event:write"}) {
		t.Fatalf("grant scopes = %#v, want all selected calendar scopes", got)
	}
}

func TestLoginRecommendAuthorizationAllScopeWithoutProductsFailsBeforePlan(t *testing.T) {
	fake := &sequenceToolCaller{}
	err := RunLoginRecommendAuthorizationWithOptions(context.Background(), fake, io.Discard, LoginRecommendOptions{
		ScopeMode: LoginRecommendScopeAll,
	})
	if err == nil {
		t.Fatal("RunLoginRecommendAuthorizationWithOptions error = nil, want product-domain validation error")
	}
	if !strings.Contains(err.Error(), "至少一个授权业务域") {
		t.Fatalf("error = %v, want product-domain validation error", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("CallTool count = %d, want no empty all-scope plan call", len(fake.calls))
	}
}

func TestLoginRecommendAuthorizationConfirmedGrantsDirectly(t *testing.T) {
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"grantedScopes":["calendar.event:read"]}}`,
	}}
	if err := RunLoginRecommendAuthorizationWithOptions(context.Background(), fake, io.Discard, LoginRecommendOptions{Confirmed: true}); err != nil {
		t.Fatalf("RunLoginRecommendAuthorizationWithOptions error = %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool count = %d, want one plan + grant", len(fake.calls))
	}
	if fake.calls[1].tool != patBatchGrantToolName {
		t.Fatalf("second tool = %q, want %q", fake.calls[1].tool, patBatchGrantToolName)
	}
	if got := fake.calls[0].args["recommend"]; got != true {
		t.Fatalf("plan recommend = %#v, want true for confirmed recommend login", got)
	}
	if _, ok := fake.calls[1].args["startFlow"]; ok {
		t.Fatalf("startFlow should be omitted for confirmed recommend login")
	}
	if _, ok := fake.calls[1].args["noWait"]; ok {
		t.Fatalf("noWait should be omitted for confirmed recommend login")
	}
	if got := fake.calls[1].args["caller"]; got != patCallerAuthLoginRecommend {
		t.Fatalf("caller = %#v, want %q", got, patCallerAuthLoginRecommend)
	}
}

func TestChmod_productsAllGrantedStopsAfterPlan(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"data":{"allGranted":true,"selectedScopes":[]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")
	_ = cmd.Flags().Set("products", "calendar")
	setBatchYesForTest(t, cmd)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool count = %d, want only plan call", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchPlanToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchPlanToolName)
	}
}

func TestChmod_explicitScopesDryRunShowsBatchGrantTool(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")
	fake := &fakeToolCaller{dryRun: true}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "once")

	output, err := captureStdout(t, func() error {
		return cmd.RunE(cmd, []string{"aitable.record:read"})
	})
	if err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if !strings.Contains(output, patBatchGrantToolName) {
		t.Fatalf("dry-run output = %q, want %q", output, patBatchGrantToolName)
	}
	if strings.Contains(output, patGrantToolName+"\n") {
		t.Fatalf("dry-run output = %q, must not advertise legacy %q", output, patGrantToolName)
	}
}

// ---------------------------------------------------------------------------
// T1 · Agent-code env fallback tests
// ---------------------------------------------------------------------------

// TestChmod_agentCode_env_fallback verifies that when --agentCode is
// omitted but DINGTALK_DWS_AGENTCODE is exported, the resolver picks
// the env value up for both batch arguments and gateway-compatible env.
func TestChmod_agentCode_env_fallback(t *testing.T) {
	t.Setenv(agentCodeEnv, "qoderwork")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)

	// grant-type=once → no session-id needed; keeps the test hermetic.
	_ = cmd.Flags().Set("grant-type", "once")
	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v (must not report flag missing)", err)
	}

	if fake.gotTool != patBatchGrantToolName {
		t.Fatalf("gotTool = %q, want %q", fake.gotTool, patBatchGrantToolName)
	}
	if got := fake.gotAgentEnv; got != "qoderwork" {
		t.Fatalf("agent env = %q, want %q", got, "qoderwork")
	}
	if got := fake.gotArgs["agentCode"]; got != "qoderwork" {
		t.Fatalf("batch agentCode = %#v, want qoderwork", got)
	}
	if got := fake.gotArgs["scopes"]; !stringSliceArgEqual(got, []string{"aitable.record:read"}) {
		t.Fatalf("scopes in argv = %#v, want %#v", got, []string{"aitable.record:read"})
	}
	if _, ok := fake.gotArgs["scope"]; ok {
		t.Fatalf("unexpected legacy singular scope arg in argv: %#v", fake.gotArgs)
	}
}

func TestChmod_agentCode_reversedEnvIgnored(t *testing.T) {
	t.Setenv(agentCodeEnv, "")
	t.Setenv("DWS_DINGTALK_AGENTCODE", "compatwork")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)
	_ = cmd.Flags().Set("grant-type", "once")

	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if fake.gotTool != patBatchGrantToolName {
		t.Fatalf("gotTool = %q, want %q", fake.gotTool, patBatchGrantToolName)
	}
	if _, ok := fake.gotArgs["agentCode"]; ok {
		t.Fatalf("agentCode arg must be omitted; reversed env name must not be consumed: %#v", fake.gotArgs)
	}
	if got := fake.gotAgentEnv; got != "" {
		t.Fatalf("%s during CallTool = %q, want empty because reversed env is ignored", agentCodeEnv, got)
	}
}

func TestChmod_withoutAgentCodeLetsServerDefault(t *testing.T) {
	t.Setenv(agentCodeEnv, "")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)
	_ = cmd.Flags().Set("grant-type", "once")

	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v, want server-side default agentCode path", err)
	}
	if fake.callN != 1 {
		t.Fatalf("CallTool was invoked %d times; missing agentCode must still reach the batch caller", fake.callN)
	}
	if fake.gotTool != patBatchGrantToolName {
		t.Fatalf("gotTool = %q, want %q", fake.gotTool, patBatchGrantToolName)
	}
	if _, ok := fake.gotArgs["agentCode"]; ok {
		t.Fatalf("agentCode arg must be omitted for server default path: %#v", fake.gotArgs)
	}
	if got := fake.gotAgentEnv; got != "" {
		t.Fatalf("%s during CallTool = %q, want empty for server default path", agentCodeEnv, got)
	}
}

func TestChmod_agentCode_envServerMismatchFails(t *testing.T) {
	t.Setenv(agentCodeEnv, "dinglqdkz3mmw2xwvend")

	fake := &sequenceToolCaller{responses: []string{
		`{"success":true,"code":"OK","data":{"agentCode":"dingmbw5n9ktkkkbjv3g","grantType":"permanent","grantedScopes":[],"alreadyGrantedScopes":["chat.message:send"]}}`,
	}}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("grant-type", "permanent")

	err := cmd.RunE(cmd, []string{"chat.message:send"})
	if err == nil {
		t.Fatal("chmod RunE error = nil, want agentCode mismatch error")
	}
	if !strings.Contains(err.Error(), "dingmbw5n9ktkkkbjv3g") ||
		!strings.Contains(err.Error(), "dinglqdkz3mmw2xwvend") {
		t.Fatalf("error = %q, want both returned and expected agentCode", err.Error())
	}
	if len(fake.calls) != 1 {
		t.Fatalf("CallTool count = %d, want 1", len(fake.calls))
	}
	if got := fake.calls[0].args["agentCode"]; got != "dinglqdkz3mmw2xwvend" {
		t.Fatalf("batch grant agentCode = %#v, want DINGTALK_DWS_AGENTCODE", got)
	}
	if got := fake.calls[0].agentEnv; got != "dinglqdkz3mmw2xwvend" {
		t.Fatalf("%s during CallTool = %q, want DINGTALK_DWS_AGENTCODE", agentCodeEnv, got)
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
	if fake.calls[0].tool != patBatchGrantToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchGrantToolName)
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

func TestChmod_batchMetadataScopeErrorFallsBackToPATGrant(t *testing.T) {
	fake := &sequenceToolCaller{
		responses: []string{
			`{"success":false,"errorCode":"PAT_BATCH_SCOPE_NOT_DECLARED","data":{"scopes":["mail:send"]}}`,
			`{"success":true,"data":{"authRequestId":"req-ok"}}`,
		},
	}
	cmd := newChmodCommand(fake)
	_ = cmd.Flags().Set("agentCode", "qoderwork")
	_ = cmd.Flags().Set("grant-type", "once")

	if err := cmd.RunE(cmd, []string{"mail:send"}); err != nil {
		t.Fatalf("chmod RunE error = %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("CallTool call count = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].tool != patBatchGrantToolName {
		t.Fatalf("first tool = %q, want %q", fake.calls[0].tool, patBatchGrantToolName)
	}
	if fake.calls[1].tool != patGrantToolName {
		t.Fatalf("fallback tool = %q, want %q", fake.calls[1].tool, patGrantToolName)
	}
	if got := fake.calls[0].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("batch agentCode = %#v, want qoderwork", got)
	}
	if got := fake.calls[1].args["agentCode"]; got != "qoderwork" {
		t.Fatalf("fallback agentCode = %#v, want qoderwork", got)
	}
	if got := fake.calls[1].args["scopes"]; !stringSliceArgEqual(got, []string{"mail:send"}) {
		t.Fatalf("fallback scopes = %#v, want mail:send", got)
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

func TestIsPATBatchUnsupportedResultCaseInsensitive(t *testing.T) {
	result := &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":false,"errorCode":"pat_batch_auth_unsupported"}`}}}
	if !isPATBatchUnsupportedResult(result) {
		t.Fatal("isPATBatchUnsupportedResult() = false, want true")
	}
}

func TestIsPATBatchFallbackResultIncludesMetadataContractErrors(t *testing.T) {
	result := &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":false,"errorCode":"PAT_BATCH_SCOPE_NOT_DECLARED"}`}}}
	if !isPATBatchFallbackResult(result) {
		t.Fatal("isPATBatchFallbackResult() = false, want true")
	}
}

func TestIsPATBatchUnsupportedErrorUsesNormalizedDiagnostics(t *testing.T) {
	err := apperrors.NewAPI("business error: success=false",
		apperrors.WithReason("business_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			ServerErrorCode: "PAT_BATCH_AUTH_UNSUPPORTED",
		}),
	)
	if !isPATBatchUnsupportedError(err) {
		t.Fatal("isPATBatchUnsupportedError() = false, want true")
	}
}

func TestIsPATBatchFallbackErrorIncludesMetadataContractDiagnostics(t *testing.T) {
	err := apperrors.NewAPI("business error: success=false",
		apperrors.WithReason("business_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			ServerErrorCode: "PAT_BATCH_PRODUCT_NOT_DECLARED",
		}),
	)
	if !isPATBatchFallbackError(err) {
		t.Fatal("isPATBatchFallbackError() = false, want true")
	}
}

func TestHandleToolResult_emptyResultReturnsError(t *testing.T) {
	err := handleToolResult(nil, nil, &edition.ToolResult{})
	if err == nil {
		t.Fatal("handleToolResult error = nil, want empty PAT authorization result error")
	}
	if !strings.Contains(err.Error(), "empty PAT authorization result") {
		t.Fatalf("handleToolResult error = %q, want empty PAT authorization result", err.Error())
	}
}

func TestHandleToolResult_defaultSummarizesBatchPlan(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "json", "")
	cmd := &cobra.Command{Use: "chmod"}
	root.AddCommand(cmd)
	result := &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{
		"success": true,
		"code": "OK",
		"data": {
			"agentCode": "ding-agent",
			"allGranted": false,
			"items": [
				{"scope": "calendar.event:list"},
				{"scope": "calendar.event:create"}
			],
			"selectedScopes": ["calendar.event:create"],
			"skippedScopes": ["calendar.event:list"],
			"pendingScopes": []
		}
	}`}}}

	output, err := captureStdout(t, func() error {
		return handleToolResult(cmd, &sequenceToolCaller{dryRun: true}, result)
	})
	if err != nil {
		t.Fatalf("handleToolResult error = %v", err)
	}
	for _, want := range []string{
		"PAT authorization",
		"status: OK",
		"agentCode: ding-agent",
		"allGranted: false",
		"items: 2",
		"selected: 1",
		"skipped: 1",
		"pending: 0",
		"suggestion: rerun this command without --dry-run to grant selected scopes",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("summary output missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "calendar.event:create") || strings.Contains(output, `"items"`) {
		t.Fatalf("summary output leaked raw item details: %s", output)
	}
}

func TestHandleToolResult_explicitJSONKeepsRawPayload(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "json", "")
	cmd := &cobra.Command{Use: "chmod"}
	root.AddCommand(cmd)
	if err := root.PersistentFlags().Set("format", "json"); err != nil {
		t.Fatalf("set format error = %v", err)
	}
	text := `{"success":true,"code":"OK","data":{"items":[{"scope":"calendar.event:create"}],"selectedScopes":["calendar.event:create"]}}`
	result := &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}

	output, err := captureStdout(t, func() error {
		return handleToolResult(cmd, &sequenceToolCaller{dryRun: true}, result)
	})
	if err != nil {
		t.Fatalf("handleToolResult error = %v", err)
	}
	if !strings.Contains(output, `"items"`) || !strings.Contains(output, "calendar.event:create") {
		t.Fatalf("explicit json output did not preserve raw payload: %s", output)
	}
	if strings.Contains(output, "PAT authorization") {
		t.Fatalf("explicit json output must not be summarized: %s", output)
	}
}

func TestHandleToolResult_verboseKeepsRawPayload(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("verbose", false, "")
	cmd := &cobra.Command{Use: "chmod"}
	root.AddCommand(cmd)
	if err := root.PersistentFlags().Set("verbose", "true"); err != nil {
		t.Fatalf("set verbose error = %v", err)
	}
	text := `{"success":true,"code":"OK","data":{"items":[{"scope":"calendar.event:create"}]}}`
	result := &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}

	output, err := captureStdout(t, func() error {
		return handleToolResult(cmd, &sequenceToolCaller{}, result)
	})
	if err != nil {
		t.Fatalf("handleToolResult error = %v", err)
	}
	if !strings.Contains(output, `"items"`) || !strings.Contains(output, "calendar.event:create") {
		t.Fatalf("verbose output did not preserve raw payload: %s", output)
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
	if fake.gotTool != patBatchGrantToolName {
		t.Fatalf("gotTool = %q, want %q", fake.gotTool, patBatchGrantToolName)
	}
	if got := fake.gotAgentEnv; got != "flagval" {
		t.Fatalf("agent env = %q, want %q (flag must win over env)", got, "flagval")
	}
	if got := fake.gotArgs["agentCode"]; got != "flagval" {
		t.Fatalf("batch agentCode = %#v, want flagval", got)
	}
}

// TestChmod_agentCode_legacy_env_not_recognized is a reverse-guard: only
// DINGTALK_DWS_AGENTCODE is consumed as the env fallback. Legacy / draft names
// MUST NOT be consumed as agentCode. The request is still sent so PAT-core can
// apply its open-source default.
func TestChmod_agentCode_legacy_env_not_recognized(t *testing.T) {
	t.Setenv(agentCodeEnv, "")
	t.Setenv(agentCodeEnvCompat, "")
	t.Setenv("DWS_AGENTCODE", "legacyval")
	t.Setenv("DWS_DINGTALK_AGENTCODE", "draftval")

	fake := &fakeToolCaller{resultOK: true}
	cmd := buildChmod(t, fake)
	_ = cmd.Flags().Set("grant-type", "once")

	if err := cmd.RunE(cmd, []string{"aitable.record:read"}); err != nil {
		t.Fatalf("chmod RunE error = %v, want server-side default agentCode path", err)
	}
	if fake.callN != 1 {
		t.Fatalf("CallTool was invoked %d times; legacy env should be ignored but request should continue", fake.callN)
	}
	if _, ok := fake.gotArgs["agentCode"]; ok {
		t.Fatalf("agentCode arg must be omitted; legacy DWS_AGENTCODE must not be consumed: %#v", fake.gotArgs)
	}
	if got := fake.gotAgentEnv; got != "" {
		t.Fatalf("agent env = %q, want empty; legacy DWS_AGENTCODE must not be consumed", got)
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

	// Reverse-guard: the draft reversed spelling is intentionally ignored.
	t.Setenv(agentCodeEnv, "")
	t.Setenv("DWS_DINGTALK_AGENTCODE", "compatwork")
	if code, src := resolveAgentCodeFromEnv(); code != "" || src != "" {
		t.Errorf("resolveAgentCodeFromEnv() = (%q, %q), want empty — DWS_DINGTALK_AGENTCODE must be ignored",
			code, src)
	}

	// Empty primary → ("", "").
	t.Setenv(agentCodeEnv, "")
	t.Setenv("DWS_DINGTALK_AGENTCODE", "")
	if code, src := resolveAgentCodeFromEnv(); code != "" || src != "" {
		t.Errorf("resolveAgentCodeFromEnv() = (%q, %q), want empty", code, src)
	}

	// Reverse-guard: legacy DWS_AGENTCODE MUST NOT be picked up when the
	// canonical env is unset — it was hard-removed as a legacy alias.
	t.Setenv(agentCodeEnv, "")
	t.Setenv(agentCodeEnvCompat, "")
	t.Setenv("DWS_AGENTCODE", "legacy")
	if code, src := resolveAgentCodeFromEnv(); code != "" || src != "" {
		t.Errorf("resolveAgentCodeFromEnv() = (%q, %q), want empty — legacy DWS_AGENTCODE must be ignored",
			code, src)
	}
}
