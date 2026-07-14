// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import "testing"

func TestRuntimeToolTextPrefersCobraHelpOverGenericMCPMetadata(t *testing.T) {
	entry := runtimeSchemaEntry{
		ProductID:      "aitable",
		ToolName:       "view_get_filter",
		Title:          "获取视图 filter 配置",
		Description:    "获取指定视图当前的筛选规则数组。",
		PrimaryCLIPath: "aitable view get filter",
	}
	metadata := runtimeSchemaMetadataSources{MCP: embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{
		"aitable.view_get_filter": {
			Title:       "get_views",
			Description: "获取数据表的全部视图。",
		},
	}}}

	title, description, metadataSource, provenance, err := runtimeToolTextMetadataFromMetadata(entry, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if title != entry.Title || description != entry.Description {
		t.Fatalf("tool text = %q / %q, want Cobra Help %q / %q", title, description, entry.Title, entry.Description)
	}
	if metadataSource != "" {
		t.Fatalf("metadata source = %q, want Cobra Help", metadataSource)
	}
	for _, field := range []string{"title", "description"} {
		if provenance[field].Source != "cobra_help" {
			t.Fatalf("%s provenance = %#v, want cobra_help", field, provenance[field])
		}
	}
}

func TestRuntimeToolTextKeepsReviewedHintAboveCobraHelp(t *testing.T) {
	entry := runtimeSchemaEntry{
		ProductID:      "aitable",
		ToolName:       "query_records",
		Title:          "查询记录",
		Description:    "CLI 查询记录说明。",
		PrimaryCLIPath: "aitable record query",
	}
	metadata := runtimeSchemaMetadataSources{MCP: embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{
		"aitable.query_records": {
			Title:       "query_records",
			Description: "通用 RPC 查询说明。",
		},
	}}}

	title, description, metadataSource, provenance, err := runtimeToolTextMetadataFromMetadata(entry, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if title != entry.Title {
		t.Fatalf("title = %q, want Cobra Help title %q", title, entry.Title)
	}
	wantDescription := schemaHintForCanonicalPath("aitable.query_records").Description
	if description != wantDescription {
		t.Fatalf("description = %q, want reviewed hint %q", description, wantDescription)
	}
	if metadataSource != "tool-schema-hint" {
		t.Fatalf("metadata source = %q, want tool-schema-hint", metadataSource)
	}
	if provenance["title"].Source != "cobra_help" || provenance["description"].Source != "tool_schema_hint" {
		t.Fatalf("provenance = %#v", provenance)
	}
}
