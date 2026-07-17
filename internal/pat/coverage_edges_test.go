// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package pat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type patEdgeCaller struct {
	results []*edition.ToolResult
	errs    []error
	calls   int
	dryRun  bool
}

func (c *patEdgeCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	index := c.calls
	c.calls++
	var result *edition.ToolResult
	var err error
	if index < len(c.results) {
		result = c.results[index]
	}
	if index < len(c.errs) {
		err = c.errs[index]
	}
	return result, err
}
func (*patEdgeCaller) Format() string { return "json" }
func (c *patEdgeCaller) DryRun() bool { return c.dryRun }
func (*patEdgeCaller) Fields() string { return "" }
func (*patEdgeCaller) JQ() string     { return "" }

type patFailWriter struct{}

func (patFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func patText(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func TestCrossPlatformCoverageBrowserPolicyInjectedEdges(t *testing.T) {
	originalRead := patPolicyReadFile
	originalHome := patPolicyUserHomeDir
	originalMarshal := patPolicyMarshal
	originalWrite := patPolicyAtomicWrite
	originalEdition := edition.Get()
	t.Cleanup(func() {
		patPolicyReadFile = originalRead
		patPolicyUserHomeDir = originalHome
		patPolicyMarshal = originalMarshal
		patPolicyAtomicWrite = originalWrite
		edition.Override(originalEdition)
	})
	failure := errors.New("injected failure")

	t.Setenv("DWS_CONFIG_DIR", "/env/config")
	if got := patConfigDir(); got != "/env/config" {
		t.Fatalf("env config dir = %q", got)
	}
	t.Setenv("DWS_CONFIG_DIR", "")
	edition.Override(&edition.Hooks{ConfigDir: func() string { return "/edition/config" }})
	if got := patConfigDir(); got != "/edition/config" {
		t.Fatalf("edition config dir = %q", got)
	}
	edition.Override(&edition.Hooks{})
	patPolicyUserHomeDir = func() (string, error) { return "", failure }
	if got := patConfigDir(); got != ".dws" {
		t.Fatalf("failed home config dir = %q", got)
	}
	homeDir := filepath.Join(t.TempDir(), "home")
	patPolicyUserHomeDir = func() (string, error) { return homeDir, nil }
	if got := patConfigDir(); got != filepath.Join(homeDir, ".dws") {
		t.Fatalf("home config dir = %q", got)
	}

	patPolicyReadFile = func(string) ([]byte, error) { return nil, failure }
	if _, err := LoadBrowserPolicy("x"); !errors.Is(err, failure) {
		t.Fatalf("read error = %v", err)
	}
	patPolicyReadFile = func(string) ([]byte, error) { return []byte(`{`), nil }
	if _, err := LoadBrowserPolicy("x"); err == nil {
		t.Fatal("invalid policy decoded")
	}
	patPolicyReadFile = func(string) ([]byte, error) { return []byte(`{}`), nil }
	if policy, err := LoadBrowserPolicy("x"); err != nil || policy.Agents == nil {
		t.Fatalf("empty policy = %#v, %v", policy, err)
	}
	patPolicyReadFile = originalRead

	patPolicyMarshal = func(any, string, string) ([]byte, error) { return nil, failure }
	if err := saveBrowserPolicy("x", nil); !errors.Is(err, failure) {
		t.Fatalf("marshal error = %v", err)
	}
	patPolicyMarshal = originalMarshal
	patPolicyAtomicWrite = func(string, []byte) error { return failure }
	if err := saveBrowserPolicy("x", &BrowserPolicy{}); !errors.Is(err, failure) {
		t.Fatalf("write error = %v", err)
	}
	patPolicyAtomicWrite = originalWrite

	policyDir := t.TempDir()
	patPolicyReadFile = func(string) ([]byte, error) { return nil, failure }
	if _, err := ResolveBrowserPolicy(policyDir, ""); !errors.Is(err, failure) {
		t.Fatalf("resolve policy read error = %v", err)
	}
	if EffectiveOpenBrowser(policyDir) != true {
		t.Fatal("policy error should preserve browser-open fallback")
	}
	if _, err := SetBrowserPolicy(policyDir, "", true); !errors.Is(err, failure) {
		t.Fatalf("set policy read error = %v", err)
	}
	patPolicyReadFile = originalRead
	if _, err := ResolveBrowserPolicy(t.TempDir(), "bad code"); err == nil {
		t.Fatal("invalid agent code accepted")
	}
	if _, err := SetBrowserPolicy(t.TempDir(), "bad code", true); err == nil {
		t.Fatal("invalid policy agent code accepted")
	}
	patPolicyAtomicWrite = func(string, []byte) error { return failure }
	if _, err := SetBrowserPolicy(t.TempDir(), "agent", true); !errors.Is(err, failure) {
		t.Fatalf("agent save error = %v", err)
	}
	if _, err := SetBrowserPolicy(t.TempDir(), "", true); !errors.Is(err, failure) {
		t.Fatalf("default save error = %v", err)
	}
}

func TestCrossPlatformCoverageBrowserPolicyCommandWriteError(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	cmd := newBrowserPolicyCommand()
	cmd.SetOut(patFailWriter{})
	cmd.SetArgs([]string{"--enabled=true"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("output write error did not propagate")
	}
	if got := EffectiveOpenBrowser(os.Getenv("DWS_CONFIG_DIR")); !got {
		t.Fatal("saved browser policy was not effective")
	}
}

func TestCrossPlatformCoverageBrowserPolicyCommandSetError(t *testing.T) {
	badDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_CONFIG_DIR", badDir)
	cmd := newBrowserPolicyCommand()
	cmd.SetArgs([]string{"--enabled=true"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("policy command ignored load error")
	}
}

func TestCrossPlatformCoveragePATPureHelperEdges(t *testing.T) {
	if got := collectChmodProductCodes([]string{"", "a,a", " b "}, []string{"b", "c"}); strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("product codes = %#v", got)
	}
	if _, err := callPATBatchGrantWithLegacyFallback(t.Context(), nil, "", "", nil, nil, nil); err == nil {
		t.Fatal("nil batch grant caller accepted")
	}
	if _, err := callPATBatchPlan(t.Context(), nil, "", "", nil); err == nil {
		t.Fatal("nil batch plan caller accepted")
	}
	if _, err := callPATToolWithLegacyFallback(t.Context(), nil, "", "", "", nil, nil); err == nil {
		t.Fatal("nil legacy caller accepted")
	}
	caller := &patEdgeCaller{errs: []error{errors.New("boom")}}
	if _, err := callPATToolWithLegacyFallback(t.Context(), caller, "pat", "tool", "", nil, nil); err == nil {
		t.Fatal("empty alias error did not propagate")
	}
	caller = &patEdgeCaller{results: []*edition.ToolResult{patText(`{"code":"PAT_FORGED_IDENTITY_FIELD"}`)}, errs: []error{nil, errors.New("second failed")}}
	if _, err := callPATBatchToolWithIdentityFallback(t.Context(), caller, "agent", "", "tool", map[string]any{"agentCode": "agent"}); err == nil {
		t.Fatal("identity fallback retry error did not propagate")
	}

	if !strings.HasPrefix(newBatchClientRequestID(""), "batch-") {
		t.Fatal("default request ID prefix missing")
	}
	if got := normalizeProductCodes([]string{"", "a,a", " b "}); strings.Join(got, ",") != "a,b" {
		t.Fatalf("normalized product codes = %#v", got)
	}
	for scope, want := range map[string]string{"": "", "mail.read": "mail", "doc:read": "doc", "drive": "drive", " x.y:z ": "x"} {
		if got := productCodeFromScope(scope); got != want {
			t.Fatalf("productCodeFromScope(%q) = %q, want %q", scope, got, want)
		}
	}
	if !isEmptyToolResult(nil) || !isEmptyToolResult(&edition.ToolResult{}) || !isEmptyToolResult(&edition.ToolResult{Content: []edition.ContentBlock{{Type: "image"}}}) {
		t.Fatal("empty tool result detection failed")
	}
	if firstToolResultText(nil) != "" || firstToolResultText(&edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "x"}}}) != "" {
		t.Fatal("first text extraction failed")
	}
	if patResultAgentCode(nil) != "" || patResultAgentCode(patText(`{`)) != "" {
		t.Fatal("invalid PAT result produced agent code")
	}
	for raw, want := range map[string]string{
		`{"agentCode":"top"}`:                      "top",
		`{"data":{"agentCode":"data"}}`:            "data",
		`{"result":{"agentCode":"result"}}`:        "result",
		`{"result":{"data":{"agentCode":"deep"}}}`: "deep",
		`{}`: "",
	} {
		if got := patResultAgentCode(patText(raw)); got != want {
			t.Fatalf("agent code from %s = %q, want %q", raw, got, want)
		}
	}
	if err := ensurePATResultAgentCode(nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := ensurePATIdentityFallbackAgentCode(nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := ensurePATIdentityFallbackAgentCode(nil, "expected"); err == nil {
		t.Fatal("missing fallback agent code accepted")
	}
	if patBatchResultHasCode(nil, func(string) bool { return true }) || patBatchResultHasCode(patText(`{`), func(string) bool { return true }) {
		t.Fatal("invalid batch result matched")
	}
	if !patBatchResultHasCode(patText(`{"error_code":"match"}`), func(code string) bool { return code == "match" }) {
		t.Fatal("error_code was not matched")
	}
	if isPATBatchFallbackError(nil) || !isPATBatchFallbackError(errors.New("PAT_BATCH_AUTH_UNSUPPORTED")) {
		t.Fatal("batch fallback error detection failed")
	}
	if isToolNotRegisteredError(nil) || !isToolNotRegisteredError(errors.New("unknown tool")) {
		t.Fatal("tool registration error detection failed")
	}
	if hasScopeKeyShapeMismatch(nil, nil) || hasScopeKeyShapeMismatch(map[string]any{}, map[string]any{}) {
		t.Fatal("scope shape mismatch false positive")
	}
	canonical := map[string]any{"scopes": []string{"a"}}
	legacy := map[string]any{"scope": "a"}
	if isLegacyGrantSchemaMismatchError(nil, canonical, legacy) || isLegacyGrantSchemaMismatchError(&apperrors.PATError{RawJSON: "scope validation"}, canonical, legacy) {
		t.Fatal("legacy mismatch accepted excluded error")
	}
	for _, message := range []string{"missing parameter", "scope missing_scope", "scope invalid permission denied", "scope unknown"} {
		_ = isLegacyGrantSchemaMismatchError(errors.New(message), canonical, legacy)
	}
	if normalizedPATErrorText(nil) != "" {
		t.Fatal("nil normalized error text was non-empty")
	}
	typed := apperrors.NewAPI("message", apperrors.WithReason("reason"), apperrors.WithHint("hint"))
	if got := normalizedPATErrorText(typed); !strings.Contains(got, "reason") || !strings.Contains(got, "hint") {
		t.Fatalf("normalized typed error = %q", got)
	}
}

func TestCrossPlatformCoveragePATPlanExtractionEdges(t *testing.T) {
	invalidResults := []*edition.ToolResult{nil, patText(`{`), patText(`{}`)}
	for _, result := range invalidResults {
		_, _ = extractLoginRecommendProducts(result)
		_, _ = extractBatchPlanAllGranted(result)
		_, _ = extractSelectedScopes(result)
		_, _ = extractSelectedScopesAllowEmpty(result)
	}
	productsJSON := `{"data":{"selectedScopes":["mail.read",12,""],"items":[12,{"scope":""},{"scope":"mail.read","productName":"","displayName":"Read"},{"scope":"mail.write","operationSummary":"Write"},{"scope":"mail.other","operationSummary":"Write"},{"scope":"mail.more","operationSummary":"More"},{"scope":"mail.last","operationSummary":"Last"}]}}`
	products, err := extractLoginRecommendProducts(patText(productsJSON))
	if err != nil || len(products) != 1 || products[0].ProductCode != "mail" || products[0].SelectedScopeCount != 1 {
		t.Fatalf("products = %#v, %v", products, err)
	}
	fallbackJSON := `{"data":{"selectedScopes":["doc.read","doc.write","",12],"items":[]}}`
	products, err = extractLoginRecommendProducts(patText(fallbackJSON))
	if err != nil || len(products) != 1 || products[0].ScopeCount != 2 {
		t.Fatalf("fallback products = %#v, %v", products, err)
	}
	if _, err := extractSelectedScopes(patText(`{"data":{}}`)); err == nil {
		t.Fatal("empty selected scopes accepted")
	}
	if got, err := extractSelectedScopes(patText(`{"data":{"allGranted":true}}`)); err != nil || len(got) != 0 {
		t.Fatalf("all-granted scopes = %#v, %v", got, err)
	}
	if _, err := extractSelectedScopes(patText(`{"data":{"selectedScopes":[12," "]}}`)); err == nil {
		t.Fatal("non-string scopes accepted")
	}
	if got, err := extractSelectedScopes(patText(`{"data":{"selectedScopes":[12," "],"allGranted":true}}`)); err != nil || len(got) != 0 {
		t.Fatalf("filtered all-granted scopes = %#v, %v", got, err)
	}
	if got, err := extractSelectedScopesAllowEmpty(patText(`{"data":{"selectedScopes":[12," ","one"]}}`)); err != nil || len(got) != 1 {
		t.Fatalf("allow-empty scopes = %#v, %v", got, err)
	}
}

func TestCrossPlatformCoveragePATResultAndFlagEdges(t *testing.T) {
	if err := handleToolResult(nil, nil, nil); err == nil {
		t.Fatal("nil result accepted")
	}
	if err := handleToolResultForWriter(io.Discard, nil, nil); err == nil {
		t.Fatal("nil writer result accepted")
	}
	if err := classifyToolResultText(nil); err == nil {
		t.Fatal("nil result classified successfully")
	}
	nonText := &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "x"}}}
	if err := handleToolResultForWriter(io.Discard, nonText, nil); err == nil {
		t.Fatal("non-text result accepted")
	}
	if err := classifyToolResultText(nonText); err != nil {
		t.Fatal(err)
	}
	patFailure := patText(`{"code":"PAT_NO_PERMISSION"}`)
	if _, err := captureStdout(t, func() error { return handleToolResult(nil, nil, patFailure) }); err == nil {
		t.Fatal("PAT error result was accepted by stdout handler")
	}
	if _, err := captureStdout(t, func() error { return handleToolResult(nil, nil, nonText) }); err == nil {
		t.Fatal("non-text result was accepted by stdout handler")
	}
	if err := handleToolResultForWriter(io.Discard, patFailure, nil); err == nil {
		t.Fatal("PAT error result accepted")
	}
	if err := classifyToolResultText(patFailure); err == nil {
		t.Fatal("PAT error was not classified")
	}
	if err := handleToolResultForWriter(nil, patText(`{"data":{}}`), nil); err != nil {
		t.Fatalf("nil output writer: %v", err)
	}
	var buf bytes.Buffer
	if err := handleToolResultForWriter(&buf, patText(`plain text`), nil); err != nil || !strings.Contains(buf.String(), "plain text") {
		t.Fatalf("plain writer result = %q, %v", buf.String(), err)
	}

	if commandBoolFlag(nil, "verbose") || commandFlagChanged(nil, "format") || commandStringFlag(nil, "format") != "" {
		t.Fatal("nil command flag helpers returned values")
	}
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().Bool("verbose", true, "")
	root.PersistentFlags().String("format", "raw", "")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	if !commandBoolFlag(child, "verbose") || commandStringFlag(child, "format") != "raw" {
		t.Fatal("persistent flags were not resolved")
	}
	if commandFlagChanged(child, "format") {
		t.Fatal("unchanged format reported changed")
	}
	if err := root.PersistentFlags().Set("format", "json"); err != nil {
		t.Fatal(err)
	}
	if !commandFlagChanged(child, "format") {
		t.Fatal("changed persistent format not detected")
	}
	local := &cobra.Command{Use: "local"}
	local.Flags().Bool("verbose", true, "")
	local.Flags().String("format", "json", "")
	if !commandBoolFlag(local, "verbose") || commandStringFlag(local, "format") != "json" {
		t.Fatal("local flags were not resolved")
	}
	if err := local.Flags().Set("format", "raw"); err != nil {
		t.Fatal(err)
	}
	if !commandFlagChanged(local, "format") {
		t.Fatal("changed local format not detected")
	}
}

