// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type sheetStyleDryRunCaller struct {
	format string
	calls  int
}

func (c *sheetStyleDryRunCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{}, nil
}

func (c *sheetStyleDryRunCaller) Format() string { return c.format }
func (*sheetStyleDryRunCaller) DryRun() bool     { return true }
func (*sheetStyleDryRunCaller) Fields() string   { return "" }
func (*sheetStyleDryRunCaller) JQ() string       { return "" }

func TestCrossPlatformCoverageRangeBatchSetStyleDryRunNeverCallsRemote(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	batchPath := filepath.Join(t.TempDir(), "styles.json")
	if err := os.WriteFile(batchPath, []byte(`[{"sheetId":"Sheet1","range":"A1:B2","fontWeight":"bold"}]`), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}

	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			caller := &sheetStyleDryRunCaller{format: format}
			InitDeps(caller)
			var output bytes.Buffer
			deps.Out.w = &output
			deps.Out.errW = &output

			cmd := newRangeBatchSetStyleCmd()
			cmd.SetArgs([]string{"--node", "NODE_ID", "--batch", batchPath})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("batch-set-style dry-run error: %v", err)
			}
			if caller.calls != 0 {
				t.Fatalf("remote CallTool count = %d, want 0", caller.calls)
			}
			preview := output.String()
			if format == "json" {
				var payload map[string]any
				if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
					t.Fatalf("JSON dry-run stdout must be one document: %v\n%s", err, preview)
				}
				// batch-set-style 现在组装为一次 batch_update 原子提交，
				// dry-run 因此是单条标准记录，而不是本地循环产生的 results 数组。
				if payload["tool"] != "batch_update" {
					t.Fatalf("JSON dry-run tool = %#v, want batch_update", payload["tool"])
				}
				args, _ := payload["arguments"].(map[string]any)
				ops := decodeBatchOperationsJSON(t, args)
				if len(ops) != 1 {
					t.Fatalf("JSON dry-run operationsJson = %#v, want 1 item", args["operationsJson"])
				}
				op, _ := ops[0].(map[string]any)
				if op["toolName"] != "set_cell_range" {
					t.Fatalf("JSON dry-run operation toolName = %#v", op["toolName"])
				}
			} else {
				for _, want := range []string{"Tool:", "batch_update", "set_cell_range", "Arguments:"} {
					if !strings.Contains(preview, want) {
						t.Fatalf("dry-run preview missing %q:\n%s", want, preview)
					}
				}
			}
		})
	}
}

func TestRangeBatchSetStylePropagatesJSONWriteFailure(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	batchPath := filepath.Join(t.TempDir(), "styles.json")
	if err := os.WriteFile(batchPath, []byte(`[{"sheetId":"Sheet1","range":"A1:B2","fontWeight":"bold"}]`), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}

	caller := &sheetStyleDryRunCaller{format: "json"}
	InitDeps(caller)
	deps.Out.w = forcedJSONWriteFailure{}

	cmd := newRangeBatchSetStyleCmd()
	cmd.SetArgs([]string{"--node", "NODE_ID", "--batch", batchPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "forced JSON output failure") {
		t.Fatalf("error = %v, want JSON write failure", err)
	}
	if caller.calls != 0 {
		t.Fatalf("remote CallTool count = %d, want 0", caller.calls)
	}
}

// 边框每条边只认 style / color。写错的键（colour）或类型不对的 color 此前被静默
// 忽略：命令报成功，却画出一条没有颜色的边框。这与 --sheets / --styles 拒绝未知键
// 是同一条不变量，三条使用路径（单区域 flag、batch、create-with-data --styles）共用
// parseBorderStyles，所以核心在这里一次验完，再逐路径确认拒绝发生在发请求之前。
func TestParseBorderStylesRejectsUnknownAndMistypedEdgeFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want string
	}{
		{"near-miss-color-key", `{"top":{"style":"solid","colour":"#f00"}}`, `未知字段 "colour"`},
		{"unknown-key", `{"top":{"style":"solid","width":2}}`, `未知字段 "width"`},
		{"camel-case-alias-hinted", `{"top":{"style":"solid","Color":"#f00"}}`, `应为 "color"`},
		{"color-not-string", `{"top":{"style":"solid","color":123}}`, "color 必须是字符串，实际是 float64"},
		{"color-empty-string", `{"top":{"style":"solid","color":""}}`, "color 不能为空字符串"},
		{"style-not-string", `{"top":{"style":123}}`, "style 必须是字符串，实际是 float64"},
		{"style-empty-string", `{"top":{"style":""}}`, "缺少 style"},
		{"style-missing", `{"top":{"color":"#f00"}}`, "缺少 style"},
		{"style-null", `{"top":{"style":null}}`, "缺少 style"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseBorderStyles(tc.json)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if err != nil && !strings.Contains(err.Error(), "--border-styles-json.top") {
				t.Fatalf("err = %v, want the edge name in the message", err)
			}
		})
	}

	// 合法形态不能被误拒：只给 style、给 style+color、显式 null color（等同省略）
	for _, tc := range []struct {
		name      string
		json      string
		wantColor any
	}{
		{"style-only", `{"top":{"style":"solid"}}`, nil},
		{"style-and-color", `{"top":{"style":"solid","color":"#FF0000"}}`, "#FF0000"},
		{"explicit-null-color", `{"top":{"style":"solid","color":null}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := parseBorderStyles(tc.json)
			if err != nil {
				t.Fatalf("parseBorderStyles(%s) = %v, want accepted", tc.json, err)
			}
			edge, _ := out["top"].(map[string]any)
			if edge["style"] != "solid" {
				t.Fatalf("style = %#v, want solid", edge["style"])
			}
			if edge["color"] != tc.wantColor {
				t.Fatalf("color = %#v, want %#v", edge["color"], tc.wantColor)
			}
		})
	}
}

// 单区域与 batch 两条路径：非法边框配置必须在组装/下发之前失败。
func TestBorderStyleRejectionHappensBeforeAnyRemoteCall(t *testing.T) {
	const badBorder = `{"top":{"style":"solid","colour":"#f00"}}`
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	batchPath := filepath.Join(t.TempDir(), "styles.json")
	fixture := `[{"sheetId":"Sheet1","range":"A1:B2","borderStylesJson":"{\"top\":{\"style\":\"solid\",\"colour\":\"#f00\"}}"}]`
	if err := os.WriteFile(batchPath, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"single-range", []string{"--node", "NODE_ID", "--sheet-id", "S", "--range", "A1:B2", "--border-styles-json", badBorder}},
		{"batch-ranges", []string{"--node", "NODE_ID", "--ranges", `["Sheet1!A1:B2"]`, "--border-styles-json", badBorder}},
		{"batch-file", []string{"--node", "NODE_ID", "--batch", batchPath}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &sheetStyleDryRunCaller{format: "json"}
			InitDeps(caller)
			var output bytes.Buffer
			deps.Out.w = &output
			deps.Out.errW = &output

			cmd := newRangeSetStyleCmd()
			if tc.name != "single-range" {
				cmd = newRangeBatchSetStyleCmd()
			}
			cmd.SetArgs(tc.args)
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), `未知字段 "colour"`) {
				t.Fatalf("err = %v, want the unknown border field rejected", err)
			}
			if caller.calls != 0 {
				t.Fatalf("remote CallTool count = %d, want 0", caller.calls)
			}
		})
	}
}
