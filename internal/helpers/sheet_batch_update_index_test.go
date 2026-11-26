// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageSheetBatchUpdateIndexRestrictionIsBatchOnly(t *testing.T) {
	for _, tc := range []struct {
		flag     string
		property string
		value    any
	}{
		{"name", "title", "renamed"},
		{"title", "title", "renamed"},
		{"hidden", "hidden", false},
		{"frozen-row-count", "frozenRowCount", 0},
		{"frozen-column-count", "frozenColumnCount", 0},
		{"tab-color", "tabColor", ""},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			input := map[string]any{"sheet-id": "stable-sheet-id", "index": 0, tc.flag: tc.value}
			standalone, err := BuildUpdateSheetArgs(input)
			if err != nil {
				t.Fatalf("standalone combination rejected: %v", err)
			}
			want := map[string]any{"sheetId": "stable-sheet-id", "index": 0, tc.property: tc.value}
			if !reflect.DeepEqual(standalone, want) {
				t.Fatalf("standalone args = %#v, want %#v", standalone, want)
			}
			_, err = BuildBatchUpdateSheetArgs(input)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != apperrors.ExitCodeValidation {
				t.Fatalf("batch error = %v, want validation / exit 3", err)
			}
			for _, required := range []string{"index 只能单独更新", tc.flag, "先更新其他属性，再单独更新 index", "相同、稳定的 sheet-id"} {
				if !strings.Contains(err.Error(), required) {
					t.Fatalf("batch error %q missing %q", err, required)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageSheetBatchUpdateMixedIndexFailsBeforeRemoteCall(t *testing.T) {
	operations := `[{"toolName":"update","input":{"sheet-id":"stable-sheet-id","hidden":false}},{"toolName":"update","input":{"sheet-id":"stable-sheet-id","index":0,"frozen-row-count":2}}]`
	for _, mode := range []string{"strict", "continue-on-error"} {
		t.Run(mode, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			args := []string{"--node", "node", "--operations", operations}
			if mode == "continue-on-error" {
				args = append(args, "--continue-on-error")
			}
			err := executeSheetBatchCommand(t, caller, batchUpdateCoverageCommand(), args...)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != apperrors.ExitCodeValidation {
				t.Fatalf("wrapped error = %v, want validation / exit 3", err)
			}
			if !strings.Contains(err.Error(), "operations[1] 翻译失败: update:") {
				t.Fatalf("original operation index lost: %v", err)
			}
			if caller.calls != 0 {
				t.Fatalf("invalid batch made %d remote calls", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetBatchUpdateSplitIndexPreservesOrderAndValues(t *testing.T) {
	caller := &scriptedToolCaller{}
	operations := `[{"toolName":"update","input":{"sheet-id":"stable-sheet-id","name":"renamed","hidden":false,"frozen-row-count":0,"frozen-column-count":0,"tab-color":""}},{"toolName":"update","input":{"sheet-id":"stable-sheet-id","index":0}}]`
	if err := executeSheetBatchCommand(t, caller, batchUpdateCoverageCommand(), "--node", "node", "--operations", operations); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 || caller.tool != "batch_update" {
		t.Fatalf("batch calls = %d, tool = %q", caller.calls, caller.tool)
	}
	want := []any{
		map[string]any{
			"toolName": "update_sheet",
			"input": map[string]any{
				"sheetId": "stable-sheet-id", "title": "renamed", "hidden": false,
				"frozenRowCount": json.Number("0"), "frozenColumnCount": json.Number("0"), "tabColor": "",
			},
		},
		map[string]any{
			"toolName": "update_sheet",
			"input":    map[string]any{"sheetId": "stable-sheet-id", "index": json.Number("0")},
		},
	}
	if got := decodeBatchOperationsJSON(t, caller.args); !reflect.DeepEqual(got, want) {
		t.Fatalf("translated batch = %#v, want %#v", got, want)
	}
}
