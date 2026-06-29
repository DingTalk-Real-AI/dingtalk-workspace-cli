package cli_compat_test

import (
	"bytes"
	"encoding/json"
	"testing"
)

// ── base list ───────────────────────────────────────────────

func TestAitableBaseList_should_call_list_bases(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	err := execCmd(t, root, []string{"aitable", "base", "list"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "list_bases")
	assertCallCount(t, cap, 1)
}

// ── base search ─────────────────────────────────────────────

func TestAitableBaseSearch_should_call_search_bases(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "search"}, map[string]string{
		"query": "项目管理",
	})
	assertToolName(t, cap, "search_bases")
	assertToolArg(t, cap, "query", "项目管理")
}

func TestAitableBaseSearch_without_query_should_call_list_bases(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "base", "search"}, map[string]string{
		"cursor": "NEXT_CURSOR",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "list_bases")
	assertToolArg(t, cap, "cursor", "NEXT_CURSOR")
	assertArgNotPresent(t, cap, "query")
	assertCallCount(t, cap, 1)
}

// ── base get ────────────────────────────────────────────────

func TestAitableBaseGet_should_call_get_base(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "get"}, map[string]string{
		"base-id": "BASE_001",
	})
	assertToolName(t, cap, "get_base")
	assertToolArg(t, cap, "baseId", "BASE_001")
}

// ── base create ─────────────────────────────────────────────

func TestAitableBaseCreate_should_call_create_base(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "create"}, map[string]string{
		"name": "新表格",
	})
	assertToolName(t, cap, "create_base")
	assertToolArg(t, cap, "baseName", "新表格")
}

func TestAitableBaseCreate_should_pass_template_id(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "create"}, map[string]string{
		"name": "模板表格", "template-id": "TPL_001",
	})
	assertToolArg(t, cap, "templateId", "TPL_001")
}

// ── base update ─────────────────────────────────────────────

func TestAitableBaseUpdate_should_call_update_base(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "update"}, map[string]string{
		"base-id": "BASE_001", "name": "新名称",
	})
	assertToolName(t, cap, "update_base")
	assertToolArg(t, cap, "baseId", "BASE_001")
	assertToolArg(t, cap, "newBaseName", "新名称")
}

func TestAitableBaseUpdate_should_pass_description(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "update"}, map[string]string{
		"base-id": "B1", "name": "N", "desc": "备注",
	})
	assertToolArg(t, cap, "description", "备注")
}

// ── base delete ─────────────────────────────────────────────

func TestAitableBaseDelete_should_call_delete_base(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "delete"}, map[string]string{
		"base-id": "BASE_DEL",
	})
	assertToolName(t, cap, "delete_base")
	assertToolArg(t, cap, "baseId", "BASE_DEL")
}

func TestAitableBaseDelete_should_pass_reason(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "delete"}, map[string]string{
		"base-id": "B1", "reason": "不再需要",
	})
	assertToolArg(t, cap, "reason", "不再需要")
}

func TestAitableBaseDelete_should_not_call_mcp_in_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "base", "delete"}, map[string]string{
		"base-id": "B1",
	})
	assertCallCount(t, cap, 0)
}

// ── table get ───────────────────────────────────────────────

func TestAitableTableGet_should_call_get_tables(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "table", "get"}, map[string]string{
		"base-id": "BASE_001",
	})
	assertToolName(t, cap, "get_tables")
	assertToolArg(t, cap, "baseId", "BASE_001")
}

func TestAitableTableGet_should_pass_table_ids(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "table", "get"}, map[string]string{
		"base-id": "B1", "table-ids": "tbl1,tbl2",
	})
	assertToolArg(t, cap, "tableIds", []string{"tbl1", "tbl2"})
}

// ── table create ────────────────────────────────────────────

func TestAitableTableCreate_should_call_create_table(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "table", "create"}, map[string]string{
		"base-id": "B1", "name": "任务表",
		"fields": `[{"fieldName":"名称","type":"text"}]`,
	})
	assertToolName(t, cap, "create_table")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "tableName", "任务表")
}