func TestCrossPlatformCoveragePATAuthorizationSummaryEdges(t *testing.T) {
	if formatPATAuthorizationSummary("not json", nil) != "" || formatPATAuthorizationSummary(`{}`, nil) != "" {
		t.Fatal("invalid summary payload rendered")
	}
	dry := &patEdgeCaller{dryRun: true}
	texts := []string{
		`{"success":true,"data":{"agentCode":"a","grantType":"once","allGranted":true,"items":[],"selectedScopes":[],"grantedScopes":[],"alreadyGrantedScopes":[],"skippedScopes":[],"pendingScopes":[]}}`,
		`{"code":"OK","data":{"selectedScopes":["a"]}}`,
		`{"data":{"selectedScopes":["a"]}}`,
		`{"data":{"pendingScopes":["a"]}}`,
		`{"data":{"items":["a"],"selectedScopes":"wrong"}}`,
	}
	for i, text := range texts {
		caller := edition.ToolCaller(nil)
		if i == 1 {
			caller = dry
		}
		if got := formatPATAuthorizationSummary(text, caller); got == "" {
			t.Fatalf("summary %d was empty", i)
		}
	}
	if _, ok := countField(map[string]any{"values": []string{"a"}}, "values"); !ok {
		t.Fatal("[]string count was not recognized")
	}
	if _, ok := countField(map[string]any{}, "missing"); ok {
		t.Fatal("missing count was recognized")
	}
	if _, ok := countField(map[string]any{"bad": 1}, "bad"); ok {
		t.Fatal("invalid count was recognized")
	}
}

