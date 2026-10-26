// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableDatasourceCaller struct {
	calls   []aitableTestCall
	respond func(context.Context, string) (string, error)
	dryRun  bool
}

func (c *aitableDatasourceCaller) CallTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableTestCall{server: server, tool: tool, args: args})
	text := `{"status":"success","data":{"tableId":"tbl_test","taskId":"task_test"}}`
	if c.respond != nil {
		var err error
		text, err = c.respond(ctx, tool)
		if err != nil {
			return nil, err
		}
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{
		Type: "text",
		Text: text,
	}}}, nil
}

func (c *aitableDatasourceCaller) CallReadTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, server, tool, args)
}
func (*aitableDatasourceCaller) Format() string { return "json" }
func (c *aitableDatasourceCaller) DryRun() bool { return c.dryRun }
func (*aitableDatasourceCaller) Fields() string { return "" }
func (*aitableDatasourceCaller) JQ() string     { return "" }

func runAitableDatasourceCommand(t *testing.T, args ...string) (*aitableDatasourceCaller, error) {
	caller := &aitableDatasourceCaller{}
	return caller, runAitableDatasourceCommandWithCaller(t, context.Background(), caller, args...)
}

func runAitableDatasourceCommandWithCaller(t *testing.T, ctx context.Context, caller *aitableDatasourceCaller, args ...string) error {
	t.Helper()
	testseam.Protect(t, &os.Args)

	InitDepsForTest(t, caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	os.Args = append([]string{"dws", "aitable", "datasource"}, args...)

	root := newAitableCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"datasource"}, args...))
	return corecmd.ExecuteContextForTest(root, ctx)
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

func TestAitableDatasourceCreateWithAutoSyncSetting(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "create",
		"--base-id", "BASE123", "--datasource-type", "OA",
		"--source-config", `{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}`,
		"--auto",
		"--auto-sync-setting", `{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}`)
	if err != nil {
		t.Fatalf("create with auto-sync-setting should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "create_datasource" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	if _, exists := caller.calls[0].args["fieldIds"]; exists {
		t.Fatal("fieldIds must not be sent")
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
	if err == nil || !strings.Contains(err.Error(), "source-config") {
		t.Fatalf("error = %v, want at least one config change required", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("MCP should not be called when no changes provided")
	}
}

func TestAitableDatasourceUpdateWithAutoOnlyRejectsIncompleteReadback(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update", "--base-id", "BASE123", "--table-id", "TBL456", "--auto")
	if err == nil || !strings.Contains(err.Error(), "source-config") || len(caller.calls) != 1 || caller.calls[0].tool != "get_datasource_config" {
		t.Fatalf("missing sourceConfig: err=%v calls=%v", err, caller.calls)
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
		"--base-id", "BASE123", "--table-id", "TBL456", "--source-config", `{}`, "--auto=false")
	if err != nil {
		t.Fatalf("update with --auto=false should succeed: %v", err)
	}
	if v, ok := caller.calls[0].args["auto"]; !ok || v != false {
		t.Fatalf("auto = %v, want false", v)
	}
}

func TestAitableDatasourceUpdateWithAutoSyncSetting(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456", "--source-config", `{}`,
		"--auto-sync-setting", `{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}`)
	if err != nil {
		t.Fatalf("update with auto-sync-setting should succeed: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "update_datasource_config" {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	if _, exists := caller.calls[0].args["fieldIds"]; exists {
		t.Fatal("fieldIds must not be sent")
	}
	if caller.calls[0].args["autoSyncSetting"] != `{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}` {
		t.Fatalf("autoSyncSetting not passed as raw string: %v", caller.calls[0].args["autoSyncSetting"])
	}
}

func TestAitableDatasourceUpdateRejectsInvalidAutoSyncSetting(t *testing.T) {
	_, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456", "--source-config", `{}`,
		"--auto-sync-setting", `not-json`)
	if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
		t.Fatalf("error = %v, want auto-sync-setting validation error", err)
	}
}

func TestAitableDatasourceUpdateRejectsNonObjectAutoSyncSetting(t *testing.T) {
	cases := []string{`[]`, `"text"`, `1`, `true`, `null`}
	for _, raw := range cases {
		_, err := runAitableDatasourceCommand(t, "update",
			"--base-id", "BASE123", "--table-id", "TBL456", "--source-config", `{}`,
			"--auto-sync-setting", raw)
		if err == nil || !strings.Contains(err.Error(), "auto-sync-setting") {
			t.Fatalf("auto-sync-setting %q: error = %v, want auto-sync-setting validation error", raw, err)
		}
	}
}

