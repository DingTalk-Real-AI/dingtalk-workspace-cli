// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatFavoritesCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type chatFavoritesCaller struct {
	calls []chatFavoritesCall
}

func (c *chatFavoritesCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, chatFavoritesCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*chatFavoritesCaller) Format() string { return "json" }
func (*chatFavoritesCaller) DryRun() bool   { return false }
func (*chatFavoritesCaller) Fields() string { return "" }
func (*chatFavoritesCaller) JQ() string     { return "" }

func executeChatFavoritesCommand(t *testing.T, caller *chatFavoritesCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	deps.Out.w = io.Discard

	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return root.Execute()
}

func TestChatFavoritesCommandsRegistered(t *testing.T) {
	root := newChatCommand()
	tests := []struct {
		name  string
		flags []string
	}{
		{name: "add-favorite", flags: []string{"open-message-id", "open-conversation-id"}},
		{name: "remove-favorite", flags: []string{"open-message-id", "open-conversation-id"}},
		{name: "list-favorites", flags: []string{"cursor", "size"}},
	}

	for _, tt := range tests {
		cmd, remaining, err := root.Find([]string{"message", tt.name})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("dws chat message %s not registered: cmd=%v remaining=%v err=%v", tt.name, cmd, remaining, err)
		}
		for _, flag := range tt.flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("chat message %s: missing flag --%s", tt.name, flag)
			}
		}
	}
}

func TestChatFavoriteMutationMappings(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		toolName string
	}{
		{name: "add", command: "add-favorite", toolName: "add_message_favorite"},
		{name: "remove", command: "remove-favorite", toolName: "remove_message_favorite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &chatFavoritesCaller{}
			err := executeChatFavoritesCommand(t, caller,
				"message", tt.command,
				"--open-message-id", "msg-1",
				"--open-conversation-id", "cid-1",
			)
			if err != nil {
				t.Fatalf("chat message %s returned error: %v", tt.command, err)
			}

			want := chatFavoritesCall{
				productID: "im",
				toolName:  tt.toolName,
				args: map[string]any{
					"openMessageId":      "msg-1",
					"openConversationId": "cid-1",
				},
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
				t.Fatalf("calls = %#v, want %#v", caller.calls, want)
			}
		})
	}
}

func TestChatFavoriteMutationsRequireBothIDs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "add missing message", args: []string{"message", "add-favorite", "--open-conversation-id", "cid-1"}},
		{name: "add missing conversation", args: []string{"message", "add-favorite", "--open-message-id", "msg-1"}},
		{name: "remove missing message", args: []string{"message", "remove-favorite", "--open-conversation-id", "cid-1"}},
		{name: "remove missing conversation", args: []string{"message", "remove-favorite", "--open-message-id", "msg-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &chatFavoritesCaller{}
			err := executeChatFavoritesCommand(t, caller, tt.args...)
			if err == nil {
				t.Fatal("command without all required IDs returned nil error")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("remote calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestChatListFavoritesSuppliesOpenDefaults(t *testing.T) {
	caller := &chatFavoritesCaller{}
	if err := executeChatFavoritesCommand(t, caller, "message", "list-favorites"); err != nil {
		t.Fatalf("list-favorites returned error: %v", err)
	}

	want := chatFavoritesCall{
		productID: "im",
		toolName:  "list_message_favorites",
		args: map[string]any{
			"cursor": int64(0),
			"size":   "20",
		},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, want)
	}
}

func TestChatListFavoritesMapsExplicitPagination(t *testing.T) {
	caller := &chatFavoritesCaller{}
	err := executeChatFavoritesCommand(t, caller,
		"message", "list-favorites", "--cursor", "42", "--size", "50")
	if err != nil {
		t.Fatalf("list-favorites returned error: %v", err)
	}

	want := map[string]any{"cursor": int64(42), "size": "50"}
	if len(caller.calls) != 1 || caller.calls[0].productID != "im" || caller.calls[0].toolName != "list_message_favorites" || !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("calls = %#v, want im/list_message_favorites %#v", caller.calls, want)
	}
}

func TestChatListFavoritesRejectsInvalidSize(t *testing.T) {
	for _, size := range []string{"-1", "0", "101"} {
		t.Run(size, func(t *testing.T) {
			caller := &chatFavoritesCaller{}
			err := executeChatFavoritesCommand(t, caller, "message", "list-favorites", "--size", size)
			if err == nil || !strings.Contains(err.Error(), "--size must be between 1 and 100") {
				t.Fatalf("error = %v, want size validation error", err)
			}
			if got := apperrors.ExitCode(err); got != 3 {
				t.Fatalf("exit code = %d, want validation code 3", got)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("remote calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestChatListFavoritesRejectsNegativeCursor(t *testing.T) {
	caller := &chatFavoritesCaller{}
	err := executeChatFavoritesCommand(t, caller, "message", "list-favorites", "--cursor", "-1")
	if err == nil || !strings.Contains(err.Error(), "--cursor must be greater than or equal to 0") {
		t.Fatalf("error = %v, want cursor validation error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls = %d, want 0", len(caller.calls))
	}
}
