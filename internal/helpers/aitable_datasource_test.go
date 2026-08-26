// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableDatasourceCaller struct {
	calls []aitableTestCall
}

func (c *aitableDatasourceCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableTestCall{server: server, tool: tool, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{
		Type: "text",
		Text: `{"status":"success","data":{"tableId":"tbl_test","taskId":"task_test"}}`,
	}}}, nil
}

func (*aitableDatasourceCaller) Format() string { return "json" }
func (*aitableDatasourceCaller) DryRun() bool   { return false }
func (*aitableDatasourceCaller) Fields() string { return "" }
func (*aitableDatasourceCaller) JQ() string     { return "" }

func runAitableDatasourceCommand(t *testing.T, args ...string) (*aitableDatasourceCaller, error) {
	t.Helper()
	testseam.Protect(t, &os.Args)

	caller := &aitableDatasourceCaller{}
	InitDepsForTest(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	os.Args = append([]string{"dws", "aitable", "datasource"}, args...)

	root := newAitableCommand()
	root.SetArgs(append([]string{"datasource"}, args...))
	return caller, root.Execute()
}

func TestAitableDatasourceSyncRejectsMissingTableIDs(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "sync", "--base-id", "BASE123")
	if err == nil || !strings.Contains(err.Error(), "table-ids") {
		t.Fatalf("error = %v, want table-ids required", err)
	}
}

func TestAitableDatasourceSyncRejectsTooManyTableIDs(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "sync", "--base-id", "BASE123",
		"--table-ids", "T1,T2,T3,T4,T5,T6")
	if err == nil || !strings.Contains(err.Error(), "1-5") {
		t.Fatalf("error = %v, want 1-5 limit", err)
	}
}

func TestAitableDatasourceSyncRejectsEmptyTableIDs(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "sync", "--base-id", "BASE123",
		"--table-ids", "")
	if err == nil || !strings.Contains(err.Error(), "table-ids") {
		t.Fatalf("error = %v, want table-ids error", err)
	}
}

func TestAitableDatasourceSyncAcceptsBoundaryFive(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "sync", "--base-id", "BASE123",
		"--table-ids", "T1,T2,T3,T4,T5")
	if err != nil {
		t.Fatalf("5 table-ids should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "run_datasource_sync" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
}

func TestAitableDatasourceSyncAcceptsSingleTableID(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "sync", "--base-id", "BASE123",
		"--table-ids", "T1")
	if err != nil {
		t.Fatalf("single table-id should succeed: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(caller.calls))
	}
}

func TestAitableDatasourceSyncStatusRejectsTooManyTaskIDs(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "sync-status",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--task-ids", "TK1,TK2,TK3,TK4,TK5,TK6")
	if err == nil || !strings.Contains(err.Error(), "requires 1-5") {
		t.Fatalf("error = %v, want 1-5 limit", err)
	}
}

func TestAitableDatasourceSyncStatusAcceptsFiveTaskIDs(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "sync-status",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--task-ids", "TK1,TK2,TK3,TK4,TK5")
	if err != nil {
		t.Fatalf("5 task-ids should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "get_datasource_sync_status" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
}

func TestAitableDatasourceSyncStatusRequiresTaskIDs(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "sync-status",
		"--base-id", "BASE123", "--table-id", "TBL456")
	if err == nil || !strings.Contains(err.Error(), "task-ids") {
		t.Fatalf("error = %v, want task-ids required", err)
	}
}

func TestAitableDatasourceSyncStatusRejectsMissingTableID(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "sync-status", "--base-id", "BASE123")
	if err == nil || !strings.Contains(err.Error(), "table-id") {
		t.Fatalf("error = %v, want table-id required", err)
	}
}

func TestAitableDatasourceGetConfigRejectsMissingTableID(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "get-config", "--base-id", "BASE123")
	if err == nil || !strings.Contains(err.Error(), "table-id") {
		t.Fatalf("error = %v, want table-id required", err)
	}
}

func TestAitableDatasourceGetConfigSuccess(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "get-config",
		"--base-id", "BASE123", "--table-id", "TBL456")
	if err != nil {
		t.Fatalf("get-config should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "get_datasource_config" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	if caller.calls[0].args["tableId"] != "TBL456" {
		t.Fatalf("tableId = %v, want TBL456", caller.calls[0].args["tableId"])
	}
}

func TestAitableDatasourceListSourcesSuccess(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "list-sources",
		"--base-id", "BASE123", "--datasource-type", "OA")
	if err != nil {
		t.Fatalf("list-sources should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "list_datasource_sources" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
}

func TestAitableDatasourceListSourcesRejectsMissingType(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "list-sources", "--base-id", "BASE123")
	if err == nil || !strings.Contains(err.Error(), "datasource-type") {
		t.Fatalf("error = %v, want datasource-type required", err)
	}
}

