// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package sheet

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type sheetCoverageCaller struct {
	responses map[string][]string
	failures  map[string]error
	history   []string
	arguments []map[string]any
}

func (caller *sheetCoverageCaller) CallTool(_ context.Context, _, tool string, arguments map[string]any) (*edition.ToolResult, error) {
	caller.history = append(caller.history, tool)
	caller.arguments = append(caller.arguments, arguments)
	if err := caller.failures[tool]; err != nil {
		return nil, err
	}
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*sheetCoverageCaller) Format() string { return "json" }
func (*sheetCoverageCaller) DryRun() bool   { return false }
func (*sheetCoverageCaller) Fields() string { return "" }
func (*sheetCoverageCaller) JQ() string     { return "" }

func runSheetCoverage(t *testing.T, declaration shortcut.Shortcut, caller *sheetCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	service := &cobra.Command{Use: "sheet"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"sheet", declaration.Command}, args...))
	return root.Execute()
}

func TestCrossPlatformCoverageSheetListRejectsFalseEmptySuccess(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"empty response":        {},
		"missing success":       {"sheets": []any{}},
		"success false":         {"success": false, "errorMessage": "denied"},
		"missing collection":    {"success": true},
		"wrong collection type": {"success": true, "sheets": map[string]any{}},
		"bad element":           {"success": true, "sheets": []any{"bad"}},
		"empty element":         {"success": true, "sheets": []any{map[string]any{}}},
		"missing stable id":     {"success": true, "sheets": []any{map[string]any{"name": "Sheet1"}}},
		"wrong stable id type":  {"success": true, "sheets": []any{map[string]any{"sheetId": 1, "name": "Sheet1"}}},
		"missing title":         {"success": true, "sheets": []any{map[string]any{"sheetId": "s1"}}},
		"duplicate stable id":   {"success": true, "sheets": []any{map[string]any{"sheetId": "s1", "name": "One"}, map[string]any{"sheetId": "s1", "name": "Two"}}},
		"wrong optional index":  {"success": true, "sheets": []any{map[string]any{"sheetId": "s1", "name": "One", "sheetIndex": -1.0}}},
		"wrong optional count":  {"success": true, "sheets": []any{map[string]any{"sheetId": "s1", "name": "One", "rowCount": "many"}}},
		"wrong column count":    {"success": true, "sheets": []any{map[string]any{"sheetId": "s1", "name": "One", "colCount": 1.5}}},
		"empty visibility":      {"success": true, "sheets": []any{map[string]any{"sheetId": "s1", "name": "One", "visibility": " "}}},
		"wrong visibility":      {"success": true, "sheets": []any{map[string]any{"sheetId": "s1", "name": "One", "hidden": 1}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := projectSheetList(payload, ""); err == nil {
				t.Fatalf("payload unexpectedly accepted: %#v", payload)
			}
		})
	}

	payload := map[string]any{"success": true, "sheets": []any{
		map[string]any{"sheetId": "s1", "name": "Alpha"},
		map[string]any{"sheetId": "s2", "name": "Beta"},
	}}
	nonempty, err := projectSheetList(payload, "Alpha")
	if err != nil || len(nonempty) != 1 || nonempty[0]["sheetId"] != "s1" {
		t.Fatalf("known nonempty projection = %#v, err=%v", nonempty, err)
	}
	empty, err := projectSheetList(payload, "guaranteed-missing")
	if err != nil || len(empty) != 0 {
		t.Fatalf("guaranteed empty projection = %#v, err=%v", empty, err)
	}
	optionalPayload := map[string]any{"success": true, "sheets": []any{
		map[string]any{
			"sheetId": "s1", "title": "Alpha", "index": float64(0),
			"hidden": true, "row_count": float64(5), "colCount": float64(2),
		},
		map[string]any{"sheetId": "s2", "title": "Beta", "visibility": " visible "},
	}}
	optional, err := projectSheetList(optionalPayload, "")
	if err != nil || len(optional) != 2 {
		t.Fatalf("optional field projection=%#v err=%v", optional, err)
	}
	if optional[0]["index"] != float64(0) || optional[0]["visibility"] != true || optional[0]["rowCount"] != float64(5) || optional[0]["columnCount"] != float64(2) {
		t.Fatalf("numeric/bool optional projection=%#v", optional[0])
	}
	if optional[1]["visibility"] != "visible" {
		t.Fatalf("string visibility projection=%#v", optional[1])
	}
}

