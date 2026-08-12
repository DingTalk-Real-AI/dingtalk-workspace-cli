package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type docGroupMentionCall struct {
	server string
	tool   string
	args   map[string]any
}

type docGroupMentionCaller struct {
	calls []docGroupMentionCall
	err   error
}

func (c *docGroupMentionCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, docGroupMentionCall{server: server, tool: tool, args: args})
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*docGroupMentionCaller) Format() string { return "json" }
func (*docGroupMentionCaller) DryRun() bool   { return false }
func (*docGroupMentionCaller) Fields() string { return "" }
func (*docGroupMentionCaller) JQ() string     { return "" }

func executeDocGroupMentionCommand(t *testing.T, caller *docGroupMentionCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	root := newDocCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageDocCommentGroupMentionFlagsAndMappings(t *testing.T) {
	root := newDocCommand()
	for _, name := range []string{"create", "reply", "update"} {
		cmd, remaining, err := root.Find([]string{"comment", name})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("find comment %s: remaining=%v err=%v", name, remaining, err)
		}
		flag := cmd.Flags().Lookup("mentioned-open-conversation-id")
		if flag == nil || flag.Value.Type() != "stringSlice" {
			t.Fatalf("comment %s group mention flag = %#v, want stringSlice", name, flag)
		}
	}

	tests := []struct {
		name string
		args []string
		tool string
		key  string
	}{
		{
			name: "create",
			args: []string{
				"comment", "create", "--node", "doc-1", "--content", "body",
				"--mentioned-open-conversation-id", "oc-1",
			},
			tool: "create_comment",
		},
		{
			name: "reply",
			args: []string{
				"comment", "reply", "--node", "doc-1", "--comment-key", "comment-1", "--content", "body",
				"--mentioned-open-conversation-id", "oc-1",
			},
			tool: "reply_comment",
			key:  "replyCommentKey",
		},
		{
			name: "update",
			args: []string{
				"comment", "update", "--node", "doc-1", "--comment-key", "comment-1", "--content", "body",
				"--mentioned-open-conversation-id", "oc-1",
			},
			tool: "update_comment",
			key:  "commentKey",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &docGroupMentionCaller{}
			if err := executeDocGroupMentionCommand(t, caller, tt.args...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.server != "doc-comment" || call.tool != tt.tool {
				t.Fatalf("target = %s/%s, want doc-comment/%s", call.server, call.tool, tt.tool)
			}
			if got := call.args["mentionedOpenConversationIds"]; !reflect.DeepEqual(got, []string{"oc-1"}) {
				t.Fatalf("mentionedOpenConversationIds = %#v", got)
			}
			if tt.key != "" && call.args[tt.key] != "comment-1" {
				t.Fatalf("%s = %#v", tt.key, call.args[tt.key])
			}
		})
	}
}

func TestCrossPlatformCoverageDocCommentGroupMentionsTrimDeduplicateAndPreserveUserMentions(t *testing.T) {
	caller := &docGroupMentionCaller{}
	err := executeDocGroupMentionCommand(
		t,
		caller,
		"comment", "update", "--node", "doc-1", "--comment-key", "comment-1", "--content", "body",
		"--mention", "user-1, user-2",
		"--mentioned-open-conversation-id", "oc-1, oc-2",
		"--mentioned-open-conversation-id", "oc-1",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(caller.calls))
	}
	args := caller.calls[0].args
	if got := args["mentionedUserIds"]; !reflect.DeepEqual(got, []string{"user-1", "user-2"}) {
		t.Fatalf("mentionedUserIds = %#v", got)
	}
	if got := args["mentionedOpenConversationIds"]; !reflect.DeepEqual(got, []string{"oc-1", "oc-2"}) {
		t.Fatalf("mentionedOpenConversationIds = %#v", got)
	}
}

