// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"strings"
	"testing"
)

// The markdown leaf commands (fetch/create/overwrite/patch/diff) fast-return a
// dry-run preview before reaching deps.Caller.CallTool/CallReadTool, so their
// dry-run branches were bypassing the delegation-auth decorator. These tests
// verify markdownDryRunDelegationPrecheck now gates every dry-run preview on
// check_capability when --principal-user-id is set, aligns each precheck target
// with the command's real first delegated call, and stays a no-op otherwise.

const (
	markdownDelegationAllowed = `{"allowed":true}`
	markdownDelegationDenied  = `{"allowed":false,"denialReason":"NO_PERM","denialMessage":"denied"}`
)

func newMarkdownDelegationInner(readResult string) *docDelegationHelpersTestCaller {
	return &docDelegationHelpersTestCaller{
		checkRes: textToolResult(markdownDelegationAllowed),
		readRes:  textToolResult(readResult),
	}
}

// runMarkdownDelegationGroup installs a raw dry-run caller then executes the
// markdown group through cobra so the group's PersistentPreRunE wraps
// deps.Caller in the delegation-auth decorator exactly as the real CLI does.
func runMarkdownDelegationGroup(t *testing.T, inner *docDelegationHelpersTestCaller, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	out, _ := installHelpersCoreDeps(t, inner)
	err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil, args...)
	return out, err
}

// assertMarkdownDelegationCheck verifies check_capability fired once through the
// dry-run read channel with the expected mcpToolKey (proving the resolved
// serverID.toolName) and nodeId, and that no business CallTool passthrough
// happened during dry-run.
func assertMarkdownDelegationCheck(t *testing.T, inner *docDelegationHelpersTestCaller, wantToolKey, wantNodeID string) {
	t.Helper()
	if len(inner.readCalls) != 1 {
		t.Fatalf("readCalls = %d, want 1 check_capability via ReadTool", len(inner.readCalls))
	}
	rc := inner.readCalls[0]
	if rc.server != capabilityServerID || rc.tool != checkCapTool {
		t.Fatalf("check routed to %s.%s, want %s.%s", rc.server, rc.tool, capabilityServerID, checkCapTool)
	}
	if rc.args["mcpToolKey"] != wantToolKey {
		t.Fatalf("mcpToolKey = %v, want %s", rc.args["mcpToolKey"], wantToolKey)
	}
	if rc.args["nodeId"] != wantNodeID {
		t.Fatalf("nodeId = %v, want %s", rc.args["nodeId"], wantNodeID)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("calls = %d, want 0 (dry-run must not passthrough to CallTool)", len(inner.calls))
	}
}

