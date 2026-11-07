package helpers

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func agentOpenAPIViaDocProcess(t *testing.T) {
	t.Helper()
	testseam.Protect(t, &os.Args)
	os.Args = []string{"dws", "doc"}
}

func agentOpenAPIViaAssertExtraInfo(t *testing.T, args map[string]any, want string) {
	t.Helper()
	// Assert the serialized request contract as well as the Go value: a JSON
	// string containing encoded extraInfo must not pass as an object.
	wire, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded["extraInfo"]; !reflect.DeepEqual(got, map[string]any{"agentName": want}) {
		t.Fatalf("actual tools/call arguments extraInfo = %#v, want object {agentName:%q}", got, want)
	}
}

func TestCrossPlatformCoverageAgentOpenAPIViaFourDocCommandsUseEnvironment(t *testing.T) {
	agentOpenAPIViaDocProcess(t)
	commands := []struct {
		name string
		tool string
		args []string
		body map[string]any
	}{
		{"update", "update_document", []string{"update", "--node", "node", "--mode", "append", "--content", "body"}, map[string]any{"markdown": "body", "mode": "append"}},
		{"block-update", "update_document_block", []string{"block", "update", "--node", "node", "--block-id", "block", "--content", "body"}, map[string]any{"blockId": "block", "element": map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": "body"}}}},
		{"block-delete", "delete_document_block", []string{"block", "delete", "--node", "node", "--block-id", "block-a,block-b", "--yes"}, map[string]any{"blockId": "block-a,block-b"}},
		{"block-insert", "insert_document_block", []string{"block", "insert", "--node", "node", "--content", "body", "--ref-block", "block"}, map[string]any{"referenceBlockId": "block", "element": map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": "body"}}}},
	}
	environments := []struct {
		name  string
		value string
		unset bool
		want  string
	}{
		{"unset", "", true, "unknown"},
		{"empty", "", false, "unknown"},
		{"whitespace", " \t\r\n ", false, "unknown"},
		{"named", " example-agent ", false, "example-agent"},
		{"future-agent", " Future-Agent ", false, "Future-Agent"},
	}
	for _, command := range commands {
		for _, environment := range environments {
			t.Run(command.name+"/"+environment.name, func(t *testing.T) {
				t.Setenv("DINGTALK_AGENT", environment.value)
				if environment.unset {
					if err := os.Unsetenv("DINGTALK_AGENT"); err != nil {
						t.Fatal(err)
					}
				}
				caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
				installScriptedCaller(t, caller)
				if err := runDocCoverageCommand(t, caller, command.args...); err != nil {
					t.Fatal(err)
				}
				if caller.calls != 1 || caller.server != "doc" || caller.tool != command.tool {
					t.Fatalf("calls = %d, destination = %s/%s", caller.calls, caller.server, caller.tool)
				}
				if caller.args["nodeId"] != "node" {
					t.Fatalf("nodeId = %#v", caller.args["nodeId"])
				}
				for key, want := range command.body {
					if !reflect.DeepEqual(caller.args[key], want) {
						t.Errorf("body field %s = %#v, want %#v", key, caller.args[key], want)
					}
				}
				agentOpenAPIViaAssertExtraInfo(t, caller.args, environment.want)
			})
		}
	}
}

func TestCrossPlatformCoverageAgentOpenAPIViaJSONMLCommandBranches(t *testing.T) {
	agentOpenAPIViaDocProcess(t)
	t.Setenv("DINGTALK_AGENT", " example-agent ")
	for _, tc := range []struct {
		name string
		tool string
		args []string
	}{
		{"update", "update_document", []string{"update", "--node", "node", "--mode", "overwrite", "--yes", "--content-format", "jsonml", "--content", `["root",{},["p",{},"body"]]`}},
		{"block-update", "update_document_block", []string{"block", "update", "--node", "node", "--block-id", "block", "--content-format", "jsonml", "--element", `["p",{},"body"]`}},
		{"block-insert", "insert_document_block", []string{"block", "insert", "--node", "node", "--content-format", "jsonml", "--element", `["p",{},"body"]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
			installScriptedCaller(t, caller)
			if err := runDocCoverageCommand(t, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.tool != tc.tool || caller.args["format"] != "jsonml" {
				t.Fatalf("calls = %d, tool = %s, args = %#v", caller.calls, caller.tool, caller.args)
			}
			body, _ := caller.args["jsonml"].(string)
			if !json.Valid([]byte(body)) || !strings.Contains(body, `"body"`) {
				t.Fatalf("JSONML body lost: %q", body)
			}
			agentOpenAPIViaAssertExtraInfo(t, caller.args, "example-agent")
		})
	}
}

type agentOpenAPIViaChunkCaller struct {
	*scriptedToolCaller
	afterFirst func()
}

func (c *agentOpenAPIViaChunkCaller) CallTool(ctx context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	result, err := c.scriptedToolCaller.CallTool(ctx, server, tool, args)
	if c.calls == 1 && c.afterFirst != nil {
		c.afterFirst()
	}
	return result, err
}

func TestCrossPlatformCoverageAgentOpenAPIViaChunkedWritesKeepSourceAndContent(t *testing.T) {
	agentOpenAPIViaDocProcess(t)
	for _, tc := range []struct {
		name   string
		tool   string
		args   map[string]any
		want   string
		create bool
		first  string
	}{
		{"update", "update_document", map[string]any{"nodeId": "node", "mode": "overwrite"}, "First-Agent", false, "overwrite"},
		{"existing-object", "update_document", map[string]any{"nodeId": "node", "mode": "append", "extraInfo": map[string]any{"agentName": "Snapshot-Agent"}}, "Snapshot-Agent", false, "append"},
		{"create-followup", "create_document", map[string]any{"name": "doc"}, "First-Agent", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DINGTALK_AGENT", " First-Agent ")
			caller := &agentOpenAPIViaChunkCaller{
				scriptedToolCaller: &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"nodeId":"node"}`}}},
				afterFirst:         func() { t.Setenv("DINGTALK_AGENT", "Second-Agent") },
			}
			InitDepsForTest(t, caller)
			deps.Out.w = io.Discard
			deps.Out.errW = io.Discard
			// Fixed, hand-derived chunk boundaries keep the content assertion
			// independent of the production splitter.
			out, err := chunkedWrite(context.Background(), tc.tool, tc.args, "aaa\n\nbbb\n\nccc", "doc update", 5)
			if err != nil {
				t.Fatal(err)
			}
			if out.written != 3 || caller.calls != 3 {
				t.Fatalf("written = %d, calls = %d, want 3", out.written, caller.calls)
			}
			for i, wantMarkdown := range []string{"aaa", "bbb", "ccc"} {
				args := caller.argsLog[i]
				if args["markdown"] != wantMarkdown {
					t.Errorf("chunk[%d] markdown = %#v, want %q", i, args["markdown"], wantMarkdown)
				}
				if tc.create && i == 0 {
					if caller.toolLog[i] != "create_document" || args["name"] != "doc" {
						t.Fatalf("first create arguments = %#v", args)
					}
					if _, exists := args["extraInfo"]; exists {
						t.Fatalf("create_document acquired extraInfo: %#v", args)
					}
					continue
				}
				wantMode := "append"
				if i == 0 {
					wantMode = tc.first
				}
				if caller.toolLog[i] != "update_document" || args["nodeId"] != "node" || args["mode"] != wantMode {
					t.Fatalf("chunk[%d] route/body = %s %#v", i, caller.toolLog[i], args)
				}
				agentOpenAPIViaAssertExtraInfo(t, args, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageAgentOpenAPIViaSharedCallPathsScopeAndKeepObjects(t *testing.T) {
	t.Setenv("DINGTALK_AGENT", " example-agent ")
	for _, path := range []struct {
		name string
		call func(context.Context, string, string, map[string]any) error
	}{
		{"print", CallMCPToolOnServerContext},
		{"text", func(ctx context.Context, server, tool string, args map[string]any) error {
			_, err := CallMCPToolTextOnServerContext(ctx, server, tool, args)
			return err
		}},
	} {
		for _, tc := range []struct {
			server string
			tool   string
			want   string
		}{
			{"doc", "update_document", "example-agent"},
			{"doc", "update_document_block", "example-agent"},
			{"doc", "delete_document_block", "example-agent"},
			{"doc", "insert_document_block", "example-agent"},
			{"doc", "create_document", ""},
			{"doc", "update_document_style", ""},
			{"doc", "get_document_content", ""},
			{"sheet", "update_document", ""},
		} {
			t.Run(path.name+"/"+tc.server+"/"+tc.tool, func(t *testing.T) {
				caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
				installScriptedCaller(t, caller)
				args := map[string]any{"nodeId": "node", "content": "body"}
				if err := path.call(context.Background(), tc.server, tc.tool, args); err != nil {
					t.Fatal(err)
				}
				if caller.calls != 1 || caller.server != tc.server || caller.tool != tc.tool || caller.args["content"] != "body" {
					t.Fatalf("actual call = %s/%s %#v", caller.server, caller.tool, caller.args)
				}
				if tc.want == "" {
					if !reflect.DeepEqual(caller.args, args) || len(caller.args) != 2 {
						t.Fatalf("non-edit arguments changed: %#v", caller.args)
					}
					return
				}
				agentOpenAPIViaAssertExtraInfo(t, caller.args, tc.want)
			})
		}
		t.Run(path.name+"/existing-object", func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}
			installScriptedCaller(t, caller)
			args := map[string]any{"nodeId": "node", "extraInfo": map[string]any{"agentName": "Snapshot-Agent"}}
			if err := path.call(context.Background(), "doc", "update_document", args); err != nil {
				t.Fatal(err)
			}
			agentOpenAPIViaAssertExtraInfo(t, caller.args, "Snapshot-Agent")
		})
	}
}
