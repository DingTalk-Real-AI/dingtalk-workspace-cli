package cli_compat_test

import "testing"

// ── devdoc article search ──────────────────────────────────

func TestDevdocArticleSearch_should_call_correct_tool(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	err := execCmd(t, root, []string{"devdoc", "article", "search"}, map[string]string{
		"keyword": "MCP", "page": "1", "size": "10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "search_open_platform_docs_rag")
}

func TestDevdocArticleSearch_should_pass_keyword(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "article", "search"}, map[string]string{
		"keyword": "openConversationId", "page": "1", "size": "10",
	})
	assertToolArg(t, cap, "keyword", "openConversationId")
	assertToolArg(t, cap, "page", float64(1))
	assertToolArg(t, cap, "size", float64(10))
}

func TestDevdocArticleSearch_should_not_call_when_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "article", "search"}, map[string]string{
		"keyword": "MCP", "page": "1", "size": "10",
	})
	assertCallCount(t, cap, 0)
}

// ── devdoc error diagnose ──────────────────────────────────

func TestDevdocErrorDiagnose_should_call_correct_tool(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	err := execCmd(t, root, []string{"devdoc", "error", "diagnose"}, map[string]string{
		"request-id": "req-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "search_open_error_code_rag")
}

func TestDevdocErrorDiagnose_should_pass_request_id(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "error", "diagnose"}, map[string]string{
		"request-id": "req-123", "page": "2", "size": "5",
	})
	assertToolArg(t, cap, "requestId", "req-123")
	assertToolArg(t, cap, "page", float64(2))
	assertToolArg(t, cap, "size", float64(5))
}

func TestDevdocErrorDiagnose_should_map_trace_id_alias(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "error", "diagnose"}, map[string]string{
		"trace-id": "trace-abc", "api": "创建日程",
	})
	assertToolArg(t, cap, "requestId", "trace-abc")
	assertToolArg(t, cap, "query", "创建日程")
	assertArgNotPresent(t, cap, "traceId")
	assertArgNotPresent(t, cap, "apiName")
}

func TestDevdocErrorTroubleshootAlias_should_pass_error_context(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "error", "troubleshoot"}, map[string]string{
		"error-code": "33012", "error-message": "missing scope", "context": "create calendar failed",
	})
	assertToolArg(t, cap, "errorCode", "33012")
	assertToolArg(t, cap, "query", "missing scope create calendar failed")
}

func TestDevdocErrorDiagnose_should_merge_cli_only_context_into_query(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	err := execCmd(t, root, []string{"devdoc", "error", "diagnose"}, map[string]string{
		"query":         "机器人回调失败",
		"error-message": "missing scope",
		"api":           "创建日程",
		"context":       "应用无权限",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolArg(t, cap, "query", "机器人回调失败 missing scope 创建日程 应用无权限")
	assertArgNotPresent(t, cap, "apiName")
	assertArgNotPresent(t, cap, "errorMessage")
	assertArgNotPresent(t, cap, "context")
}

func TestDevdocErrorDiagnose_should_reject_api_without_primary_input(t *testing.T) {
	cap := setupTestDepsWithPreview(t, "devdoc")
	root := buildRoot()
	err := execCmd(t, root, []string{"devdoc", "error", "diagnose"}, map[string]string{
		"api": "创建日程",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	assertCallCount(t, cap, 0)
}

func TestDevdocErrorDiagnose_should_not_call_when_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "error", "diagnose"}, map[string]string{
		"request-id": "req-123",
	})
	assertCallCount(t, cap, 0)
}
