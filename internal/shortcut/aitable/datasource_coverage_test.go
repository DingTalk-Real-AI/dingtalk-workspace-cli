// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type datasourceCoverageCaller struct {
	err    error
	resp   string
	argLog []map[string]any
}

func (c *datasourceCoverageCaller) CallTool(_ context.Context, _, _ string, args map[string]any) (*edition.ToolResult, error) {
	c.argLog = append(c.argLog, args)
	if c.err != nil {
		return nil, c.err
	}
	text := c.resp
	if text == "" {
		text = `{"status":"success","data":{}}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (c *datasourceCoverageCaller) Format() string { return "json" }
func (c *datasourceCoverageCaller) DryRun() bool   { return false }
func (c *datasourceCoverageCaller) Fields() string { return "" }
func (c *datasourceCoverageCaller) JQ() string     { return "" }

func runDatasourceShortcutCLI(t *testing.T, caller *datasourceCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := newPlatformCoverageRoot()
	root.SetArgs(append([]string{"aitable"}, args...))
	return root.Execute()
}

// ── DatasourceCreate error paths ─────────────────────────────────────────────

func TestCrossPlatformCoverageDatasourceCreateRejectsInvalidSourceConfig(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-create", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", "not-json")
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config parse error", err)
	}
}

func TestCrossPlatformCoverageDatasourceCreateRejectsInvalidAutoSyncSetting(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-create", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`,
		"--auto-sync-setting", "not-json")
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting parse error", err)
	}
}

