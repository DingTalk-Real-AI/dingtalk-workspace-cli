// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import "strings"

// PR #1485 经管理员批准整体退役 recruit，合并提交：
// 9a2d37268c62ba0790e83c48da5119d6afa67ea5。
// 这是已完成退役的精确回执，不是任意产品删除的审批机制。authoritative
// wrapper 从 merge-base 复制本包；候选分支修改此回执不能自行获得豁免。
func reviewedRecruitRetirement(current, baseline Snapshot) bool {
	expected := map[string]bool{
		"dws recruit":            true,
		"dws recruit job":        true,
		"dws recruit job list":   true,
		"dws recruit job get":    true,
		"dws recruit job create": true,
	}
	// 包括其他命令的别名：只要招聘路径仍可接受，就不属于整体退役。
	for path := range acceptedPathIndex(current) {
		if path == "dws recruit" || strings.HasPrefix(path, "dws recruit ") {
			return false
		}
	}
	for _, command := range baseline.Commands {
		if !(command.Path == "dws recruit" || strings.HasPrefix(command.Path, "dws recruit ")) {
			continue
		}
		if !expected[command.Path] || !command.Runnable || command.Hidden || len(command.Aliases) != 0 {
			return false
		}
		delete(expected, command.Path)
	}
	return len(expected) == 0
}

// IsReviewedRecruitTool 精确限定同一退役回执中的三个 Schema 工具和 CLI 路径。
// Schema 检查器同样由 merge-base 编译，不使用候选分支中的回执。
func IsReviewedRecruitTool(toolID, cliPath string) bool {
	paths := map[string]string{
		"recruit.list_jobs":      "recruit job list",
		"recruit.get_job_detail": "recruit job get",
		"recruit.create_job":     "recruit job create",
	}
	path, ok := paths[toolID]
	return ok && path == cliPath
}
