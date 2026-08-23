// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package smart

import (
	"bytes"
	"context"
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type aitableResolverCaller struct {
	text  string
	calls int
	tool  string
	args  map[string]any
}

func (c *aitableResolverCaller) CallTool(_ context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	c.tool = tool
	c.args = args
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, nil
}
func (*aitableResolverCaller) Format() string { return "json" }
func (*aitableResolverCaller) DryRun() bool   { return false }
func (*aitableResolverCaller) Fields() string { return "" }
func (*aitableResolverCaller) JQ() string     { return "" }

func runAITableResolverCLI(t *testing.T, caller *aitableResolverCaller, args ...string) (string, error) {
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
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), err
}

func TestCrossPlatformCoverageResolveBaseCLIExactMatchE2E(t *testing.T) {
	caller := &aitableResolverCaller{text: `{"data":{"bases":[{"baseId":"b1","baseName":"项目归档"},{"baseId":"b2","baseName":"项目"}],"hasMore":false}}`}
	out, err := runAITableResolverCLI(t, caller, "aitable", "+resolve-base", "--name", "项目")
	if err != nil {
		t.Fatalf("resolve base CLI error = %v", err)
	}
	for _, want := range []string{`"resolved": true`, `"matchType": "exact"`, `"baseId": "b2"`} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("output missing %s: %s", want, out)
		}
	}
	if caller.tool != "search_bases" || caller.args["query"] != "项目" {
		t.Fatalf("resolver call = %s %#v", caller.tool, caller.args)
	}
}

func TestCrossPlatformCoverageResolveBaseCLIAmbiguityIsFailureE2E(t *testing.T) {
	caller := &aitableResolverCaller{text: `{"bases":[{"baseId":"b1","baseName":"项目"},{"baseId":"b2","baseName":"项目"}]}`}
	out, err := runAITableResolverCLI(t, caller, "aitable", "+resolve-base", "--name", "项目")
	if err == nil || out != "" {
		t.Fatalf("ambiguous resolver = output:%q err:%v", out, err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "target_ambiguous" {
		t.Fatalf("ambiguous resolver error = %#v", err)
	}
}

func TestCrossPlatformCoverageResolveTableCLIFuzzyRequiresOptInE2E(t *testing.T) {
	payload := `{"tables":[{"tableId":"t1","tableName":"任务归档"}]}`
	caller := &aitableResolverCaller{text: payload}
	out, err := runAITableResolverCLI(t, caller, "aitable", "+resolve-table", "--base", "base", "--name", "任务")
	if err == nil || out != "" {
		t.Fatalf("implicit fuzzy resolver = output:%q err:%v", out, err)
	}
	caller = &aitableResolverCaller{text: payload}
	out, err = runAITableResolverCLI(t, caller, "aitable", "+resolve-table", "--base", "base", "--name", "任务", "--fuzzy")
	if err != nil || !bytes.Contains([]byte(out), []byte(`"matchType": "fuzzy"`)) || !bytes.Contains([]byte(out), []byte(`"tableId": "t1"`)) {
		t.Fatalf("explicit fuzzy resolver = output:%q err:%v", out, err)
	}
}

func TestCrossPlatformCoverageResolveCLIUnknownResponseIsNotNotFoundE2E(t *testing.T) {
	for name, payload := range map[string]string{
		"empty response":        "",
		"missing collection":    `{"success":true}`,
		"wrong collection type": `{"bases":{}}`,
		"malformed candidate":   `{"bases":[{"baseName":"项目"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &aitableResolverCaller{text: payload}
			out, err := runAITableResolverCLI(t, caller, "aitable", "+resolve-base", "--name", "项目")
			if err == nil || out != "" {
				t.Fatalf("unknown response = output:%q err:%v", out, err)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "target_invalid_response" {
				t.Fatalf("unknown response error = %#v", err)
			}
		})
	}

	caller := &aitableResolverCaller{text: `{"bases":[]}`}
	out, err := runAITableResolverCLI(t, caller, "aitable", "+resolve-base", "--name", "项目")
	if err == nil || out != "" {
		t.Fatalf("explicit empty candidates = output:%q err:%v", out, err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "target_not_found" {
		t.Fatalf("explicit empty candidates error = %#v", err)
	}
}