func TestCrossPlatformCoverageDatasourceCreatePropagatesCallMCPError(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{err: errors.New("mcp-error")},
		"+datasource-create", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`)
	if err == nil || !strings.Contains(err.Error(), "mcp-error") {
		t.Fatalf("error = %v, want mcp-error", err)
	}
}

func TestCrossPlatformCoverageDatasourceCreateRejectsEmptyFieldIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-create", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`,
		"--field-ids", "")
	if err == nil || !strings.Contains(err.Error(), "field-ids") {
		t.Fatalf("error = %v, want field-ids empty error", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when empty field-ids is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceCreateRejectsWhitespaceOnlyFieldIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-create", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`,
		"--field-ids", " , , ")
	if err == nil || !strings.Contains(err.Error(), "field-ids") {
		t.Fatalf("error = %v, want field-ids empty error", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when whitespace-only field-ids is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceCreateRejectsEmptyAutoSyncSetting(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-create", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`,
		"--auto-sync-setting", "")
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting empty error", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when empty auto-sync-setting is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceCreatePassesFieldIDsThrough(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-create", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`,
		"--field-ids", "fld1,fld2")
	if err != nil {
		t.Fatalf("create with valid --field-ids should succeed: %v", err)
	}
	if len(caller.argLog) != 1 {
		t.Fatalf("expected exactly 1 MCP call, got %d", len(caller.argLog))
	}
	got, _ := caller.argLog[0]["fieldIds"].([]string)
	if len(got) != 2 || got[0] != "fld1" || got[1] != "fld2" {
		t.Fatalf("fieldIds = %#v, want [fld1 fld2]", got)
	}
}

// ── DatasourceUpdate error paths ─────────────────────────────────────────────

func TestCrossPlatformCoverageDatasourceUpdateRejectsInvalidSourceConfig(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1",
		"--source-config", "not-json")
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config parse error", err)
	}
}

func TestCrossPlatformCoverageDatasourceUpdateRejectsInvalidAutoSyncSetting(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1",
		"--auto-sync-setting", "not-json")
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting parse error", err)
	}
}

func TestCrossPlatformCoverageDatasourceUpdateRejectsNoChanges(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1")
	if err == nil || !strings.Contains(err.Error(), "至少需要一个配置变更") {
		t.Fatalf("error = %v, want no-changes error", err)
	}
}

func TestCrossPlatformCoverageDatasourceUpdatePropagatesCallMCPError(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{err: errors.New("mcp-error")},
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1", "--auto")
	if err == nil || !strings.Contains(err.Error(), "mcp-error") {
		t.Fatalf("error = %v, want mcp-error", err)
	}
}

func TestCrossPlatformCoverageDatasourceUpdateRejectsEmptyFieldIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1",
		"--field-ids", "")
	if err == nil || !strings.Contains(err.Error(), "field-ids") {
		t.Fatalf("error = %v, want field-ids empty error", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when empty field-ids is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceUpdateRejectsWhitespaceOnlyFieldIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1",
		"--field-ids", " , ")
	if err == nil || !strings.Contains(err.Error(), "field-ids") {
		t.Fatalf("error = %v, want field-ids empty error", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when whitespace-only field-ids is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceUpdateRejectsEmptyAutoSyncSetting(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1",
		"--auto-sync-setting", "")
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting empty error", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when empty auto-sync-setting is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceUpdatePassesFieldIDsThrough(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-update", "--base-id", "BASE1", "--table-id", "TBL1",
		"--field-ids", "fldA,fldB,fldC")
	if err != nil {
		t.Fatalf("update with valid --field-ids should succeed: %v", err)
	}
	if len(caller.argLog) != 1 {
		t.Fatalf("expected exactly 1 MCP call, got %d", len(caller.argLog))
	}
	got, _ := caller.argLog[0]["fieldIds"].([]string)
	if len(got) != 3 || got[0] != "fldA" || got[1] != "fldB" || got[2] != "fldC" {
		t.Fatalf("fieldIds = %#v, want [fldA fldB fldC]", got)
	}
}

// ── DatasourceSync error paths ────────────────────────────────────────────────

func TestCrossPlatformCoverageDatasourceSyncShortcutRejectsTooManyTableIDs(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-sync", "--base-id", "BASE1", "--table-ids", "T1,T2,T3,T4,T5,T6")
	if err == nil || !strings.Contains(err.Error(), "1-5") {
		t.Fatalf("error = %v, want 1-5 limit", err)
	}
}

func TestCrossPlatformCoverageDatasourceSyncShortcutPropagatesCallMCPError(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{err: errors.New("mcp-error")},
		"+datasource-sync", "--base-id", "BASE1", "--table-ids", "TBL1")
	if err == nil || !strings.Contains(err.Error(), "mcp-error") {
		t.Fatalf("error = %v, want mcp-error", err)
	}
}

// ── DatasourceSyncStatus error paths ─────────────────────────────────────────

func TestCrossPlatformCoverageDatasourceSyncStatusShortcutRejectsTooManyTaskIDs(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-sync-status", "--base-id", "BASE1", "--table-id", "TBL1",
		"--task-ids", "T1,T2,T3,T4,T5,T6")
	if err == nil || !strings.Contains(err.Error(), "1-5") {
		t.Fatalf("error = %v, want 1-5 limit", err)
	}
}

func TestCrossPlatformCoverageDatasourceSyncStatusShortcutPropagatesCallMCPError(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{err: errors.New("mcp-error")},
		"+datasource-sync-status", "--base-id", "BASE1", "--table-id", "TBL1",
		"--task-ids", "TASK1")
	if err == nil || !strings.Contains(err.Error(), "mcp-error") {
		t.Fatalf("error = %v, want mcp-error", err)
	}
}

// ── DatasourceSync whitespace trimming ──────────────────────────────────────

func TestCrossPlatformCoverageDatasourceSyncRejectsWhitespaceOnlyTableIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-sync", "--base-id", "BASE1", "--table-ids", " , ")
	if err == nil || (!strings.Contains(err.Error(), "table-ids") && !strings.Contains(err.Error(), "1-5")) {
		t.Fatalf("error = %v, want table-ids rejection", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when whitespace-only table-ids is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceSyncTrimsWhitespaceFromTableIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-sync", "--base-id", "BASE1", "--table-ids", " TBL1 , TBL2 ")
	if err != nil {
		t.Fatalf("sync with whitespace-padded --table-ids should succeed: %v", err)
	}
	if len(caller.argLog) != 1 {
		t.Fatalf("expected exactly 1 MCP call, got %d", len(caller.argLog))
	}
	got, _ := caller.argLog[0]["tableIds"].([]string)
	if len(got) != 2 || got[0] != "TBL1" || got[1] != "TBL2" {
		t.Fatalf("tableIds = %#v, want [TBL1 TBL2]", got)
	}
}

// ── DatasourceSyncStatus whitespace trimming ───────────────────────────────

func TestCrossPlatformCoverageDatasourceSyncStatusRejectsWhitespaceOnlyTaskIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-sync-status", "--base-id", "BASE1", "--table-id", "TBL1",
		"--task-ids", " , ")
	if err == nil || (!strings.Contains(err.Error(), "task-ids") && !strings.Contains(err.Error(), "1-5")) {
		t.Fatalf("error = %v, want task-ids rejection", err)
	}
	if len(caller.argLog) != 0 {
		t.Fatalf("MCP should not be called when whitespace-only task-ids is rejected, got %d calls", len(caller.argLog))
	}
}

func TestCrossPlatformCoverageDatasourceSyncStatusTrimsWhitespaceFromTaskIDs(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller,
		"+datasource-sync-status", "--base-id", "BASE1", "--table-id", "TBL1",
		"--task-ids", " TASK1 , TASK2 ")
	if err != nil {
		t.Fatalf("sync-status with whitespace-padded --task-ids should succeed: %v", err)
	}
	if len(caller.argLog) != 1 {
		t.Fatalf("expected exactly 1 MCP call, got %d", len(caller.argLog))
	}
	got, _ := caller.argLog[0]["taskIds"].([]string)
	if len(got) != 2 || got[0] != "TASK1" || got[1] != "TASK2" {
		t.Fatalf("taskIds = %#v, want [TASK1 TASK2]", got)
	}
}

// ── DatasourceGetConfig error path ───────────────────────────────────────────

func TestCrossPlatformCoverageDatasourceGetConfigShortcutPropagatesCallMCPError(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{err: errors.New("mcp-error")},
		"+datasource-get-config", "--base-id", "BASE1", "--table-id", "TBL1")
	if err == nil || !strings.Contains(err.Error(), "mcp-error") {
		t.Fatalf("error = %v, want mcp-error", err)
	}
}

// ── DatasourceListSources error path ─────────────────────────────────────────

func TestCrossPlatformCoverageDatasourceListSourcesShortcutPropagatesCallMCPError(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{err: errors.New("mcp-error")},
		"+datasource-list-sources", "--base-id", "BASE1", "--datasource-type", "OA")
	if err == nil || !strings.Contains(err.Error(), "mcp-error") {
		t.Fatalf("error = %v, want mcp-error", err)
	}
}

// ── DatasourceGetFields error paths ──────────────────────────────────────────

func TestCrossPlatformCoverageDatasourceGetFieldsShortcutRejectsInvalidSourceConfig(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{},
		"+datasource-get-fields", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", "not-json")
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config parse error", err)
	}
}

func TestCrossPlatformCoverageDatasourceGetFieldsShortcutPropagatesCallMCPError(t *testing.T) {
	err := runDatasourceShortcutCLI(t, &datasourceCoverageCaller{err: errors.New("mcp-error")},
		"+datasource-get-fields", "--base-id", "BASE1", "--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`)
	if err == nil || !strings.Contains(err.Error(), "mcp-error") {
		t.Fatalf("error = %v, want mcp-error", err)
	}
}
