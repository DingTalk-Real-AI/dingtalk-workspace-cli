// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"strings"
	"testing"
)

// TestAssembledSchemaContractReviewToolsAreDeprecationOnly guards the CR
// requirement: after full Schema assembly, review_* tools must not advertise
// retired MCP business capabilities (benefit/create/analysis/result).
func TestAssembledSchemaContractReviewToolsAreDeprecationOnly(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)

	wantTools := []string{
		"contract.review_benefit",
		"contract.review_create",
		"contract.review_analysis",
		"contract.review_result",
	}
	forbidden := []string{
		"查询合同审查权益",
		"创建合同审查任务，提交合同文件进行 AI 审查",
		"解析合同文件并返回摘要和审查推荐",
		"按任务 ID 查询合同审查结果",
		"用户要查看合同审查的权益额度或使用情况",
		"用户要对合同文件发起 AI 审查",
		"用户要解析合同文件获取摘要和审查建议",
		"用户已创建审查任务后要查询审查结果",
	}

	for _, canonical := range wantTools {
		tool, ok := snapshot.Tools[canonical]
		if !ok {
			t.Fatalf("assembled Schema missing review tool %q", canonical)
		}
		joined := strings.Join([]string{
			schemaContractString(tool["description"]),
			schemaContractString(tool["agent_summary"]),
			strings.Join(schemaContractStringSlice(tool["use_when"]), "\n"),
		}, "\n")
		if !strings.Contains(joined, "弃用") && !strings.Contains(joined, "兼容") {
			t.Fatalf("assembled %s must be deprecation/compat oriented:\n%s", canonical, joined)
		}
		for _, claim := range forbidden {
			if strings.Contains(joined, claim) {
				t.Fatalf("assembled %s still claims retired business capability %q:\n%s", canonical, claim, joined)
			}
		}
		for _, use := range schemaContractStringSlice(tool["use_when"]) {
			if !strings.Contains(use, "弃用兼容说明") {
				t.Fatalf("assembled %s UseWhen must be deprecation-notice only, got %q", canonical, use)
			}
		}
	}

	root := NewRootCommand()
	for _, path := range [][]string{
		{"contract", "review", "benefit"},
		{"contract", "review", "create"},
		{"contract", "review", "analysis"},
		{"contract", "review", "result"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil || cmd == nil {
			t.Fatalf("Find(%v): %v", path, err)
		}
		if cmd.Deprecated == "" || cmd.IsAvailableCommand() {
			t.Fatalf("%v must be Deprecated and not IsAvailableCommand (deprecated=%q available=%v)", path, cmd.Deprecated, cmd.IsAvailableCommand())
		}
	}
}