func TestAitableDatasourceUpdateRejectsEmptyExplicitFieldIDs(t *testing.T) {
	caller, err := runAitableDatasourceCommand(t, "update",
		"--base-id", "BASE123", "--table-id", "TBL456", "--source-config", `{}`,
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
		"--base-id", "BASE123", "--table-id", "TBL456", "--source-config", `{}`,
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

// runAitablePluginCreate executes datasource plugin-create through the unified result store.
func runAitablePluginCreate(t *testing.T, caller *aitableTestCaller, args ...string) (map[string]any, error) {
	t.Helper()
	InitDepsForTest(t, caller)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(append(append([]string{"datasource", "plugin-create"}, args...), "--format=json"))
	ctx, _ := output.WithResultStore(context.Background())
	cmd, err := root.ExecuteContextC(ctx)
	if err != nil {
		return nil, err
	}
	code, emitted, err := output.EmitStoredResult(cmd)
	if err != nil || !emitted || code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d emitted=%v err=%v stderr=%s", code, emitted, err, stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope %q: %v", stdout, err)
	}
	return envelope, nil
}

func TestCrossPlatformCoverageAitableDatasourcePluginCreateSuccess(t *testing.T) {
	data := map[string]any{"pluginType": "minutes", "baseId": "BASE", "tableId": "TBL", "flowId": "FLOW", "documentUrl": "https://example.test/doc"}
	raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
	caller := &aitableTestCaller{responses: []string{string(raw)}}
	envelope, err := runAitablePluginCreate(t, caller, "--base-id", "BASE", "--plugin-type", "minutes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "create_plugin_datasource" ||
		!reflect.DeepEqual(caller.calls[0].args, map[string]any{"baseId": "BASE", "pluginType": "minutes"}) {
		t.Fatalf("unexpected calls: %#v", caller.calls)
	}
	if envelope["ok"] != true || envelope["outcome"] != "success" || !reflect.DeepEqual(envelope["data"], data) {
		t.Fatalf("bad envelope: %#v", envelope)
	}
}

func TestCrossPlatformCoverageAitableDatasourcePluginCreateRejectsInvalidInputBeforeCall(t *testing.T) {
	for name, args := range map[string][]string{
		"missing plugin type": {"--base-id", "BASE"},
		"missing base id":     {"--plugin-type", "minutes"},
		"uppercase type":      {"--base-id", "BASE", "--plugin-type", "Minutes"},
		"unsupported type":    {"--base-id", "BASE", "--plugin-type", "oa"},
	} {
		t.Run(name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			_, err := runAitablePluginCreate(t, caller, args...)
			if err == nil || apperrors.ExitCode(err) == 0 || len(caller.calls) != 0 {
				t.Fatalf("err=%v exit=%d calls=%#v", err, apperrors.ExitCode(err), caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableDatasourcePluginCreateDryRunSkipsCall(t *testing.T) {
	caller := &aitableTestCaller{dryRun: true}
	envelope, err := runAitablePluginCreate(t, caller, "--base-id", "BASE", "--plugin-type", "minutes")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	if len(caller.calls) != 0 || data["executed"] != false || data["tool"] != "create_plugin_datasource" {
		t.Fatalf("dry-run executed or bad preview: calls=%#v envelope=%#v", caller.calls, envelope)
	}
}

func TestCrossPlatformCoverageAitableDatasourcePluginCreateUnknownOutcome(t *testing.T) {
	cases := map[string]*aitableTestCaller{
		"missing tableId": {responses: []string{`{"success":true,"data":{"flowId":"FLOW"}}`}},
		"missing flowId":  {responses: []string{`{"success":true,"data":{"tableId":"TBL"}}`}},
		"missing data":    {responses: []string{`{"success":true}`}},
		"transport error": {errors: []error{errors.New("connection reset")}},
	}
	for name, caller := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := runAitablePluginCreate(t, caller, "--base-id", "BASE", "--plugin-type", "minutes")
			var structured *apperrors.Error
			if !errors.As(err, &structured) || structured.Reason != "create_outcome_unknown" ||
				structured.Retryable || !structured.RetryableSet ||
				structured.ExecutionStarted == nil || !*structured.ExecutionStarted ||
				!strings.Contains(structured.Hint, "table get") || len(caller.calls) != 1 {
				t.Fatalf("unsafe unknown outcome: %#v / %v / calls=%d", structured, err, len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageAitableDatasourcePluginCreateKeepsDefiniteBusinessError(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"success":false,"errorCode":"INVALID_BASE_ID","errorMsg":"base not found"}`}}
	_, err := runAitablePluginCreate(t, caller, "--base-id", "BASE", "--plugin-type", "minutes")
	var structured *apperrors.Error
	if err == nil || (errors.As(err, &structured) && structured.Reason == "create_outcome_unknown") ||
		!strings.Contains(err.Error(), "INVALID_BASE_ID") || len(caller.calls) != 1 {
		t.Fatalf("definite business error rewritten: %v", err)
	}
}
