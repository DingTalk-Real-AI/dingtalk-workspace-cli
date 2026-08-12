// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageInterfaceMetadataFailureAndConflictEdges(t *testing.T) {
	root := t.TempDir()
	display := filepath.ToSlash("interface.json")
	opts := Options{Root: root, InterfaceMetadataPath: "interface.json", ToolPaths: map[string]string{"sample.tool": "sample tool"}}
	out := &File{Tools: map[string]ToolMetadata{}}
	stats := &Stats{}
	if err := applyInterfaceMetadataFallback(out, nil, opts, stats, sourceTracker{}); err == nil || !strings.Contains(err.Error(), "was not loaded") {
		t.Fatalf("missing file error = %v", err)
	}
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"json", `{`, "decode interface metadata"},
		{"version", `{"version":2}`, "unsupported version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]sourceFile{display: {display: display, data: []byte(tc.body)}}
			err := applyInterfaceMetadataFallback(&File{Tools: map[string]ToolMetadata{}}, files, opts, &Stats{}, sourceTracker{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	body := `{"version":1,"source":"mcp","tools":{"sample.tool":{"description":"summary text","interface_ref":{"product_id":"sample","rpc_name":"tool"}}}}`
	files := map[string]sourceFile{display: {display: display, data: []byte(body)}}
	ref := &InterfaceRef{ProductID: "other", RPCName: "tool"}
	out = &File{Tools: map[string]ToolMetadata{"sample.tool": {InterfaceRef: ref, interfaceRefPresent: true, interfaceRefRank: selectionRankMCPFallback, interfaceRefOrigin: "existing"}}}
	if err := applyInterfaceMetadataFallback(out, files, opts, &Stats{}, sourceTracker{}); err == nil {
		t.Fatal("interface ref conflict succeeded")
	}

	ref = &InterfaceRef{ProductID: "sample", RPCName: "tool"}
	out = &File{Tools: map[string]ToolMetadata{"sample.tool": {
		InterfaceRef: ref, interfaceRefPresent: true, interfaceRefRank: selectionRankMCPFallback, interfaceRefOrigin: "existing",
		InterfaceMode: "local", interfaceModePresent: true, interfaceModeRank: selectionRankMCPFallback, interfaceModeOrigin: "existing",
	}}}
	if err := applyInterfaceMetadataFallback(out, files, opts, &Stats{}, sourceTracker{}); err == nil {
		t.Fatal("interface mode conflict succeeded")
	}
}

func TestCrossPlatformCoverageInterfaceSummaryRemainingEdges(t *testing.T) {
	if got := summarizeInterfaceDescription("\n\nusable summary", 0); got != "usable summary" {
		t.Fatalf("default max summary = %q", got)
	}
	if got := summarizeInterfaceDescription("long summary without punctuation", 1); got != "l..." {
		t.Fatalf("one-rune summary = %q", got)
	}
	if got := firstInterfaceSentence("This is first. Second"); got != "This is first." {
		t.Fatalf("ASCII sentence = %q", got)
	}
	if interfaceIdentifierOnly("") {
		t.Fatal("empty identifier classified as identifier-only")
	}
	if interfaceIdentifierOnly("含中文") {
		t.Fatal("non-ASCII summary classified as identifier-only")
	}
	if !interfaceIdentifierOnly("search_open_platform-docs.v1") {
		t.Fatal("identifier-only summary must be detected")
	}
	if got := summarizeInterfaceDescription("search_open_platform_docs", 40); got != "" {
		t.Fatalf("identifier description = %q, want empty", got)
	}
	if got := summarizeInterfaceDescription("**查询日历。** 第二句不保留。", 40); got != "查询日历。" {
		t.Fatalf("sentence summary = %q", got)
	}
	if got := summarizeInterfaceDescription("这是一个没有句号而且明显超过允许长度的接口描述文本", 12); got != "这是一个没有句号而..." {
		t.Fatalf("truncated summary = %q", got)
	}
	if got := summarizeInterfaceDescription("First half, then a very long continuation without a terminal mark", 20); !strings.HasSuffix(got, "...") {
		t.Fatalf("punctuation-cut summary = %q", got)
	}
	if got := interfaceSummarySource(interfaceMetadataFile{Source: "  mcp-tools  ", SourceRevision: "abcdef1234567890"}); got != "mcp-tools@abcdef123456" {
		t.Fatalf("interfaceSummarySource = %q", got)
	}
	if got := interfaceSummarySource(interfaceMetadataFile{}); got != "mcp-interface" {
		t.Fatalf("default interfaceSummarySource = %q", got)
	}
}

func TestOverallCoverageGapInterfaceMetadataFallbackSuccessPath(t *testing.T) {
	if err := applyInterfaceMetadataFallback(&File{}, nil, Options{}, &Stats{}, sourceTracker{}); err != nil {
		t.Fatalf("empty InterfaceMetadataPath error = %v", err)
	}
	if hasSurfaceAgentSummary(File{Tools: map[string]ToolMetadata{
		"blank": {AgentSummary: "", agentSummaryPresent: false},
	}}, nil, "blank") {
		t.Fatal("absent agent summary must not match")
	}
	if got := summarizeInterfaceDescription("first line\n\nsecond paragraph ignored", 80); got != "first line" {
		t.Fatalf("paragraph break summary = %q", got)
	}

	display := filepath.ToSlash("interface.json")
	body := `{
  "version": 1,
  "source": "mcp-tools-list+cli-registry",
  "source_revision": "abcdef1234567890",
  "source_hash": "sha256:interface",
  "tools": {
    "calendar.get": {
      "description": "读取指定日历。",
      "interface_ref": {"product_id": "calendar", "rpc_name": "get"}
    },
    "calendar.list": {
      "description": "列出当前用户可访问的日历。后续句子不应进入 summary。"
    },
    "calendar.raw": {"description": "raw_tool_name"},
    "outside.tool": {"description": "不在公开命令面"}
  }
}`
	files := map[string]sourceFile{display: {display: display, data: []byte(body)}}
	out := &File{Tools: map[string]ToolMetadata{
		"calendar.get": {AgentSummary: "已有摘要", agentSummaryPresent: true},
	}}
	stats := &Stats{}
	if err := applyInterfaceMetadataFallback(out, files, Options{
		InterfaceMetadataPath: "interface.json",
		ToolPaths: map[string]string{
			"calendar.get":  "calendar get",
			"calendar.list": "calendar list",
			"calendar.raw":  "calendar raw",
		},
	}, stats, sourceTracker{}); err != nil {
		t.Fatal(err)
	}
	audit := stats.InterfaceMetadata
	if audit == nil || audit.SourceTools != 4 || audit.SurfaceTools != 3 ||
		audit.EligibleSummaries != 2 || audit.AppliedSummaries != 1 ||
		audit.PreservedSummaries != 1 {
		t.Fatalf("interface audit = %#v", audit)
	}
	if len(audit.RejectedTools) != 1 || audit.RejectedTools[0] != "calendar.raw" ||
		len(audit.OutsideSurface) != 1 || audit.OutsideSurface[0] != "outside.tool" {
		t.Fatalf("interface audit paths = %#v", audit)
	}
	list := out.Tools["calendar.list"]
	if list.AgentSummary != "列出当前用户可访问的日历。" {
		t.Fatalf("applied MCP summary = %q", list.AgentSummary)
	}
	if list.AgentSummarySource != "mcp-tools-list+cli-registry@abcdef123456" {
		t.Fatalf("applied summary source = %q", list.AgentSummarySource)
	}
	if list.Reviewed == nil || *list.Reviewed {
		t.Fatalf("applied reviewed = %#v, want false", list.Reviewed)
	}
	if list.InterfaceRef != nil {
		t.Fatalf("list tool unexpectedly gained interface_ref: %#v", list.InterfaceRef)
	}
	get := out.Tools["calendar.get"]
	if get.InterfaceRef == nil || get.InterfaceRef.ProductID != "calendar" || get.InterfaceRef.RPCName != "get" {
		t.Fatalf("preserved tool interface_ref = %#v", get.InterfaceRef)
	}
	if get.AgentSummary != "已有摘要" {
		t.Fatalf("preserved summary overwritten: %#v", get)
	}
}
