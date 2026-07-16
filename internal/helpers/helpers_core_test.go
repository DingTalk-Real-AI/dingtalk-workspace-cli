package helpers

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type helpersCoreCaller struct {
	result *edition.ToolResult
	err    error
	format string
	dry    bool
	calls  int
}

func (c *helpersCoreCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return c.result, c.err
}
func (c *helpersCoreCaller) Format() string { return c.format }
func (c *helpersCoreCaller) DryRun() bool   { return c.dry }
func (*helpersCoreCaller) Fields() string   { return "" }
func (*helpersCoreCaller) JQ() string       { return "" }

func textToolResult(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func installHelpersCoreDeps(t *testing.T, caller *helpersCoreCaller) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	old := deps
	t.Cleanup(func() { deps = old })
	InitDeps(caller)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = errOut
	return out, errOut
}

func TestCrossPlatformCoverageSharedDependenciesRoutingAndWrappers(t *testing.T) {
	oldDeps := deps
	deps = nil
	if GetCaller() != nil || GetFormatter() == nil {
		t.Fatal("nil dependency accessors returned unexpected values")
	}
	deps = oldDeps

	caller := &helpersCoreCaller{format: "json", result: textToolResult(`{"ok":true}`)}
	installHelpersCoreDeps(t, caller)
	if GetCaller() != caller || GetFormatter() != deps.Out {
		t.Fatal("initialized dependency accessors returned unexpected values")
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"dws", "--format", "json", "doc", "get"}
	if got := resolveProductID(); got != "doc" {
		t.Fatalf("resolveProductID() = %q", got)
	}
	os.Args = []string{"dws", "unknown"}
	if got := resolveProductID(); got != "" {
		t.Fatalf("unknown resolveProductID() = %q", got)
	}
	if _, err := callMCPToolReturnText(context.Background(), "tool", nil); err == nil {
		t.Fatal("unroutable return-text call should fail")
	}
	os.Args = []string{"dws", "doc"}
	if got, err := callMCPToolReturnText(context.Background(), "tool", nil); err != nil || got != `{"ok":true}` {
		t.Fatalf("callMCPToolReturnText() = %q, %v", got, err)
	}
	if err := CallMCPToolOnServer("doc", "tool", nil); err != nil {
		t.Fatalf("CallMCPToolOnServer(): %v", err)
	}

	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("inherited", "fallback", "")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	if got := MustGetStringFlag(child, "inherited"); got != "fallback" {
		t.Fatalf("MustGetStringFlag() = %q", got)
	}
	_ = GroupRunE(root, nil)
}

func TestCrossPlatformCoverageMCPReturnTextClassification(t *testing.T) {
	caller := &helpersCoreCaller{}
	installHelpersCoreDeps(t, caller)
	cases := []struct {
		name string
		text string
	}{
		{"gateway", `{"errorCode":"DWS_SERVICE_UNAUTHORIZED"}`},
		{"not-logged-in", `{"error":"Missing service_id or access_key"}`},
		{"pat", `{"code":"PAT_NO_PERMISSION"}`},
		{"business-bool", `{"success":false,"message":"failed"}`},
		{"business-string", `{"success":"false","errorMsg":"failed"}`},
		{"business-error", `{"error":"failed"}`},
	}
	for _, tc := range cases {
		caller.result = textToolResult(tc.text)
		if _, err := callMCPToolReturnTextOnServer(context.Background(), "server", "tool", nil); err == nil {
			t.Errorf("%s response should be classified", tc.name)
		}
	}
	caller.result = &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "ignored"}, {Type: "text"}}}
	if got, err := callMCPToolReturnTextOnServer(context.Background(), "server", "tool", nil); err != nil || got != "" {
		t.Fatalf("empty text result = %q, %v", got, err)
	}
	caller.result = textToolResult("shortcut result")
	if got, err := CallMCPToolTextOnServer("server", "tool", nil); err != nil || got != "shortcut result" {
		t.Fatalf("exported text result = %q, %v", got, err)
	}
	caller.err = errors.New("request failed with PAT_HIGH_RISK_NO_PERMISSION")
	if _, err := callMCPToolReturnTextOnServer(context.Background(), "server", "tool", nil); err == nil {
		t.Fatal("PAT transport error should be classified")
	}
	caller.err = errors.New("ordinary transport failure")
	if _, err := callMCPToolReturnTextOnServer(context.Background(), "server", "tool", nil); err == nil {
		t.Fatal("ordinary transport error should be wrapped")
	}
}

