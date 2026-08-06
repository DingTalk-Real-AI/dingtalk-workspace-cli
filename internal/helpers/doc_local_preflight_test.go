package helpers

import (
	"errors"
	"strings"
	"testing"
)

func TestDocLocalPreflightRejectsBeforeMCP(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"info-node", []string{"info"}, "--node"},
		{"read-node", []string{"read"}, "--node"},
		{"create-name", []string{"create"}, "--name"},
		{"create-content-conflict", []string{"create", "--name", "x", "--content", "a", "--content-file", "b"}, "不能同时"},
		{"create-file-missing", []string{"create", "--name", "x", "--content-file", "/definitely/not/found.md"}, "不可读取"},
		{"update-node", []string{"update", "--content", "x", "--mode", "append"}, "--node"},
		{"update-content-conflict", []string{"update", "--node", "n", "--content", "a", "--content-file", "b", "--mode", "append"}, "不能同时"},
		{"update-mode-missing", []string{"update", "--node", "n", "--content", "a"}, "--mode"},
		{"update-mode-invalid", []string{"update", "--node", "n", "--content", "a", "--mode", "merge"}, "append 或 overwrite"},
		{"update-index-mode", []string{"update", "--node", "n", "--content", "a", "--mode", "overwrite", "--index", "1"}, "--index"},
		{"block-list-node", []string{"block", "list"}, "--node"},
		{"block-list-range", []string{"block", "list", "--node", "n", "--start-index", "4", "--end-index", "2"}, "不能小于"},
		{"block-insert-node", []string{"block", "insert", "--text", "x"}, "--node"},
		{"block-insert-where", []string{"block", "insert", "--node", "n", "--text", "x", "--where", "middle"}, "before 或 after"},
		{"block-insert-ref", []string{"block", "insert", "--node", "n", "--text", "x", "--where", "before"}, "--ref-block"},
		{"block-insert-level", []string{"block", "insert", "--node", "n", "--heading", "x", "--level", "7"}, "1 到 6"},
		{"block-update-node", []string{"block", "update", "--block-id", "b", "--text", "x"}, "--node"},
		{"block-update-id", []string{"block", "update", "--node", "n", "--text", "x"}, "--block-id"},
		{"block-delete-id", []string{"block", "delete", "--node", "n"}, "--block-id"},
		{"media-download-resource", []string{"media", "download", "--node", "n"}, "--resource-id"},
		{"media-insert-file", []string{"media", "insert", "--node", "n"}, "--file"},
		{"comment-create-content", []string{"comment", "create", "--node", "n"}, "--content"},
		{"comment-reply-key", []string{"comment", "reply", "--node", "n", "--content", "x"}, "--comment-key"},
		{"comment-update-content", []string{"comment", "update", "--node", "n", "--comment-key", "k"}, "--content"},
		{"comment-delete-key", []string{"comment", "delete", "--node", "n"}, "--comment-key"},
		{"inline-block", []string{"comment", "create-inline", "--node", "n", "--content", "x", "--start", "0", "--end", "1"}, "--block-id"},
		{"inline-range", []string{"comment", "create-inline", "--node", "n", "--block-id", "b", "--content", "x", "--start", "2", "--end", "1"}, "start < end"},
		{"export-output", []string{"export", "--node", "n"}, "--output"},
		{"export-job", []string{"export", "get"}, "--job-id"},
		{"import-file", []string{"import", "--workspace", "w"}, "--file"},
		{"import-target", []string{"import", "--file", "/definitely/not/found.docx"}, "不可读取"},
		{"import-task", []string{"import", "get"}, "--task-id"},
		{"version-node", []string{"version", "list"}, "--node"},
		{"version-number", []string{"version", "revert", "--node", "n"}, "--version"},
		{"template-query", []string{"template", "search"}, "--query"},
		{"template-id", []string{"template", "apply"}, "--template-id"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newDocCommand()
			cmd.SetArgs(tc.args)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.ExitCode() != ExitValidation {
				t.Fatalf("error = %#v, want validation CLIError", err)
			}
		})
	}
}