func TestAitableTableCreate_without_fields_should_pass_empty_fields(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "table", "create"}, map[string]string{
		"base-id": "B1", "name": "空字段表",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "create_table")
	assertToolArg(t, cap, "fields", []any{})
}

// ── table update ────────────────────────────────────────────

func TestAitableTableUpdate_should_call_update_table(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "table", "update"}, map[string]string{
		"base-id": "B1", "table-id": "TBL_001", "name": "新表名",
	})
	assertToolName(t, cap, "update_table")
	assertToolArg(t, cap, "newTableName", "新表名")
}

// ── table delete ────────────────────────────────────────────

func TestAitableTableDelete_should_call_delete_table(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "table", "delete"}, map[string]string{
		"base-id": "B1", "table-id": "TBL_DEL",
	})
	assertToolName(t, cap, "delete_table")
	assertToolArg(t, cap, "tableId", "TBL_DEL")
}

func TestAitableTableDelete_should_pass_reason(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "table", "delete"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "reason": "测试清理",
	})
	assertToolArg(t, cap, "reason", "测试清理")
}

// ── field get ───────────────────────────────────────────────

func TestAitableFieldGet_should_call_get_fields(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "field", "get"}, map[string]string{
		"base-id": "B1", "table-id": "T1",
	})
	assertToolName(t, cap, "get_fields")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "tableId", "T1")
}

func TestAitableFieldGet_should_pass_field_ids(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "field", "get"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "field-ids": "fld1,fld2",
	})
	assertToolArg(t, cap, "fieldIds", []string{"fld1", "fld2"})
}

// ── field create ────────────────────────────────────────────

func TestAitableFieldCreate_should_call_create_fields(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "field", "create"}, map[string]string{
		"base-id": "B1", "table-id": "T1",
		"fields": `[{"fieldName":"状态","type":"singleSelect"}]`,
	})
	assertToolName(t, cap, "create_fields")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "tableId", "T1")
}

// ── field update ────────────────────────────────────────────

func TestAitableFieldUpdate_should_call_update_field(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "field", "update"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "field-id": "FLD_001",
		"name": "新字段名",
	})
	assertToolName(t, cap, "update_field")
	assertToolArg(t, cap, "fieldId", "FLD_001")
	assertToolArg(t, cap, "newFieldName", "新字段名")
}

func TestAitableFieldUpdate_should_pass_config_json(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "field", "update"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "field-id": "F1",
		"config": `{"options":[{"name":"X"}]}`,
	})
	expected := map[string]any{
		"options": []any{map[string]any{"name": "X"}},
	}
	assertToolArg(t, cap, "config", expected)
}

func TestAitableFieldUpdate_should_pass_ai_config_json(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "field", "update"}, map[string]string{
		"base-id":   "B1",
		"table-id":  "T1",
		"field-id":  "F1",
		"ai-config": `{"outputType":"text","prompt":[{"type":"text","value":"请总结"}]}`,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := map[string]any{
		"outputType": "text",
		"prompt":     []any{map[string]any{"type": "text", "value": "请总结"}},
	}
	assertToolName(t, cap, "update_field")
	assertToolArg(t, cap, "aiConfig", expected)
}

func TestAitableFieldUpdate_without_mutation_should_fail_before_call(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	err := execCmd(t, root, []string{"aitable", "field", "update"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "field-id": "F1",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	assertCallCount(t, cap, 0)
}

// ── field delete ────────────────────────────────────────────

func TestAitableFieldDelete_should_call_delete_field(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "field", "delete"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "field-id": "FLD_DEL",
	})
	assertToolName(t, cap, "delete_field")
	assertToolArg(t, cap, "fieldId", "FLD_DEL")
}

// ── record query ────────────────────────────────────────────

func TestAitableRecordQuery_should_call_query_records(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "record", "query"}, map[string]string{
		"base-id": "B1", "table-id": "T1",
	})
	assertToolName(t, cap, "query_records")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "tableId", "T1")
}

func TestAitableRecordQuery_should_pass_record_ids(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "record", "query"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "record-ids": "rec1,rec2",
	})
	assertToolArg(t, cap, "recordIds", []string{"rec1", "rec2"})
}

