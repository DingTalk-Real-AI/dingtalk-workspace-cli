// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageChatAuditMCPUIDPayload(t *testing.T) {
	// Preserve the existing MCP argument names and pass identity values unchanged.
	// Identity resolution belongs to the connector mapping, not the CLI.
	for _, status := range []string{"AuditApprove", "AuditDelete", "AuditIgnore", "AuditRefuse", "AuditBlock"} {
		t.Run(status, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{
				"chat", "+chat-audit-join", "--group", "cid-fixture", "--record-id", "123",
				"--applicant", "D-applicant", "--inviter", "D-inviter",
				"--status", status, "--description", "reviewed request", "--yes",
			})
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %#v, want one approval with no identity conversion", fake.calls)
			}
			call := fake.calls[0]
			want := map[string]any{
				"openConversationId": "cid-fixture", "applyRecordId": 123,
				"applicantUid": "D-applicant", "inviterUid": "D-inviter",
				"status": status, "auditDescription": "reviewed request",
			}
			if call.product != "im" || call.tool != "audit_join_group" || !reflect.DeepEqual(call.args, want) {
				t.Fatalf("call = %#v, want im/audit_join_group %#v", call, want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatAuditShortcutRejectsRecordStates(t *testing.T) {
	for _, status := range []string{"AuditApplying", "auditapprove", "AuditUnknown"} {
		t.Run(status, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{
				"chat", "+chat-audit-join", "--group", "cid-fixture", "--record-id", "123",
				"--applicant", "D-applicant", "--inviter", "D-inviter", "--status", status, "--yes",
			})
			if err := corecmd.ExecuteForTest(root); err == nil {
				t.Fatal("unsupported action was accepted")
			}
			if len(fake.calls) != 0 {
				t.Fatalf("unsupported action called MCP: %#v", fake.calls)
			}
		})
	}
}
