// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	patcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type patShortcutCaller struct{ calls int }

func (caller *patShortcutCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	caller.calls++
	return nil, nil
}
func (*patShortcutCaller) Format() string { return "json" }
func (*patShortcutCaller) DryRun() bool   { return false }
func (*patShortcutCaller) Fields() string { return "" }
func (*patShortcutCaller) JQ() string     { return "" }

type patAtomicCall struct {
	tool string
	args map[string]any
}

type patAtomicFixtureCaller struct {
	dryRun    bool
	responses []string
	calls     []patAtomicCall
}

func (caller *patAtomicFixtureCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	caller.calls = append(caller.calls, patAtomicCall{tool: tool, args: copied})
	response := `{"success":true,"code":"OK","data":{}}`
	if index := len(caller.calls) - 1; index < len(caller.responses) {
		response = caller.responses[index]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}
func (*patAtomicFixtureCaller) Format() string      { return "json" }
func (caller *patAtomicFixtureCaller) DryRun() bool { return caller.dryRun }
func (*patAtomicFixtureCaller) Fields() string      { return "" }
func (*patAtomicFixtureCaller) JQ() string          { return "" }

func runPATShortcut(t *testing.T, declaration shortcut.Shortcut, caller *patShortcutCaller, input string, args ...string) (string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	service := &cobra.Command{Use: "pat"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(append([]string{"pat", declaration.Command}, args...))
	executed, err := root.ExecuteC()
	if err == nil {
		_, _, err = output.EmitStoredResult(executed)
	}
	return stdout.String(), err
}

func runPATAtomicRoute(t *testing.T, caller *patAtomicFixtureCaller, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("verbose", false, "")
	patcore.RegisterCommands(root, caller)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(append([]string{"pat", "chmod"}, args...))

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writePipe
	_, runErr := root.ExecuteC()
	_ = writePipe.Close()
	os.Stdout = oldStdout
	outputBytes, readErr := io.ReadAll(readPipe)
	_ = readPipe.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(outputBytes), runErr
}

func runPATNativeBrowserPolicy(t *testing.T, caller *patAtomicFixtureCaller, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("verbose", false, "")
	patcore.RegisterCommands(root, caller)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(append([]string{"pat", "browser-policy"}, args...))
	_, err := root.ExecuteC()
	return stdout.String(), err
}

func TestCrossPlatformCoveragePATAuthorizeRouteUsesAtomicPlanFallbackPendingAndLedger(t *testing.T) {
	plan := `{"success":true,"code":"OK","data":{"items":[{"scope":"synthetic.item:read","disposition":"selected"}],"selectedScopes":["synthetic.item:read"],"skippedScopes":[],"pendingScopes":[]}}`
	pending := `{"success":true,"code":"OK","data":{"items":[{"scope":"synthetic.item:read","status":"pending"}],"selectedScopes":["synthetic.item:read"],"grantedScopes":[],"skippedScopes":[],"pendingScopes":["synthetic.item:read"]}}`

	preconfirm := &patAtomicFixtureCaller{responses: []string{plan, pending}}
	if _, err := runPATAtomicRoute(t, preconfirm, "--product", "synthetic", "--grant-type", "once", "--format", "json"); err == nil {
		t.Fatal("unconfirmed atomic authorization unexpectedly succeeded")
	}
	if len(preconfirm.calls) != 0 {
		t.Fatalf("atomic calls before confirmation = %d, want 0", len(preconfirm.calls))
	}

	dryRun := &patAtomicFixtureCaller{dryRun: true, responses: []string{plan}}
	dryOutput, err := runPATAtomicRoute(t, dryRun, "--product", "synthetic", "--grant-type", "once", "--dry-run", "--format", "json")
	if err != nil {
		t.Fatalf("atomic plan dry-run: %v", err)
	}
	if len(dryRun.calls) != 1 || dryRun.calls[0].tool != "pat.batch_plan" || dryRun.calls[0].args["dryRun"] != true {
		t.Fatalf("dry-run route = %#v", dryRun.calls)
	}
	if !strings.Contains(dryOutput, `"items"`) || !strings.Contains(dryOutput, `"selectedScopes"`) || !strings.Contains(dryOutput, `"pendingScopes"`) {
		t.Fatal("dry-run did not preserve the synthetic per-scope plan ledger")
	}

	confirmed := &patAtomicFixtureCaller{responses: []string{plan, pending}}
	confirmedOutput, err := runPATAtomicRoute(t, confirmed, "--product", "synthetic", "--grant-type", "once", "--yes", "--format", "json")
	if err != nil {
		t.Fatalf("confirmed atomic route: %v", err)
	}
	if len(confirmed.calls) != 2 || confirmed.calls[0].tool != "pat.batch_plan" || confirmed.calls[1].tool != "pat.batch_grant" {
		t.Fatalf("confirmed route = %#v", confirmed.calls)
	}
	if scopes, ok := confirmed.calls[1].args["scopes"].([]string); !ok || len(scopes) != 1 || scopes[0] != "synthetic.item:read" {
		t.Fatalf("grant scopes = %#v", confirmed.calls[1].args["scopes"])
	}
	for _, key := range []string{"agentCode", "sessionId", "orgId", "uid"} {
		if _, present := confirmed.calls[1].args[key]; present {
			t.Fatalf("synthetic fixture unexpectedly carried identity key %s", key)
		}
	}
	if !strings.Contains(confirmedOutput, `"items"`) || !strings.Contains(confirmedOutput, `"pendingScopes"`) || !strings.Contains(confirmedOutput, `"status":"pending"`) {
		t.Fatal("confirmed route did not preserve the synthetic pending per-scope ledger")
	}

	fallback := &patAtomicFixtureCaller{responses: []string{
		`{"success":false,"errorCode":"PAT_BATCH_SCOPE_NOT_DECLARED","data":{}}`,
		`{"success":true,"code":"OK","data":{"items":[{"scope":"synthetic.item:read","status":"granted"}],"grantedScopes":["synthetic.item:read"],"pendingScopes":[]}}`,
	}}
	if _, err := runPATAtomicRoute(t, fallback, "synthetic.item:read", "--grant-type", "once", "--yes", "--format", "json"); err != nil {
		t.Fatalf("atomic fallback route: %v", err)
	}
	if len(fallback.calls) != 2 || fallback.calls[0].tool != "pat.batch_grant" || fallback.calls[1].tool != "pat.grant" {
		t.Fatalf("fallback route = %#v", fallback.calls)
	}
}

func TestCrossPlatformCoveragePATBrowserPolicyConfirmationDryRunReadbackAndCleanup(t *testing.T) {
	proofRoot := t.TempDir()
	exactDir := filepath.Join(proofRoot, "exact")
	if err := os.Mkdir(exactDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_CONFIG_DIR", exactDir)
	policyPath := filepath.Join(exactDir, "pat_policy.json")
	caller := &patShortcutCaller{}

	if _, err := runPATShortcut(t, BrowserPolicy, caller, "", "--enabled=true"); err == nil {
		t.Fatal("unconfirmed write unexpectedly succeeded")
	}
	if caller.calls != 0 {
		t.Fatalf("remote calls before confirmation = %d", caller.calls)
	}
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("policy file exists before confirmation: %v", err)
	}

	if _, err := runPATShortcut(t, BrowserPolicy, caller, "", "--enabled=true", "--dry-run"); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run created policy file: %v", err)
	}

	if _, err := runPATShortcut(t, BrowserPolicy, caller, "yes\n", "--enabled=true"); err != nil {
		t.Fatalf("confirmed write: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("local policy Shortcut made %d remote calls", caller.calls)
	}
	stored, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	if bytes.Contains(stored, []byte("agentCode")) || !bytes.Contains(stored, []byte(`"openBrowser": true`)) {
		t.Fatal("default policy did not persist the expected redacted setting")
	}
	if err := os.Remove(policyPath); err != nil {
		t.Fatalf("cleanup policy: %v", err)
	}
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatalf("policy cleanup left residue: %v", err)
	}

	rawDir := filepath.Join(proofRoot, "raw")
	if err := os.Mkdir(rawDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_CONFIG_DIR", rawDir)
	rawPolicyPath := filepath.Join(rawDir, "pat_policy.json")
	rawCaller := &patAtomicFixtureCaller{}
	rawOutput, err := runPATNativeBrowserPolicy(t, rawCaller, "--enabled=false", "--format", "json")
	if err != nil {
		t.Fatalf("raw browser policy write: %v", err)
	}
	if len(rawCaller.calls) != 0 {
		t.Fatalf("raw local policy command made %d remote calls", len(rawCaller.calls))
	}
	var rawReceipt patcore.BrowserPolicySelection
	if err := json.Unmarshal([]byte(rawOutput), &rawReceipt); err != nil {
		t.Fatalf("parse raw browser policy receipt: %v", err)
	}
	if rawReceipt.Scope != "default" || rawReceipt.Source != "default" || rawReceipt.OpenBrowser || rawReceipt.AgentCode != "" {
		t.Fatal("raw browser policy did not return a terminal receipt for the requested target")
	}
	rawStored, err := os.ReadFile(rawPolicyPath)
	if err != nil {
		t.Fatalf("read raw policy: %v", err)
	}
	if bytes.Contains(rawStored, []byte("agentCode")) || !bytes.Contains(rawStored, []byte(`"openBrowser": false`)) {
		t.Fatal("raw default policy did not persist the expected redacted setting")
	}
	if err := os.Remove(rawPolicyPath); err != nil {
		t.Fatalf("cleanup raw policy: %v", err)
	}
	if _, err := os.Stat(rawPolicyPath); !os.IsNotExist(err) {
		t.Fatalf("raw policy cleanup left residue: %v", err)
	}
}

func TestCrossPlatformCoveragePATBrowserPolicyOutputOmitsAgentIdentity(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	caller := &patShortcutCaller{}
	outputText, err := runPATShortcut(t, BrowserPolicy, caller, "", "--enabled=false", "--agent-code", "DWS_TEST_AGENT", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(outputText, "DWS_TEST_AGENT") || strings.Contains(outputText, "agentCode") || strings.Contains(outputText, "agent-code") {
		t.Fatal("public PAT Shortcut output exposed agent identity")
	}
}

func TestCrossPlatformCoveragePATBrowserPolicyWriteReadbackFailures(t *testing.T) {
	failure := errors.New("fixture failure")
	t.Run("write failure", func(t *testing.T) {
		testseam.Swap(t, &patBrowserPolicyConfigDir, func() string { return t.TempDir() })
		testseam.Swap(t, &patSetBrowserPolicy, func(string, string, bool) (patcore.BrowserPolicySelection, error) {
			return patcore.BrowserPolicySelection{}, failure
		})
		if _, err := runPATShortcut(t, BrowserPolicy, &patShortcutCaller{}, "", "--enabled=true", "--yes"); !errors.Is(err, failure) {
			t.Fatalf("write error = %v", err)
		}
	})

	t.Run("read failure", func(t *testing.T) {
		testseam.Swap(t, &patBrowserPolicyConfigDir, func() string { return t.TempDir() })
		testseam.Swap(t, &patSetBrowserPolicy, func(string, string, bool) (patcore.BrowserPolicySelection, error) {
			return patcore.BrowserPolicySelection{Scope: "default", OpenBrowser: true}, nil
		})
		testseam.Swap(t, &patReadBrowserPolicy, func(string, string) (patcore.BrowserPolicySelection, error) {
			return patcore.BrowserPolicySelection{}, failure
		})
		if _, err := runPATShortcut(t, BrowserPolicy, &patShortcutCaller{}, "", "--enabled=true", "--yes"); !errors.Is(err, failure) {
			t.Fatalf("read error = %v", err)
		}
	})

	t.Run("mismatched readback", func(t *testing.T) {
		testseam.Swap(t, &patBrowserPolicyConfigDir, func() string { return t.TempDir() })
		testseam.Swap(t, &patSetBrowserPolicy, func(string, string, bool) (patcore.BrowserPolicySelection, error) {
			return patcore.BrowserPolicySelection{Scope: "default", OpenBrowser: true}, nil
		})
		testseam.Swap(t, &patReadBrowserPolicy, func(string, string) (patcore.BrowserPolicySelection, error) {
			return patcore.BrowserPolicySelection{Scope: "default", OpenBrowser: false}, nil
		})
		if _, err := runPATShortcut(t, BrowserPolicy, &patShortcutCaller{}, "", "--enabled=true", "--yes"); err == nil {
			t.Fatal("mismatched readback succeeded")
		}
	})
}

func TestCrossPlatformCoveragePATInvalidAndUnavailablePathsMakeZeroRemoteCalls(t *testing.T) {
	caller := &patShortcutCaller{}
	if _, err := runPATShortcut(t, BrowserPolicy, caller, "", "--enabled=true", "--agent-code", "invalid value", "--yes"); err == nil {
		t.Fatal("invalid agent code unexpectedly succeeded")
	}
	if _, err := runPATShortcut(t, Authorize, caller, "", "--yes"); err == nil {
		t.Fatal("unavailable authorization Shortcut unexpectedly succeeded")
	}
	if caller.calls != 0 {
		t.Fatalf("invalid/unavailable PAT paths made %d remote calls", caller.calls)
	}
}

func TestCrossPlatformCoveragePATShortcutContracts(t *testing.T) {
	if BrowserPolicy.Hidden || BrowserPolicy.Availability == shortcut.AvailabilityUnavailable {
		t.Fatal("browser policy must be public/available before semantic decoration")
	}
	if BrowserPolicy.OutputRollout != output.RolloutUnifiedActive || BrowserPolicy.Contract.Result == nil || BrowserPolicy.Contract.DryRun == nil || BrowserPolicy.Contract.DryRun.PreviewKind != "request" || strings.TrimSpace(BrowserPolicy.Safety.Effect) == "" {
		t.Fatal("public browser policy lacks Result/Safety/DryRun/unified output")
	}
	if !Authorize.Hidden || Authorize.Availability != shortcut.AvailabilityUnavailable || Authorize.Contract.Interface.Availability != "unavailable" {
		t.Fatal("PAT authorization Shortcut must stay hidden/unavailable")
	}
	if !strings.Contains(Authorize.Contract.Interface.Reason, "classified=routed") || !strings.Contains(Authorize.Contract.Interface.Reason, "blocked_fixture") {
		t.Fatal("PAT authorization must separate the routed implementation from unproved live write evidence")
	}
}
