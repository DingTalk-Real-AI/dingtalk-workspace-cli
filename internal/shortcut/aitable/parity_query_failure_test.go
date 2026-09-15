// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageParityQueryBoundariesAndViewRouting(t *testing.T) {
	for _, args := range [][]string{{"--export-output", "rows.ndjson"}, {"--max-records", "2"}, {"--all", "--max-records", "0"}} {
		c := &upsertByKeyCaller{}
		if _, err := runAITableCompositeCLI(t, c, "+record-query", append([]string{"--base-id", "b", "--table-id", "t"}, args...)...); err == nil || len(c.calls) != 0 {
			t.Fatal(args, err)
		}
	}
	c := &upsertByKeyCaller{}
	helpers.InitDepsForTest(t, c)
	cmd := &cobra.Command{Use: "query"}
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().String("cursor", "", "")
	_ = cmd.Flags().Set("all", "true")
	_ = cmd.Flags().Set("cursor", "x")
	if err := executeRecordQuery(shortcut.RuntimeContextForTest(cmd, RecordQuery), nil); err == nil || len(c.calls) != 0 {
		t.Fatal(err)
	}
	c = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"records":[{"recordId":"a"},{"recordId":"b"}],"hasMore":false}`}}}
	if _, err := runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--all", "--record-ids", "a,b", "--max-records", "1"); err == nil {
		t.Fatal("ignored result bound")
	}
	for _, sc := range []string{"+record-query", "+record-bulk-patch"} {
		for _, fail := range []bool{false, true} {
			steps := []upsertByKeyStep{{text: `{"views":[{"viewId":"v","viewType":"Grid","filter":[],"sort":[]}]}`}, {text: `{"records":[],"hasMore":false}`}}
			if fail {
				steps = []upsertByKeyStep{{err: fmt.Errorf("view unavailable")}}
			}
			c := &upsertByKeyCaller{steps: steps}
			args := []string{"--base-id", "b", "--table-id", "t", "--view-id", "v", "--all"}
			if sc == "+record-bulk-patch" {
				args = append(args, "--patch", `{"f":"value"}`, "--yes")
			}
			out, err := runAITableCompositeCLI(t, c, sc, args...)
			if (err != nil) != fail {
				t.Fatal(sc, out, err)
			}
			if !fail && c.calls[1].args["viewId"] != nil {
				t.Fatal("forwarded unsupported viewId")
			}
		}
	}
	c = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"success":true,"status":"success","data":{}}`}}}
	helpers.InitDepsForTest(t, c)
	cmd = &cobra.Command{Use: "query"}
	rt := shortcut.RuntimeContextForTest(cmd, RecordQuery)
	rows, err := queryAllRecords(rt, map[string]any{"cursor": "start", "limit": 1}, 10)
	if err != nil || len(rows) != 0 || c.calls[0].args["limit"] != 1 || c.calls[0].args["cursor"] != "start" {
		t.Fatal(rows, err, c.calls)
	}
}
func TestCrossPlatformCoverageParityFilteredViewFailureBoundaries(t *testing.T) {
	good := upsertByKeyStep{text: `{"views":[{"viewId":"v","viewType":"Grid","filter":[]}]}`}
	for _, steps := range [][]upsertByKeyStep{
		{{err: fmt.Errorf("read failed")}}, {{text: `{}`}}, {{text: `{"views":[{"viewId":"v"},{"viewId":"v"}]}`}},
		{good, {err: fmt.Errorf("write failed")}},
	} {
		c := &upsertByKeyCaller{steps: steps}
		out, err := runAITableCompositeCLI(t, c, "+view-update", "--base-id", "b", "--table-id", "t", "--view-id", "v", "--config", `{"filter":[]}`, "--yes")
		if err == nil || out != "" {
			t.Fatal(out, err)
		}
	}
	c := &upsertByKeyCaller{}
	out, err := runAITableCompositeCLI(t, c, "+view-update", "--base-id", "b", "--table-id", "t", "--view-id", "v", "--config", `{"filter":[]}`, "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err)
	}
	for _, v := range []any{nil, "unknown", map[string]any{"operator": "and", "operands": 1}} {
		if !reflect.DeepEqual(canonicalParityViewFilter(v), v) {
			t.Fatal("changed unknown filter", v)
		}
	}
	leaves := []any{map[string]any{"operator": "exist", "operands": []any{"a"}}, map[string]any{"operator": "exist", "operands": []any{"b"}}}
	if !reflect.DeepEqual(canonicalParityViewFilter(leaves), map[string]any{"operator": "and", "operands": leaves}) {
		t.Fatal("lost conjunction")
	}
}
func TestCrossPlatformCoverageParityAliasPreservesExistingFlagOwnership(t *testing.T) {
	for _, flags := range [][]shortcut.Flag{
		{{Name: "query", Type: shortcut.FlagString}, {Name: "keyword", Type: shortcut.FlagString}},
		{{Name: "query", Type: shortcut.FlagString}, {Name: "other", Type: shortcut.FlagString, Aliases: []string{"keyword"}}},
	} {
		s := withAITableParityAliases(shortcut.Shortcut{Command: "+custom", Flags: flags})
		if len(s.Flags[0].Aliases) != 0 {
			t.Fatal("alias claimed existing flag")
		}
	}
	// If a caller installs an incompatible flag type after declaration, alias
	// normalization must reject it instead of silently using the primary default.
	s := withAITableParityAliases(shortcut.Shortcut{Command: "+custom", Flags: []shortcut.Flag{{Name: "query", Type: shortcut.FlagString}}})
	cmd := &cobra.Command{Use: "custom"}
	cmd.Flags().Int("query", 0, "")
	cmd.Flags().String("keyword", "", "")
	_ = cmd.Flags().Set("keyword", "not-an-integer")
	if err := s.Validate(shortcut.RuntimeContextForTest(cmd, s)); err == nil {
		t.Fatal("invalid alias conversion ignored")
	}
}

