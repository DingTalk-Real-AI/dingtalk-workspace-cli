// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// 跨 range 累计单元格预算（对齐飞书 checkBatchStampBudget）。
//
// 每个 "S!A1:Z1000" 是 26×1000=26000 格：单独看都在 buildStyleCells 的
// 单区域上限（rows≤1000、rows×cols≤30000）之内，所以只有累计预算能拦住它们。
// 7 个 = 182000 通过，8 个 = 208000 越过 200000 上限。
const budgetTestRange = "A1:Z1000"

func budgetTestRanges(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("Sheet%d!%s", i+1, budgetTestRange))
	}
	return out
}

func newBatchStyleCmdWithStyle(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := newRangeBatchSetStyleCmd()
	if err := cmd.Flags().Set("bg-color", "#FFF2CC"); err != nil {
		t.Fatalf("set --bg-color: %v", err)
	}
	return cmd
}

func TestBatchStyleRangesRespectAggregateCellBudget(t *testing.T) {
	// 7 个区域刚好在预算内：必须成功，且每个区域各产出一个 op。
	within, err := json.Marshal(budgetTestRanges(7))
	if err != nil {
		t.Fatal(err)
	}
	ops, err := buildBatchStyleOpsFromRanges(newBatchStyleCmdWithStyle(t), string(within))
	if err != nil {
		t.Fatalf("7 个区域（累计 182000 格）应在预算内: %v", err)
	}
	if len(ops) != 7 {
		t.Fatalf("operations = %d, want 7", len(ops))
	}

	// 第 8 个把累计推到 208000，越过 200000 上限。
	over, err := json.Marshal(budgetTestRanges(8))
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildBatchStyleOpsFromRanges(newBatchStyleCmdWithStyle(t), string(over))
	if err == nil {
		t.Fatal("8 个区域（累计 208000 格）必须被累计预算拒绝")
	}
	for _, want := range []string{"--ranges[7]", "208000", "200000"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want contains %q", err, want)
		}
	}
}

func TestBatchStyleFileRespectsAggregateCellBudgetAndItemCap(t *testing.T) {
	writeBatch := func(t *testing.T, items []map[string]any) string {
		t.Helper()
		data, err := json.Marshal(items)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "styles.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write batch fixture: %v", err)
		}
		return path
	}
	item := func(i int) map[string]any {
		return map[string]any{
			"sheetId":    fmt.Sprintf("Sheet%d", i+1),
			"range":      budgetTestRange,
			"fontWeight": "bold",
		}
	}

	// 累计预算：8 条 × 26000 = 208000 > 200000。
	overBudget := make([]map[string]any, 0, 8)
	for i := 0; i < 8; i++ {
		overBudget = append(overBudget, item(i))
	}
	_, err := buildBatchStyleOpsFromFile(writeBatch(t, overBudget))
	if err == nil {
		t.Fatal("--batch 累计 208000 格必须被拒绝")
	}
	for _, want := range []string{"第 8/8 条", "208000", "200000"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want contains %q", err, want)
		}
	}

	// 条数上限与 --ranges 对齐：101 条应在展开任何矩阵之前就被拒。
	tooMany := make([]map[string]any, 0, maxBatchStyleRanges+1)
	for i := 0; i < maxBatchStyleRanges+1; i++ {
		tooMany = append(tooMany, map[string]any{
			"sheetId": "Sheet1", "range": "A1", "fontWeight": "bold",
		})
	}
	_, err = buildBatchStyleOpsFromFile(writeBatch(t, tooMany))
	if err == nil || !strings.Contains(err.Error(), "--batch 最多 100 条") {
		t.Fatalf("error = %v, want --batch item cap", err)
	}

	// 预算内的正常配置仍然照常展开。
	ops, err := buildBatchStyleOpsFromFile(writeBatch(t, []map[string]any{item(0), item(1)}))
	if err != nil {
		t.Fatalf("2 条应在预算内: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("operations = %d, want 2", len(ops))
	}
}