func TestAitableRecordQuery_should_normalize_sort_order_and_query_keyword(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "record", "query"}, map[string]string{
		"base-id":  "B1",
		"table-id": "T1",
		"query":    "待办",
		"sort":     `[{"fieldId":"fld_due","order":"desc"}]`,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "query_records")
	assertToolArg(t, cap, "keyword", "待办")
	assertArgNotPresent(t, cap, "query")
	assertToolArg(t, cap, "sort", []any{map[string]any{
		"fieldId":   "fld_due",
		"direction": "desc",
	}})
}

// ── record create ───────────────────────────────────────────

func TestAitableRecordCreate_should_call_create_records(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "record", "create"}, map[string]string{
		"base-id": "B1", "table-id": "T1",
		"records": `[{"cells":{"fld1":"hello"}}]`,
	})
	assertToolName(t, cap, "create_records")
	assertToolArg(t, cap, "baseId", "B1")
}

// ── record update ───────────────────────────────────────────

func TestAitableRecordUpdate_should_call_update_records(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "record", "update"}, map[string]string{
		"base-id": "B1", "table-id": "T1",
		"records": `[{"recordId":"rec1","cells":{"fld1":"updated"}}]`,
	})
	assertToolName(t, cap, "update_records")
}

// ── record delete ───────────────────────────────────────────

func TestAitableRecordDelete_should_call_delete_records(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "record", "delete"}, map[string]string{
		"base-id": "B1", "table-id": "T1", "record-ids": "rec1,rec2",
	})
	assertToolName(t, cap, "delete_records")
	assertToolArg(t, cap, "recordIds", []string{"rec1", "rec2"})
	assertCallCount(t, cap, 1)
}

// ── template search ─────────────────────────────────────────

func TestAitableTemplateSearch_should_call_search_templates(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	_ = execCmd(t, root, []string{"aitable", "template", "search"}, map[string]string{
		"query": "项目管理",
	})
	assertToolName(t, cap, "search_templates")
	assertToolArg(t, cap, "query", "项目管理")
}

// ── export data ─────────────────────────────────────────────

func TestAitableExportData_should_match_wukong_flag_surface(t *testing.T) {
	root := buildRoot()
	cmd, _, err := root.Find([]string{"aitable", "export", "data"})
	if err != nil {
		t.Fatalf("find export data command: %v", err)
	}
	for _, name := range []string{"base-id", "scope", "format", "task-id", "table-id", "view-id", "timeout-ms"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected --%s flag to exist", name)
		}
	}
	for _, name := range []string{"export-format", "output", "timeout-sec"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Fatalf("flag --%s should not be exposed by Wukong-compatible export data", name)
		}
	}
}

func TestAitableExportData_should_call_export_data_with_wukong_payload(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"aitable", "export", "data",
		"--base-id", "B1",
		"--scope", "table",
		"--table-id", "T1",
		"--format", "excel",
		"--timeout-ms", "1000",
		"--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var inv struct {
		Invocation struct {
			Tool   string         `json:"tool"`
			Params map[string]any `json:"params"`
			DryRun bool           `json:"dry_run"`
		} `json:"invocation"`
	}
	var flatInv struct {
		Tool   string         `json:"tool"`
		Params map[string]any `json:"params"`
		DryRun bool           `json:"dry_run"`
	}
	if err := json.Unmarshal(out.Bytes(), &inv); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\noutput:\n%s", err, out.String())
	}
	if inv.Invocation.Tool != "" {
		cap.record(inv.Invocation.Tool, inv.Invocation.Params, "")
	} else if err := json.Unmarshal(out.Bytes(), &flatInv); err == nil && flatInv.Tool != "" {
		cap.record(flatInv.Tool, flatInv.Params, "")
	}

	assertToolName(t, cap, "export_data")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "scope", "table")
	assertToolArg(t, cap, "tableId", "T1")
	assertToolArg(t, cap, "format", "excel")
	assertToolArg(t, cap, "timeoutMs", 1000)
	assertArgNotPresent(t, cap, "__async__")
	assertArgNotPresent(t, cap, "__output__")
	assertCallCount(t, cap, 1)
}

// ── view ────────────────────────────────────────────────────

