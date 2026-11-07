// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageRetiredRecruitProductAbsent(t *testing.T) {
	root := NewRootCommand()
	for _, command := range root.Commands() {
		if command.Name() == "recruit" {
			t.Fatal("开源命令树仍挂载招聘产品")
		}
	}
	if DirectRuntimeProductIDs()["recruit"] {
		t.Fatal("开源运行时仍注册招聘产品")
	}
	for _, server := range edition.Get().SupplementServers() {
		if server.ID == "recruit" {
			t.Fatal("开源发行版仍内置招聘 MCP 端点")
		}
	}

	snapshot := fullSchemaSnapshotForTest(t)
	for _, product := range snapshot.Catalog["products"].([]map[string]any) {
		if product["id"] == "recruit" {
			t.Fatal("开源 Schema 仍交付招聘产品")
		}
	}
	for canonical := range snapshot.Tools {
		if strings.HasPrefix(canonical, "recruit.") {
			t.Errorf("开源 Schema 仍交付招聘工具 %q", canonical)
		}
	}
}