func TestAddBatchStyleCellsAccumulatesAcrossCalls(t *testing.T) {
	var total int64
	if err := addBatchStyleCells(&total, 1000, 26); err != nil {
		t.Fatalf("first range: %v", err)
	}
	if total != 26000 {
		t.Fatalf("total = %d, want 26000", total)
	}
	// 单次就越界也必须拦住（累计器同时覆盖单区域巨量输入）。
	var single int64
	if err := addBatchStyleCells(&single, 1000, 1000); err == nil {
		t.Fatal("1000x1000=1000000 必须超出累计预算")
	}
}

// styleFlagNames 是手工维护的，必须与 bindStyleFlags 实际绑定的 flag 严格一致：
// 漏登记会让 --batch 的互斥检查放过一个静默失效的 flag。
func TestStyleFlagNamesMatchBoundFlags(t *testing.T) {
	probe := &cobra.Command{Use: "probe"}
	bindStyleFlags(probe)

	bound := map[string]bool{}
	probe.Flags().VisitAll(func(f *pflag.Flag) { bound[f.Name] = true })

	listed := map[string]bool{}
	for _, name := range styleFlagNames {
		if listed[name] {
			t.Fatalf("styleFlagNames 重复登记 %q", name)
		}
		listed[name] = true
		if !bound[name] {
			t.Errorf("styleFlagNames 登记了 %q，但 bindStyleFlags 没有绑定它", name)
		}
	}
	for name := range bound {
		if !listed[name] {
			t.Errorf("bindStyleFlags 绑定了 %q，但 styleFlagNames 漏登记（--batch 互斥检查会放过它）", name)
		}
	}
}

func TestBatchSetStyleRejectsStyleFlagsInBatchMode(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	batchPath := filepath.Join(t.TempDir(), "styles.json")
	if err := os.WriteFile(batchPath, []byte(`[{"sheetId":"Sheet1","range":"A1:B2","fontWeight":"bold"}]`), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}

	caller := &sheetStyleDryRunCaller{format: "json"}
	InitDeps(caller)

	cmd := newRangeBatchSetStyleCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--node", "NODE_ID", "--batch", batchPath, "--bg-color", "#FFF2CC", "--font-family", "Arial"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("--batch 搭配命令行样式 flag 必须报错，而不是静默忽略")
	}
	for _, want := range []string{"--batch 模式下样式来自配置文件", "--bg-color", "--font-family"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want contains %q", err, want)
		}
	}
	if caller.calls != 0 {
		t.Fatalf("remote CallTool count = %d, want 0", caller.calls)
	}
}

// buildBatchStyleOpsFromRanges 的每条前置校验都必须在组装 operations 之前拦住。
func TestBatchStyleRangesRejectsInvalidInput(t *testing.T) {
	tooMany := make([]string, 0, maxBatchStyleRanges+1)
	for i := 0; i <= maxBatchStyleRanges; i++ {
		tooMany = append(tooMany, `"Sheet1!A1"`)
	}

	for _, tc := range []struct {
		name   string
		ranges string
		style  bool
		want   string
	}{
		{name: "bad-json", ranges: `[`, style: true, want: "--ranges JSON 解析失败"},
		{name: "empty-array", ranges: `[]`, style: true, want: "--ranges 不能为空数组"},
		{
			name:   "over-range-cap",
			ranges: "[" + strings.Join(tooMany, ",") + "]",
			style:  true,
			want:   "--ranges 最多 100 项",
		},
		{
			name:   "missing-sheet-prefix",
			ranges: `["A1:B2"]`,
			style:  true,
			want:   "必须包含工作表前缀",
		},
		{
			name:   "blank-sheet-prefix",
			ranges: `[" !A1:B2"]`,
			style:  true,
			want:   "必须包含工作表前缀",
		},
		{
			name:   "blank-range-after-prefix",
			ranges: `["Sheet1! "]`,
			style:  true,
			want:   "必须包含工作表前缀",
		},
		{
			name:   "bad-range-address",
			ranges: `["Sheet1!zz"]`,
			style:  true,
			want:   "解析失败",
		},
		{
			name:   "no-style-flag",
			ranges: `["Sheet1!A1:B2"]`,
			style:  false,
			want:   "至少需要指定一个样式参数",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRangeBatchSetStyleCmd()
			if tc.style {
				if err := cmd.Flags().Set("bg-color", "#FFF2CC"); err != nil {
					t.Fatalf("set --bg-color: %v", err)
				}
			}
			_, err := buildBatchStyleOpsFromRanges(cmd, tc.ranges)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
		})
	}
}

