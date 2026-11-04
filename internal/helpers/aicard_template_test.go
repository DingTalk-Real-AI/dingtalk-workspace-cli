// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/validate"
)

type aicardTemplateTestCaller struct {
	aicardCaller
	invocations int
	service     string
	tool        string
	args        map[string]any
	result      transport.ToolCallResult
	err         error
}

func (c *aicardTemplateTestCaller) InvokeAicardTemplate(_ context.Context, service, tool string, args map[string]any) (publishedmcp.ValidatedInvocationResult, error) {
	c.invocations++
	c.service, c.tool, c.args = service, tool, args
	return publishedmcp.ValidatedInvocationResult{InputSchemaValidation: "fresh_core_subset_snapshot", InputSchemaDigest: strings.Repeat("a", 64), Result: c.result}, c.err
}

func executeAicardTemplate(t *testing.T, caller edition.ToolCaller, args ...string) (map[string]any, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)
	root := newAicardCommand()
	root.PersistentFlags().String("format", "json", "Output format")
	root.PersistentFlags().Bool("dry-run", false, "Local validation only")
	root.SilenceErrors, root.SilenceUsage = true, true
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"template"}, args...))
	ctx, _ := output.WithResultStore(t.Context())
	cmd, err := corecmd.ExecuteContextCForTest(root, ctx)
	if err != nil {
		return nil, err
	}
	code, _, err := output.EmitStoredResult(cmd)
	if err != nil || code != 0 {
		t.Fatalf("emit code=%d err=%v output=%s", code, err, buf.String())
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v: %s", err, buf.String())
	}
	return result["data"].(map[string]any), nil
}