func TestCrossPlatformCoverageSheetReadRejectsMalformedAndIncompleteResults(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{
			"success": true, "cells": []any{[]any{map[string]any{"value": "x"}}},
			"colIndices": []any{"A"}, "rowIndices": []any{float64(1)},
			"hasMore": false, "truncationReasons": []any{},
			"resolvedRange": "A1:A1", "returnedRange": "A1:A1",
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing success":       func(v map[string]any) { delete(v, "success") },
		"success false":         func(v map[string]any) { v["success"] = false },
		"missing cells":         func(v map[string]any) { delete(v, "cells") },
		"wrong cells type":      func(v map[string]any) { v["cells"] = map[string]any{} },
		"bad row":               func(v map[string]any) { v["cells"] = []any{"bad"} },
		"bad cell":              func(v map[string]any) { v["cells"] = []any{[]any{"bad"}} },
		"missing columns":       func(v map[string]any) { delete(v, "colIndices") },
		"bad column":            func(v map[string]any) { v["colIndices"] = []any{1} },
		"missing rows":          func(v map[string]any) { delete(v, "rowIndices") },
		"bad row index":         func(v map[string]any) { v["rowIndices"] = []any{1.5} },
		"row count mismatch":    func(v map[string]any) { v["rowIndices"] = []any{} },
		"column count mismatch": func(v map[string]any) { v["cells"] = []any{[]any{}} },
		"missing hasMore":       func(v map[string]any) { delete(v, "hasMore") },
		"wrong hasMore type":    func(v map[string]any) { v["hasMore"] = "false" },
		"truncated": func(v map[string]any) {
			v["hasMore"] = true
			v["truncationReasons"] = []any{"max_cells"}
		},
		"truncated without reason": func(v map[string]any) {
			v["hasMore"] = true
			v["truncationReasons"] = []any{}
		},
		"missing truncation reasons": func(v map[string]any) { delete(v, "truncationReasons") },
		"wrong truncation reason":    func(v map[string]any) { v["truncationReasons"] = []any{1} },
		"contradictory completion":   func(v map[string]any) { v["truncationReasons"] = []any{"max_cells"} },
		"wrong optional field":       func(v map[string]any) { v["message"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			payload := valid()
			mutate(payload)
			if _, err := projectSheetRead(payload); err == nil {
				t.Fatalf("payload unexpectedly accepted: %#v", payload)
			}
		})
	}

	projected, err := projectSheetRead(valid())
	if err != nil || projected["complete"] != true || projected["hasMore"] != false {
		t.Fatalf("valid projection=%#v err=%v", projected, err)
	}
}

