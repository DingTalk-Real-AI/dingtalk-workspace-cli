package cli_compat_test

import (
	"testing"
)

// ── task create ─────────────────────────────────────────────

func TestTodoTaskCreate_should_call_create_personal_todo(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "create"}, map[string]string{
		"title":     "修复线上Bug",
		"executors": "userId1,userId2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "create_personal_todo")
}

func TestTodoTaskCreate_should_pass_subject_and_executorIds(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "create"}, map[string]string{
		"title":     "提交报告",
		"executors": "userId1",
	})
	assertToolArg(t, cap, "PersonalTodoCreateVO", map[string]any{
		"subject":     "提交报告",
		"executorIds": []any{"userId1"},
	})
}

func TestTodoTaskCreate_should_pass_priority(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "create"}, map[string]string{
		"title":     "紧急任务",
		"executors": "userId1",
		"priority":  "40",
	})
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	vo, ok := last.Args["PersonalTodoCreateVO"].(map[string]any)
	if !ok {
		t.Fatalf("expected PersonalTodoCreateVO map, got %T", last.Args["PersonalTodoCreateVO"])
	}
	if vo["priority"] != float64(40) {
		t.Errorf("expected priority=40, got %v", vo["priority"])
	}
}

func TestTodoTaskCreate_should_error_when_title_missing(t *testing.T) {
	_ = setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "create"}, map[string]string{
		"executors": "userId1",
	})
	if err == nil {
		t.Fatal("expected error when --title is missing")
	}
}

func TestTodoTaskCreate_should_error_when_executors_missing(t *testing.T) {
	_ = setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "create"}, map[string]string{
		"title": "测试待办",
	})
	if err == nil {
		t.Fatal("expected error when --executors is missing")
	}
}

func TestTodoTaskCreate_should_accept_subject_alias(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "create"}, map[string]string{
		"subject":   "通过别名创建",
		"executors": "userId1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "create_personal_todo")
}

func TestTodoTaskCreate_should_not_call_mcp_in_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "create"}, map[string]string{
		"title":     "Dry Run",
		"executors": "userId1",
	})
	assertCallCount(t, cap, 0)
}

// ── task list ───────────────────────────────────────────────

func TestTodoTaskList_should_call_get_user_todos(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "list"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "get_user_todos_in_current_org")
}

func TestTodoTaskList_should_pass_status_filter(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "list"}, map[string]string{
		"status": "false",
	})
	assertToolArg(t, cap, "isDone", "false")
	assertToolArg(t, cap, "todoStatus", "false")
}

// ── task update ─────────────────────────────────────────────

func TestTodoTaskUpdate_should_call_update_todo_task(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "update"}, map[string]string{
		"task-id": "TASK_001",
		"title":   "新标题",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "update_todo_task")
}

func TestTodoTaskUpdate_should_pass_TodoUpdateRequest(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "update"}, map[string]string{
		"task-id": "TASK_001",
		"title":   "更新标题",
	})
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	req, ok := last.Args["TodoUpdateRequest"].(map[string]any)
	if !ok {
		t.Fatalf("expected TodoUpdateRequest map, got %T", last.Args["TodoUpdateRequest"])
	}
	if req["taskId"] != "TASK_001" {
		t.Errorf("expected taskId=TASK_001, got %v", req["taskId"])
	}
	if req["subject"] != "更新标题" {
		t.Errorf("expected subject=更新标题, got %v", req["subject"])
	}
}

func TestTodoTaskUpdate_should_pass_done_flag(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "update"}, map[string]string{
		"task-id": "TASK_001",
		"done":    "true",
	})
	last := cap.last()
	if last == nil {
		t.Fatal("no MCP call captured")
	}
	req, ok := last.Args["TodoUpdateRequest"].(map[string]any)
	if !ok {
		t.Fatalf("expected TodoUpdateRequest map, got %T", last.Args["TodoUpdateRequest"])
	}
	if req["isDone"] != true {
		t.Errorf("expected isDone=true, got %v", req["isDone"])
	}
}

func TestTodoTaskUpdate_should_error_when_task_id_missing(t *testing.T) {
	_ = setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "update"}, map[string]string{
		"title": "新标题",
	})
	if err == nil {
		t.Fatal("expected error when --task-id is missing")
	}
}

func TestTodoTaskUpdate_should_not_call_mcp_in_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "update"}, map[string]string{
		"task-id": "TASK_001",
		"title":   "Dry Run",
	})
	assertCallCount(t, cap, 0)
}

// ── task done ───────────────────────────────────────────────

func TestTodoTaskDone_should_call_update_todo_done_status(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "done"}, map[string]string{
		"task-id": "TASK_001",
		"status":  "true",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "update_todo_done_status")
}

func TestTodoTaskDone_should_pass_taskId_and_isDone(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "done"}, map[string]string{
		"task-id": "TASK_001",
		"status":  "true",
	})
	assertToolArg(t, cap, "taskId", "TASK_001")
	assertToolArg(t, cap, "isDone", "true")
}

func TestTodoTaskDone_should_error_when_task_id_missing(t *testing.T) {
	_ = setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "done"}, map[string]string{
		"status": "true",
	})
	if err == nil {
		t.Fatal("expected error when --task-id is missing")
	}
}

func TestTodoTaskDone_should_error_when_status_missing(t *testing.T) {
	_ = setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "done"}, map[string]string{
		"task-id": "TASK_001",
	})
	if err == nil {
		t.Fatal("expected error when --status is missing")
	}
}

// ── task get ────────────────────────────────────────────────

func TestTodoTaskGet_should_call_query_todo_detail(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "get"}, map[string]string{
		"task-id": "TASK_001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "get_todo_detail")
}

func TestTodoTaskGet_should_pass_taskId(t *testing.T) {
	cap := setupTestDeps(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "get"}, map[string]string{
		"task-id": "TASK_002",
	})
	assertToolArg(t, cap, "taskId", "TASK_002")
}

func TestTodoTaskGet_should_error_when_task_id_missing(t *testing.T) {
	_ = setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "get"}, nil)
	if err == nil {
		t.Fatal("expected error when --task-id is missing")
	}
}

// ── task delete ─────────────────────────────────────────────

func TestTodoTaskDelete_should_call_delete_todo(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "delete"}, map[string]string{
		"task-id": "TASK_DEL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "delete_todo")
}

func TestTodoTaskDelete_should_pass_taskId(t *testing.T) {
	cap := setupTestDepsAutoConfirm(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "delete"}, map[string]string{
		"task-id": "TASK_DEL",
	})
	assertToolArg(t, cap, "taskId", "TASK_DEL")
}

func TestTodoTaskDelete_should_error_when_task_id_missing(t *testing.T) {
	_ = setupTestDeps(t, "todo")
	root := buildRoot()
	err := execCmd(t, root, []string{"todo", "task", "delete"}, nil)
	if err == nil {
		t.Fatal("expected error when --task-id is missing")
	}
}

func TestTodoTaskDelete_should_not_call_mcp_in_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "todo")
	root := buildRoot()
	_ = execCmd(t, root, []string{"todo", "task", "delete"}, map[string]string{
		"task-id": "TASK_001",
	})
	assertCallCount(t, cap, 0)
}
