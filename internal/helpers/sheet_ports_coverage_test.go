package helpers

import (
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSheetCommentCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want guardedMutationCall
	}{
		{
			name: "list minimal",
			args: []string{"comment", "list", "--node", "node-1"},
			want: guardedMutationCall{
				productID: "doc-comment",
				toolName:  "list_sheet_comments",
				args:      map[string]any{"nodeId": "node-1"},
			},
		},
		{
			name: "list with cell and status filters",
			args: []string{
				"comment", "list",
				"--node", "node-1",
				"--sheet-id", "Sheet1",
				"--range", "A2",
				"--resolve-status", "unresolved",
				"--limit", "10",
				"--cursor", "cur-1",
			},
			want: guardedMutationCall{
				productID: "doc-comment",
				toolName:  "list_sheet_comments",
				args: map[string]any{
					"nodeId":        "node-1",
					"sheetId":       "Sheet1",
					"range":         "A2",
					"resolveStatus": "unresolved",
					"pageSize":      10,
					"nextToken":     "cur-1",
				},
			},
		},
		{
			name: "create with mention parsing",
			args: []string{
				"comment", "create",
				"--node", "node-1",
				"--sheet-id", "Sheet1",
				"--range", "A2",
				"--content", "check this",
				"--mention", "uid1, uid2 ,,uid3",
			},
			want: guardedMutationCall{
				productID: "doc-comment",
				toolName:  "create_sheet_comment",
				args: map[string]any{
					"nodeId":           "node-1",
					"content":          "check this",
					"sheetId":          "Sheet1",
					"range":            "A2",
					"mentionedUserIds": []string{"uid1", "uid2", "uid3"},
				},
			},
		},
		{
			name: "create without mention",
			args: []string{
				"comment", "create",
				"--node", "node-1",
				"--sheet-id", "Sheet1",
				"--range", "B3",
				"--content", "plain",
			},
			want: guardedMutationCall{
				productID: "doc-comment",
				toolName:  "create_sheet_comment",
				args: map[string]any{
					"nodeId":  "node-1",
					"content": "plain",
					"sheetId": "Sheet1",
					"range":   "B3",
				},
			},
		},
		{
			name: "reply with emoji and mention",
			args: []string{
				"comment", "reply",
				"--node", "node-1",
				"--comment-key", "ck-1",
				"--content", "heart",
				"--emoji",
				"--mention", "uid1",
			},
			want: guardedMutationCall{
				productID: "doc-comment",
				toolName:  "reply_comment",
				args: map[string]any{
					"nodeId":           "node-1",
					"content":          "heart",
					"replyCommentKey":  "ck-1",
					"emoji":            true,
					"mentionedUserIds": []string{"uid1"},
				},
			},
		},
		{
			name: "reply plain text",
			args: []string{
				"comment", "reply",
				"--node", "node-1",
				"--comment-key", "ck-2",
				"--content", "confirmed",
			},
			want: guardedMutationCall{
				productID: "doc-comment",
				toolName:  "reply_comment",
				args: map[string]any{
					"nodeId":          "node-1",
					"content":         "confirmed",
					"replyCommentKey": "ck-2",
				},
			},
		},
		{
			name: "update",
			args: []string{
				"comment", "update",
				"--node", "node-1",
				"--comment-key", "ck-3",
				"--content", "revised",
			},
			want: guardedMutationCall{
				productID: "doc-comment",
				toolName:  "update_comment",
				args: map[string]any{
					"nodeId":     "node-1",
					"commentKey": "ck-3",
					"content":    "revised",
				},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newSheetCommand, test.args...)
			if err != nil {
				t.Fatalf("sheet %s returned error: %v", strings.Join(test.args, " "), err)
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], test.want) {
				t.Fatalf("tool calls = %#v, want %#v", caller.calls, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetCommentRequiredFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "list without node",
			args:    []string{"comment", "list"},
			wantErr: "--node",
		},
		{
			name:    "create without content sheet-id range",
			args:    []string{"comment", "create", "--node", "node-1"},
			wantErr: "--content, --sheet-id, --range",
		},
		{
			name:    "reply without comment-key",
			args:    []string{"comment", "reply", "--node", "node-1", "--content", "text"},
			wantErr: "--comment-key",
		},
		{
			name:    "update without content",
			args:    []string{"comment", "update", "--node", "node-1", "--comment-key", "ck-1"},
			wantErr: "--content",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newSheetCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("err = %v, want message containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %#v, want none", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetVersionCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want guardedMutationCall
	}{
		{
			name: "save",
			args: []string{"version", "save", "--node", "node-1"},
			want: guardedMutationCall{
				productID: "doc",
				toolName:  "save_doc_version",
				args:      map[string]any{"nodeId": "node-1"},
			},
		},
		{
			name: "list minimal",
			args: []string{"version", "list", "--node", "node-1"},
			want: guardedMutationCall{
				productID: "doc",
				toolName:  "list_doc_versions",
				args:      map[string]any{"nodeId": "node-1"},
			},
		},
		{
			name: "list with pagination pass-through",
			args: []string{"version", "list", "--node", "node-1", "--limit", "10", "--cursor", "cur-1"},
			want: guardedMutationCall{
				productID: "doc",
				toolName:  "list_doc_versions",
				args: map[string]any{
					"nodeId":     "node-1",
					"maxResults": 10,
					"nextCursor": "cur-1",
				},
			},
		},
		{
			name: "list ls alias",
			args: []string{"version", "ls", "--node", "node-2"},
			want: guardedMutationCall{
				productID: "doc",
				toolName:  "list_doc_versions",
				args:      map[string]any{"nodeId": "node-2"},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newSheetCommand, test.args...)
			if err != nil {
				t.Fatalf("sheet %s returned error: %v", strings.Join(test.args, " "), err)
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], test.want) {
				t.Fatalf("tool calls = %#v, want %#v", caller.calls, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetVersionRequiredFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "save without node",
			args:    []string{"version", "save"},
			wantErr: "--node",
		},
		{
			name:    "list without node",
			args:    []string{"version", "list"},
			wantErr: "--node",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newSheetCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("err = %v, want message containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %#v, want none", caller.calls)
			}
		})
	}
}
