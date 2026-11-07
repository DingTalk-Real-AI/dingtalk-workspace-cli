// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func requireMindInvalidReceipt(t *testing.T, r output.CommandResult, fragment string) {
	t.Helper()
	e, err := output.EnvelopeFromResult(r)
	if err != nil {
		t.Fatal(err)
	}
	if e.Outcome != output.OutcomeFailure || e.Error == nil || e.Error.Subtype != "invalid_response" || !strings.Contains(e.Error.Message, fragment) {
		t.Fatalf("receipt did not fail closed (%s): %#v", fragment, e)
	}
}

func TestCrossPlatformCoverageMindCreateReceiptDoesNotClaimUnconfirmedContent(t *testing.T) {
	tests := []struct {
		name, want string
		change     func(map[string]any, map[string]any)
	}{
		{"file absent", "nodeId", func(a, r map[string]any) { delete(r, "nodeId") }},
		{"initializing content mismatch", "READY", func(a, r map[string]any) { r["status"] = "INITIALIZING" }},
		{"revision absent", "revision", func(a, r map[string]any) { delete(r, "revision") }},
		{"revision malformed", "revision", func(a, r map[string]any) { r["revision"] = "01" }},
		{"source malformed", "missing", func(a, r map[string]any) { a["source"] = `{}` }},
		{"page map absent", "mapping", func(a, r map[string]any) { delete(r, "pageIdMap") }},
		{"topic map absent", "mapping", func(a, r map[string]any) { delete(r, "nodeIdMap") }},
		{"missing requested page", "pageIdMap", func(a, r map[string]any) { r["pageIdMap"] = map[string]any{"other": "page"} }},
		{"empty mapping key", "mapping", func(a, r map[string]any) { r["pageIdMap"] = map[string]any{"": "page"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"requestId": "req", "source": mindTestSource}
			receipt := mindArgs(t, `{"requestId":"req","nodeId":"file","status":"READY","requestedContentApplied":true,"revision":"0","pageIdMap":{"p":"page"},"nodeIdMap":{"r":"root"}}`)
			tc.change(args, receipt)
			requireMindInvalidReceipt(t, mindWriteResult("create_mind", args, receipt), tc.want)
		})
	}
}

func TestCrossPlatformCoverageMindMutationReceiptRejectsWrongEffects(t *testing.T) {
	tests := []struct {
		name, tool, want string
		change           func(map[string]any, map[string]any)
	}{
		{"missing read", "update_mind_node", "readVersion", func(a, r map[string]any) { delete(r, "readVersion") }},
		{"wrong version", "update_mind_node", "expectedVersion", func(a, r map[string]any) { a["expectedVersion"] = "3" }},
		{"wrong topic", "update_mind_node", "topicId", func(a, r map[string]any) { r["topicId"] = "other" }},
		{"no change wrong text", "update_mind_node", "NO_CHANGE", func(a, r map[string]any) {
			r["status"] = "NO_CHANGE"
			r["applied"] = false
			r["node"] = map[string]any{"id": "topic", "text": "other"}
		}},
		{"no node", "update_mind_node", "confirmed node", func(a, r map[string]any) { delete(r, "node") }},
		{"wrong node id", "update_mind_node", "node ID", func(a, r map[string]any) { r["node"] = map[string]any{"id": "other"} }},
		{"no created mapping", "create_mind_node", "mapping", func(a, r map[string]any) { delete(r, "nodeIdMap") }},
		{"wrong created mapping", "create_mind_node", "nodeIdMap", func(a, r map[string]any) { r["nodeIdMap"] = map[string]any{"temp": "other"} }},
		{"wrong parent", "create_mind_node", "parent", func(a, r map[string]any) { r["node"].(map[string]any)["parentId"] = "other" }},
		{"delete count unknown", "delete_mind_node", "deletedCountKnown", func(a, r map[string]any) {}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"requestId": "req", "nodeId": "file", "pageId": "page", "topicId": "topic", "parentTopicId": "root", "node": map[string]any{"id": "temp", "type": "topic", "text": "New"}, "patch": map[string]any{"text": "New"}}
			r := mindArgs(t, `{"requestId":"req","nodeId":"file","pageId":"page","topicId":"topic","status":"APPLIED","applied":true,"readVersion":"0","committedVersion":"1","node":{"id":"topic","text":"New","parentId":"root"},"nodeIdMap":{"temp":"topic"}}`)
			tc.change(args, r)
			requireMindInvalidReceipt(t, mindWriteResult(tc.tool, args, r), tc.want)
		})
	}
}

func TestCrossPlatformCoverageMindQueryRejectsMalformedPages(t *testing.T) {
	cases := []struct {
		name, want string
		change     func(map[string]any)
	}{
		{"file", "nodeId", func(r map[string]any) { r["nodeId"] = "other" }},
		{"version", "readVersion", func(r map[string]any) { delete(r, "readVersion") }},
		{"schema", "mind-v1", func(r map[string]any) { r["schemaVersion"] = "2" }},
		{"pages", "page array", func(r map[string]any) { r["items"] = nil }},
		{"more", "hasMore", func(r map[string]any) { delete(r, "hasMore") }},
		{"truncation", "truncated", func(r map[string]any) { delete(r, "truncated") }},
		{"page identity", "identity", func(r map[string]any) { r["items"] = []any{nil} }},
		{"wrong page", "requested page", func(r map[string]any) { r["items"].([]any)[0].(map[string]any)["id"] = "other" }},
		{"nodes", "nodes", func(r map[string]any) { r["items"].([]any)[0].(map[string]any)["nodes"] = nil }},
		{"topic", "topic", func(r map[string]any) { r["items"].([]any)[0].(map[string]any)["nodes"] = []any{nil} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := mindArgs(t, `{"nodeId":"file","readVersion":"0","schemaVersion":"1.0","catalogVersion":"mind-v1","hasMore":false,"truncated":false,"items":[{"id":"page","rootId":"root","nodes":[]}]}`)
			tc.change(r)
			requireMindInvalidReceipt(t, mindQueryResult("list_mind_nodes", map[string]any{"nodeId": "file", "pageId": "page"}, r), tc.want)
		})
	}
}