func TestCrossPlatformCoverageLoginRecommendWrappers(t *testing.T) {
	if err := RunLoginRecommendAuthorization(t.Context(), nil, io.Discard); err == nil {
		t.Fatal("nil login recommend caller accepted")
	}
	if _, err := PlanLoginRecommendAuthorization(t.Context(), nil); err == nil {
		t.Fatal("nil plan caller accepted")
	}
	allGranted := patText(`{"data":{"selectedScopes":[],"allGranted":true,"items":[]}}`)
	caller := &patEdgeCaller{results: []*edition.ToolResult{allGranted}}
	var buf bytes.Buffer
	if err := RunLoginRecommendAuthorization(t.Context(), caller, &buf); err != nil || buf.Len() == 0 {
		t.Fatalf("login recommend wrapper = %q, %v", buf.String(), err)
	}
	caller = &patEdgeCaller{results: []*edition.ToolResult{allGranted}}
	plan, err := PlanLoginRecommendAuthorization(t.Context(), caller)
	if err != nil || !plan.AllGranted {
		t.Fatalf("login recommend plan = %#v, %v", plan, err)
	}
}

func TestCrossPlatformCoverageChmodCommandRemainingBranches(t *testing.T) {
	cmd := newChmodCommand(nil)
	if err := cmd.Args(cmd, []string{"scope"}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Args(cmd, nil); err == nil {
		t.Fatal("empty chmod arguments accepted")
	}
	cmd = newChmodCommand(nil)
	if err := cmd.Flags().Parse([]string{"--recommend"}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Args(cmd, nil); err != nil {
		t.Fatalf("recommend args: %v", err)
	}
	cmd = newChmodCommand(nil)
	if err := cmd.Flags().Parse([]string{"--product", "mail"}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Args(cmd, nil); err != nil {
		t.Fatalf("product args: %v", err)
	}

	run := func(c edition.ToolCaller, args ...string) error {
		command := newChmodCommand(c)
		command.SetArgs(args)
		return command.Execute()
	}
	dryError := &patEdgeCaller{dryRun: true, errs: []error{errors.New("plan failed")}}
	if err := run(dryError, "--recommend"); err == nil {
		t.Fatal("dry-run plan error did not propagate")
	}
	dryMismatch := &patEdgeCaller{dryRun: true, results: []*edition.ToolResult{patText(`{"data":{"agentCode":"other"}}`)}}
	if err := run(dryMismatch, "--recommend", "--agentCode", "expected"); err == nil {
		t.Fatal("dry-run plan agent mismatch accepted")
	}
	dryPreview := &patEdgeCaller{dryRun: true}
	if err := run(dryPreview, "scope", "--grant-type", "session", "--session-id", "sid"); err != nil {
		t.Fatalf("dry-run session preview: %v", err)
	}
	if err := run(nil, "scope"); err == nil {
		t.Fatal("nil chmod runtime accepted")
	}
	planError := &patEdgeCaller{errs: []error{errors.New("plan failed")}}
	if err := run(planError, "--recommend"); err == nil {
		t.Fatal("plan error did not propagate")
	}
	planMismatch := &patEdgeCaller{results: []*edition.ToolResult{patText(`{"data":{"agentCode":"other"}}`)}}
	if err := run(planMismatch, "--recommend", "--agentCode", "expected"); err == nil {
		t.Fatal("plan agent mismatch accepted")
	}
	badScopes := &patEdgeCaller{results: []*edition.ToolResult{patText(`{"data":{}}`)}}
	if err := run(badScopes, "--recommend"); err == nil {
		t.Fatal("invalid selected scopes accepted")
	}
}

func TestCrossPlatformCoverageLoginRecommendErrorMatrix(t *testing.T) {
	caller := &patEdgeCaller{errs: []error{errors.New("plan failed")}}
	if _, err := PlanLoginRecommendAuthorization(t.Context(), caller); err == nil {
		t.Fatal("plan wrapper error did not propagate")
	}
	if _, _, _, err := planLoginRecommend(t.Context(), &patEdgeCaller{}, nil, false); err == nil {
		t.Fatal("all-scope plan without products accepted")
	}
	if _, _, _, err := planLoginRecommend(t.Context(), &patEdgeCaller{errs: []error{errors.New("call failed")}}, nil, true); err == nil {
		t.Fatal("plan call error did not propagate")
	}
	if _, _, _, err := planLoginRecommend(t.Context(), &patEdgeCaller{results: []*edition.ToolResult{patText(`{"code":"PAT_NO_PERMISSION"}`)}}, nil, true); err == nil {
		t.Fatal("classified plan result accepted")
	}
	if _, _, _, err := planLoginRecommend(t.Context(), &patEdgeCaller{results: []*edition.ToolResult{patText(`{}`)}}, nil, true); err == nil {
		t.Fatal("malformed plan result accepted")
	}

	validProductPlan := patText(`{"data":{"selectedScopes":["mail.read"],"items":[{"scope":"mail.read","productCode":"mail"}]}}`)
	initial := &LoginRecommendPlan{Result: validProductPlan, Scopes: []string{"mail.read"}}
	if err := RunLoginRecommendAuthorizationWithOptions(t.Context(), &patEdgeCaller{}, io.Discard, LoginRecommendOptions{
		InitialPlan: initial,
		ProductSelector: func([]LoginRecommendProduct) ([]string, error) {
			return nil, errors.New("selection failed")
		},
	}); err == nil {
		t.Fatal("selector error did not propagate")
	}
	if err := RunLoginRecommendAuthorizationWithOptions(t.Context(), &patEdgeCaller{}, io.Discard, LoginRecommendOptions{
		InitialPlan: initial,
		ProductSelector: func([]LoginRecommendProduct) ([]string, error) {
			return nil, nil
		},
	}); err == nil {
		t.Fatal("empty product selection accepted")
	}
	if err := RunLoginRecommendAuthorizationWithOptions(t.Context(), &patEdgeCaller{errs: []error{errors.New("replan failed")}}, io.Discard, LoginRecommendOptions{
		InitialPlan: initial,
		ProductSelector: func([]LoginRecommendProduct) ([]string, error) {
			return []string{"mail"}, nil
		},
	}); err == nil {
		t.Fatal("selector replan error did not propagate")
	}
	if err := RunLoginRecommendAuthorizationWithOptions(t.Context(), &patEdgeCaller{}, io.Discard, LoginRecommendOptions{
		ScopeMode:   LoginRecommendScopeAll,
		InitialPlan: &LoginRecommendPlan{Result: patText(`{"data":{"selectedScopes":[],"items":[]}}`), Scopes: []string{"synthetic"}},
		ProductSelector: func([]LoginRecommendProduct) ([]string, error) {
			return nil, nil
		},
	}); err == nil {
		t.Fatal("all-scope mode without products accepted")
	}
	allGranted := patText(`{"data":{"selectedScopes":[],"allGranted":true,"items":[]}}`)
	var output bytes.Buffer
	if err := RunLoginRecommendAuthorizationWithOptions(t.Context(), &patEdgeCaller{results: []*edition.ToolResult{allGranted}}, &output, LoginRecommendOptions{
		InitialPlan: initial,
		ProductSelector: func([]LoginRecommendProduct) ([]string, error) {
			return []string{"mail"}, nil
		},
	}); err != nil || output.Len() == 0 {
		t.Fatalf("all-granted replan = %q, %v", output.String(), err)
	}
	if err := RunLoginRecommendAuthorizationWithOptions(t.Context(), &patEdgeCaller{errs: []error{errors.New("grant failed")}}, io.Discard, LoginRecommendOptions{
		InitialPlan: initial,
	}); err == nil {
		t.Fatal("grant error did not propagate")
	}
	if err := RunLoginRecommendAuthorizationWithOptions(t.Context(), &patEdgeCaller{}, io.Discard, LoginRecommendOptions{
		InitialPlan: &LoginRecommendPlan{Result: patText(`{`), Scopes: []string{"scope"}},
		ProductSelector: func([]LoginRecommendProduct) ([]string, error) {
			return []string{"mail"}, nil
		},
	}); err == nil {
		t.Fatal("invalid initial plan products accepted")
	}
}

func TestCrossPlatformCoverageLegacySchemaMismatchWithoutValidationKeyword(t *testing.T) {
	canonical := map[string]any{"scopes": []string{"a"}}
	legacy := map[string]any{"scope": "a"}
	if isLegacyGrantSchemaMismatchError(errors.New("scope problem"), canonical, legacy) {
		t.Fatal("ambiguous scope error triggered legacy fallback")
	}
}
