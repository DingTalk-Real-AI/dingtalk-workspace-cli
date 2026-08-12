package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type docReadScopeCall struct {
	server string
	tool   string
	args   map[string]any
}

type docReadScopeCaller struct {
	calls  []docReadScopeCall
	text   string
	err    error
	format string
}

func (c *docReadScopeCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, docReadScopeCall{server: server, tool: tool, args: args})
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, nil
}

func (c *docReadScopeCaller) Format() string {
	if c.format == "" {
		return "json"
	}
	return c.format
}
func (*docReadScopeCaller) DryRun() bool   { return false }
func (*docReadScopeCaller) Fields() string { return "" }
func (*docReadScopeCaller) JQ() string     { return "" }

func installDocReadScopeCaller(t *testing.T, caller *docReadScopeCaller) *bytes.Buffer {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	var output bytes.Buffer
	InitDeps(caller)
	deps.Out.w = &output
	deps.Out.errW = &output
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})
	return &output
}

func executeDocReadScopeCommand(t *testing.T, caller *docReadScopeCaller, args ...string) error {
	t.Helper()
	installDocReadScopeCaller(t, caller)
	root := newDocCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageDocReadScopeFlagsAndValidation(t *testing.T) {
	root := newDocCommand()
	read, remaining, err := root.Find([]string{"read"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find read: remaining=%v err=%v", remaining, err)
	}
	for _, name := range []string{"scope", "tags", "max-depth", "start-block-id", "end-block-id"} {
		if read.Flags().Lookup(name) == nil {
			t.Fatalf("doc read missing --%s", name)
		}
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "scope needs jsonml",
			args: []string{"read", "--node", "doc-1", "--scope", "outline"},
			want: "--scope/--tags requires",
		},
		{
			name: "tags need scope",
			args: []string{"read", "--node", "doc-1", "--content-format", "jsonml", "--tags", "h1"},
			want: "--tags requires --scope tags",
		},
		{
			name: "invalid scope",
			args: []string{"read", "--node", "doc-1", "--content-format", "jsonml", "--scope", "invalid"},
			want: "invalid --scope",
		},
		{
			name: "tags only with tags scope",
			args: []string{"read", "--node", "doc-1", "--content-format", "jsonml", "--scope", "outline", "--tags", "h1"},
			want: "--tags only works",
		},
		{
			name: "tags scope needs tags",
			args: []string{"read", "--node", "doc-1", "--content-format", "jsonml", "--scope", "tags"},
			want: "--tags is required",
		},
		{
			name: "range needs start",
			args: []string{"read", "--node", "doc-1", "--content-format", "jsonml", "--scope", "range"},
			want: "--start-block-id is required",
		},
		{
			name: "section needs start",
			args: []string{"read", "--node", "doc-1", "--content-format", "jsonml", "--scope", "section"},
			want: "--start-block-id is required",
		},
		{
			name: "end only with range",
			args: []string{"read", "--node", "doc-1", "--content-format", "jsonml", "--scope", "outline", "--end-block-id", "end"},
			want: "--end-block-id only works",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &docReadScopeCaller{}
			err := executeDocReadScopeCommand(t, caller, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageDocReadScopeMapsArgumentsAndWritesOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "fragment.json")
	caller := &docReadScopeCaller{
		text: `{"jsonml":"[\"fragment\",{\"source\":\"range\"},[\"p\",{},\"body\"]]"}`,
	}
	err := executeDocReadScopeCommand(
		t,
		caller,
		"read", "--node", "doc-1", "--content-format", "jsonml",
		"--scope", "range", "--start-block-id", "start", "--end-block-id", "end",
		"--max-depth", "3", "--output", outputPath,
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	wantArgs := map[string]any{
		"nodeId":       "doc-1",
		"format":       "jsonml",
		"scope":        "range",
		"maxDepth":     3,
		"startBlockId": "start",
		"endBlockId":   "end",
	}
	if call.server != "doc" || call.tool != "get_document_content" || !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("call = %#v, want doc/get_document_content %#v", call, wantArgs)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(content), "\n") || !strings.Contains(string(content), `"fragment"`) {
		t.Fatalf("output was not pretty JSONML fragment: %s", content)
	}

	t.Run("tags omit unset optional arguments", func(t *testing.T) {
		caller := &docReadScopeCaller{
			text: `{"jsonml":"[\"fragment\",{\"source\":\"tags\"},[\"h1\",{},\"title\"]]"}`,
		}
		output := installDocReadScopeCaller(t, caller)
		root := newDocCommand()
		root.SilenceErrors = true
		root.SilenceUsage = true
		root.SetArgs([]string{
			"read", "--node", "doc-2", "--content-format", "jsonml",
			"--scope", "tags", "--tags", "h1,h2",
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		want := map[string]any{
			"nodeId": "doc-2",
			"format": "jsonml",
			"scope":  "tags",
			"tags":   "h1,h2",
		}
		if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
		if !strings.Contains(output.String(), `"fragment"`) {
			t.Fatalf("stdout = %q", output.String())
		}
	})
}

func TestCrossPlatformCoverageRunDocReadScopeResponseBranches(t *testing.T) {
	tests := []struct {
		name     string
		caller   *docReadScopeCaller
		output   string
		wantErr  string
		wantText string
	}{
		{
			name:    "call error",
			caller:  &docReadScopeCaller{err: errors.New("read failed")},
			wantErr: "read failed",
		},
		{
			name:    "invalid response",
			caller:  &docReadScopeCaller{text: `{`},
			wantErr: "failed to parse MCP response",
		},
		{
			name:     "missing fragment",
			caller:   &docReadScopeCaller{text: `{}`},
			wantText: `"matched": false`,
		},
		{
			name:    "invalid fragment is rejected",
			caller:  &docReadScopeCaller{text: `{"jsonml":"not-json"}`},
			wantErr: "invalid JSONML fragment",
		},
		{
			name:    "write failure",
			caller:  &docReadScopeCaller{text: `{"jsonml":"[]"}`},
			output:  t.TempDir(),
			wantErr: "failed to write output file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := installDocReadScopeCaller(t, tt.caller)
			err := runDocReadScope("doc-1", "", "", 0, false, "", "ignored", tt.output)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("runDocReadScope: %v", err)
			}
			if tt.wantText != "" && !strings.Contains(output.String(), tt.wantText) {
				t.Fatalf("output = %q, want %q", output.String(), tt.wantText)
			}
			if tt.name == "missing fragment" && !json.Valid(output.Bytes()) {
				t.Fatalf("empty-result stdout is not valid JSON: %q", output.String())
			}
			if len(tt.caller.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(tt.caller.calls))
			}
			wantMinimal := map[string]any{"nodeId": "doc-1", "format": "jsonml"}
			if !reflect.DeepEqual(tt.caller.calls[0].args, wantMinimal) {
				t.Fatalf("args = %#v, want %#v", tt.caller.calls[0].args, wantMinimal)
			}
		})
	}

	t.Run("output receipt is valid JSON", func(t *testing.T) {
		caller := &docReadScopeCaller{text: `{"jsonml":"[\"fragment\",{}]"}`}
		stdout := installDocReadScopeCaller(t, caller)
		outputPath := filepath.Join(t.TempDir(), "fragment.json")
		if err := runDocReadScope("doc-1", "outline", "", 0, false, "", "", outputPath); err != nil {
			t.Fatalf("runDocReadScope: %v", err)
		}
		var receipt map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
			t.Fatalf("output receipt is not valid JSON: %q: %v", stdout.String(), err)
		}
		if receipt["success"] != true || receipt["output"] != outputPath {
			t.Fatalf("output receipt = %#v", receipt)
		}
	})

	t.Run("human output reports missing fragment", func(t *testing.T) {
		caller := &docReadScopeCaller{text: `{}`, format: "raw"}
		stdout := installDocReadScopeCaller(t, caller)
		if err := runDocReadScope("doc-1", "", "", 0, false, "", "", ""); err != nil {
			t.Fatalf("runDocReadScope: %v", err)
		}
		if !strings.Contains(stdout.String(), "未匹配到节点") {
			t.Fatalf("human empty-result output = %q", stdout.String())
		}
	})

	t.Run("human output reports written fragment", func(t *testing.T) {
		caller := &docReadScopeCaller{text: `{"jsonml":"[\"fragment\",{}]"}`, format: "raw"}
		stdout := installDocReadScopeCaller(t, caller)
		outputPath := filepath.Join(t.TempDir(), "fragment.json")
		if err := runDocReadScope("doc-1", "outline", "", 0, false, "", "", outputPath); err != nil {
			t.Fatalf("runDocReadScope: %v", err)
		}
		if !strings.Contains(stdout.String(), "JSONML fragment 已写入 "+outputPath) {
			t.Fatalf("human receipt output = %q", stdout.String())
		}
	})
}
