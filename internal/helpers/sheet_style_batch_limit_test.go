// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type batchStyleLimitCaller struct {
	calls int
	tool  string
	args  map[string]any
}

func (c *batchStyleLimitCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	c.tool, c.args = tool, args
	return &edition.ToolResult{}, nil
}

func (*batchStyleLimitCaller) Format() string { return "json" }
func (*batchStyleLimitCaller) DryRun() bool   { return false }
func (*batchStyleLimitCaller) Fields() string { return "" }
func (*batchStyleLimitCaller) JQ() string     { return "" }

// Exercise the prepared command and its actual dispatch boundary with a recording
// caller. Literal counts keep the server's 20-operation limit independent of the
// implementation constant, including when continue-on-error is requested.
func TestCrossPlatformCoverageBatchStyleOperationLimit(t *testing.T) {
	for _, mode := range []string{"ranges", "batch"} {
		for _, count := range []int{1, 20, 21, 100, 101} {
			for _, continueOnError := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/continue=%t", mode, count, continueOnError), func(t *testing.T) {
					caller := &batchStyleLimitCaller{}
					testseam.Swap(t, &deps, &Deps{
						Caller: caller,
						Out:    NewFormatterWithWriters(io.Discard, io.Discard),
					})
					args := []string{"--node", "NODE_ID"}
					if mode == "ranges" {
						ranges := make([]string, count)
						for i := range ranges {
							ranges[i] = fmt.Sprintf("Sheet%d!A%d", i%2+1, i+1)
						}
						data, err := json.Marshal(ranges)
						if err != nil {
							t.Fatal(err)
						}
						args = append(args, "--ranges", string(data), "--font-weight", "bold")
					} else {
						items := make([]map[string]any, count)
						for i := range items {
							items[i] = map[string]any{
								"sheetId": fmt.Sprintf("Sheet%d", i%2+1), "range": fmt.Sprintf("A%d", i+1), "fontWeight": "bold",
							}
						}
						data, err := json.Marshal(items)
						if err != nil {
							t.Fatal(err)
						}
						path := filepath.Join(t.TempDir(), "styles.json")
						if err := os.WriteFile(path, data, 0o600); err != nil {
							t.Fatal(err)
						}
						args = append(args, "--batch", path)
					}
					if continueOnError {
						args = append(args, "--continue-on-error")
					}
					cmd := newRangeBatchSetStyleCmd()
					cmd.SetOut(io.Discard)
					cmd.SetErr(io.Discard)
					cmd.SilenceUsage, cmd.SilenceErrors = true, true
					cmd.SetArgs(args)
					err := corecmd.ExecuteForTest(cmd)
					if count > 20 {
						var typed *apperrors.Error
						if !stderrors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || apperrors.ExitCode(err) != 3 {
							t.Fatalf("error = %v, want validation / exit 3", err)
						}
						for _, want := range []string{"--" + mode, "最多 20", fmt.Sprintf("当前 %d", count), "拆成多次调用"} {
							if !strings.Contains(err.Error(), want) {
								t.Errorf("error = %v, want %q", err, want)
							}
						}
						if caller.calls != 0 {
							t.Fatalf("CallTool count = %d, want 0", caller.calls)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if caller.calls != 1 || caller.tool != "batch_update" || caller.args["nodeId"] != "NODE_ID" {
						t.Fatalf("dispatch = %d, %s, %#v; want one batch_update", caller.calls, caller.tool, caller.args)
					}
					if value, present := caller.args["continueOnError"]; present != continueOnError || (present && value != true) {
						t.Fatalf("continueOnError = %#v (present=%t), want %t", value, present, continueOnError)
					}
					operations := decodeBatchOperationsJSON(t, caller.args)
					if len(operations) != count {
						t.Fatalf("operations = %d, want %d", len(operations), count)
					}
					for i, raw := range operations {
						op := raw.(map[string]any)
						input := op["input"].(map[string]any)
						if op["toolName"] != "set_cell_range" || input["sheetId"] != fmt.Sprintf("Sheet%d", i%2+1) || input["rangeAddress"] != fmt.Sprintf("A%d", i+1) {
							t.Fatalf("operation %d = %#v; order or target changed", i, op)
						}
					}
				})
			}
		}
	}
}