func TestAitableDatasourceGetFieldsRejectsMissingSourceConfig(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "get-fields",
		"--base-id", "BASE123", "--datasource-type", "OA")
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config required", err)
	}
}

func TestAitableDatasourceGetFieldsRejectsInvalidSourceConfig(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "get-fields",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `not-json`)
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config validation error", err)
	}
}

func TestAitableDatasourceGetFieldsRejectsNonObjectSourceConfig(t *testing.T) {
	cases := []string{`[]`, `"text"`, `1`, `true`, `null`}
	for _, raw := range cases {
		_, err := runAitableDatasourceCommand(t, "get-fields",
			"--base-id", "BASE123", "--datasource-type", "OA",
			"--source-config", raw)
		if err == nil || !strings.Contains(err.Error(), "source-config") {
			t.Fatalf("source-config %q: error = %v, want source-config validation error", raw, err)
		}
	}
}

func TestAitableDatasourceGetFieldsSuccess(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "get-fields",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}`)
	if err != nil {
		t.Fatalf("get-fields should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "get_datasource_fields" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	if caller.calls[0].args["sourceConfig"] != `{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}` {
		t.Fatalf("sourceConfig not passed as raw string: %v", caller.calls[0].args["sourceConfig"])
	}
}

func TestAitableDatasourceCreateRejectsMissingSourceConfig(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "create",
		"--base-id", "BASE123", "--datasource-type", "OA")
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config required", err)
	}
}

func TestAitableDatasourceCreateRejectsInvalidSourceConfig(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "create",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `not-json`)
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config validation error", err)
	}
}

func TestAitableDatasourceCreateRejectsNonObjectSourceConfig(t *testing.T) {
	cases := []string{`[]`, `"text"`, `1`, `true`, `null`}
	for _, raw := range cases {
		_, err := runAitableDatasourceCommand(t, "create",
			"--base-id", "BASE123", "--datasource-type", "OA",
			"--source-config", raw)
		if err == nil || !strings.Contains(err.Error(), "source-config") {
			t.Fatalf("source-config %q: error = %v, want source-config validation error", raw, err)
		}
	}
}

func TestAitableDatasourceCreateSuccess(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "create",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}`)
	if err != nil {
		t.Fatalf("create should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "create_datasource" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	if v, ok := caller.calls[0].args["auto"]; !ok || v != false {
		t.Fatalf("auto = %v, want false when not provided", v)
	}
}

