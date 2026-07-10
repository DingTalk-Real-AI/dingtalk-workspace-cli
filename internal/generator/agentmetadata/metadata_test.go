// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agentmetadata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateCompilesSkillSemantics(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "skills/mono/SKILL.md", "# DWS\n"+
		"## 意图判断决策树\n"+
		"用户提到\"日程/会议室\" → `calendar`\n"+
		"用户提到\"待办/任务提醒\" → `todo`\n"+
		"用户提到\"群消息\" → `chat`（机器人配置走 `dev`）\n\n"+
		"## 危险操作确认\n"+
		"| 产品 | 命令 | 说明 |\n"+
		"|---|---|---|\n"+
		"| `calendar` | `event delete` | 删除日程，不可逆 |\n\n"+
		"### 确认流程\n")
	writeFixture(t, root, "skills/mono/references/intent-guide.md", "# 意图路由指南\n"+
		"## 易混淆场景快速对照表\n"+
		"| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |\n"+
		"|---|---|---|---|---|\n"+
		"| \"建项目跟踪表\" | 结构化数据 | `aitable` | `todo` | 有行列 |\n")
	writeFixture(t, root, "skills/mono/references/products/calendar.md", "# 日历\n"+
		"### 查询日程\nUsage:\n  dws calendar event list [flags]\n"+
		"Example:\n  dws calendar event list --start 2026-01-01 --end 2026-01-02\n\n"+
		"### 搜索会议室\nUsage:\n  dws calendar room search [flags]\n"+
		"Example:\n  dws calendar room search # 不传时间时使用默认窗口\n\n"+
		"### 创建日程\nUsage:\n  dws calendar event create [flags]\n"+
		"Example:\n  dws calendar event create --title \"评审会\" --start 2026-01-01 --end 2026-01-02\n\n"+
		"### 删除日程\nUsage:\n  dws calendar event delete [flags]\n\n"+
		"## 意图判断\n用户说\"日程/会议\":\n"+
		"- 查看 → `event list`\n"+
		"- 创建/约会 → `event create`\n"+
		"- 取消/删除 → `event delete`\n")
	writeFixture(t, root, "skills/mono/references/products/dev.md", "# dev\n"+
		"```bash\n"+
		"# 创建/更新机器人配置（upsert）\n"+
		"dws dev app robot config --unified-app-id app --name bot\n"+
		"\n"+
		"# 删除应用（不可逆，需二次确认）\n"+
		"dws dev app delete --unified-app-id app --yes\n"+
		"```\n")

	metadata, stats, err := Generate(Options{
		Root:            root,
		SkillPath:       "skills/mono/SKILL.md",
		ProductsDir:     "skills/mono/references/products",
		IntentGuidePath: "skills/mono/references/intent-guide.md",
		MaxExamples:     2,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if metadata.Version != CurrentVersion || metadata.SourceHash == "" {
		t.Fatalf("metadata header = %#v", metadata)
	}
	if got := metadata.Products["calendar"].UseWhen; len(got) != 1 || got[0] != "日程/会议室" {
		t.Fatalf("calendar use_when = %#v", got)
	}
	if got := metadata.Products["todo"].AvoidWhen; len(got) != 1 || got[0] != "建项目跟踪表；结构化数据" {
		t.Fatalf("todo avoid_when = %#v", got)
	}
	if got := metadata.Products["chat"].UseWhen; len(got) != 1 || got[0] != "群消息" {
		t.Fatalf("chat use_when = %#v", got)
	}
	if got := metadata.Products["dev"].UseWhen; len(got) != 0 {
		t.Fatalf("note target polluted dev use_when = %#v", got)
	}
	if _, ok := metadata.Tools["calendar room search # 不传时间时使用默认窗口"]; ok {
		t.Fatalf("inline comment was treated as a command path: %#v", metadata.Tools)
	}
	if _, ok := metadata.Tools["calendar room search"]; !ok {
		t.Fatalf("calendar room search missing from metadata: %#v", metadata.Tools)
	}
	create := metadata.Tools["calendar event create"]
	if len(create.UseWhen) != 1 || create.Effect != "write" || len(create.Examples) != 1 {
		t.Fatalf("create metadata = %#v", create)
	}
	deleteMeta := metadata.Tools["calendar event delete"]
	if deleteMeta.Risk != "high" || deleteMeta.Confirmation != "user_required" || deleteMeta.Effect != "destructive" {
		t.Fatalf("delete metadata = %#v", deleteMeta)
	}
	devConfig := metadata.Tools["dev app robot config"]
	if len(devConfig.UseWhen) != 1 || devConfig.UseWhen[0] != "创建/更新机器人配置（upsert）" || devConfig.Effect != "write" || len(devConfig.Examples) != 1 {
		t.Fatalf("dev config metadata = %#v", devConfig)
	}
	devDelete := metadata.Tools["dev app delete"]
	if devDelete.Effect != "destructive" || devDelete.EffectSource != "skill-comment" || devDelete.Risk != "high" || devDelete.Confirmation != "user_required" {
		t.Fatalf("dev delete safety metadata = %#v", devDelete)
	}
	if stats.RiskRules != 1 || stats.ToolIntents != 5 {
		t.Fatalf("stats = %#v", stats)
	}

	again, _, err := Generate(Options{
		Root:            root,
		SkillPath:       "skills/mono/SKILL.md",
		ProductsDir:     "skills/mono/references/products",
		IntentGuidePath: "skills/mono/references/intent-guide.md",
	})
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if again.SourceHash != metadata.SourceHash {
		t.Fatalf("source hash is not deterministic: %q != %q", again.SourceHash, metadata.SourceHash)
	}
}

func writeFixture(t *testing.T, root, relative, body string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestClassifyEffectVerbIncludesLocalPolicyCommands(t *testing.T) {
	for _, verb := range []string{"browser-policy", "chmod"} {
		if got := classifyEffectVerb(verb); got != "write" {
			t.Errorf("classifyEffectVerb(%q) = %q, want write", verb, got)
		}
	}
}
