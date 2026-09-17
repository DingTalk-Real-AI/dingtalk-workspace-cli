// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableOutputCompatibilityCaller struct{ tools []string }

func (c *aitableOutputCompatibilityCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.tools = append(c.tools, tool)
	if args["baseId"] != "fixture-base" || args["tableId"] != "fixture-table" {
		return nil, fmt.Errorf("unexpected scope: %v", args)
	}
	var text string
	switch tool {
	case "query_records":
		text = `{"records":[{"recordId":"fixture-row","cells":{"title":"nonempty value"}}],"hasMore":false}`
	case "get_fields":
		text = `{"fields":[{"fieldId":"title","fieldName":"Title","type":"text"}]}`
	default:
		return nil, fmt.Errorf("unexpected tool %s", tool)
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*aitableOutputCompatibilityCaller) Format() string { return "json" }
func (*aitableOutputCompatibilityCaller) DryRun() bool   { return false }
func (*aitableOutputCompatibilityCaller) Fields() string { return "" }
func (*aitableOutputCompatibilityCaller) JQ() string     { return "" }

// Exercise the real root output sink and shortcut together. The mock only
// replaces the remote transport; no output flags or lifecycle hooks are removed.
func TestCrossPlatformCoverageAITableRecordQueryPreservesGlobalOutput(t *testing.T) {
	for _, flag := range []string{"--output", "-o"} {
		for _, export := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/export=%t", flag, export), func(t *testing.T) {
				t.Chdir(t.TempDir())
				args := []string{"aitable", "+record-query", "--base-id", "fixture-base", "--table-id", "fixture-table", "--format", "json", flag, "result.json"}
				if export {
					args = append(args, "--all", "--export-output", "rows.ndjson")
				}
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				root := NewRootCommand()
				caller := &aitableOutputCompatibilityCaller{}
				helpers.InitDepsForTest(t, caller)
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				root.SetArgs(args)
				leaf, _, err := root.Find([]string{"aitable", "+record-query"})
				if err != nil {
					t.Fatal(err)
				}
				inherited := leaf.InheritedFlags().Lookup("output")
				if inherited == nil || inherited.Shorthand != "o" || leaf.LocalNonPersistentFlags().Lookup("output") != nil {
					t.Fatal("global output was shadowed")
				}
				if err = root.Execute(); err != nil {
					t.Fatalf("execute: %v; stderr=%s", err, stderr.String())
				}
				result, err := os.ReadFile(filepath.Join(".", "result.json"))
				if err != nil {
					t.Fatal(err)
				}
				wantTools := []string{"query_records"}
				if export {
					wantTools = append(wantTools, "get_fields")
					data, err := os.ReadFile("rows.ndjson")
					if err != nil {
						t.Fatal(err)
					}
					if string(data) != `{"cells":{"title":"nonempty value"},"recordId":"fixture-row"}`+"\n" {
						t.Fatalf("unexpected NDJSON: %s", data)
					}
					for _, part := range []string{`"recordCount": 1`, `"complete": true`, `"output": "rows.ndjson"`, fmt.Sprintf(`"sha256": "%x"`, sha256.Sum256(data))} {
						if !strings.Contains(string(result), part) {
							t.Fatalf("manifest missing %s: %s", part, result)
						}
					}
				} else {
					for _, part := range []string{`"recordId": "fixture-row"`, `"title": "nonempty value"`} {
						if !strings.Contains(string(result), part) {
							t.Fatalf("saved result missing %s: %s", part, result)
						}
					}
					if _, err := os.Stat("rows.ndjson"); !os.IsNotExist(err) {
						t.Fatalf("unexpected artifact: %v", err)
					}
				}
				if !reflect.DeepEqual(caller.tools, wantTools) {
					t.Fatalf("calls=%v want=%v", caller.tools, wantTools)
				}
				if stdout.Len() != 0 {
					t.Fatalf("global output leaked to stdout: %s", stdout.String())
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAITableRecordQueryRejectsOutputCollision(t *testing.T) {
	t.Chdir(t.TempDir())
	args := []string{"aitable", "+record-query", "--base-id", "fixture-base", "--table-id", "fixture-table", "--all", "--export-output", "rows.ndjson", "-o", "./rows.ndjson", "--format", "json"}
	testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
	root := NewRootCommand()
	caller := &aitableOutputCompatibilityCaller{}
	helpers.InitDepsForTest(t, caller)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "must target different files") {
		t.Fatalf("collision error: %v", err)
	}
	if len(caller.tools) != 0 {
		t.Fatalf("collision made RPCs: %v", caller.tools)
	}
	if _, err := os.Stat("rows.ndjson"); !os.IsNotExist(err) {
		t.Fatalf("collision published a file: %v", err)
	}
}