func TestCrossPlatformCoverageMindBoundaryTransportAndLocalIO(t *testing.T) {
	for _, raw := range []string{`[]`, `{"result":{},"success":"true"}`, `{"success":true,"result":{"success":false}}`, `{"pages":[],"nodes":{}}`, `{"pages":[null],"nodes":[]}`, `{"pages":[],"nodes":[null]}`} {
		if _, err := decodeMindReply(raw); err == nil {
			t.Fatalf("accepted invalid wire response %s", raw)
		}
	}
	r, err := decodeMindReply(`{"topicId":"child","text":"","parentTopicId":"root"}`)
	if err != nil || r["node"].(map[string]any)["parentId"] != "root" {
		t.Fatalf("parent lost: %v %v", r, err)
	}
	if _, err := mindWireArgs(map[string]any{"patch": make(chan int)}); err == nil {
		t.Fatal("unencodable patch accepted")
	}
	for _, v := range []any{"-1", true} {
		if _, err := strconvMindVersion(v); err == nil {
			t.Fatalf("invalid count %v accepted", v)
		}
	}
	if err := mindReplyVersion(map[string]any{}, "revision", false); err != nil {
		t.Fatal(err)
	}
	e, _ := output.EnvelopeFromResult(mindFailure("create_mind", map[string]any{"hint": "check original request"}, "unknown_effect", "unknown"))
	if e.Error.Hint != "check original request" {
		t.Fatal("recovery hint lost")
	}
	dir := t.TempDir()
	if _, err := readMindJSON(dir); err == nil {
		t.Fatal("directory treated as JSON file")
	}
	large := filepath.Join(dir, "large.json")
	if err := os.WriteFile(large, []byte("{"+strings.Repeat(" ", 2*1024*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"{" + strings.Repeat(" ", 2*1024*1024) + "}", large} {
		// Non-space payload prevents the inline trim from reducing the bound.
		if raw[0] == '{' {
			raw = "{" + strings.Repeat("x", 2*1024*1024) + "}"
		}
		if _, err := readMindJSON(raw); err == nil {
			t.Fatal("oversized input accepted")
		}
	}
	if _, err := loadMindTopic(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing topic file accepted")
	}
	c := &mindTestCaller{dry: true}
	installMindCaller(t, c)
	if _, err := callMindResult(&cobra.Command{}, "create_mind", map[string]any{"source": "{}"}); err == nil {
		t.Fatal("dry run accepted invalid source")
	}
	c.dry = false
	if _, err := callMindResult(&cobra.Command{}, "update_mind_node", map[string]any{"patch": make(chan int)}); err == nil || c.calls != 0 {
		t.Fatal("unencodable patch reached MCP")
	}
	c.response = `{"success":1}`
	result, err := callMindResult(&cobra.Command{}, "create_mind", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	e, _ = output.EnvelopeFromResult(result)
	if e.Outcome != output.OutcomeFailure || e.Error.Subtype != "upstream_error" {
		t.Fatalf("nonboolean success accepted: %#v", e)
	}
}

func TestCrossPlatformCoverageMindPendingProfileWithoutMetadata(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	valid := output.Pending(map[string]any{}, &output.OperationInfo{ID: "req", State: "INITIALIZING", NextCommand: "dws mind create"})
	got, err := mindBindRecoveryProfile(&cobra.Command{}, valid)
	if err != nil || got.Outcome() != output.OutcomePending {
		t.Fatalf("pending without profile: %v", err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "corp:user", "")
	if _, err := mindBindRecoveryProfile(cmd, output.Pending(nil, nil)); err == nil {
		t.Fatal("malformed pending accepted")
	}
}

type mindTransportFailureCaller struct {
	mindTestCaller
	failure error
}

func (c *mindTransportFailureCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return nil, c.failure
}

func TestCrossPlatformCoverageMindTransportFailureAndPublicRegistration(t *testing.T) {
	installMindCaller(t, &mindTestCaller{})
	sentinel := errors.New("connection interrupted")
	InitDeps(&mindTransportFailureCaller{failure: sentinel})
	if _, err := callMindResult(&cobra.Command{}, "list_mind_nodes", map[string]any{}); err == nil || !strings.Contains(err.Error(), sentinel.Error()) {
		t.Fatalf("transport failure lost: %v", err)
	}
	// Build through the real public factory; testing constructors alone misses
	// whether the product can actually be discovered from the root registry.
	found := false
	for _, r := range publicFactories {
		if r.name == "mind" {
			h := r.factory()
			if h == nil {
				t.Fatal("nil mind handler")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("mind product not registered")
	}
}
