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
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// devEventToolNames 是本次新增的九个命令对应工具标识（含本地 lint），取自叶子期望表，
// 避免在本文件再抄一张会漂移的表。
func devEventToolNames() []string {
	wants := devEventLeafExpectations()
	tools := make([]string, 0, len(wants))
	for _, want := range wants {
		tools = append(tools, want.tool)
	}
	return tools
}

func devEventSubCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	found, rest, err := root.Find(path)
	if err != nil {
		t.Fatalf("Find(%v) error = %v", path, err)
	}
	if len(rest) > 0 {
		t.Fatalf("Find(%v) left unresolved args %v: 命令未挂载", path, rest)
	}
	return found
}

// TestDevTreeChildrenUnchangedByEventGroup 验证新增 dev event 组没有挤掉或改名
// 任何既有 dev 子命令：dev 的直接子命令集合仍是 app/connect/doc/liteapp/mcp
// （liteapp 随 main 的轻应用工作合入），加上本次新增的 event。
func TestDevTreeChildrenUnchangedByEventGroup(t *testing.T) {
	root := newDevAppTestRoot(&fakeDevEventRunner{})
	dev := devEventSubCommand(t, root, "dev")

	var got []string
	for _, child := range dev.Commands() {
		got = append(got, child.Name())
	}
	for _, name := range []string{"app", "connect", "doc", "liteapp", "mcp", "event"} {
		if !containsString(got, name) {
			t.Fatalf("dev children = %v, want %q present", got, name)
		}
	}
	if len(got) != 6 {
		t.Fatalf("dev children = %v, want exactly app/connect/doc/liteapp/mcp/event", got)
	}

	group := devEventSubCommand(t, root, "dev", "event")
	if group.Args == nil {
		t.Fatal("dev event group Args == nil, want NoArgs: 组本身不派发")
	}
	leaves := group.Commands()
	wants := devEventLeafExpectations()
	if len(leaves) != len(wants) {
		t.Fatalf("dev event has %d leaves, want %d", len(leaves), len(wants))
	}
	// Cobra 默认按字母序渲染子命令，这里只断言集合，不断言声明顺序。
	var gotLeaves []string
	for _, leaf := range leaves {
		gotLeaves = append(gotLeaves, leaf.Name())
	}
	for _, want := range wants {
		if !containsString(gotLeaves, want.use) {
			t.Fatalf("dev event leaves = %v, want %q mounted", gotLeaves, want.use)
		}
	}
}

// TestDevAppEventSubscriptionCommandsStillResolve 是三条命令面边界里最容易被新
// 命令挤掉的一条：dev app event list/subscribe/unsubscribe 管理的是「应用接收
// 哪些钉钉回调事件」，与本次新增的 dev event（应用产生什么事件）不同层，必须
// 原样可用且不被同名工具覆盖。
func TestDevAppEventSubscriptionCommandsStillResolve(t *testing.T) {
	root := newDevAppTestRoot(&fakeDevEventRunner{})
	newTools := map[string]bool{}
	for _, tool := range devEventToolNames() {
		newTools[tool] = true
	}
	for _, use := range []string{"list", "subscribe", "unsubscribe"} {
		cmd := devEventSubCommand(t, root, "dev", "app", "event", use)
		tool := cmd.Annotations["mcp-tool"]
		if strings.TrimSpace(tool) == "" {
			t.Fatalf("dev app event %s has no mcp-tool annotation", use)
		}
		if newTools[tool] {
			t.Fatalf("dev app event %s dispatches %q, which is a dev event definition tool: 命令面被覆盖", use, tool)
		}
	}
}

// TestDevEventToolNamesAreUniqueAcrossDevTree 验证九个新工具名在整棵 dev 树上
// 各只出现一次：既没有与既有命令撞名，也没有在组内重复挂载。
func TestDevEventToolNamesAreUniqueAcrossDevTree(t *testing.T) {
	root := newDevAppTestRoot(&fakeDevEventRunner{})
	counts := map[string]int{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if tool := strings.TrimSpace(cmd.Annotations["mcp-tool"]); tool != "" {
			counts[tool]++
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)

	for _, tool := range devEventToolNames() {
		if counts[tool] != 1 {
			t.Fatalf("tool %q is mounted %d times on the dev tree, want exactly 1", tool, counts[tool])
		}
	}
}

// TestDevEventGroupHelpDeclaresRoutingBoundaries 验证组级 --help 文本自身就带
// 两条路由边界：Agent 在只读组帮助、还没进到叶子时也不该走错命令面。
func TestDevEventGroupHelpDeclaresRoutingBoundaries(t *testing.T) {
	root := newDevAppTestRoot(&fakeDevEventRunner{})
	group := devEventSubCommand(t, root, "dev", "event")
	long := group.Long
	for _, boundary := range []string{"dev app event", "dws event"} {
		if !strings.Contains(long, boundary) {
			t.Fatalf("dev event Long = %q, want it to route away from %q", long, boundary)
		}
	}
}

// TestDevEventLeavesDoNotReuseDevAppToolNames 是一次显式的反向核对：九条叶子的
// 工具名都带 app_event_definition 语义，不与 devapp 既有的应用生命周期工具重名。
// 撞名会让 op-app 网关把配置态请求派发到错误的处理器。
func TestDevEventLeavesDoNotReuseDevAppToolNames(t *testing.T) {
	root := newDevAppTestRoot(&fakeDevEventRunner{})
	app := devEventSubCommand(t, root, "dev", "app")
	existing := map[string]bool{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if tool := strings.TrimSpace(cmd.Annotations["mcp-tool"]); tool != "" {
			existing[tool] = true
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(app)
	if len(existing) == 0 {
		t.Fatal("dev app subtree declares no mcp-tool annotations; the fixture is not exercising the existing surface")
	}
	for _, tool := range devEventToolNames() {
		if existing[tool] {
			t.Fatalf("tool %q already exists under dev app: 新命令复用了既有工具名", tool)
		}
	}
}
