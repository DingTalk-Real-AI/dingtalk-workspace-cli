// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageURLResolveApplicationNodeIdentity(t *testing.T) {
	// A guide document's generated navigation node ID differs from the referenced
	// document's dentryUuid. Only the former belongs in applicationId.
	const directory = `{"items":[{"nodeId":"document-node","nodeType":"document","parentSectionId":"","name":"guide"},{"nodeId":"dashboard-node","nodeType":"dashboard","parentSectionId":""}],"total":2}`
	for _, tc := range []struct {
		id, kind string
	}{
		{"document-node", "document"}, {"dashboard-node", "dashboard"}, {"document-dentry-uuid", ""},
	} {
		t.Run(tc.id, func(t *testing.T) {
			caller := &urlResolveE2ECaller{results: []*edition.ToolResult{urlResult(`{"data":` + directory + `}`)}}
			out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/b?applicationId="+tc.id, "--verify")
			if len(caller.calls) != 1 || caller.calls[0].product != serverHelper || caller.calls[0].tool != "list_nsheet_nodes" || !reflect.DeepEqual(caller.calls[0].args, map[string]any{"baseId": "b"}) {
				t.Fatalf("must read actual navigation nodes once: %#v", caller.calls)
			}
			if tc.kind == "" {
				var typed *apperrors.Error
				if err == nil || out != "" || !errors.As(err, &typed) || typed.Reason != "target_verification_failed" {
					t.Fatalf("document UUID was accepted as an Application node: %s %v", out, err)
				}
				return
			}
			var envelope struct {
				Data struct {
					Verified     bool              `json:"verified"`
					Verification map[string]string `json:"verification"`
				} `json:"data"`
			}
			if err != nil || json.Unmarshal([]byte(out), &envelope) != nil || !envelope.Data.Verified || envelope.Data.Verification["entityType"] != tc.kind || envelope.Data.Verification["id"] != tc.id {
				t.Fatalf("real Application node was not verified: %s %v", out, err)
			}
		})
	}
}

func TestCrossPlatformCoverageURLResolveAutomationBackgroundVerification(t *testing.T) {
	for _, tc := range []struct{ query, tool, server, body string }{
		{"entrance=automation&sheetId=background&viewId=stale", "get_base", serverMain, `{"baseId":"b"}`},
		{"entrance=automation&applicationId=background", "get_base", serverMain, `{"baseId":"b"}`},
		{"entrance=automation&sheetId=background&viewId=stale&flowId=f", "get_workflow", serverHelper, `{"flowId":"f"}`},
		{"entrance=automation&applicationId=background&flowId=f", "get_workflow", serverHelper, `{"flowId":"f"}`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			caller := &urlResolveE2ECaller{results: []*edition.ToolResult{urlResult(tc.body)}}
			out, err := runURLResolveCLI(t, caller, "--url", "https://alidocs.dingtalk.com/i/nodes/b?"+tc.query, "--verify")
			if err != nil {
				t.Fatal(err, out)
			}
			want := map[string]any{"baseId": "b"}
			if tc.tool == "get_workflow" {
				want["workflowId"] = "f"
			}
			if len(caller.calls) != 1 || caller.calls[0].product != tc.server || caller.calls[0].tool != tc.tool || !reflect.DeepEqual(caller.calls[0].args, want) {
				t.Fatalf("background objects must not become verification targets: %#v", caller.calls)
			}
		})
	}
}