func TestCrossPlatformCoverageMarkdownFetchDryRunDelegationAllowed(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		toolKey  string
		wantNode string
	}{
		{"auto route probes drive.get_file_info", []string{"markdown", "fetch", "--principal-user-id", "u1", "--node", "n1"}, "drive.get_file_info", "n1"},
		{"space route downloads via drive", []string{"markdown", "fetch", "--principal-user-id", "u1", "--node", "n1", "--space-id", "s1"}, "drive.download_file", "n1"},
		{"workspace route downloads via doc", []string{"markdown", "fetch", "--principal-user-id", "u1", "--node", "n1", "--workspace", "w1"}, "doc.download_file", "n1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := newMarkdownDelegationInner(markdownDelegationAllowed)
			out, err := runMarkdownDelegationGroup(t, inner, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			assertMarkdownDelegationCheck(t, inner, tc.toolKey, tc.wantNode)
			if !strings.Contains(out.String(), "dry_run") {
				t.Fatalf("output = %q, want dry-run preview", out.String())
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownFetchDryRunDelegationDenied(t *testing.T) {
	inner := newMarkdownDelegationInner(markdownDelegationDenied)
	out, err := runMarkdownDelegationGroup(t, inner,
		"markdown", "fetch", "--principal-user-id", "u1", "--node", "n1")
	if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
		t.Fatalf("error = %v, want [DELEGATION_AUTH_DENIED] prefix", err)
	}
	if strings.Contains(out.String(), "dry_run") {
		t.Fatalf("output = %q, must not render preview on denial", out.String())
	}
}

func TestCrossPlatformCoverageMarkdownCreateDryRunDelegationAllowed(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		toolKey  string
		wantNode string
	}{
		{"space route uploads via drive with parentId", []string{"markdown", "create", "--principal-user-id", "u1", "--content", "hi", "--name", "x.md", "--space-id", "s1", "--folder", "f1"}, "drive.get_upload_info", "f1"},
		{"doc route uploads with workspace and folder", []string{"markdown", "create", "--principal-user-id", "u1", "--content", "hi", "--name", "x.md", "--workspace", "w1", "--folder", "f1"}, "doc.get_file_upload_info", "f1"},
		{"folder-only route authorizes the domain probe", []string{"markdown", "create", "--principal-user-id", "u1", "--content", "hi", "--name", "x.md", "--folder", "f1"}, "drive.get_file_info", "f1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := newMarkdownDelegationInner(markdownDelegationAllowed)
			out, err := runMarkdownDelegationGroup(t, inner, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			assertMarkdownDelegationCheck(t, inner, tc.toolKey, tc.wantNode)
			if !strings.Contains(out.String(), "dry_run") {
				t.Fatalf("output = %q, want dry-run preview", out.String())
			}
		})
	}
}

// A create without any space/workspace/folder produces empty upload step1 args,
// so the precheck reports DELEGATION_AUTH_NOT_SUPPORTED — matching the
// non-dry-run path where the same empty get_file_upload_info call is gated.
func TestCrossPlatformCoverageMarkdownCreateDryRunDelegationNotSupported(t *testing.T) {
	inner := newMarkdownDelegationInner(markdownDelegationAllowed)
	out, err := runMarkdownDelegationGroup(t, inner,
		"markdown", "create", "--principal-user-id", "u1", "--content", "hi", "--name", "x.md")
	if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_NOT_SUPPORTED]") {
		t.Fatalf("error = %v, want [DELEGATION_AUTH_NOT_SUPPORTED] prefix", err)
	}
	if len(inner.readCalls) != 0 || len(inner.calls) != 0 {
		t.Fatalf("readCalls=%d calls=%d, want 0 (empty node short-circuits before remote check)", len(inner.readCalls), len(inner.calls))
	}
	if strings.Contains(out.String(), "dry_run") {
		t.Fatalf("output = %q, must not render preview when unsupported", out.String())
	}
}

func TestCrossPlatformCoverageMarkdownOverwriteDryRunDelegation(t *testing.T) {
	t.Run("auto route reads drive.get_file_info", func(t *testing.T) {
		inner := newMarkdownDelegationInner(markdownDelegationAllowed)
		out, err := runMarkdownDelegationGroup(t, inner,
			"markdown", "overwrite", "--principal-user-id", "u1", "--node", "n1")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		assertMarkdownDelegationCheck(t, inner, "drive.get_file_info", "n1")
		if !strings.Contains(out.String(), "dry_run") {
			t.Fatalf("output = %q, want dry-run preview", out.String())
		}
	})
	t.Run("workspace route reads doc.get_document_info", func(t *testing.T) {
		inner := newMarkdownDelegationInner(markdownDelegationAllowed)
		out, err := runMarkdownDelegationGroup(t, inner,
			"markdown", "overwrite", "--principal-user-id", "u1", "--node", "n1", "--workspace", "w1")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		assertMarkdownDelegationCheck(t, inner, "doc.get_document_info", "n1")
		if !strings.Contains(out.String(), "dry_run") {
			t.Fatalf("output = %q, want dry-run preview", out.String())
		}
	})
	t.Run("denial suppresses preview", func(t *testing.T) {
		inner := newMarkdownDelegationInner(markdownDelegationDenied)
		out, err := runMarkdownDelegationGroup(t, inner,
			"markdown", "overwrite", "--principal-user-id", "u1", "--node", "n1")
		if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
			t.Fatalf("error = %v, want [DELEGATION_AUTH_DENIED] prefix", err)
		}
		if strings.Contains(out.String(), "dry_run") {
			t.Fatalf("output = %q, must not render preview on denial", out.String())
		}
	})
}

