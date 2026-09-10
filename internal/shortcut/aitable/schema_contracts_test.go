// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageAITableLegacyContractLedgerIsExact(t *testing.T) {
	if got, want := len(reviewedAITableShortcutContractCommands), 53; got != want {
		t.Fatalf("legacy contract migration ledger = %d, want %d", got, want)
	}
	for command := range reviewedAITableShortcutContractCommands {
		found := false
		for _, item := range shortcut.All() {
			if item.Service == "aitable" && item.Command == command {
				found = true
				if item.Contract.Empty() {
					t.Errorf("%s: migration did not deliver Contract", command)
				}
				if item.Safety.Confirmation == "" {
					t.Errorf("%s: migration did not deliver explicit Safety", command)
				}
				break
			}
		}
		if !found {
			t.Errorf("stale migration ledger entry %s", command)
		}
	}
}

func TestCrossPlatformCoverageRecordQueryContractGuidesPaginationAndValueNormalization(t *testing.T) {
	item := RecordQuery
	for _, required := range []string{
		"单页行数据",
		"nextCursor 显式续页",
		"完整读取全表时不要使用本 Shortcut",
		"--all --page-limit 0",
		"不是同一结果模型",
		"禁止相互拼接、转换或混合推导",
	} {
		if !strings.Contains(item.Intent, required) {
			t.Errorf("RecordQuery intent missing %q", required)
		}
	}
	selection := item.Contract.Selection
	for _, required := range []string{
		"字段和值必须先按字段类型解析",
		"需要全部、完整、汇总、统计、导出或逐条处理全表数据",
		"不要手写 cursor 循环或把当前页当全量",
	} {
		if !strings.Contains(selection.AgentSummary, required) && !containsAny(selection.UseWhen, required) && !containsAny(selection.AvoidWhen, required) {
			t.Errorf("RecordQuery selection missing %q", required)
		}
	}
	flags := map[string]string{}
	for _, flag := range item.Flags {
		flags[flag.Name] = flag.Desc
	}
	for name, required := range map[string]string{
		"field-ids": "不要传字段中文名",
		"filters":   "禁止原值透传",
		"sort":      "direction 仅用 asc/desc",
		"cursor":    "成功返回空续页属于正常情况",
	} {
		if !strings.Contains(flags[name], required) {
			t.Errorf("RecordQuery flag %s missing %q in %q", name, required, flags[name])
		}
	}
	for _, required := range []string{"records 为空时仍以 nextCursor 是否为空判断", "不得复用旧 cursor 或自行构造"} {
		if !strings.Contains(flags["cursor"], required) {
			t.Errorf("RecordQuery cursor description missing %q in %q", required, flags["cursor"])
		}
	}
	filters := flags["filters"]
	for _, required := range []string{"完整读一遍表头", "aisearch person", "contact +resolve-dept", "chat +chat-search", "结构化 ID 数组"} {
		if !strings.Contains(filters, required) {
			t.Errorf("RecordQuery filters description missing %q in %q", required, filters)
		}
	}
}

func containsAny(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
