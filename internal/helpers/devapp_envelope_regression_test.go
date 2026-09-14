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
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// devAppRegressionLeafPaths 遍历 dev app 命令树，收集全部叶子路径（无子命令
// 即叶子）。核销清单的「清单 ⇆ 命令树」双向绑定依赖它：新增叶子若不在清单
// 里，清单测试失败，强制信封覆盖随叶子同批落盘（AC-28/M1.8）。
func devAppRegressionLeafPaths(t *testing.T) []string {
	t.Helper()
	root := newDevAppTestRoot(devAppFamilyContentRunner(map[string]any{}))
	appCmd, _, err := root.Find([]string{"dev", "app"})
	if err != nil {
		t.Fatalf("Find(dev app) error = %v", err)
	}
	var leaves []string
	var traverse func(c *cobra.Command, prefix []string)
	traverse = func(c *cobra.Command, prefix []string) {
		if !c.HasSubCommands() {
			leaves = append(leaves, strings.Join(prefix, " "))
			return
		}
		for _, sub := range c.Commands() {
			traverse(sub, append(append([]string{}, prefix...), sub.Name()))
		}
	}
	traverse(appCmd, nil)
	sort.Strings(leaves)
	return leaves
}

// devAppEnvelopeRegressionChecklist 是 dev app 树 32 叶子的信封核销清单
// （队列 B138，AC-28/M1.8）：每条给出可执行 argv。读叶子走成功路径
// （outcome=success），写叶子带 --dry-run 走预览路径（outcome=success +
// dry_run=true）。argv 与轮 11-B0 族验证测试同源（各叶子 flag 已逐一核对）。
func devAppEnvelopeRegressionChecklist() []struct {
	path  string
	args  []string
	write bool
} {
	return []struct {
		path  string
		args  []string
		write bool
	}{
		{"list", []string{"dev", "app", "list", "--name", "DemoApp"}, false},
		{"get", []string{"dev", "app", "get", "--unified-app-id", "u-1"}, false},
		{"create", []string{"dev", "app", "create", "--name", "DemoApp", "--dry-run"}, true},
		{"update", []string{"dev", "app", "update", "--unified-app-id", "u-1", "--name", "NewName", "--dry-run"}, true},
		{"delete", []string{"dev", "app", "delete", "--unified-app-id", "u-1", "--dry-run"}, true},
		{"disable", []string{"dev", "app", "disable", "--unified-app-id", "u-1", "--dry-run"}, true},
		{"enable", []string{"dev", "app", "enable", "--unified-app-id", "u-1", "--dry-run"}, true},
		{"credentials get", []string{"dev", "app", "credentials", "get", "--unified-app-id", "u-1"}, false},
		{"webapp get", []string{"dev", "app", "webapp", "get", "--unified-app-id", "u-1"}, false},
		{"webapp config", []string{"dev", "app", "webapp", "config", "--unified-app-id", "u-1", "--homepage-url", "https://example.com", "--dry-run"}, true},
		{"permission list", []string{"dev", "app", "permission", "list", "--unified-app-id", "u-1"}, false},
		{"permission add", []string{"dev", "app", "permission", "add", "--unified-app-id", "u-1", "--scope-values", "Contact.User.mobile", "--dry-run"}, true},
		{"permission remove", []string{"dev", "app", "permission", "remove", "--unified-app-id", "u-1", "--scope-values", "Contact.User.mobile", "--dry-run"}, true},
		{"member list", []string{"dev", "app", "member", "list", "--unified-app-id", "u-1"}, false},
		{"member add", []string{"dev", "app", "member", "add", "--unified-app-id", "u-1", "--user-ids", "user-1", "--member-type", "DEVELOPER", "--dry-run"}, true},
		{"member remove", []string{"dev", "app", "member", "remove", "--unified-app-id", "u-1", "--user-ids", "user-1", "--member-type", "DEVELOPER", "--dry-run"}, true},
		{"security config", []string{"dev", "app", "security", "config", "--unified-app-id", "u-1", "--redirect-urls", "https://cb.example.invalid/cb", "--dry-run"}, true},
		{"robot submit", []string{"dev", "app", "robot", "submit", "--name", "智能体", "--robot-name", "小助手", "--desc", "审批问答", "--dry-run"}, true},
		{"robot result", []string{"dev", "app", "robot", "result", "--task-id", "t-1"}, false},
		{"robot get", []string{"dev", "app", "robot", "get", "--unified-app-id", "u-1"}, false},
		{"robot config", []string{"dev", "app", "robot", "config", "--unified-app-id", "u-1", "--name", "小助手", "--dry-run"}, true},
		{"robot enable", []string{"dev", "app", "robot", "enable", "--unified-app-id", "u-1", "--dry-run"}, true},
		{"robot disable", []string{"dev", "app", "robot", "disable", "--unified-app-id", "u-1", "--dry-run"}, true},
		{"version create", []string{"dev", "app", "version", "create", "--unified-app-id", "u-1", "--desc", "新增机器人", "--dry-run"}, true},
		{"version list", []string{"dev", "app", "version", "list", "--unified-app-id", "u-1"}, false},
		{"version get", []string{"dev", "app", "version", "get", "--unified-app-id", "u-1", "--version-id", "v-1"}, false},
		{"version check-approval", []string{"dev", "app", "version", "check-approval", "--unified-app-id", "u-1", "--version-id", "v-1"}, false},
		{"version publish", []string{"dev", "app", "version", "publish", "--unified-app-id", "u-1", "--version-id", "v-1", "--dry-run"}, true},
		{"version status", []string{"dev", "app", "version", "status", "--unified-app-id", "u-1", "--version-id", "v-1"}, false},
		{"event list", []string{"dev", "app", "event", "list", "--unified-app-id", "u-1"}, false},
		{"event subscribe", []string{"dev", "app", "event", "subscribe", "--unified-app-id", "u-1", "--event-codes", "a,b", "--dry-run"}, true},
		{"event unsubscribe", []string{"dev", "app", "event", "unsubscribe", "--unified-app-id", "u-1", "--event-codes", "a,b", "--dry-run"}, true},
	}
}

