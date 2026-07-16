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
	"strings"
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

	metadata, stats, err := generateFromSources(Options{
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

	again, _, err := generateFromSources(Options{
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

func TestGenerateRecursesProductReferencesAndJoinsMultilineExamples(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "skills/mono/SKILL.md", "# DWS\n## 意图判断决策树\n用户提到\"表格\" → `sheet`\n")
	writeFixture(t, root, "skills/mono/references/intent-guide.md", "# Intent guide\n")
	writeFixture(t, root, "skills/mono/references/products/sheet.md", "# Sheet\n")
	writeFixture(t, root, "skills/mono/references/products/sheet/sheet-style.md", "# Style\n"+
		"## 使用场景\n"+
		"用户说\"设置样式\":\n"+
		"- 只改样式 → `range set-style`\n\n"+
		"```bash\n"+
		"# 设置单元格样式\n"+
		"dws sheet range set-style --node node-1 \\\n"+
		"  --sheet-id sheet-1 --range A1:B2 --bg-color '#FFFFFF'\n"+
		"```\n")

	metadata, stats, err := generateFromSources(Options{
		Root:            root,
		SkillPath:       "skills/mono/SKILL.md",
		ProductsDir:     "skills/mono/references/products",
		IntentGuidePath: "skills/mono/references/intent-guide.md",
		ToolPaths: map[string]string{
			"sheet range set-style": "sheet range set-style",
		},
		ProductIDs:       map[string]bool{"sheet": true},
		SurfaceToolCount: 1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	tool, ok := metadata.Tools["sheet range set-style"]
	if !ok {
		t.Fatalf("recursive sheet metadata missing: %#v", metadata.Tools)
	}
	if len(tool.UseWhen) != 2 || len(tool.Examples) != 1 {
		t.Fatalf("tool metadata = %#v", tool)
	}
	wantExample := "dws sheet range set-style --node node-1 --sheet-id sheet-1 --range A1:B2 --bg-color '#FFFFFF'"
	if tool.Examples[0] != wantExample {
		t.Fatalf("example = %q, want %q", tool.Examples[0], wantExample)
	}
	if stats.SourceFiles != 4 || stats.UnmatchedTools != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	if len(stats.SourceProducts) != 1 || stats.SourceProducts[0] != "sheet" {
		t.Fatalf("source products = %#v", stats.SourceProducts)
	}
}

func TestGenerateReportsUnmatchedReferenceLocationsAndCandidates(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "skills/mono/SKILL.md", "# DWS\n## 意图判断决策树\n用户提到\"表格\" → `sheet`\n")
	writeFixture(t, root, "skills/mono/references/intent-guide.md", "# Intent guide\n")
	writeFixture(t, root, "skills/mono/references/products/sheet.md", "# Sheet\n")
	writeFixture(t, root, "skills/mono/references/products/sheet/style.md", "# Style\n"+
		"## 使用场景\n"+
		"用户说\"使用旧样式命令\":\n"+
		"- 设置样式 → `range set-styles`\n\n"+
		"Usage:\n"+
		"  dws sheet range set-style [flags]\n")

	metadata, stats, err := generateFromSources(Options{
		Root:            root,
		SkillPath:       "skills/mono/SKILL.md",
		ProductsDir:     "skills/mono/references/products",
		IntentGuidePath: "skills/mono/references/intent-guide.md",
		ToolPaths: map[string]string{
			"sheet range set-style": "sheet range set-style",
		},
		ProductIDs:       map[string]bool{"sheet": true, "live": true},
		SurfaceToolCount: 1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if stats.UnmatchedTools != 1 || len(stats.UnmatchedReferences) != 1 {
		t.Fatalf("unmatched stats = %#v", stats)
	}
	unmatched := stats.UnmatchedReferences[0]
	if unmatched.ToolPath != "sheet range set-styles" || unmatched.Line != 4 {
		t.Fatalf("unmatched reference = %#v", unmatched)
	}
	if unmatched.Source != "skills/mono/references/products/sheet/style.md" {
		t.Fatalf("unmatched source = %q", unmatched.Source)
	}
	if len(unmatched.Candidates) == 0 || unmatched.Candidates[0] != "sheet range set-style" {
		t.Fatalf("unmatched candidates = %#v", unmatched.Candidates)
	}
	if len(stats.SurfaceProductsWithoutRouting) != 1 || stats.SurfaceProductsWithoutRouting[0] != "live" {
		t.Fatalf("surface products without routing = %#v", stats.SurfaceProductsWithoutRouting)
	}
	audit := BuildAudit(metadata, stats)
	if len(audit.UnmatchedReferences) != 1 || audit.Coverage.UnmatchedSkillTools != 1 {
		t.Fatalf("audit = %#v", audit)
	}
}

func TestValidateEffectiveToolProjectionKeepsManualOnlyCommand(t *testing.T) {
	opts := Options{
		ToolPaths: map[string]string{
			"base.get_item":         "base item get",
			"base item get":         "base item get",
			"base item legacy":      "base item get",
			"helper.add_item":       "helper item add",
			"helper item add":       "helper item add",
			"helper item add-alias": "helper item add",
		},
		SurfaceToolCount: 2,
	}
	file := File{Tools: map[string]ToolMetadata{
		"base item get":   {AgentSummary: "Get an item"},
		"helper item add": {AgentSummary: "Add an item"},
	}}
	if err := validateEffectiveToolProjection(file, opts); err != nil {
		t.Fatalf("validateEffectiveToolProjection() error = %v", err)
	}

	delete(file.Tools, "helper item add")
	err := validateEffectiveToolProjection(file, opts)
	if err == nil || !strings.Contains(err.Error(), "missing=[helper item add]") {
		t.Fatalf("missing manual-only metadata error = %v", err)
	}
	seedEffectiveToolProjection(&file, opts.ToolPaths)
	if _, ok := file.Tools["helper item add"]; !ok {
		t.Fatal("manual-only command was not materialized in Agent metadata")
	}
	if err := validateEffectiveToolProjection(file, opts); err != nil {
		t.Fatalf("seeded manual-only projection error = %v", err)
	}
}

func TestValidateEffectiveToolProjectionRejectsCountOnlyFalseGreen(t *testing.T) {
	opts := Options{
		ToolPaths: map[string]string{
			"base.get_item":   "base item get",
			"base item get":   "base item get",
			"helper.add_item": "helper item add",
			"helper item add": "helper item add",
		},
		SurfaceToolCount: 1,
	}
	file := File{Tools: map[string]ToolMetadata{
		"base item get":   {},
		"helper item add": {},
	}}
	err := validateEffectiveToolProjection(file, opts)
	if err == nil || !strings.Contains(err.Error(), "count 1 disagrees with unique projected tools 2") {
		t.Fatalf("count mismatch error = %v", err)
	}
}

func TestGenerateMergesVersionedHintsByCanonicalPath(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "skills/mono/SKILL.md", "# DWS\n## 意图判断决策树\n用户提到\"日程\" → `calendar`\n")
	writeFixture(t, root, "skills/mono/references/intent-guide.md", "# Intent guide\n")
	writeFixture(t, root, "skills/mono/references/products/calendar.md", "# Calendar\n")
	writeFixture(t, root, "internal/cli/schema_hints/imported/wukong.json", `{
  "version": 1,
  "source": {"kind": "imported", "name": "dws-wukong", "revision": "1234567890abcdef"},
  "products": {
    "calendar": {"agent_summary": "管理日历日程"}
  },
  "tools": {
    "calendar.get_calendar_detail": {
      "agent_summary": "获取日程详情",
      "risk": "high",
      "confirmation": "user_required",
      "examples": ["dws calendar event get --id EVENT_ID"]
    }
  }
}`)
	writeFixture(t, root, "internal/cli/schema_hints/calendar.json", `{
  "version": 1,
  "source": {"kind": "explicit", "name": "schema-review"},
  "tools": {
    "calendar.get_calendar_detail": {
      "agent_summary": "读取一个日程的完整详情",
      "use_when": ["已经取得 eventId，需要查看详情"],
	  "avoid_when": ["需要修改日程时不要使用"],
	  "examples": ["dws calendar event get --id reviewed"],
	  "interface_ref": {"product_id": "calendar", "rpc_name": "get_calendar_detail"},
	  "interface_mode": "mcp",
	  "availability": "available",
      "reviewed": true
    },
    "aitable.base_copy": {
	  "interface_ref": {"product_id": "aitable", "rpc_name": "copy_base"},
	  "interface_mode": "mcp",
	  "availability": "available"
    }
  }
}`)

	metadata, stats, err := generateFromSources(Options{
		Root:            root,
		SkillPath:       "skills/mono/SKILL.md",
		ProductsDir:     "skills/mono/references/products",
		IntentGuidePath: "skills/mono/references/intent-guide.md",
		HintsDir:        "internal/cli/schema_hints",
		ToolPaths: map[string]string{
			"calendar.get_calendar_detail": "calendar event get",
			"calendar event get":           "calendar event get",
			"aitable.base_copy":            "aitable base copy",
		},
		ProductIDs:       map[string]bool{"calendar": true, "aitable": true},
		SurfaceToolCount: 2,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	product := metadata.Products["calendar"]
	if product.AgentSummary != "管理日历日程" || product.AgentSummarySource != "dws-wukong@1234567890ab" {
		t.Fatalf("product metadata = %#v", product)
	}
	tool := metadata.Tools["calendar event get"]
	if tool.AgentSummary != "读取一个日程的完整详情" || tool.AgentSummarySource != "schema-review" {
		t.Fatalf("tool summary = %#v", tool)
	}
	if len(tool.Examples) != 1 || tool.Examples[0] != "dws calendar event get --id reviewed" || len(tool.UseWhen) != 1 || len(tool.AvoidWhen) != 1 || tool.Risk != "high" || tool.Confirmation != "user_required" {
		t.Fatalf("merged tool metadata = %#v", tool)
	}
	if tool.InterfaceMode != "mcp" || tool.Availability != "available" {
		t.Fatalf("interface disposition = %#v", tool)
	}
	if tool.Reviewed == nil || !*tool.Reviewed {
		t.Fatalf("reviewed = %#v", tool.Reviewed)
	}
	if stats.HintFiles != 2 || stats.HintTools != 3 || metadata.Coverage.ToolsWithSummary != 1 {
		t.Fatalf("hint stats = %#v coverage=%#v", stats, metadata.Coverage)
	}
	interfaceOnly, exists := metadata.Tools["aitable base copy"]
	if !exists || interfaceOnly.InterfaceRef == nil {
		t.Fatalf("interface-only hint was not retained: %#v", interfaceOnly)
	}
	if interfaceOnly.InterfaceRef.ProductID != "aitable" || interfaceOnly.InterfaceRef.RPCName != "copy_base" {
		t.Fatalf("interface ref = %#v", interfaceOnly.InterfaceRef)
	}
}

func TestMergeToolMetadataUsesSourcePrecedenceAcrossAliases(t *testing.T) {
	merged, err := mergeToolMetadata(
		ToolMetadata{Risk: "medium", riskRank: selectionRankImported, Confirmation: "not_required", confirmationRank: selectionRankImported},
		ToolMetadata{Risk: "high", riskRank: selectionRankExplicit, Confirmation: "user_required", confirmationRank: selectionRankExplicit},
		"calendar event delete",
	)
	if err != nil {
		t.Fatalf("mergeToolMetadata() error = %v", err)
	}
	if merged.Risk != "high" || merged.Confirmation != "user_required" {
		t.Fatalf("merged safety = %s/%s, want high/user_required", merged.Risk, merged.Confirmation)
	}

	merged, err = mergeToolMetadata(
		ToolMetadata{Risk: "high", riskRank: selectionRankReviewedExplicit, Confirmation: "user_required", confirmationRank: selectionRankReviewedExplicit},
		ToolMetadata{Risk: "low", riskRank: selectionRankImported, Confirmation: "not_required", confirmationRank: selectionRankImported},
		"calendar event delete",
	)
	if err != nil {
		t.Fatalf("mergeToolMetadata() error = %v", err)
	}
	if merged.Risk != "high" || merged.Confirmation != "user_required" {
		t.Fatalf("lower-precedence safety replaced the winner: %s/%s", merged.Risk, merged.Confirmation)
	}
}

func TestGenerateAppliesReviewedSkillReferenceDispositions(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "skills/mono/SKILL.md", "# DWS\n")
	writeFixture(t, root, "skills/mono/references/intent-guide.md", "# Intent guide\n")
	writeFixture(t, root, "skills/mono/references/products/sheet.md", "# Sheet\n"+
		"dws sheet range old --node NODE\n"+
		"dws sheet range removed --node NODE\n")
	writeFixture(t, root, "internal/cli/schema_hints/reference-review.json", `{
  "version": 1,
  "source": {"kind": "explicit", "name": "reference-review"},
  "reference_review": {
    "sheet range old": {"status": "alias", "target": "sheet range read", "reason": "renamed"},
    "sheet range removed": {"status": "stale", "reason": "removed from the public surface"}
  }
}`)

	metadata, stats, err := generateFromSources(Options{
		Root:            root,
		SkillPath:       "skills/mono/SKILL.md",
		ProductsDir:     "skills/mono/references/products",
		IntentGuidePath: "skills/mono/references/intent-guide.md",
		HintsDir:        "internal/cli/schema_hints",
		ToolPaths: map[string]string{
			"sheet.range_read": "sheet range read",
			"sheet range read": "sheet range read",
		},
		ProductIDs:       map[string]bool{"sheet": true},
		SurfaceToolCount: 1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, ok := metadata.Tools["sheet range read"]; !ok {
		t.Fatalf("reviewed alias was not merged: %#v", metadata.Tools)
	}
	if stats.UnmatchedTools != 1 || stats.unreviewedSkillTools != 0 || len(stats.UnmatchedReferences) != 1 {
		t.Fatalf("reference review stats = %#v", stats)
	}
	reference := stats.UnmatchedReferences[0]
	if reference.ToolPath != "sheet range removed" || reference.Review == nil || reference.Review.Status != "stale" {
		t.Fatalf("reviewed unmatched reference = %#v", reference)
	}
}

func TestClassifyEffectPathUsesActionSegment(t *testing.T) {
	tests := map[string]string{
		"aitable view get visible-fields":            "read",
		"aitable view update visible-fields":         "write",
		"mail message batch-move":                    "write",
		"sheet filter-view get-criteria":             "read",
		"sheet filter-view update-criteria":          "write",
		"minutes record pause":                       "write",
		"chat message download-media":                "read",
		"dev app version primary-doc-get":            "read",
		"dev app version role-update":                "write",
		"calendar event suggest":                     "read",
		"aitable record primary-doc-create":          "write",
		"chat message combine-forward":               "write",
		"chat group-mute-member":                     "write",
		"devdoc article legacy-search-open-platform": "read",
		"sheet media-upload":                         "write",
	}
	for path, want := range tests {
		if got := classifyEffectPath(path); got != want {
			t.Errorf("classifyEffectPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestApplyDefaultSafetyFillsOnlyMissingFields(t *testing.T) {
	read := ToolMetadata{Effect: "read"}
	applyDefaultSafety(&read)
	if read.Risk != "low" || read.Confirmation != "not_required" || read.Idempotency != "idempotent" {
		t.Fatalf("read defaults = %#v", read)
	}

	write := ToolMetadata{Effect: "write"}
	applyDefaultSafety(&write)
	if write.Risk != "medium" || write.Confirmation != "not_required" || write.Idempotency != "unknown" {
		t.Fatalf("write defaults = %#v", write)
	}

	danger := ToolMetadata{Effect: "destructive", Risk: "high", Confirmation: "user_required"}
	applyDefaultSafety(&danger)
	if danger.Risk != "high" || danger.Confirmation != "user_required" {
		t.Fatalf("danger defaults overwrote explicit policy: %#v", danger)
	}

	downgradedDanger := ToolMetadata{Effect: "destructive", Risk: "medium", Confirmation: "not_required"}
	applyDefaultSafety(&downgradedDanger)
	if downgradedDanger.Risk != "medium" || downgradedDanger.Confirmation != "not_required" {
		t.Fatalf("explicit safety values were overwritten by defaults: %#v", downgradedDanger)
	}
}
