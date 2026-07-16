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

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/usage"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type crossPlatformCoverageCaller struct {
	args   map[string]any
	dryRun bool
}

func (c *crossPlatformCoverageCaller) CallTool(_ context.Context, _, _ string, args map[string]any) (*edition.ToolResult, error) {
	c.args = args
	return &edition.ToolResult{}, nil
}

func (*crossPlatformCoverageCaller) Format() string { return "json" }
func (c *crossPlatformCoverageCaller) DryRun() bool { return c.dryRun }
func (*crossPlatformCoverageCaller) Fields() string { return "id,name" }
func (*crossPlatformCoverageCaller) JQ() string     { return ".result" }

func TestCrossPlatformCoverageCloneToolArgsDefensiveCopy(t *testing.T) {
	args := map[string]any{"page": 1, "query": "keep"}
	cloned := cloneToolArgs(args)
	args["page"] = 2
	args["extra"] = true

	if got := cloned["page"]; got != 1 {
		t.Fatalf("cloned page = %#v, want 1", got)
	}
	if _, ok := cloned["extra"]; ok {
		t.Fatal("clone changed after source map mutation")
	}
}

func TestCrossPlatformCoverageCloneToolArgsEmpty(t *testing.T) {
	if got := cloneToolArgs(nil); got != nil {
		t.Fatalf("nil clone = %#v, want nil", got)
	}
	if got := cloneToolArgs(map[string]any{}); got != nil {
		t.Fatalf("empty clone = %#v, want nil", got)
	}
}

func TestCrossPlatformCoverageRecordingToolCaller(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_USAGE_TRACKING", "1")

	inner := &crossPlatformCoverageCaller{}
	caller := newRecordingToolCaller(inner)
	args := map[string]any{"open_conversation_id": "cid_x", "text": "private"}
	if _, err := caller.CallTool(context.Background(), "chat", "send_message", args); err != nil {
		t.Fatal(err)
	}
	if inner.args["open_conversation_id"] != "cid_x" {
		t.Fatalf("forwarded args = %#v", inner.args)
	}
	if caller.Format() != "json" || caller.Fields() != "id,name" || caller.JQ() != ".result" || caller.DryRun() {
		t.Fatal("recording caller did not delegate output settings")
	}
	records, err := usage.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Product != "chat" || records[0].Tool != "send_message" {
		t.Fatalf("usage records = %#v", records)
	}
	if _, leaked := records[0].SampleArgs["text"]; leaked {
		t.Fatal("sensitive text must not be recorded")
	}

	inner.dryRun = true
	if _, err := caller.CallTool(context.Background(), "chat", "send_message", args); err != nil {
		t.Fatal(err)
	}
	records, err = usage.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("dry-run call must not be recorded: %#v", records)
	}
}

func TestCrossPlatformCoverageRootPublishesShortcutCommands(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	root := NewRootCommand(context.Background())
	for _, path := range [][]string{{"shortcut", "list"}, {"calendar", "+today"}} {
		cmd, remaining, err := root.Find(path)
		if err != nil || len(remaining) != 0 || cmd == nil {
			t.Fatalf("root.Find(%v) = cmd=%v remaining=%v err=%v", path, cmd, remaining, err)
		}
	}
	if _, err := os.Stat(filepath.Join(configDir, "audit", ".audit.lock")); !os.IsNotExist(err) {
		t.Fatalf("constructing the root command opened an audit lock: %v", err)
	}
}