func TestCrossPlatformCoverageSheetShortcutsValidateBeforeCallsAndUseExactTools(t *testing.T) {
	noCall := &sheetCoverageCaller{responses: map[string][]string{}}
	if err := runSheetCoverage(t, ListSheets, noCall); err == nil || len(noCall.history) != 0 {
		t.Fatalf("missing node err=%v calls=%v", err, noCall.history)
	}
	for _, declaration := range []shortcut.Shortcut{ListSheets, Read} {
		t.Run(declaration.Command+" blank node", func(t *testing.T) {
			blank := &sheetCoverageCaller{responses: map[string][]string{}}
			err := runSheetCoverage(t, declaration, blank, "--node", "   ")
			if err == nil || !strings.Contains(err.Error(), "--node") || len(blank.history) != 0 {
				t.Fatalf("blank node err=%v calls=%v", err, blank.history)
			}
		})
	}
	for _, declaration := range []shortcut.Shortcut{ListSheets, Read} {
		blankCommand := corecmd.New(shortcut.FromShortcut(declaration))
		if err := blankCommand.Flags().Set("node", "   "); err != nil {
			t.Fatal(err)
		}
		err := declaration.Validate(shortcut.RuntimeContextForTest(blankCommand, declaration))
		if err == nil || !strings.Contains(err.Error(), "--node") {
			t.Fatalf("%s direct blank node validation=%v", declaration.Command, err)
		}
	}
	for _, test := range []struct {
		declaration shortcut.Shortcut
		flag        string
	}{
		{declaration: ListSheets, flag: "title"},
		{declaration: Read, flag: "sheet-id"},
		{declaration: Read, flag: "range"},
	} {
		t.Run(test.declaration.Command+" blank "+test.flag, func(t *testing.T) {
			caller := &sheetCoverageCaller{responses: map[string][]string{}}
			err := runSheetCoverage(t, test.declaration, caller, "--node", "node", "--"+test.flag, "   ")
			if err == nil || !strings.Contains(err.Error(), "--"+test.flag) || len(caller.history) != 0 {
				t.Fatalf("blank --%s err=%v calls=%v", test.flag, err, caller.history)
			}
		})
	}

	listCaller := &sheetCoverageCaller{responses: map[string][]string{
		"get_all_sheets": {`{"success":true,"sheets":[{"sheetId":"s1","name":"Alpha"}]}`},
	}}
	if err := runSheetCoverage(t, ListSheets, listCaller, "--node", "node", "--title", "Alpha"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(listCaller.history, ","); got != "get_all_sheets" {
		t.Fatalf("list history=%q", got)
	}

	readCaller := &sheetCoverageCaller{responses: map[string][]string{
		"get_cell_infos": {`{"success":true,"cells":[[{"value":"x"}]],"colIndices":["A"],"rowIndices":[1],"hasMore":false,"truncationReasons":[]}`},
	}}
	if err := runSheetCoverage(t, Read, readCaller,
		"--node", "node", "--sheet-id", "sheet-1", "--range", "A1:A1", "--value-render-option", "raw_value"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(readCaller.history, ","); got != "get_cell_infos" {
		t.Fatalf("read history=%q", got)
	}
	if len(readCaller.arguments) != 1 || readCaller.arguments[0]["nodeId"] != "node" || readCaller.arguments[0]["sheetId"] != "sheet-1" || readCaller.arguments[0]["range"] != "A1:A1" || readCaller.arguments[0]["valueRenderOption"] != "raw_value" {
		t.Fatalf("read arguments=%#v", readCaller.arguments)
	}

	dryRun := &sheetCoverageCaller{responses: map[string][]string{}}
	if err := runSheetCoverage(t, Read, dryRun,
		"--node", "node", "--sheet-id", "sheet-1", "--range", "A1:A1", "--value-render-option", "raw_value", "--dry-run"); err != nil {
		t.Fatalf("read dry-run: %v", err)
	}
	if len(dryRun.history) != 0 {
		t.Fatalf("read dry-run reached remote calls=%v", dryRun.history)
	}

	readFailure := &sheetCoverageCaller{
		responses: map[string][]string{},
		failures:  map[string]error{"get_cell_infos": errors.New("fixture transport failure")},
	}
	if err := runSheetCoverage(t, Read, readFailure, "--node", "node"); err == nil || len(readFailure.history) != 1 {
		t.Fatalf("read transport err=%v calls=%v", err, readFailure.history)
	}

	incomplete := &sheetCoverageCaller{responses: map[string][]string{
		"get_cell_infos": {`{"success":true,"cells":[[{"value":"x"}]],"colIndices":["A"],"rowIndices":[1],"hasMore":true,"truncationReasons":["max_cells"],"resolvedRange":"A1:A2","returnedRange":"A1:A1"}`},
	}}
	err := runSheetCoverage(t, Read, incomplete, "--node", "node")
	if err == nil || !strings.Contains(err.Error(), "截断") || len(incomplete.history) != 1 {
		t.Fatalf("incomplete err=%v calls=%v", err, incomplete.history)
	}
}

func TestCrossPlatformCoverageSheetRuntimeRejectsEmptyMalformedAndWrongCollection(t *testing.T) {
	for name, payload := range map[string]string{
		"empty body":       "",
		"malformed json":   "{",
		"success string":   `{"success":"true","sheets":[]}`,
		"missing array":    `{"success":true}`,
		"wrong array type": `{"success":true,"sheets":{}}`,
		"bad item":         `{"success":true,"sheets":["bad"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller := &sheetCoverageCaller{responses: map[string][]string{"get_all_sheets": {payload}}}
			err := runSheetCoverage(t, ListSheets, caller, "--node", "node")
			if err == nil || len(caller.history) != 1 {
				t.Fatalf("payload=%q err=%v calls=%v", payload, err, caller.history)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetPublicContractsAreReviewedAndUnified(t *testing.T) {
	for _, declaration := range []shortcut.Shortcut{ListSheets, Read} {
		if declaration.Contract.Empty() || declaration.Contract.Result == nil {
			t.Errorf("%s missing Contract.Result", declaration.Command)
		}
		if declaration.Contract.Pagination != nil {
			t.Errorf("%s publishes unsupported pagination", declaration.Command)
		}
		if declaration.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s rollout=%q", declaration.Command, declaration.OutputRollout)
		}
		if declaration.Safety.Confirmation != "not_required" || declaration.Safety.Effect != "read" {
			t.Errorf("%s safety=%+v", declaration.Command, declaration.Safety)
		}
	}
	if Read.Contract.DryRun == nil || Read.Contract.DryRun.PreviewKind != contract.DryRunPreviewRequest || Read.Contract.DryRun.RemoteReads {
		t.Errorf("read dry-run contract=%+v", Read.Contract.DryRun)
	}
	for _, test := range []struct {
		declaration    shortcut.Shortcut
		wantProperties map[string]string
	}{
		{declaration: ListSheets, wantProperties: map[string]string{"node": "node", "title": "title"}},
		{declaration: Read, wantProperties: map[string]string{"node": "node", "sheet-id": "sheetId", "range": "range", "value-render-option": "valueRenderOption"}},
	} {
		got := make(map[string]string, len(test.declaration.Contract.Parameters))
		for _, parameter := range test.declaration.Contract.Parameters {
			got[parameter.Name] = parameter.Property
		}
		if !reflect.DeepEqual(got, test.wantProperties) {
			t.Errorf("%s parameter properties=%v, want %v", test.declaration.Command, got, test.wantProperties)
		}
	}
	for _, test := range []struct {
		declaration shortcut.Shortcut
		sensitive   []string
		constraints []string
	}{
		{declaration: ListSheets, sensitive: []string{"sheets.sheetId", "sheets.title"}, constraints: []string{"node", "title"}},
		{declaration: Read, sensitive: []string{"cells"}, constraints: []string{"node", "sheet-id", "range"}},
	} {
		if !reflect.DeepEqual(test.declaration.Contract.Result.SensitivePaths, test.sensitive) {
			t.Errorf("%s sensitive paths=%v, want %v", test.declaration.Command, test.declaration.Contract.Result.SensitivePaths, test.sensitive)
		}
		gotConstraints := make([]string, 0, len(test.declaration.Constraints))
		for _, constraint := range test.declaration.Constraints {
			if constraint.Kind != shortcut.ConstraintCustom || len(constraint.Flags) != 1 || !strings.Contains(constraint.Description, "--"+constraint.Flags[0]) {
				t.Errorf("%s malformed custom constraint=%+v", test.declaration.Command, constraint)
				continue
			}
			gotConstraints = append(gotConstraints, constraint.Flags[0])
		}
		if !reflect.DeepEqual(gotConstraints, test.constraints) {
			t.Errorf("%s custom constraint flags=%v, want %v", test.declaration.Command, gotConstraints, test.constraints)
		}
	}
}