// 空白工作表标识必须在下发之前被拒绝 —— 三条入口共用同一条不变量。
//
// 只按原始串里 ! 的位置判断，" !A1:B2" 会拆出空工作表名，操作却照样带着
// sheetId:"" 提交：服务端要么让整批 batch_update 失败，要么落到默认工作表而不是
// 用户指定的那张表（后者更糟，因为命令报成功）。--batch 的纯空白 sheetId 同理，
// 它此前只挡 == ""。三条路径都必须在发请求之前失败。
func TestBlankSheetIdentifierIsRejectedBeforeAnyRemoteCall(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	batchDir := t.TempDir()
	writeBatch := func(t *testing.T, name, body string) string {
		t.Helper()
		p := filepath.Join(batchDir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write batch fixture: %v", err)
		}
		return p
	}

	for _, tc := range []struct {
		name string
		cmd  func() *cobra.Command
		args func(t *testing.T) []string
		want string
	}{
		{
			name: "set-style-ranges-blank-sheet",
			cmd:  newRangeBatchSetStyleCmd,
			args: func(*testing.T) []string {
				return []string{"--node", "NODE_ID", "--ranges", `[" !A1:B2"]`, "--bg-color", "#FFF2CC"}
			},
			want: "必须包含工作表前缀",
		},
		{
			name: "set-style-batch-blank-sheet-id",
			cmd:  newRangeBatchSetStyleCmd,
			args: func(t *testing.T) []string {
				p := writeBatch(t, "blank-sheet.json", `[{"sheetId":"   ","range":"A1:B2","fontWeight":"bold"}]`)
				return []string{"--node", "NODE_ID", "--batch", p}
			},
			want: "缺少 sheetId 或 range",
		},
		{
			name: "set-style-batch-blank-range",
			cmd:  newRangeBatchSetStyleCmd,
			args: func(t *testing.T) []string {
				p := writeBatch(t, "blank-range.json", `[{"sheetId":"Sheet1","range":"  ","fontWeight":"bold"}]`)
				return []string{"--node", "NODE_ID", "--batch", p}
			},
			want: "缺少 sheetId 或 range",
		},
		{
			name: "batch-clear-ranges-blank-sheet",
			cmd:  rangeBatchClearCoverageCommand,
			args: func(*testing.T) []string {
				return []string{"--node", "NODE_ID", "--ranges", `[" !A1:B2"]`}
			},
			want: "必须包含工作表前缀",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &sheetStyleDryRunCaller{format: "json"}
			InitDeps(caller)
			deps.Out.w = io.Discard
			deps.Out.errW = io.Discard

			cmd := tc.cmd()
			cmd.SetArgs(tc.args(t))
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("remote CallTool count = %d, want 0", caller.calls)
			}
		})
	}
}

// 判空看修剪后的值，但下发仍用原值：sheetId 可以是工作表**名**，名字允许带首尾
// 空格，替用户修剪会指向另一张表。--batch 是需要精确名字时的入口，必须原样透传。
func TestBatchStyleSheetIDIsSentVerbatimNotTrimmed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "padded.json")
	if err := os.WriteFile(p, []byte(`[{"sheetId":" Sheet1 ","range":" A1:B2 ","fontWeight":"bold"}]`), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}

	ops, err := buildBatchStyleOpsFromFile(p)
	if err != nil {
		t.Fatalf("buildBatchStyleOpsFromFile = %v, want accepted", err)
	}
	if len(ops) != 1 {
		t.Fatalf("operations = %d, want 1", len(ops))
	}
	op, _ := ops[0].(map[string]any)
	input, _ := op["input"].(map[string]any)
	if input["sheetId"] != " Sheet1 " {
		t.Fatalf("sheetId = %#v, want %q (verbatim)", input["sheetId"], " Sheet1 ")
	}
	if input["rangeAddress"] != " A1:B2 " {
		t.Fatalf("rangeAddress = %#v, want %q (verbatim)", input["rangeAddress"], " A1:B2 ")
	}
}
