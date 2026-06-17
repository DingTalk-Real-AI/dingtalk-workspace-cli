package cli_compat_test

import "testing"

// ── devdoc article search ──────────────────────────────────

func TestDevdocArticleSearch_should_call_correct_tool(t *testing.T) {
	cap := setupTestDeps(t, "devdoc")
	root := buildRoot()
	err := execCmd(t, root, []string{"devdoc", "article", "search"}, map[string]string{
		"query": "MCP", "page": "1", "size": "10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertToolName(t, cap, "search_open_platform_docs_rag")
}

func TestDevdocArticleSearch_should_pass_keyword(t *testing.T) {
	cap := setupTestDeps(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "article", "search"}, map[string]string{
		"query": "openConversationId", "page": "1", "size": "10",
	})
	assertToolArg(t, cap, "keyword", "openConversationId")
	assertArgNotPresent(t, cap, "query")
	assertToolArg(t, cap, "size", float64(10))
	assertArgNotPresent(t, cap, "pageSize")
	if _, ok := cap.last().Args["CliRagSearchReqVO"]; ok {
		t.Fatalf("CliRagSearchReqVO wrapper must not be sent for RAG search: %v", cap.last().Args)
	}
}

func TestDevdocArticleSearch_should_pass_cursor(t *testing.T) {
	cap := setupTestDeps(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "article", "search"}, map[string]string{
		"query": "Webhook", "cursor": "3", "size": "10",
	})
	assertToolArg(t, cap, "cursor", "3")
	assertArgNotPresent(t, cap, "page")
}

func TestDevdocArticleSearch_should_not_call_when_dry_run(t *testing.T) {
	cap := setupTestDepsWithDryRun(t, "devdoc")
	root := buildRoot()
	_ = execCmd(t, root, []string{"devdoc", "article", "search"}, map[string]string{
		"query": "MCP", "page": "1", "size": "10",
	})
	assertCallCount(t, cap, 0)
}