func TestCrossPlatformCoverageAicardTemplateGetPreservesContentAndUnknownVersion(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	path := filepath.Join(t.TempDir(), "retrieved.json")
	caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": aicardTestSnapshot}}}
	data, err := executeAicardTemplate(t, caller, "get", "--template-id", "template-1", "--version", "9223372036854775807", "--out", path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(path)
	if err != nil || string(file) != aicardTestSnapshot || caller.invocations != 1 || caller.tool != "get_aicard_template" {
		t.Fatalf("retrieval: %v calls=%d tool=%s", err, caller.invocations, caller.tool)
	}
	if caller.args["version"] != "9223372036854775807" || caller.args["templateId"] != "template-1" || data["versionConfirmed"] != false || data["version"] != nil {
		t.Fatalf("version was rounded or falsely confirmed: %#v %#v", caller.args, data)
	}
	if data["templateDigest"] == "" || data["inputSchemaDigest"] != strings.Repeat("a", 64) {
		t.Fatal(data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows reports 0666 or 0444 for regular files; access there is governed by ACLs.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("unsafe output permissions: %v", info.Mode())
	}
}

func TestCrossPlatformCoverageAicardTemplateWirePassesTransportValidation(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte(aicardTestSnapshot), "", "  "); err != nil {
		t.Fatal(err)
	}
	crlf := strings.ReplaceAll(indented.String(), "\n", "\r\n") + "\r\n"
	// Legal inside JSON strings, rejected raw by the transport; U+E0001 needs a surrogate pair.
	format := strings.Replace(aicardTestSnapshot, "中文", "中文👨\u200d👩\u200d👧\u2028\u202e\ufeff\u2066\u007f\U000E0001", 1)
	compactDigest, err := validateAicardTemplate([]byte(aicardTestSnapshot))
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"crlf": crlf, "format characters": format} {
		if validate.RejectControlChars(contents, "file") == nil {
			t.Fatalf("%s fixture does not exercise the transport rule", name)
		}
		caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": "2"}}}
		result, err := executeAicardTemplate(t, caller, "create", "--file", aicardTestFile(t, contents))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		wire, _ := caller.args["file"].(string)
		if err := validate.RejectControlChars(wire, "file"); err != nil {
			t.Fatalf("%s: the transport would reject the request: %v", name, err)
		}
		var sent, original any
		if json.Unmarshal([]byte(wire), &sent) != nil || json.Unmarshal([]byte(contents), &original) != nil || !reflect.DeepEqual(sent, original) {
			t.Fatalf("%s: the wire changed template content: %s", name, wire)
		}
		digest, err := validateAicardTemplate([]byte(contents))
		if err != nil || result["templateDigest"] != digest {
			t.Fatalf("%s: digest drift: %v %v", name, err, result["templateDigest"])
		}
		if name == "crlf" && digest != compactDigest {
			t.Fatal("line endings changed the content digest")
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateDryRunAndOverwriteBarriers(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, args := range [][]string{
		{"create", "--file", file, "--dry-run"},
		{"update", "--template-id", "t", "--file", file, "--dry-run"},
		{"get", "--template-id", "t", "--version", "1", "--out", filepath.Join(t.TempDir(), "new.json"), "--dry-run"},
	} {
		caller := &aicardTemplateTestCaller{err: errors.New("must not call")}
		data, err := executeAicardTemplate(t, caller, args...)
		if err != nil || caller.invocations != 0 || data["executed"] != false {
			t.Fatalf("dry-run %v: %#v %v calls=%d", args, data, err, caller.invocations)
		}
		if path, ok := data["filePath"].(string); ok {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("dry-run created a file: %v", err)
			}
		}
	}
	caller := &aicardTemplateTestCaller{}
	if _, err := executeAicardTemplate(t, caller, "get", "--template-id", "t", "--version", "1", "--out", file); err == nil || caller.invocations != 0 {
		t.Fatalf("existing output accepted: %v", err)
	}
	contents, _ := os.ReadFile(file)
	if string(contents) != aicardTestSnapshot {
		t.Fatal("existing file changed")
	}
}

func TestCrossPlatformCoverageAicardTemplateWritesUseRawJSONAndAssignedVersion(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, operation := range []string{"create", "update"} {
		caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": "2"}}}
		args := []string{operation, "--file", file}
		if operation == "update" {
			args = append(args, "--template-id", "t")
		}
		result, err := executeAicardTemplate(t, caller, args...)
		if err != nil || caller.invocations != 1 || caller.args["file"] != aicardTestSnapshot || result["version"] != "2" || result["templateId"] != "t" {
			t.Fatalf("%s: %v %#v %#v", operation, err, caller.args, result)
		}
		if operation == "create" && caller.args["templateId"] != nil || operation == "update" && caller.args["templateId"] != "t" {
			t.Fatal(caller.args)
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateRejectsInvalidInputBeforeDiscovery(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	for _, version := range []string{"0", "01", "-1", "+1", "1.0", "9223372036854775808", "latest"} {
		caller := &aicardTemplateTestCaller{}
		_, err := executeAicardTemplate(t, caller, "get", "--template-id", "t", "--version", version, "--out", filepath.Join(t.TempDir(), "out.json"))
		var classified *apperrors.Error
		if !errors.As(err, &classified) || classified.Category != apperrors.CategoryValidation || caller.invocations != 0 {
			t.Fatalf("invalid version %q: %v", version, err)
		}
	}
	for _, contents := range []string{"", "[]", "{}", aicardTestSnapshot + " {}", strings.Replace(aicardTestSnapshot, `"version":"v1.0"`, `"version":"v1.0","version":"v1.0"`, 1), string([]byte{0xff}), strings.Repeat("x", aicardTemplateMaxBytes+1), `[{"version":"v1.0","updateDataModel":{"surfaceId":"s","path":"/","value":{}}}]`} {
		caller := &aicardTemplateTestCaller{}
		_, err := executeAicardTemplate(t, caller, "create", "--file", aicardTestFile(t, contents))
		if err == nil || caller.invocations != 0 {
			t.Fatalf("invalid file accepted: %v calls=%d", err, caller.invocations)
		}
	}
	for _, args := range [][]string{{"get", "--version", "1", "--out", "unused"}, {"update", "--file", "unused"}, {"create"}, {"get", "extra"}, {"search", "query"}} {
		caller := &aicardTemplateTestCaller{}
		if _, err := executeAicardTemplate(t, caller, args...); err == nil || caller.invocations != 0 {
			t.Fatalf("missing args accepted: %v", args)
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateFailureDoesNotRetryOrChangeIdentity(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, test := range []struct {
		err    error
		result transport.ToolCallResult
	}{
		{err: errors.New("sensitive endpoint token must not appear")},
		{result: transport.ToolCallResult{IsError: true}},
		{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": false}}},
		{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "different", "version": "2"}}},
		{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": float64(2)}}},
		{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": "0"}}},
		{result: transport.ToolCallResult{StructuredContent: map[string]any{"templateId": "t", "version": "2"}}},
	} {
		caller := &aicardTemplateTestCaller{result: test.result, err: test.err}
		_, err := executeAicardTemplate(t, caller, "update", "--template-id", "t", "--file", file)
		if err == nil || caller.invocations != 1 || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("failure handling: %v calls=%d", err, caller.invocations)
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateTextResultAndBadFile(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	valid, _ := json.Marshal(map[string]any{"success": true, "file": aicardTestSnapshot})
	for _, test := range []struct {
		text    string
		success bool
	}{
		{string(valid), true},
		{`{"success":true,"file":"[]"}`, false},
		{`{"success":true,"file":123}`, false},
		{`{"success":true,"success":false}`, false},
		{`not JSON`, false},
		{`[1]`, false},
	} {
		path := filepath.Join(t.TempDir(), "out.json")
		caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{Blocks: []transport.ContentBlock{{Type: "text", Text: test.text}}}}
		_, err := executeAicardTemplate(t, caller, "get", "--template-id", "t", "--version", "1", "--out", path)
		if (err == nil) != test.success || caller.invocations != 1 {
			t.Fatalf("text result: %v calls=%d", err, caller.invocations)
		}
		if _, statErr := os.Stat(path); !test.success && !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("bad result wrote a file: %v", statErr)
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateMissingServiceExplainsConfiguration(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "")
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, test := range []struct {
		name string
		args []string
	}{
		{"create", []string{"create", "--file", file}},
		{"get", []string{"get", "--template-id", "t", "--version", "1", "--out", filepath.Join(t.TempDir(), "card.json")}},
		{"update", []string{"update", "--template-id", "t", "--file", file}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, dryRun := range []bool{false, true} {
				caller := &aicardTemplateTestCaller{}
				args := append([]string(nil), test.args...)
				if dryRun {
					args = append(args, "--dry-run")
				}
				_, err := executeAicardTemplate(t, caller, args...)
				if err == nil || !strings.Contains(err.Error(), "--mcp-id") || !strings.Contains(err.Error(), aicardTemplateServiceEnv) {
					t.Fatalf("missing service configuration hint: dry-run=%v error=%v", dryRun, err)
				}
				if caller.invocations != 0 {
					t.Fatalf("missing configuration invoked the service: %d", caller.invocations)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAicardTemplateServiceSelectionAndDigest(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "configured-service")
	for _, override := range []string{"", "explicit-service"} {
		caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": "1"}}}
		args := []string{"create", "--file", aicardTestFile(t, aicardTestSnapshot)}
		if override != "" {
			args = append(args, "--mcp-id", override)
		}
		if _, err := executeAicardTemplate(t, caller, args...); err != nil {
			t.Fatal(err)
		}
		want := override
		if want == "" {
			want = "configured-service"
		}
		if caller.service != want {
			t.Fatalf("service %q != %q", caller.service, want)
		}
	}
	first := aicardTestSnapshot
	var parsed any
	decoder := json.NewDecoder(strings.NewReader(first))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil {
		t.Fatal(err)
	}
	formatted, _ := json.MarshalIndent(parsed, "", "  ")
	a, err := validateAicardTemplate([]byte(first))
	if err != nil {
		t.Fatal(err)
	}
	b, err := validateAicardTemplate(formatted)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("formatting changed digest")
	}
	base := strings.Replace(aicardTestSnapshot, `"dataModel":{}`, `"dataModel":{"number":9007199254740992}`, 1)
	changed := strings.Replace(base, "9007199254740992", "9007199254740993", 1)
	a, err = validateAicardTemplate([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	b, err = validateAicardTemplate([]byte(changed))
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("large integer was rounded in digest")
	}
}

func TestCrossPlatformCoverageAicardTemplateOutputRacesAndMalformedResponses(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	for _, result := range []transport.ToolCallResult{
		{},
		{Content: map[string]any{"success": true, "file": aicardTestSnapshot}},
		{Blocks: []transport.ContentBlock{{Type: "image"}, {Type: "text", Text: " "}}},
		{Blocks: []transport.ContentBlock{{Type: "text", Text: `null`}}},
		{Blocks: []transport.ContentBlock{{Type: "text", Text: `{"success":true,"file":"[]"}`}, {Type: "text", Text: `{}`}}},
	} {
		path := filepath.Join(t.TempDir(), "out.json")
		caller := &aicardTemplateTestCaller{result: result}
		data, err := executeAicardTemplate(t, caller, "get", "--template-id", "t", "--version", "1", "--out", path)
		if result.Content != nil {
			if err != nil || data["executed"] != true {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("malformed response accepted")
		}
	}
	missingParent := filepath.Join(t.TempDir(), "missing", "out.json")
	caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": aicardTestSnapshot}}}
	if _, err := executeAicardTemplate(t, caller, "get", "--template-id", "t", "--version", "1", "--out", missingParent); err == nil {
		t.Fatal("missing output parent accepted")
	}
	path := aicardTestFile(t, "retain existing contents")
	if err := writeAicardTemplateFile(path, []byte("replacement"), ""); err == nil {
		t.Fatal("exclusive write overwrote an existing file")
	}
	contents, _ := os.ReadFile(path)
	if string(contents) != "retain existing contents" {
		t.Fatal("existing file changed")
	}
}

func TestCrossPlatformCoverageAicardTemplateMissingCallerAndTypedErrors(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	path := aicardTestFile(t, aicardTestSnapshot)
	if _, err := executeAicardTemplate(t, &aicardCaller{}, "create", "--file", path); err == nil {
		t.Fatal("missing template capability accepted")
	}
	failure := apperrors.NewAuth("Selected profile has no access")
	caller := &aicardTemplateTestCaller{err: failure}
	_, err := executeAicardTemplate(t, caller, "create", "--file", path)
	if !errors.Is(err, failure) || caller.invocations != 1 {
		t.Fatalf("typed error lost or retried: %v", err)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "absent"), t.TempDir()} {
		caller := &aicardTemplateTestCaller{}
		if _, err := executeAicardTemplate(t, caller, "create", "--file", path); err == nil || caller.invocations != 0 {
			t.Fatal("invalid file reached service")
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateExactBudgetAndInlineInitialization(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	inline := `[{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{},"components":[{"id":"root","component":"Text","text":"Fixture"}]}}]`
	exact := inline + strings.Repeat(" ", aicardTemplateMaxBytes-len(inline))
	if _, err := validateAicardTemplate([]byte(exact)); err != nil {
		t.Fatal("exact file budget rejected", err)
	}
	if _, err := validateAicardTemplate([]byte(exact + " ")); err == nil {
		t.Fatal("file budget bypassed")
	}
	caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": "1"}}}
	if _, err := executeAicardTemplate(t, caller, "create", "--file", aicardTestFile(t, inline)); err != nil || caller.invocations != 1 {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAicardTemplatePinnedDigests(t *testing.T) {
	// Fixed vectors pin the published normalization contract independently of the serializer.
	// Unicode is intentional input for the content-digest boundary.
	for _, tc := range []struct{ name, input, want string }{
		{"ascii", `[{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{},"components":[{"id":"root","component":"Text","text":"Hello"}]}}]`, "sha256:json-sorted-number-preserving-v1:3b1257750b3dae10733d174f4bb837db1907d7b2c5362d80291a3ee999d08e2c"},
		{"unicode-large-integer", `[{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{"number":9007199254740993,"emoji":"😀","html":"<b>&</b>"},"components":[{"id":"root","component":"Text","text":"Example"}]}}]`, "sha256:json-sorted-number-preserving-v1:982b65edd7c1f8eec8b18a6f28e01937eec14b9bc3d4319719058d3ee0aff704"},
		{"decimal-lexeme", `[{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{"number":1.0},"components":[{"id":"root","component":"Text","text":"Example"}]}}]`, "sha256:json-sorted-number-preserving-v1:83f03b10cfc42f7cba949054a36072422807968227304342c1680dd4218ca3d5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateAicardTemplate([]byte(tc.input))
			if err != nil || got != tc.want {
				t.Fatalf("digest contract changed: got=%s want=%s err=%v", got, tc.want, err)
			}
		})
	}
}

func TestCrossPlatformCoverageAicardTemplateLeadingBOM(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	original := []byte(aicardTestSnapshot)
	withBOM := append([]byte{0xef, 0xbb, 0xbf}, original...)
	want, err := validateAicardTemplate(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := validateAicardTemplate(withBOM)
	if err != nil || got != want {
		t.Fatalf("BOM changed validation/digest: %v", err)
	}
	caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": "1"}}}
	if _, err := executeAicardTemplate(t, caller, "create", "--file", aicardTestFile(t, string(withBOM))); err != nil {
		t.Fatal(err)
	}
	wire, _ := caller.args["file"].(string)
	if !json.Valid([]byte(wire)) || bytes.HasPrefix([]byte(wire), []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("BOM reached transport")
	}
	caller.result = transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": string(withBOM)}}
	out := filepath.Join(t.TempDir(), "retrieved.json")
	if _, err := executeAicardTemplate(t, caller, "get", "--template-id", "t", "--version", "1", "--out", out); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(saved, original) {
		t.Fatalf("BOM retrieval was not normalized: %v", err)
	}
	if _, err := validateAicardTemplate(append([]byte{' '}, withBOM...)); err == nil {
		t.Fatal("embedded BOM was accepted")
	}
}

func TestCrossPlatformCoverageAicardTemplateRequiresExplicitService(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "")
	caller := &aicardTemplateTestCaller{}
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, args := range [][]string{
		{"create", "--file", file},
		{"update", "--template-id", "t", "--file", file},
		{"get", "--template-id", "t", "--version", "1", "--out", filepath.Join(t.TempDir(), "new.json")},
	} {
		if _, err := executeAicardTemplate(t, caller, args...); err == nil || caller.invocations != 0 {
			t.Fatalf("missing environment route reached the service: %v", err)
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateBOMByteLimit(t *testing.T) {
	withBOM := append([]byte{0xef, 0xbb, 0xbf}, []byte(aicardTestSnapshot)...)
	withBOM = append(withBOM, bytes.Repeat([]byte{' '}, aicardTemplateMaxBytes-len(withBOM))...)
	if _, err := validateAicardTemplate(withBOM); err != nil {
		t.Fatalf("exact byte limit with BOM was rejected: %v", err)
	}
	if _, err := validateAicardTemplate(append(withBOM, ' ')); err == nil {
		t.Fatal("BOM allowed the input byte limit to be exceeded")
	}
}
