// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package agentmetadata

import "testing"

func TestGenerateUsesMCPDescriptionsAsUnreviewedFallback(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "skills/mono/SKILL.md", "# DWS\n## 意图判断决策树\n用户提到\"日程\" -> `calendar`\n")
	writeFixture(t, root, "skills/mono/references/intent-guide.md", "# Intent guide\n")
	writeFixture(t, root, "skills/mono/references/products/calendar.md", "# Calendar\n")
	writeFixture(t, root, "internal/cli/schema_hints/imported/wukong.json", `{
  "version": 1,
  "source": {"kind": "imported", "name": "dws-wukong", "revision": "1234567890abcdef"},
  "tools": {
    "calendar.get_calendar": {"agent_summary": "读取指定日历"}
  }
}`)
	writeFixture(t, root, "internal/cli/schema_mcp_metadata.json", `{
  "version": 1,
  "source": "mcp-tools-list+cli-registry",
  "source_revision": "abcdef1234567890",
  "source_hash": "sha256:interface",
  "tools": {
    "calendar.get_calendar": {"description": "MCP 原始读取描述。"},
    "calendar.list_calendars": {
      "description": "列出当前用户可访问的日历。后续句子不应进入 summary。",
      "interface_ref": {"product_id": "calendar", "rpc_name": "list_calendars"}
    },
    "calendar.raw_tool": {"description": "raw_tool_name"},
    "outside.tool": {"description": "不在公开命令面"}
  }
}`)

	metadata, stats, err := generateFromSources(Options{
		Root:                  root,
		SkillPath:             "skills/mono/SKILL.md",
		ProductsDir:           "skills/mono/references/products",
		IntentGuidePath:       "skills/mono/references/intent-guide.md",
		HintsDir:              "internal/cli/schema_hints",
		InterfaceMetadataPath: "internal/cli/schema_mcp_metadata.json",
		ToolPaths: map[string]string{
			"calendar.get_calendar":   "calendar book get",
			"calendar.list_calendars": "calendar book list",
			"calendar.raw_tool":       "calendar raw",
		},
		ProductIDs:       map[string]bool{"calendar": true},
		SurfaceToolCount: 3,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	get := metadata.Tools["calendar book get"]
	if get.AgentSummary != "读取指定日历" || get.AgentSummarySource != "dws-wukong@1234567890ab" {
		t.Fatalf("reviewed/imported summary was not preserved: %#v", get)
	}
	list := metadata.Tools["calendar book list"]
	if list.AgentSummary != "列出当前用户可访问的日历。" {
		t.Fatalf("MCP summary = %q", list.AgentSummary)
	}
	if list.AgentSummarySource != "mcp-tools-list+cli-registry@abcdef123456" {
		t.Fatalf("MCP summary source = %q", list.AgentSummarySource)
	}
	if list.InterfaceRef == nil || list.InterfaceRef.ProductID != "calendar" || list.InterfaceRef.RPCName != "list_calendars" {
		t.Fatalf("MCP interface ref = %#v", list.InterfaceRef)
	}
	refProvenance := list.FieldProvenance["interface_ref"]
	if refProvenance.Value != "calendar.list_calendars" || refProvenance.Precedence != "mcp_fallback" || refProvenance.Source == "" {
		t.Fatalf("MCP interface ref provenance = %#v", refProvenance)
	}
	if list.Reviewed == nil || *list.Reviewed {
		t.Fatalf("MCP fallback reviewed = %#v, want false", list.Reviewed)
	}
	raw, exists := metadata.Tools["calendar raw"]
	if !exists {
		t.Fatal("effective command without an eligible MCP summary must retain an Agent metadata projection")
	}
	if raw.AgentSummary != "" {
		t.Fatalf("identifier-only MCP description became an Agent summary: %#v", raw)
	}
	audit := stats.InterfaceMetadata
	if audit == nil || audit.SourceTools != 4 || audit.SurfaceTools != 3 ||
		audit.EligibleSummaries != 2 || audit.AppliedSummaries != 1 ||
		audit.PreservedSummaries != 1 {
		t.Fatalf("interface audit = %#v", audit)
	}
	if len(audit.RejectedTools) != 1 || audit.RejectedTools[0] != "calendar.raw_tool" ||
		len(audit.OutsideSurface) != 1 || audit.OutsideSurface[0] != "outside.tool" {
		t.Fatalf("interface audit paths = %#v", audit)
	}
}

func TestSummarizeInterfaceDescriptionBoundsAndRejectsIdentifiers(t *testing.T) {
	if got := summarizeInterfaceDescription("search_open_platform_docs", 40); got != "" {
		t.Fatalf("identifier summary = %q, want empty", got)
	}
	if got := summarizeInterfaceDescription("**查询日历。** 第二句不保留。", 40); got != "查询日历。" {
		t.Fatalf("sentence summary = %q", got)
	}
	got := summarizeInterfaceDescription("这是一个没有句号而且明显超过允许长度的接口描述文本", 12)
	if got != "这是一个没有句号而..." {
		t.Fatalf("truncated summary = %q", got)
	}
}