func TestCrossPlatformCoverageMarkdownPatchDryRunDelegation(t *testing.T) {
	t.Run("auto route reads drive.get_file_info", func(t *testing.T) {
		inner := newMarkdownDelegationInner(markdownDelegationAllowed)
		out, err := runMarkdownDelegationGroup(t, inner,
			"markdown", "patch", "--principal-user-id", "u1", "--node", "n1", "--pattern", "a", "--content", "b")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		assertMarkdownDelegationCheck(t, inner, "drive.get_file_info", "n1")
		if !strings.Contains(out.String(), "dry_run") {
			t.Fatalf("output = %q, want dry-run preview", out.String())
		}
	})
	t.Run("denial suppresses preview", func(t *testing.T) {
		inner := newMarkdownDelegationInner(markdownDelegationDenied)
		out, err := runMarkdownDelegationGroup(t, inner,
			"markdown", "patch", "--principal-user-id", "u1", "--node", "n1", "--pattern", "a", "--content", "b")
		if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
			t.Fatalf("error = %v, want [DELEGATION_AUTH_DENIED] prefix", err)
		}
		if strings.Contains(out.String(), "dry_run") {
			t.Fatalf("output = %q, must not render preview on denial", out.String())
		}
	})
}

func TestCrossPlatformCoverageMarkdownDiffDryRunDelegation(t *testing.T) {
	t.Run("reads drive.get_file_info before diff preview", func(t *testing.T) {
		inner := newMarkdownDelegationInner(markdownDelegationAllowed)
		_, err := runMarkdownDelegationGroup(t, inner,
			"markdown", "diff", "--principal-user-id", "u1", "--node", "n1", "--version", "1")
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		assertMarkdownDelegationCheck(t, inner, "drive.get_file_info", "n1")
	})
	t.Run("denial suppresses preview", func(t *testing.T) {
		inner := newMarkdownDelegationInner(markdownDelegationDenied)
		_, err := runMarkdownDelegationGroup(t, inner,
			"markdown", "diff", "--principal-user-id", "u1", "--node", "n1", "--version", "1")
		if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
			t.Fatalf("error = %v, want [DELEGATION_AUTH_DENIED] prefix", err)
		}
	})
}

// Without --principal-user-id the group never installs the decorator and the
// precheck reads an empty principal, so it must be a complete no-op: no
// check_capability, no passthrough, preview rendered as before.
func TestCrossPlatformCoverageMarkdownDryRunDelegationNoPrincipalNoop(t *testing.T) {
	inner := newMarkdownDelegationInner(markdownDelegationAllowed)
	out, err := runMarkdownDelegationGroup(t, inner, "markdown", "fetch", "--node", "n1")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.readCalls) != 0 || len(inner.calls) != 0 {
		t.Fatalf("readCalls=%d calls=%d, want 0 without --principal-user-id", len(inner.readCalls), len(inner.calls))
	}
	if !strings.Contains(out.String(), "dry_run") {
		t.Fatalf("output = %q, want dry-run preview unaffected", out.String())
	}
}

// When --principal-user-id is set but deps.Caller is somehow not the
// delegation-auth decorator (never happens in the real flow, since the group
// only wraps when the flag is set), the precheck type-assertion fails closed to
// nil so the preview still renders without a spurious remote check.
func TestCrossPlatformCoverageMarkdownDryRunDelegationUndecoratedCallerNoop(t *testing.T) {
	inner := newMarkdownDelegationInner(markdownDelegationAllowed)
	out, _ := installHelpersCoreDeps(t, inner)

	cmd := newMarkdownFetchCmd()
	cmd.Flags().String(FlagPrincipalUserID, "", "")
	if err := cmd.Flags().Set(FlagPrincipalUserID, "u1"); err != nil {
		t.Fatalf("set principal flag: %v", err)
	}
	if err := cmd.Flags().Set("node", "n1"); err != nil {
		t.Fatalf("set node flag: %v", err)
	}
	if err := runMarkdownFetch(cmd, nil); err != nil {
		t.Fatalf("runMarkdownFetch() error = %v", err)
	}
	if len(inner.readCalls) != 0 || len(inner.calls) != 0 {
		t.Fatalf("readCalls=%d calls=%d, want 0 when caller is not decorated", len(inner.readCalls), len(inner.calls))
	}
	if !strings.Contains(out.String(), "dry_run") {
		t.Fatalf("output = %q, want dry-run preview", out.String())
	}
}