func TestCrossPlatformCoverageMCPOutputModesAndDevdocFormatting(t *testing.T) {
	caller := &helpersCoreCaller{format: "json", result: textToolResult(`{"url":"https://example.test/?a=1&b=2"}`)}
	out, _ := installHelpersCoreDeps(t, caller)
	if err := callMCPToolInternalOpts("server", "tool", map[string]any{"x": 1}, true); err != nil {
		t.Fatalf("unescaped JSON output: %v", err)
	}
	if !strings.Contains(out.String(), "&") {
		t.Fatalf("unescaped output = %q", out.String())
	}

	out.Reset()
	caller.format = "raw"
	caller.result = textToolResult("plain text")
	if err := callMCPToolInternalOpts("server", "tool", nil, false); err != nil || !strings.Contains(out.String(), "plain text") {
		t.Fatalf("raw output = %q, %v", out.String(), err)
	}

	out.Reset()
	caller.format = "table"
	caller.result = textToolResult(`{"Result":{"Items":[{"Title":"<em>Match</em>","URL":"https://example.test"}],"currentPage":1,"totalCount":2,"hasMore":true}}`)
	if err := callMCPToolInternalOpts("server", "search_open_platform_docs", nil, false); err != nil {
		t.Fatalf("devdoc table output: %v", err)
	}
	if !strings.Contains(out.String(), "Match") || strings.Contains(out.String(), "<em>") {
		t.Fatalf("devdoc table = %q", out.String())
	}
	out.Reset()
	if !formatDevdocSearchTable(`{"Result":{"Items":[]}}`) || !strings.Contains(out.String(), "no matching") {
		t.Fatalf("empty devdoc table = %q", out.String())
	}
	if formatDevdocSearchTable("{") {
		t.Fatal("invalid devdoc JSON should not format")
	}

	out.Reset()
	caller.result = &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "image"}}}
	if err := callMCPToolInternalOpts("server", "tool", nil, false); err != nil || out.Len() == 0 {
		t.Fatalf("non-text result output = %q, %v", out.String(), err)
	}

	caller.dry = true
	before := caller.calls
	if err := callMCPToolInternalOpts("server", "tool", map[string]any{"x": 1}, false); err != nil || caller.calls != before {
		t.Fatalf("dry run called tool: err=%v calls=%d/%d", err, caller.calls, before)
	}
}

