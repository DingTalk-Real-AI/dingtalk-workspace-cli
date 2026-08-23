// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type urlResolveE2ECaller struct {
	results []*edition.ToolResult
	errors  []error
	calls   []aitableURLCall
}

type aitableURLCall struct {
	product string
	tool    string
	args    map[string]any
}

func (c *urlResolveE2ECaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	c.calls = append(c.calls, aitableURLCall{product: product, tool: tool, args: cloned})
	index := len(c.calls) - 1
	if index < len(c.errors) && c.errors[index] != nil {
		return nil, c.errors[index]
	}
	if index >= len(c.results) {
		return nil, errors.New("unexpected URL resolver tool call")
	}
	return c.results[index], nil
}

func (*urlResolveE2ECaller) Format() string { return "json" }
func (*urlResolveE2ECaller) DryRun() bool   { return false }
func (*urlResolveE2ECaller) Fields() string { return "" }
func (*urlResolveE2ECaller) JQ() string     { return "" }

func urlResult(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func runURLResolveCLI(t *testing.T, caller *urlResolveE2ECaller, args ...string) (string, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"aitable", "+url-resolve"}, args...))
	err := root.Execute()
	return stdout.String(), err
}

func TestCrossPlatformCoverageURLResolveCLIParseOnlyE2E(t *testing.T) {
	caller := &urlResolveE2ECaller{}
	out, err := runURLResolveCLI(t, caller,
		"--url", "https://alidocs.dingtalk.com/i/nodes/base-1?iframeQuery=sheetId%3Dtable-1%26viewId%3Dview-1",
	)
	if err != nil {
		t.Fatalf("URL resolver returned error: %v", err)
	}
	for _, want := range []string{`"status": "resolved"`, `"baseId": "base-1"`, `"tableId": "table-1"`, `"viewId": "view-1"`, `"verified": false`} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("output missing %s:\n%s", want, out)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("parse-only resolver made MCP calls: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageURLResolveCLIVerifiesDeepestTargetE2E(t *testing.T) {
	caller := &urlResolveE2ECaller{results: []*edition.ToolResult{
		urlResult(`{"data":{"tables":[{"tableId":"table-1","tableName":"任务"}]}}`),
	}}
	out, err := runURLResolveCLI(t, caller,
		"--url", "https://alidocs.dingtalk.com/i/nodes/base-1?iframeQuery=sheetId%3Dtable-1",
		"--verify",
	)
	if err != nil {
		t.Fatalf("verified URL resolver returned error: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte(`"verificationStatus": "verified"`)) || !bytes.Contains([]byte(out), []byte(`"tool": "get_tables"`)) {
		t.Fatalf("verified output = %s", out)
	}
	if len(caller.calls) != 1 || caller.calls[0].product != "aitable" || caller.calls[0].tool != "get_tables" {
		t.Fatalf("verification calls = %#v", caller.calls)
	}
	wantArgs := map[string]any{"baseId": "base-1", "tableIds": []string{"table-1"}}
	if !reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("verification args = %#v, want %#v", caller.calls[0].args, wantArgs)
	}
}

func TestCrossPlatformCoverageURLResolveCLIDoesNotMistakeUnknownForVerified(t *testing.T) {
	tests := []struct {
		name   string
		result *edition.ToolResult
	}{
		{name: "nil result", result: nil},
		{name: "empty content", result: &edition.ToolResult{}},
		{name: "empty text", result: urlResult("")},
		{name: "unknown object", result: urlResult(`{"success":true}`)},
		{name: "legal empty table list", result: urlResult(`{"data":{"tables":[]}}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &urlResolveE2ECaller{results: []*edition.ToolResult{test.result}}
			out, err := runURLResolveCLI(t, caller,
				"--url", "https://alidocs.dingtalk.com/i/nodes/base-1?iframeQuery=sheetId%3Dtable-1",
				"--verify",
			)
			if err == nil {
				t.Fatalf("%s was treated as verified; output=%s", test.name, out)
			}
			if out != "" {
				t.Fatalf("%s emitted success output: %q", test.name, out)
			}
		})
	}
}

func TestCrossPlatformCoverageURLResolveCLIInvalidURLFailsBeforeMCP(t *testing.T) {
	caller := &urlResolveE2ECaller{}
	out, err := runURLResolveCLI(t, caller, "--url", "https://example.com/i/nodes/base", "--verify")
	if err == nil || out != "" || len(caller.calls) != 0 {
		t.Fatalf("invalid URL result = output:%q err:%v calls:%#v", out, err, caller.calls)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "invalid_aitable_url" {
		t.Fatalf("invalid URL error = %#v", err)
	}
}
