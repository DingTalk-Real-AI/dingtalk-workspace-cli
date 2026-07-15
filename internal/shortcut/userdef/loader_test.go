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

package userdef

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type captureCaller struct{ args map[string]any }

func (c *captureCaller) CallTool(_ context.Context, _, _ string, args map[string]any) (*edition.ToolResult, error) {
	c.args = args
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"ok":true}`}}}, nil
}
func (c *captureCaller) Format() string { return "json" }
func (c *captureCaller) DryRun() bool   { return false }
func (c *captureCaller) Fields() string { return "" }
func (c *captureCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageValidate(t *testing.T) {
	cases := []struct {
		name string
		s    Spec
		ok   bool
	}{
		{"ok", Spec{Service: "chat", Command: "+notify", Exec: ExecSpec{Tool: "send_message"}}, true},
		{"no service", Spec{Command: "+x", Exec: ExecSpec{Tool: "t"}}, false},
		{"no plus", Spec{Service: "chat", Command: "notify", Exec: ExecSpec{Tool: "t"}}, false},
		{"no tool", Spec{Service: "chat", Command: "+x"}, false},
	}
	for _, c := range cases {
		err := Validate(c.s)
		if (err == nil) != c.ok {
			t.Errorf("%s: Validate ok=%v, err=%v", c.name, c.ok, err)
		}
	}
}

func TestCrossPlatformCoverageCompileFlagsAndDefaults(t *testing.T) {
	s := Spec{
		Service: "chat", Command: "+notify-team", Product: "chat",
		Exec: ExecSpec{Tool: "send_message", Bind: map[string]string{
			"open_conversation_id": "cid_x",
			"text":                 "${text}",
		}},
		Flags: []FlagSpec{{Name: "text", Type: "string", Required: true, Desc: "内容"}},
	}
	sc := Compile(s)
	if sc.Service != "chat" || sc.Command != "+notify-team" {
		t.Fatalf("bad identity: %+v", sc)
	}
	if sc.Risk != shortcut.RiskRead {
		t.Errorf("risk default = %q, want read", sc.Risk)
	}
	if sc.Description == "" || sc.Intent == "" {
		t.Error("Description/Intent should be auto-filled when empty")
	}
	if len(sc.Flags) != 1 || sc.Flags[0].Name != "text" {
		t.Errorf("flags = %+v", sc.Flags)
	}
	if sc.Execute == nil {
		t.Error("Execute must be set")
	}
}

func TestCrossPlatformCoverageFlagRef(t *testing.T) {
	if n, ok := flagRef("${text}"); !ok || n != "text" {
		t.Errorf("flagRef(${text}) = %q,%v", n, ok)
	}
	if _, ok := flagRef("cid_x"); ok {
		t.Error("constant must not be a flag ref")
	}
}

func TestCrossPlatformCoverageCompileBindsOptionalDefault(t *testing.T) {
	s := Spec{
		Service: "defaulttest", Command: "+run", Product: "chat",
		Exec:  ExecSpec{Tool: "send_message", Bind: map[string]string{"limit": "${limit}"}},
		Flags: []FlagSpec{{Name: "limit", Type: "int", Default: "20"}},
	}
	caller := &captureCaller{}
	helpers.InitDeps(caller)
	shortcut.Register(Compile(s))

	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	root.SetArgs([]string{"defaulttest", "+run"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := caller.args["limit"]; got != 20 {
		t.Fatalf("default limit = %#v, want 20", got)
	}
}

func TestCrossPlatformCoverageFilePathRejectsTraversal(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	if _, err := FilePath("../../outside", "+run"); err == nil {
		t.Fatal("expected traversal service to be rejected")
	}
	if _, err := FilePath("chat", "+../../outside"); err == nil {
		t.Fatal("expected traversal command to be rejected")
	}
	path, err := FilePath("my-team", "+notify.v2")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(Dir(), "my-team.notify.v2.yaml")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestCrossPlatformCoverageLoadSkipsConflictAndParses(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	scDir := filepath.Join(dir, "shortcuts")
	if err := os.MkdirAll(scDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A valid, non-conflicting user shortcut.
	yamlOK := `version: 1
service: myteam
command: "+notify"
product: chat
risk: write
execute:
  tool: send_message
  bind:
    open_conversation_id: "cid_x"
    text: "${text}"
flags:
  - {name: text, type: string, required: true, desc: 内容}
`
	if err := os.WriteFile(filepath.Join(scDir, "myteam.notify.yaml"), []byte(yamlOK), 0o600); err != nil {
		t.Fatal(err)
	}
	// A malformed one (no tool) — must be reported, not registered.
	if err := os.WriteFile(filepath.Join(scDir, "bad.yaml"), []byte("service: x\ncommand: \"+y\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	before := len(shortcut.All())
	n, errs := Load()
	if n != 1 {
		t.Errorf("registered = %d, want 1", n)
	}
	if len(errs) == 0 {
		t.Error("expected an error for the malformed file")
	}
	if len(shortcut.All()) != before+1 {
		t.Errorf("All() grew by %d, want 1", len(shortcut.All())-before)
	}
}
