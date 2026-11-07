// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package wiki

import (
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageWikiNodeDeleteRequiresExactPreflight(t *testing.T) {
	for _, tc := range []struct{ name, preflight, reason string }{
		{"missing workspace", `{"success":true,"nodeId":"n"}`, "missing_preflight_workspace"},
		{"empty workspace", `{"success":true,"nodeId":"n","workspaceId":""}`, "missing_preflight_workspace"},
		{"wrong workspace", `{"success":true,"nodeId":"n","workspaceId":"other"}`, "workspace_preflight_mismatch"},
		{"missing node", `{"success":true,"workspaceId":"w"}`, "missing_preflight_node_id"},
		{"empty node", `{"success":true,"nodeId":"","workspaceId":"w"}`, "missing_preflight_node_id"},
		{"wrong node", `{"success":true,"nodeId":"other","workspaceId":"w"}`, "node_preflight_mismatch"},
		{"nested failure", `{"success":true,"result":{"success":false,"nodeId":"n","workspaceId":"w"}}`, "remote_failure"},
	} {
		for _, dry := range []bool{false, true} {
			name := tc.name
			if dry {
				name += " preview"
			}
			t.Run(name, func(t *testing.T) {
				caller := &wikiCoverageCaller{responses: map[string][]string{
					"doc/get_document_info": {tc.preflight},
					"doc/delete_document":   {`{"success":true}`},
				}}
				args := []string{"+node-delete", "--workspace", "w", "--node", "n", "--yes"}
				if dry {
					args = append(args, "--dry-run")
				}
				_, err := runWikiCoverageCLI(t, caller, args...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != tc.reason {
					t.Fatalf("error=%#v, want reason %q", err, tc.reason)
				}
				if len(caller.calls) != 1 || caller.calls[0].tool != "get_document_info" {
					t.Fatalf("invalid target reached delete: %#v", caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageWikiNodeDeletePreservesVerifiedTargets(t *testing.T) {
	for _, preflight := range []string{
		`{"success":true,"nodeId":"n","workspaceId":"w"}`,
		`{"success":true,"result":{"fileId":"n","spaceId":"w"}}`,
	} {
		for _, dry := range []bool{false, true} {
			caller := &wikiCoverageCaller{responses: map[string][]string{
				"doc/get_document_info": {preflight},
				"doc/delete_document":   {`{"success":true}`},
			}}
			args := []string{"+node-delete", "--workspace", "w", "--node", "n", "--yes"}
			if dry {
				args = append(args, "--dry-run")
			}
			out, err := runWikiCoverageCLI(t, caller, args...)
			if err != nil {
				t.Fatal(err)
			}
			if dry {
				if out["executed"] != false || len(caller.calls) != 1 {
					t.Fatalf("preview wrote: out=%#v calls=%#v", out, caller.calls)
				}
			} else if out["nodeId"] != "n" || out["deleted"] != true || len(caller.calls) != 2 ||
				caller.calls[1].product != "doc" || caller.calls[1].tool != "delete_document" || caller.calls[1].args["nodeId"] != "n" {
				t.Fatalf("verified delete changed: out=%#v calls=%#v", out, caller.calls)
			}
		}
	}
}
