// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type rejectingTemplateQueryValue struct{ value string }

func (v rejectingTemplateQueryValue) String() string { return v.value }
func (v rejectingTemplateQueryValue) Type() string   { return "string" }
func (v rejectingTemplateQueryValue) Get() any       { return v.value }
func (rejectingTemplateQueryValue) Set(string) error { return errors.New("reject normalized query") }

func runAITableTemplateSearch(t *testing.T, caller *aitableTestCaller, args ...string) error {
	t.Helper()
	InitDepsForTest(t, caller)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"template", "search"}, args...))
	return root.ExecuteContext(context.Background())
}

func TestCrossPlatformCoverageAITableTemplateSearchRequiresQueryBeforeMCP(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing"},
		{name: "blank", args: []string{"--query", "   "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			err := runAITableTemplateSearch(t, caller, test.args...)
			if err == nil || !strings.Contains(err.Error(), "query") {
				t.Fatalf("query validation error = %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid query made %d MCP calls", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageAITableTemplateSearchTrimsQueryAndPreservesKeywordAlias(t *testing.T) {
	for _, args := range [][]string{
		{"--query", "  项目  "},
		{"--keyword", "  项目  "},
	} {
		caller := &aitableTestCaller{responses: []string{`{"success":true,"data":{"templates":[]}}`}}
		if err := runAITableTemplateSearch(t, caller, args...); err != nil {
			t.Fatalf("template search %v error = %v", args, err)
		}
		if len(caller.calls) != 1 || caller.calls[0].tool != "search_templates" || caller.calls[0].args["query"] != "项目" {
			t.Fatalf("template search %v calls = %#v", args, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAITableTemplateSearchHelpMarksQueryRequired(t *testing.T) {
	root := newAitableCommand()
	command, _, err := root.Find([]string{"template", "search"})
	if err != nil {
		t.Fatal(err)
	}
	flag := command.Flags().Lookup("query")
	if flag == nil || flag.Annotations == nil || len(flag.Annotations[cobra.BashCompOneRequiredFlag]) == 0 || flag.Annotations[cobra.BashCompOneRequiredFlag][0] != "true" {
		t.Fatalf("query required annotation = %#v", flag)
	}
	for _, text := range []string{command.Long, command.Example} {
		if strings.Contains(text, "返回热门") || strings.Contains(text, "不传关键词") {
			t.Fatalf("atomic help still claims unsupported fallback: %q", text)
		}
	}
}

func TestCrossPlatformCoverageAITableTemplateSearchPropagatesNormalizedFlagSetError(t *testing.T) {
	root := newAitableCommand()
	command, _, err := root.Find([]string{"template", "search"})
	if err != nil {
		t.Fatal(err)
	}
	query := command.Flags().Lookup("query")
	if query == nil {
		t.Fatal("query flag missing")
	}
	query.Value = rejectingTemplateQueryValue{value: "项目"}
	if err := command.PreRunE(command, nil); err == nil || !strings.Contains(err.Error(), "reject normalized query") {
		t.Fatalf("PreRunE error = %v", err)
	}
}