// TestDevAppEnvelopeRegressionLeafInventory 是核销清单的清单 ⇆ 命令树双向
// 绑定：dev app 树叶子集合必须与清单完全一致。新增叶子未补信封断言、或
// 清单残留已删除叶子，都会在此失败。
func TestDevAppEnvelopeRegressionLeafInventory(t *testing.T) {
	tree := devAppRegressionLeafPaths(t)

	want := make([]string, 0)
	for _, entry := range devAppEnvelopeRegressionChecklist() {
		want = append(want, entry.path)
	}
	sort.Strings(want)

	if len(tree) != len(want) {
		t.Fatalf("leaf count = %d, checklist = %d\ntree: %v\nchecklist: %v",
			len(tree), len(want), tree, want)
	}
	for i := range tree {
		if tree[i] != want[i] {
			t.Fatalf("leaf[%d] = %q, checklist = %q\ntree: %v\nchecklist: %v",
				i, tree[i], want[i], tree, want)
		}
	}
}

// regressionEnvelope 是核销断言的信封解码形状：ok 用 *bool——缺席（未走
// 统一出口）为 nil，`"ok":"true"` 字符串形态（违反 AC-02）直接解码失败。
type regressionEnvelope struct {
	OK      *bool          `json:"ok"`
	Outcome string         `json:"outcome"`
	DryRun  bool           `json:"dry_run"`
	Data    map[string]any `json:"data"`
}

// TestDevAppEnvelopeRegressionUnifiedExit 逐一执行 32 叶子并断言 stdout 恰为
// 一个统一信封（走 writeDevAppEnvelope 唯一出口）：ok 为 JSON 布尔 true、
// outcome 与读/写形态一致、data 非空。任何绕过信封直写 stdout 的叶子
// （legacy 裸 JSON/人读文案）都会因缺 ok 键或 JSON 解析失败而暴露。
func TestDevAppEnvelopeRegressionUnifiedExit(t *testing.T) {
	for _, entry := range devAppEnvelopeRegressionChecklist() {
		t.Run(entry.path, func(t *testing.T) {
			out, errBuf, err := runDevAppFamily(t,
				devAppFamilyContentRunner(map[string]any{
					"unifiedAppId": "u-1",
					"name":         "DemoApp",
					"appStatus":    "ENABLED",
				}),
				entry.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
			}

			var env regressionEnvelope
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not a single JSON envelope (unified exit bypass?): %v\n%s", err, out.String())
			}
			if env.OK == nil {
				t.Fatalf("envelope missing ok key (unified exit bypass?): %s", out.String())
			}
			if !*env.OK {
				t.Fatalf("ok = false on success/pending path (I1 violated): %s", out.String())
			}
			if entry.write {
				if env.Outcome != "success" || !env.DryRun {
					t.Fatalf("write leaf dry-run outcome/dry_run = %q/%v, want success/true: %s",
						env.Outcome, env.DryRun, out.String())
				}
			} else {
				if env.Outcome != "success" || env.DryRun {
					t.Fatalf("read leaf outcome/dry_run = %q/%v, want success/false: %s",
						env.Outcome, env.DryRun, out.String())
				}
			}
			if env.Data == nil {
				t.Fatalf("envelope data is nil: %s", out.String())
			}
		})
	}
}