func TestCrossPlatformCoverageDocCommentGroupMentionValidationAndCompatibility(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		caller := &docGroupMentionCaller{}
		err := executeDocGroupMentionCommand(
			t,
			caller,
			"comment", "create", "--node", "doc-1", "--content", "plain",
		)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if _, exists := caller.calls[0].args["mentionedOpenConversationIds"]; exists {
			t.Fatal("group mention field should be omitted")
		}
	})

	t.Run("blank", func(t *testing.T) {
		caller := &docGroupMentionCaller{}
		err := executeDocGroupMentionCommand(
			t,
			caller,
			"comment", "create", "--node", "doc-1", "--content", "body",
			"--mentioned-open-conversation-id", "   ",
		)
		if err == nil || !strings.Contains(err.Error(), "must not be empty") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("calls = %d, want 0", len(caller.calls))
		}
	})

	for _, command := range []struct {
		name string
		args []string
	}{
		{
			name: "reply rejects blank",
			args: []string{
				"comment", "reply", "--node", "doc-1", "--comment-key", "comment-1", "--content", "body",
				"--mentioned-open-conversation-id", " ",
			},
		},
		{
			name: "update rejects blank",
			args: []string{
				"comment", "update", "--node", "doc-1", "--comment-key", "comment-1", "--content", "body",
				"--mentioned-open-conversation-id", " ",
			},
		},
	} {
		t.Run(command.name, func(t *testing.T) {
			caller := &docGroupMentionCaller{}
			err := executeDocGroupMentionCommand(t, caller, command.args...)
			if err == nil || !strings.Contains(err.Error(), "must not be empty") {
				t.Fatalf("error = %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("calls = %d, want 0", len(caller.calls))
			}
		})
	}

	t.Run("emoji conflict", func(t *testing.T) {
		caller := &docGroupMentionCaller{}
		err := executeDocGroupMentionCommand(
			t,
			caller,
			"comment", "reply", "--node", "doc-1", "--comment-key", "comment-1", "--content", "like",
			"--emoji", "--mentioned-open-conversation-id", "oc-1",
		)
		if err == nil || !strings.Contains(err.Error(), "emoji replies do not support group mentions") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("calls = %d, want 0", len(caller.calls))
		}
	})

	t.Run("emoji validates blank group mention", func(t *testing.T) {
		caller := &docGroupMentionCaller{}
		err := executeDocGroupMentionCommand(
			t,
			caller,
			"comment", "reply", "--node", "doc-1", "--comment-key", "comment-1", "--content", "like",
			"--emoji", "--mentioned-open-conversation-id", " ",
		)
		if err == nil || !strings.Contains(err.Error(), "must not be empty") {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("calls = %d, want 0", len(caller.calls))
		}
	})

	t.Run("emoji without group mention remains supported", func(t *testing.T) {
		caller := &docGroupMentionCaller{}
		err := executeDocGroupMentionCommand(
			t,
			caller,
			"comment", "reply", "--node", "doc-1", "--comment-key", "comment-1", "--content", "like",
			"--emoji",
		)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if len(caller.calls) != 1 || caller.calls[0].args["emoji"] != true {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})

	t.Run("server error is not downgraded", func(t *testing.T) {
		sentinel := errors.New("group mention rejected")
		caller := &docGroupMentionCaller{err: sentinel}
		err := executeDocGroupMentionCommand(
			t,
			caller,
			"comment", "create", "--node", "doc-1", "--content", "body",
			"--mentioned-open-conversation-id", "oc-1",
		)
		if err == nil || !strings.Contains(err.Error(), sentinel.Error()) {
			t.Fatalf("error = %v", err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("calls = %d, want exactly 1", len(caller.calls))
		}
		if got := caller.calls[0].args["mentionedOpenConversationIds"]; !reflect.DeepEqual(got, []string{"oc-1"}) {
			t.Fatalf("first call lost group mentions: %#v", got)
		}
	})
}

func TestCrossPlatformCoverageCommentGroupMentionHelperErrorBranches(t *testing.T) {
	wrongType := &cobra.Command{Use: "wrong-type"}
	wrongType.Flags().String("mentioned-open-conversation-id", "", "")
	if _, err := commentGroupMentionIDs(wrongType); err == nil {
		t.Fatal("wrong flag type should fail")
	}

	emptySlice := &cobra.Command{Use: "empty-slice"}
	addCommentGroupMentionFlag(emptySlice)
	emptySlice.Flags().Lookup("mentioned-open-conversation-id").Changed = true
	if _, err := commentGroupMentionIDs(emptySlice); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty explicitly changed slice error = %v", err)
	}
}