func TestDocLocalPreflightSecondBatchRejectsBeforeMCP(t *testing.T) {
	users31 := strings.TrimSuffix(strings.Repeat("u,", 31), ",")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"search-created-negative", []string{"search", "--created-from", "-1"}, "非负毫秒"},
		{"search-created-range", []string{"search", "--created-from", "20", "--created-to", "10"}, "created-from"},
		{"search-visited-negative", []string{"search", "--visited-to", "-1"}, "非负毫秒"},
		{"search-visited-range", []string{"search", "--visited-from", "20", "--visited-to", "10"}, "visited-from"},
		{"search-limit-zero", []string{"search", "--limit", "0"}, "1 到 30"},
		{"search-limit-large", []string{"search", "--limit", "31"}, "1 到 30"},
		{"list-limit-zero", []string{"list", "--limit", "0"}, "1 到 50"},
		{"list-limit-large", []string{"list", "--limit", "51"}, "1 到 50"},
		{"read-depth-negative", []string{"read", "--node", "n", "--max-depth", "-1"}, "不能为负"},
		{"read-scope-invalid", []string{"read", "--node", "n", "--content-format", "jsonml", "--scope", "all"}, "outline"},
		{"read-scope-format", []string{"read", "--node", "n", "--scope", "outline"}, "content-format jsonml"},
		{"read-tags-missing", []string{"read", "--node", "n", "--content-format", "jsonml", "--scope", "tags"}, "--tags"},
		{"read-tags-wrong-scope", []string{"read", "--node", "n", "--content-format", "jsonml", "--scope", "outline", "--tags", "h1"}, "--tags only works"},
		{"read-range-start-missing", []string{"read", "--node", "n", "--content-format", "jsonml", "--scope", "range"}, "--start-block-id"},
		{"read-section-start-missing", []string{"read", "--node", "n", "--content-format", "jsonml", "--scope", "section"}, "--start-block-id"},
		{"read-markdown-output", []string{"read", "--node", "n", "--content-format", "markdown", "--output", "body.json"}, "Markdown 内容会直接显示在终端"},
		{"create-whitespace-name", []string{"create", "--name", "   "}, "--name"},
		{"update-empty-content", []string{"update", "--node", "n", "--content", "   ", "--mode", "append"}, "非空内容"},
		{"update-dry-run-append", []string{"update", "--node", "n", "--content", "x", "--mode", "append", "--dry-run"}, "仅用于预览 overwrite"},
		{"update-yes-append", []string{"update", "--node", "n", "--content", "x", "--mode", "append", "--yes"}, "仅用于确认 overwrite"},
		{"update-overwrite-confirm", []string{"update", "--node", "n", "--content", "x", "--mode", "overwrite"}, "必须加 --yes"},
		{"block-insert-content", []string{"block", "insert", "--node", "n"}, "必须提供一种块内容"},
		{"block-insert-text-heading", []string{"block", "insert", "--node", "n", "--text", "x", "--heading", "h"}, "不能同时"},
		{"block-insert-heading-element", []string{"block", "insert", "--node", "n", "--heading", "h", "--element", "{}"}, "不能同时"},
		{"block-insert-jsonml-text", []string{"block", "insert", "--node", "n", "--content-format", "jsonml", "--text", "x"}, "必须通过 --element"},
		{"block-insert-index", []string{"block", "insert", "--node", "n", "--text", "x", "--index", "-1"}, "--index 不能"},
		{"block-update-content", []string{"block", "update", "--node", "n", "--block-id", "b"}, "必须提供一种块内容"},
		{"block-update-text-element", []string{"block", "update", "--node", "n", "--block-id", "b", "--text", "x", "--element", "{}"}, "不能同时"},
		{"block-update-level", []string{"block", "update", "--node", "n", "--block-id", "b", "--heading", "h", "--level", "0"}, "1 到 6"},
		{"comment-limit-zero", []string{"comment", "list", "--node", "n", "--limit", "0"}, "1 到 50"},
		{"comment-limit-large", []string{"comment", "list", "--node", "n", "--limit", "51"}, "1 到 50"},
		{"comment-type", []string{"comment", "list", "--node", "n", "--type", "all"}, "global"},
		{"comment-status", []string{"comment", "list", "--node", "n", "--resolve-status", "open"}, "resolved"},
		{"reply-emoji-group", []string{"comment", "reply", "--node", "n", "--comment-key", "k", "--content", "x", "--emoji", "--mentioned-open-conversation-id", "g"}, "emoji replies do not support group mentions"},
		{"permission-users-missing", []string{"permission", "add", "--node", "n", "--role", "READER"}, "--users"},
		{"permission-users-large", []string{"permission", "add", "--node", "n", "--role", "READER", "--users", users31}, "最多处理 30"},
		{"permission-role", []string{"permission", "add", "--node", "n", "--role", "OWNER", "--users", "u"}, "MANAGER"},
		{"permission-filter-role", []string{"permission", "list", "--node", "n", "--filter-role", "ADMIN"}, "非法角色"},
		{"export-format", []string{"export", "--node", "n", "--output", "x", "--export-format", "html"}, "docx"},
	}
	if len(cases) != 39 {
		t.Fatalf("second batch has %d cases, want 39", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newDocCommand()
			cmd.SetArgs(tc.args)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.ExitCode() != ExitValidation {
				t.Fatalf("error = %#v, want validation CLIError", err)
			}
		})
	}
}
