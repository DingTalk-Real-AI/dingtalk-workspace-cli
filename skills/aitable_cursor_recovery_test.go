// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"bytes"
	"strings"
	"testing"
)

// Keep both distributions aligned with the runtime's fail-closed recovery
// fields, rather than allowing generic resume guidance to hide invalidation.
func TestCrossPlatformCoverageAITableCursorRecoveryReferences(t *testing.T) {
	paths := []string{
		"mono/references/products/aitable/aitable-record-query.md",
		"multi/dingtalk-aitable/references/aitable/aitable-record-query.md",
	}
	var previous []byte
	for _, path := range paths {
		data, err := FS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && !bytes.Equal(previous, data) {
			t.Fatal("mono/multi record pagination contracts diverge")
		}
		previous = data
		review, _, found := strings.Cut(string(data), "## 命令格式")
		if !found {
			t.Fatalf("%s must distinguish review guidance from execution recipes", path)
		}
		for _, boundary := range []string{
			"## 只评审失效游标恢复", "只查询一次其 compact Schema",
			"CURSOR_SNAPSHOT_CHANGED", "CURSOR_SNAPSHOT_UNAVAILABLE",
			"必须先等待服务端版本信息恢复", "不能整条重放",
			"保留已知目标表、recordId、原 token 和回执",
			"未知或不完整写入尚未核清前", "不建议改发 `record create`",
			"不套用下方实际查询流程",
		} {
			if !strings.Contains(review, boundary) {
				t.Errorf("%s omits review safety boundary before execution recipes: %s", path, boundary)
			}
		}
		for _, marker := range []string{
			"INVALID_CURSOR", "CURSOR_SNAPSHOT_CHANGED", "CURSOR_SNAPSHOT_UNAVAILABLE",
			"retryable=false", "discard_previous_results=true", "restart_from_first_page=true",
		} {
			if !strings.Contains(string(data), "`"+marker+"`") {
				t.Errorf("%s omits runtime recovery contract %s", path, marker)
			}
		}
	}
	root, err := FS.ReadFile("multi/dingtalk-aitable/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(root), "references/aitable/aitable-record-query.md") {
		t.Fatal("record pagination recovery reference is not discoverable")
	}
}