func TestAitableDatasourceCreateWithAuto(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "create",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}`,
		"--auto")
	if err != nil {
		t.Fatalf("create with --auto should succeed: %v", err)
	}
	if v, ok := caller.calls[0].args["auto"]; !ok || v != true {
		t.Fatalf("auto = %v, want true", v)
	}
}

func TestAitableDatasourceCreateWithFieldIDsAndAutoSyncSetting(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "create",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}`,
		"--auto",
		"--field-ids", "fldAAA,fldBBB",
		"--auto-sync-setting", `{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}`)
	if err != nil {
		t.Fatalf("create with field-ids and auto-sync-setting should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "create_datasource" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	fieldIDs, ok := caller.calls[0].args["fieldIds"].([]string)
	if !ok || len(fieldIDs) != 2 || fieldIDs[0] != "fldAAA" || fieldIDs[1] != "fldBBB" {
		t.Fatalf("fieldIds = %v, want [fldAAA fldBBB]", caller.calls[0].args["fieldIds"])
	}
	if caller.calls[0].args["autoSyncSetting"] != `{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}` {
		t.Fatalf("autoSyncSetting not passed as raw string: %v", caller.calls[0].args["autoSyncSetting"])
	}
}

func TestAitableDatasourceCreateRejectsInvalidAutoSyncSetting(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "create",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}`,
		"--auto-sync-setting", `not-json`)
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting validation error", err)
	}
}

func TestAitableDatasourceUpdateRejectsMissingTableID(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "update", "--base-id", "BASE123")
	if err == nil || !strings.Contains(err.Error(), "table-id") {
		t.Fatalf("error = %v, want table-id required", err)
	}
}

func TestAitableDatasourceUpdateRejectsInvalidSourceConfig(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--source-config", `not-json`)
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want source-config validation error", err)
	}
}

func TestAitableDatasourceUpdateRejectsNonObjectSourceConfig(t *testing.T) {
	cases := []string{`[]`, `"text"`, `1`, `true`, `null`}
	for _, raw := range cases {
		_, err := runAitableDatasourceCommand(t, "update",
			"--base-id", "BASE123", "--table-id", "TBL456",
			"--source-config", raw)
		if err == nil || !strings.Contains(err.Error(), "source-config") {
			t.Fatalf("source-config %q: error = %v, want source-config validation error", raw, err)
		}
	}
}

func TestAitableDatasourceUpdateRejectsNoChanges(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456")
	if err == nil || !strings.Contains(err.Error(), "至少需要一个配置变更") {
		t.Fatalf("error = %v, want at least one config change required", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("MCP should not be called when no changes provided")
	}
}

func TestAitableDatasourceUpdateWithAutoOnly(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456", "--auto")
	if err != nil {
		t.Fatalf("update with --auto only should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "update_datasource_config" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	if v, ok := caller.calls[0].args["auto"]; !ok || v != true {
		t.Fatalf("auto = %v, want true", v)
	}
}

func TestAitableDatasourceUpdateWithSourceConfig(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--source-config", `{"processCode":"PROC-YYYY","name":"出差申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}`)
	if err != nil {
		t.Fatalf("update with source-config should succeed: %v", err)
	}
	if caller.calls[0].args["sourceConfig"] != `{"processCode":"PROC-YYYY","name":"出差申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}` {
		t.Fatalf("sourceConfig not passed as raw string: %v", caller.calls[0].args["sourceConfig"])
	}
	if _, ok := caller.calls[0].args["auto"]; ok {
		t.Fatalf("auto should not be sent when --auto is omitted")
	}
}

func TestAitableDatasourceUpdateWithAutoFalse(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456", "--auto=false")
	if err != nil {
		t.Fatalf("update with --auto=false should succeed: %v", err)
	}
	if v, ok := caller.calls[0].args["auto"]; !ok || v != false {
		t.Fatalf("auto = %v, want false", v)
	}
}

func TestAitableDatasourceUpdateWithFieldIDsAndAutoSyncSetting(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--field-ids", "fldAAA,fldBBB",
		"--auto-sync-setting", `{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}`)
	if err != nil {
		t.Fatalf("update with field-ids and auto-sync-setting should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "update_datasource_config" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	fieldIDs, ok := caller.calls[0].args["fieldIds"].([]string)
	if !ok || len(fieldIDs) != 2 || fieldIDs[0] != "fldAAA" || fieldIDs[1] != "fldBBB" {
		t.Fatalf("fieldIds = %v, want [fldAAA fldBBB]", caller.calls[0].args["fieldIds"])
	}
	if caller.calls[0].args["autoSyncSetting"] != `{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}` {
		t.Fatalf("autoSyncSetting not passed as raw string: %v", caller.calls[0].args["autoSyncSetting"])
	}
}

func TestAitableDatasourceUpdateRejectsInvalidAutoSyncSetting(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--auto-sync-setting", `not-json`)
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting validation error", err)
	}
}

func TestAitableDatasourceUpdateRejectsNonObjectAutoSyncSetting(t *testing.T) {
	cases := []string{`[]`, `"text"`, `1`, `true`, `null`}
	for _, raw := range cases {
		_, err := runAitableDatasourceCommand(t, "update",
			"--base-id", "BASE123", "--table-id", "TBL456",
			"--auto-sync-setting", raw)
		if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
			t.Fatalf("auto-sync-setting %q: error = %v, want auto-sync-setting validation error", raw, err)
		}
	}
}

func TestAitableDatasourceUpdateRejectsEmptyExplicitFieldIDs(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--field-ids", "")
	if err == nil || !strings.Contains(err.Error(), "field-ids") {
		t.Fatalf("error = %v, want field-ids empty error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("MCP should not be called when empty field-ids is rejected")
	}
}

func TestAitableDatasourceUpdateRejectsEmptyExplicitAutoSyncSetting(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456",
		"--auto-sync-setting", "")
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting empty error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("MCP should not be called when empty auto-sync-setting is rejected")
	}
}

func TestAitableDatasourceGetFieldsRejectsMissingBaseID(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "get-fields",
		"--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`)
	if err == nil || !strings.Contains(err.Error(), "base-id") {
		t.Fatalf("error = %v, want base-id required", err)
	}
}

func TestAitableDatasourceCreateRejectsMissingBaseID(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "create",
		"--datasource-type", "OA",
		"--source-config", `{"processCode":"P","name":"N","dataType":"recent_time","iconUrl":"u","url":"v"}`)
	if err == nil || !strings.Contains(err.Error(), "base-id") {
		t.Fatalf("error = %v, want base-id required", err)
	}
}