func TestCrossPlatformCoverageCurrentUserResponseShapes(t *testing.T) {
	caller := &helpersCoreCaller{}
	installHelpersCoreDeps(t, caller)
	for _, response := range []string{
		`{"result":[{"orgEmployeeModel":{"userId":"array-user"}}]}`,
		`{"result":{"userId":"object-user"}}`,
	} {
		caller.result = textToolResult(response)
		if got, err := getCurrentUserID(context.Background()); err != nil || got == "" {
			t.Errorf("getCurrentUserID(%s) = %q, %v", response, got, err)
		}
	}
	caller.result = &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image"}, {Type: "text", Text: "{"}}}
	if _, err := getCurrentUserID(context.Background()); err == nil {
		t.Fatal("unparseable current user should fail")
	}
	caller.err = errors.New("offline")
	if _, err := getCurrentUserID(context.Background()); err == nil {
		t.Fatal("current-user transport failure should fail")
	}
}

func TestCrossPlatformCoverageCoreClassificationSuggestionsAndConfirmation(t *testing.T) {
	if classifyPATError(map[string]any{"errorCode": "PAT_LOW_RISK_NO_PERMISSION"}) == nil ||
		classifyPATError(map[string]any{"code": "other"}) != nil {
		t.Fatal("PAT classification mismatch")
	}
	pat := &PATError{RawJSON: "{}"}
	if reclassifyPATFromError(pat) != pat || reclassifyPATFromError(errors.New("plain")) != nil {
		t.Fatal("PAT reclassification mismatch")
	}
	if !strings.Contains(buildMinimalPATJSON("PAT_NO_PERMISSION"), "PAT_NO_PERMISSION") {
		t.Fatal("minimal PAT JSON omitted code")
	}
	if isBusinessError(map[string]any{"success": true}) || isBusinessError(map[string]any{}) {
		t.Fatal("successful response classified as business error")
	}
	for _, body := range []map[string]any{{"errorMsg": "one"}, {"message": "two"}, {"error": "three"}, {}} {
		_ = suggestForBusinessError(body)
	}

	previousEdition := edition.Get()
	t.Cleanup(func() { edition.Override(previousEdition) })
	edition.Override(&edition.Hooks{IsEmbedded: true})
	if notLoggedInSuggestion() != "请先登录" || !strings.Contains(authExpiredSuggestion(), "re-run") {
		t.Fatal("embedded auth suggestions changed")
	}
	edition.Override(&edition.Hooks{})
	if !strings.Contains(notLoggedInSuggestion(), "auth login") || !strings.Contains(authExpiredSuggestion(), "auth login") {
		t.Fatal("standalone auth suggestions changed")
	}

	caller := &helpersCoreCaller{}
	installHelpersCoreDeps(t, caller)
	oldArgs, oldStdin := os.Args, os.Stdin
	t.Cleanup(func() { os.Args, os.Stdin = oldArgs, oldStdin })
	os.Args = []string{"dws", "--yes"}
	if !confirmDelete("doc", "id") {
		t.Fatal("--yes should confirm")
	}
	for _, tc := range []struct {
		answer string
		want   bool
	}{{"yes\n", true}, {"Y\n", true}, {"no\n", false}} {
		path := filepath.Join(t.TempDir(), "answer")
		if err := os.WriteFile(path, []byte(tc.answer), 0o600); err != nil {
			t.Fatalf("write confirmation: %v", err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open confirmation: %v", err)
		}
		os.Stdin = file
		os.Args = []string{"dws"}
		if got := confirmDelete("doc", "id"); got != tc.want {
			t.Errorf("confirmDelete(%q) = %v", tc.answer, got)
		}
		_ = file.Close()
	}
}

func TestCrossPlatformCoverageCamelCaseAliasesAndFlagCopying(t *testing.T) {
	for input, want := range map[string]string{"base-id": "baseId", "plain": "plain", "a--b": "aB"} {
		if got := toCamelCase(input); got != want {
			t.Errorf("toCamelCase(%q) = %q, want %q", input, got, want)
		}
	}
	root := &cobra.Command{Use: "root"}
	root.Flags().Int("int-value", 0, "")
	root.Flags().Int64("long-value", 0, "")
	root.Flags().Float64("float-value", 0, "")
	root.Flags().Bool("bool-value", false, "")
	root.Flags().StringSlice("slice-value", nil, "")
	root.Flags().String("text-value", "", "")
	root.Flags().String("textValue", "existing", "")
	child := &cobra.Command{Use: "child"}
	child.Flags().String("child-value", "", "")
	root.AddCommand(child)
	RegisterCamelCaseAliases(root)
	for _, name := range []string{"intValue", "longValue", "floatValue", "boolValue", "sliceValue"} {
		flag := root.Flags().Lookup(name)
		if flag == nil || !flag.Hidden {
			t.Errorf("camel alias --%s missing or visible", name)
		}
	}
	if child.Flags().Lookup("childValue") == nil {
		t.Fatal("child camel alias missing")
	}
	if flag := root.Flags().Lookup("textValue"); flag == nil || flag.Hidden || flag.DefValue != "existing" {
		t.Fatal("existing camel-case flag should be preserved")
	}

	src, dst := &cobra.Command{Use: "src"}, &cobra.Command{Use: "dst"}
	src.Flags().String("copied", "value", "")
	copyFlags(src, dst, "missing", "copied")
	if flag := dst.Flags().Lookup("copied"); flag == nil || flag.DefValue != "value" {
		t.Fatal("copyFlags() did not copy the requested flag")
	}
}