func TestCrossPlatformCoverageParityDirectGuardsBeforeDispatch(t *testing.T) {
	c := &upsertByKeyCaller{}
	helpers.InitDepsForTest(t, c)
	cmd := &cobra.Command{Use: "guards"}
	cmd.Flags().String("name", "", "")
	_ = cmd.Flags().Set("name", "")
	rt := shortcut.RuntimeContextForTest(cmd, FieldCreate)
	op := parityAppOperation{command: "+app-page-create", flags: []shortcut.Flag{{Name: "name", Type: shortcut.FlagString}}}
	if err := executeParityApp(rt, op); err == nil {
		t.Fatal("empty app name")
	}
	if err := executeDashboardCreate(rt, false); err == nil {
		t.Fatal("empty dashboard name")
	}
	if err := executeParityForm(rt, "create"); err == nil {
		t.Fatal("empty form name")
	}
	cmd.Flags().StringSlice("resource-ids", nil, "")
	_ = cmd.Flags().Set("resource-ids", "")
	if err := executeAttachmentRemove(rt); err == nil {
		t.Fatal("empty removal selector")
	}
	cmd = &cobra.Command{Use: "remove"}
	rt = shortcut.RuntimeContextForTest(cmd, AttachmentRemove)
	if err := executeAttachmentRemove(rt); err == nil {
		t.Fatal("missing selector")
	}
	if len(c.calls) != 0 {
		t.Fatal("guard dispatched", c.calls)
	}
}

func TestCrossPlatformCoverageParityAliasesRetainOwningValidator(t *testing.T) {
	sentinel := fmt.Errorf("owning validator rejected value")
	called := false
	s := withAITableParityAliases(shortcut.Shortcut{Command: "+custom", Flags: []shortcut.Flag{{Name: "query", Type: shortcut.FlagString}}, Validate: func(rt *shortcut.RuntimeContext) error {
		called = true
		if rt.Str("query") != "value" {
			t.Fatal("validator ran before alias normalization")
		}
		return sentinel
	}})
	cmd := &cobra.Command{Use: "custom"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("keyword", "", "")
	_ = cmd.Flags().Set("keyword", "value")
	if err := s.Validate(shortcut.RuntimeContextForTest(cmd, s)); err != sentinel || !called {
		t.Fatal(err, called)
	}
}
