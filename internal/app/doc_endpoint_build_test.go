// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageDocBuildEndpointPrecedence(t *testing.T) {
	const prepub = "https://custom-doc.example.invalid/server/doc-build?key=test-only&route=a%2Fb&route=c&empty="
	for _, tc := range []struct {
		name, product, compiled, explicit, want string
		wantOverride                            bool
	}{
		{"compiled beats discovery", "doc", prepub, "", prepub, true},
		{"explicit env wins", "doc", prepub, "https://mcp-gw.dingtalk.com/server/explicit?route=env", "https://mcp-gw.dingtalk.com/server/explicit?route=env", true},
		{"trimmed compiled default", " doc ", "  " + prepub + "\n", "", prepub, true},
		{"empty default uses discovery", "doc", "", "", "https://mcp-gw.dingtalk.com/server/discovered-doc", false},
		{"whitespace default uses discovery", "doc", " \n", "", "https://mcp-gw.dingtalk.com/server/discovered-doc", false},
		{"other product keeps discovery", "drive", prepub, "", "https://mcp-gw.dingtalk.com/server/discovered-drive", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DINGTALK_DOC_MCP_URL", tc.explicit)
			t.Setenv("DINGTALK_DRIVE_MCP_URL", "")
			testseam.Swap(t, &buildDocMCPEndpoint, tc.compiled)
			testseam.Swap(t, &dynamicEndpoints, map[string]string{
				"doc":   "https://mcp-gw.dingtalk.com/server/discovered-doc",
				"drive": "https://mcp-gw.dingtalk.com/server/discovered-drive",
			})
			testseam.Swap(t, &dynamicAliases, map[string]string{})
			got, ok := directRuntimeEndpoint(tc.product, "get_document_content")
			if !ok || got != tc.want {
				t.Errorf("resolved endpoint = %q, %v; want %q, true", got, ok, tc.want)
			}
			if got := hasDirectRuntimeEndpointOverride(tc.product); got != tc.wantOverride {
				t.Errorf("override guard = %v, want %v", got, tc.wantOverride)
			}
		})
	}
}

// Exercise both gateway-rewrite consumers and the real transport. The fake
// HTTP boundary avoids external I/O while preserving URL parsing and routing.
func TestCrossPlatformCoverageDocBuildEndpointSurvivesAuthenticatedRouting(t *testing.T) {
	const prepub = "https://custom-doc.example.invalid/server/doc-build?key=test-only&route=a%2Fb&route=c&empty=&space=a+b%20c"
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DINGTALK_DOC_MCP_URL", "")
	testseam.Swap(t, &buildDocMCPEndpoint, prepub)
	testseam.Swap(t, &dynamicEndpoints, map[string]string{"doc": "https://mcp-gw.dingtalk.com/server/discovered-doc"})
	testseam.Swap(t, &dynamicAliases, map[string]string{})
	testseam.Swap(t, &pluginAuthRegistry, map[string]*PluginAuth{})
	t.Cleanup(authpkg.PushMCPBaseURLOverride("https://mcp.dingtalk.io"))
	snapshot := AccessTokenSnapshot{AccessToken: "test-token", LoginRegion: authpkg.LoginRegionInternational, LoginRegionKnown: true}
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return snapshot, nil
	})
	testseam.Swap(t, &runnerResolveAuthSnapshotForProfile, func(*runtimeRunner, context.Context, string) (AccessTokenSnapshot, error) {
		return snapshot, nil
	})
	testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	})
	var methods []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.String(); got != prepub {
			t.Errorf("wire URL = %q, want %q", got, prepub)
		}
		if req.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("request lost the selected OAuth identity")
		}
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			return nil, err
		}
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "get_document_content", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": `{"markdown":"preview"}`}}}
		default:
			t.Errorf("unexpected MCP method %q", request.Method)
		}
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: req}, nil
	})}
	runner := &runtimeRunner{transport: transport.NewClient(client), globalFlags: &GlobalFlags{}, auditSink: audit.NopSink{}}
	tools, err := runner.listProductTools(t.Context(), "doc", "get_document_content", "selected-profile")
	if err != nil || len(tools) != 1 || tools[0].Name != "get_document_content" {
		t.Fatalf("tools/list = %#v, %v", tools, err)
	}
	endpoint, ok := directRuntimeEndpoint("doc", "get_document_content")
	if !ok {
		t.Fatal("doc endpoint is unresolved")
	}
	_, err = runner.executeInvocation(t.Context(), endpoint, executor.NewHelperInvocation("doc.read", "doc", "get_document_content", map[string]any{"nodeId": "node", "suggestion": true}))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(methods, ","); got != "tools/list,tools/call" {
		t.Fatalf("MCP methods = %q, want one discovery and one explicit call", got)
	}
}