func TestAitableViewCreate_should_pass_sub_type_description_and_config(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "view", "create"}, map[string]string{
		"base-id":       "B1",
		"table-id":      "T1",
		"view-type":     "Grid",
		"view-sub-type": "Grid",
		"name":          "视图名",
		"desc":          `{"content":[]}`,
		"config":        `{"visibleFieldIds":["fld1","fld2"]}`,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "create_view")
	assertToolArg(t, cap, "viewSubType", "Grid")
	assertToolArg(t, cap, "viewDescription", map[string]any{"content": []any{}})
	assertToolArg(t, cap, "config", map[string]any{"visibleFieldIds": []any{"fld1", "fld2"}})
}

func TestAitableViewUpdate_should_pass_description(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "view", "update"}, map[string]string{
		"base-id":  "B1",
		"table-id": "T1",
		"view-id":  "V1",
		"desc":     `{"content":[]}`,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "update_view")
	assertToolArg(t, cap, "viewDescription", map[string]any{"content": []any{}})
}

func TestAitableRecordHistoryList_should_call_query_record_history(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "record", "history-list"}, map[string]string{
		"base-id":   "B1",
		"table-id":  "T1",
		"record-id": "R1",
		"limit":     "30",
		"offset":    "10",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "query_record_history")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "tableId", "T1")
	assertToolArg(t, cap, "recordId", "R1")
	assertToolArg(t, cap, "limit", 30)
	assertToolArg(t, cap, "offset", 10)
}

func TestAitableRecordUpsert_should_call_record_upsert(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "record", "upsert"}, map[string]string{
		"base-id":  "B1",
		"table-id": "T1",
		"records":  `[{"recordId":"R1","cells":{"fld":"updated"}},{"cells":{"fld":"new"}}]`,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "record_upsert")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "tableId", "T1")
}

func TestAitableViewLock_should_call_lock_or_unlock_view(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmdWithArgs(t, root, []string{"aitable", "view", "lock"}, map[string]string{
		"base-id":  "B1",
		"table-id": "T1",
		"view-id":  "V1",
	}, []string{"--off"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "lock_or_unlock_view")
	assertToolArg(t, cap, "action", "unlock")
}

func TestAitableWorkflowList_should_call_list_workflows(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "workflow", "list"}, map[string]string{
		"base-id": "B1",
		"limit":   "50",
		"offset":  "100",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "list_workflows")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "limit", 50)
	assertToolArg(t, cap, "offset", 100)
}

func TestAitableAdvpermRoleCreate_should_call_create_role(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "advperm", "role-create"}, map[string]string{
		"base-id":   "B1",
		"name":      "市场可读",
		"sub-roles": `[{"targetId":"T1","targetType":"sheet","authLevel":"read"}]`,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "create_role")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "name", "市场可读")
	assertToolArg(t, cap, "subRoles", []any{map[string]any{
		"targetId":   "T1",
		"targetType": "sheet",
		"authLevel":  "read",
	}})
}

func TestAitableSectionMoveNode_should_call_move_nsheet_node(t *testing.T) {
	cap := setupTestDeps(t, "aitable")
	root := buildRoot()
	if err := execCmd(t, root, []string{"aitable", "section", "move-node"}, map[string]string{
		"base-id":               "B1",
		"node-id":               "NODE_001",
		"new-parent-section-id": "SEC_001",
		"target-index":          "0",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "move_nsheet_node")
	assertToolArg(t, cap, "baseId", "B1")
	assertToolArg(t, cap, "nodeId", "NODE_001")
	assertToolArg(t, cap, "newParentSectionId", "SEC_001")
	assertToolArg(t, cap, "targetIndex", 0)
}

func TestAitableSurface_should_not_expose_open_source_only_commands(t *testing.T) {
	root := buildRoot()
	for _, path := range [][]string{
		{"aitable", "record", "batch-update"},
		{"aitable", "attachment", "upload-file"},
		{"aitable", "form"},
	} {
		cmd, _, err := root.Find(path)
		if err == nil && cmd != nil && !cmd.Hidden {
			t.Fatalf("command %v should not be visible in Wukong-compatible surface", path)
		}
	}
}
