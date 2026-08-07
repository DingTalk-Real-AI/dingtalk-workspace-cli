package helpers

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type docCreateRecordingCall struct {
	tool string
	args map[string]any
}

type docCreateRecordingCaller struct {
	calls []docCreateRecordingCall
}

func (c *docCreateRecordingCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.calls = append(c.calls, docCreateRecordingCall{tool: tool, args: copied})
	return textToolResult(`{"nodeId":"node-1","success":true}`), nil
}

func (*docCreateRecordingCaller) Format() string { return "json" }
func (*docCreateRecordingCaller) DryRun() bool   { return false }
func (*docCreateRecordingCaller) Fields() string { return "" }
func (*docCreateRecordingCaller) JQ() string     { return "" }

func TestDocCreatePreservesExplicitLeadingH1MatchingName(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })

	for _, content := range []string{
		"# 需求清单",
		"# 需求清单\n\n以上需求已与产品确认",
	} {
		t.Run(content, func(t *testing.T) {
			previous := deps
			caller := &docCreateRecordingCaller{}
			InitDeps(caller)
			deps.Out.w = io.Discard
			deps.Out.errW = io.Discard
			t.Cleanup(func() { deps = previous })

			root := newDocCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"create", "--name", "需求清单", "--content", content})

			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("tool calls = %#v, want one create_document call", caller.calls)
			}
			call := caller.calls[0]
			if call.tool != "create_document" {
				t.Fatalf("tool = %q, want create_document", call.tool)
			}
			if got := call.args["markdown"]; got != content {
				t.Fatalf("markdown = %#v, want exact explicit body H1 %#v", got, content)
			}
		})
	}
}
