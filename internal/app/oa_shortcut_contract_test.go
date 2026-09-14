// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestOAFinalSchemaAvailabilityMatchesReviewedExecution(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	for _, canonical := range []string{
		"oa.shortcut_approve_by",
		"oa.shortcut_done_approvals",
		"oa.shortcut_list_cc",
		"oa.shortcut_list_executed",
		"oa.shortcut_list_forms",
		"oa.shortcut_list_pending",
		"oa.shortcut_list_submitted",
		"oa.shortcut_my_initiated",
		"oa.shortcut_pending",
		"oa.shortcut_search_forms",
	} {
		tool, ok := snapshot.Tools[canonical]
		if !ok {
			t.Errorf("final Schema lacks OA tool %s", canonical)
			continue
		}
		if got := tool["availability"]; got != contract.InterfaceAvailable {
			t.Errorf("%s final availability=%v, want %q", canonical, got, contract.InterfaceAvailable)
		}
	}
}
