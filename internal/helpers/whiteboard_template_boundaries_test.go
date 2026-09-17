// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	core "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageWhiteboardTemplateReceiptValidation(t *testing.T) {
	args := map[string]any{"requestId": "r", "templateId": "t", "templateWorkspaceId": "w"}
	valid := map[string]any{"templateId": "t", "requestId": "r", "resourceType": 9, "scope": "team", "templateScope": "team", "templateWorkspaceId": "w", "verified": true}
	for _, tc := range []struct {
		field        string
		value        any
		save, create bool
	}{
		{"templateId", "", true, true}, {"requestId", "bad", true, false}, {"resourceType", 3, true, true},
		{"scope", "personal", true, false}, {"templateScope", "personal", false, true}, {"templateWorkspaceId", "", true, true},
		{"templateWorkspaceId", "wrong", true, true}, {"verified", false, false, true},
	} {
		result := cloneWhiteboardTemplateArgs(valid)
		result[tc.field] = tc.value
		if tc.save && validateWhiteboardTemplateSaveResult(result, args, core.TeamTemplateSaveTool) == nil {
			t.Fatalf("save accepted %s", tc.field)
		}
		if tc.create && validateWhiteboardTemplateCreateResult(result, args, core.TeamTemplateCreateTool) == nil {
			t.Fatalf("create accepted %s", tc.field)
		}
	}
	if validateWhiteboardTemplateSaveResult(nil, args, core.TeamTemplateSaveTool) == nil || validateWhiteboardTemplateCreateResult(nil, args, core.TeamTemplateCreateTool) == nil {
		t.Fatal("nil receipt accepted")
	}
	if validateWhiteboardTemplateDryRunResult(map[string]any{"resourceType": 9, "scope": "team"}, args, core.TeamTemplateSaveTool) == nil {
		t.Fatal("missing dry-run workspace accepted")
	}
	for _, item := range []any{nil, map[string]any{}, map[string]any{"templateId": "t", "resourceType": 3}, map[string]any{"templateId": "t", "resourceType": 9, "scope": "personal"}, map[string]any{"templateId": "t", "resourceType": 9, "templateWorkspaceId": "wrong"}} {
		if _, _, _, err := validateWhiteboardTemplatePage(map[string]any{"templates": []any{item}}, core.TeamTemplateListTool, args); err == nil {
			t.Fatalf("bad list item accepted %v", item)
		}
	}
	if _, _, _, err := validateWhiteboardTemplatePage(map[string]any{"templates": []any{}, "hasMore": true}, core.TeamTemplateListTool, args); err == nil {
		t.Fatal("missing cursor accepted")
	}
	if requireWhiteboardTemplateType(nil, nil) == nil {
		t.Fatal("missing type accepted")
	}
	for _, v := range []any{9, float64(9), json.Number("9"), " 9 "} {
		if got := whiteboardTemplateInt(v, -1); got != 9 {
			t.Fatal(got)
		}
	}
	if whiteboardTemplateInt(json.Number("bad"), -1) != -1 {
		t.Fatal("bad number accepted")
	}
	if _, err := validateWhiteboardTemplateRequestID(""); err == nil {
		t.Fatal("empty request ID accepted")
	}
	cmd := &cobra.Command{}
	cmd.Flags().Int("limit", 10, "")
	cmd.Flags().Int("max-pages", 0, "")
	if err := validateWhiteboardTemplateList(cmd, nil); err == nil {
		t.Fatal("zero max pages accepted")
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateDispatchFailures(t *testing.T) {
	for _, tc := range []struct {
		dry      bool
		response string
		tool     string
	}{
		{true, `{"success":true,"result":null}`, core.PersonalTemplateSaveTool},
		{true, ``, core.PersonalTemplateSaveTool},
		{true, `{"success":true,"result":{"scope":"personal","resourceType":9}}`, core.PersonalTemplateSaveTool},
		{false, `{"success":true,"result":{}}`, core.PersonalTemplateSaveTool},
	} {
		caller := &whiteboardTestCaller{dry: tc.dry, response: func(whiteboardTestCall, int) string { return tc.response }}
		installWhiteboardTestCaller(t, caller)
		if _, err := whiteboardTemplateResultCall(&cobra.Command{}, tc.tool, map[string]any{}); err == nil {
			t.Fatal("invalid receipt accepted")
		}
	}
	failure := errors.New("preflight unavailable")
	caller := &whiteboardTestCaller{dry: true, err: func(whiteboardTestCall, int) error { return failure }}
	installWhiteboardTestCaller(t, caller)
	if _, err := whiteboardTemplateResultCall(&cobra.Command{}, core.PersonalTemplateSaveTool, nil); err == nil {
		t.Fatal("preflight error lost")
	}
	if _, err := callWhiteboardTemplatePreview(&cobra.Command{}, core.PersonalTemplateSaveTool, nil); err == nil {
		t.Fatal("missing dryRun accepted")
	}
	for _, tc := range []struct {
		cursor string
		max    int
	}{{"1", 10}, {"", 1}} {
		caller := &whiteboardTestCaller{response: func(whiteboardTestCall, int) string {
			return `{"success":true,"templates":[],"hasMore":true,"nextCursor":"1"}`
		}}
		installWhiteboardTestCaller(t, caller)
		if _, err := callWhiteboardTemplateListResult(&cobra.Command{}, core.PersonalTemplateListTool, map[string]any{"pageAll": true, "nextCursor": tc.cursor, "maxPages": tc.max}); err == nil {
			t.Fatal("unbounded pagination accepted")
		}
	}
}

type whiteboardWithoutPreview struct{ edition.ToolCaller }

func TestCrossPlatformCoverageWhiteboardTemplateMissingPreviewCapability(t *testing.T) {
	caller := &whiteboardTestCaller{}
	installWhiteboardTestCaller(t, caller)
	testseam.Swap(t, &deps.Caller, edition.ToolCaller(&whiteboardWithoutPreview{ToolCaller: caller}))
	if _, err := callWhiteboardTemplatePreview(&cobra.Command{}, core.PersonalTemplateSaveTool, map[string]any{"dryRun": true}); err == nil {
		t.Fatal("missing preview capability accepted")
	}
}
