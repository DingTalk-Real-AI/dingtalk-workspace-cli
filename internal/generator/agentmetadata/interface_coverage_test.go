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
}
