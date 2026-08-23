package helpers

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatToolbarParentCaller struct {
	product string
	tool    string
	args    map[string]any
}

func (c *chatToolbarParentCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.product = productID
	c.tool = toolName
	c.args = args
	return textToolResult(`{"ok":true}`), nil
}

func (*chatToolbarParentCaller) Format() string { return "json" }
func (*chatToolbarParentCaller) DryRun() bool   { return false }
func (*chatToolbarParentCaller) Fields() string { return "" }
func (*chatToolbarParentCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageChatToolbarMountedThroughParentDeps(t *testing.T) {
	caller := &chatToolbarParentCaller{}
	installHelpersCoreDeps(t, caller)

	root := newChatCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if command, _, err := root.Find([]string{"toolbar", "list"}); err != nil || command == nil || command.Name() != "list" {
		t.Fatalf("find chat toolbar list = command %v, err %v", command, err)
	}

	root.SetArgs([]string{"toolbar", "list", "--conversation-id", "cidX"})
	if err := root.Execute(); err != nil {
		t.Fatalf("chat toolbar list returned error: %v", err)
	}
	if caller.product != "im" || caller.tool != "list_chat_toolbar_shortcuts" {
		t.Fatalf("tool call = %s/%s, want im/list_chat_toolbar_shortcuts", caller.product, caller.tool)
	}
	wantArgs := map[string]any{"openConversationId": "cidX"}
	if !reflect.DeepEqual(caller.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", caller.args, wantArgs)
	}
}
