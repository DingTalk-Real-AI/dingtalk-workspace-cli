// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package app

import (
	"encoding/json"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAitableNavigationFullAndCompactResult(t *testing.T) {
	paths := []string{"aitable table create", "aitable table get", "aitable +table-get", "aitable +table-bootstrap", "aitable dashboard create", "aitable dashboard get", "aitable +dashboard-get", "aitable +dashboard-create", "aitable field create", "aitable field update", "aitable view create", "aitable view update name", "aitable form create", "aitable form update", "aitable workflow create", "aitable workflow update", "aitable app get", "aitable app update", "aitable app page create", "aitable app page update", "aitable +field-create", "aitable +field-update", "aitable +view-update", "aitable +view-preset-apply", "aitable +form-update", "aitable +workflow-deploy", "aitable +app-page-create", "aitable +form-create"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			full := executeShortcutSchemaQuery(t, "--cli-path", path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			root := NewRootCommand()
			leaf, _, findErr := root.Find(strings.Fields(path))
			if findErr != nil {
				t.Fatal(findErr)
			}
			declaration, ok := contractfinal.RuntimeContractFinal(leaf)
			if !ok || declaration.Result == nil {
				t.Fatalf("missing declared Result: %s", path)
			}
			// The current framework deliberately hides Result for legacy output.
			// Keep that boundary; changing a URL must not change the output envelope.
			if !output.UsesUnifiedResult(leaf) {
				if full["result"] != nil || compact["result"] != nil {
					t.Fatalf("legacy Result leaked: %s", path)
				}
				declared, err := json.Marshal(declaration.Result)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(declared), "url") || !strings.Contains(string(declared), "links") {
					t.Fatalf("incomplete declared navigation: %s", declared)
				}
				return
			}
			if full["result"] == nil || !schemaContractJSONEqual(full["result"], compact["result"]) {
				t.Fatalf("missing or divergent Result: %s", path)
			}
			encoded, err := json.Marshal(compact["result"])
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{`"url"`, `"links"`} {
				if !strings.Contains(string(encoded), key) {
					t.Fatalf("%s missing %s: %s", path, key, encoded)
				}
			}
		})
	}
}

// TestCrossPlatformCoverageAitableWriteNavigationPublished verifies the guidance on rendered Help and both Schema projections.
func TestCrossPlatformCoverageAitableWriteNavigationPublished(t *testing.T) {
	for _, path := range []string{
		"aitable create",
		"aitable base create", "aitable base update", "aitable base copy", "aitable +base-copy",
		"aitable +base-bootstrap", "aitable +table-update", "aitable app widget create", "aitable app widget update",
		"aitable +import-file", "aitable import data",
	} {
		t.Run(path, func(t *testing.T) {
			root := NewRootCommand()
			leaf, _, err := root.Find(strings.Fields(path))
			if err != nil || !output.UsesUnifiedResult(leaf) {
				t.Fatalf("%s has no unified output: %v", path, err)
			}
			full := executeShortcutSchemaQuery(t, "--cli-path", path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			if full["result"] == nil || !schemaContractJSONEqual(full["result"], compact["result"]) {
				t.Fatal("Result missing or divergent", full["result"], compact["result"])
			}
			encoded, _ := json.Marshal(compact["result"])
			for _, key := range []string{`"url"`, `"links"`} {
				if !strings.Contains(string(encoded), key) {
					t.Fatalf("missing %s: %s", key, encoded)
				}
			}
			if path == "aitable +base-copy" && !strings.Contains(string(encoded), `"newBaseId"`) {
				t.Fatal("copied Base identity is missing from Result", string(encoded))
			}
			if path == "aitable +table-update" {
				parameter := full["parameters"].(map[string]any)["name"].(map[string]any)
				if parameter["property"] != "name" {
					t.Fatal("public name property changed", parameter)
				}
			}
			// Check the listed clauses on each delivery surface, including standalone CLI safety and final-host boundaries.
			for surface, text := range map[string]string{
				"help":    executeHelpOutput(t, append(strings.Fields(path), "--help")...),
				"full":    full["agent_summary"].(string),
				"compact": compact["agent_summary"].(string),
			} {
				for _, required := range []string{
					"收尾：生成最终回复前，按用户请求、已确认的交付范围及当前状态清点最终目标",
					"对有当前有效、身份与用途匹配入口的目标",
					"按宿主规则逐目标完成登记与呈现，并核对正文链接，不以父级入口合并替代",
					"成功回执、relation=target 或过程对象不自动成为最终产物",
					"正文链接不替代规定的登记",
					"过程已展示链接或宿主已呈现卡片，最终 Markdown 正文仍须保留最终目标的有效具名完整链接",
					"（`[对象名称](完整URL)`）",
					"缺少适用入口时说明缺口",
					"过程去重不免除此要求",
					"业务核验与宿主交付分别判断",
					"过程与链接：对用户本次要求处理且有当前有效、适用入口的对象",
					"必须逐对象展示具名完整链接",
					"不同对象不合并为单条 Base 入口",
					"不以父级替代已有目标深链",
					"仅有父级入口时明确说明，不冒充目标直达",
					"过程内已展示的同对象同有效入口可去重",
					"内部自动对象不自动纳入，用户要求的对象不因自动创建而排除",
					"入口取实际返回的 url/links 或已确认可用的 shareFormUrl",
					"target/parent 仅表调用关系",
					"身份匹配的 links 即使 relation=parent 仍是当前对象入口",
					"只有链接身份确属父级时才称父级入口",
					"不套用其他对象的 URL 替换 ID",
					"不手抄 JSON 充作源数据",
					"仅当存在明确识别为 JSON 格式的原始业务回执文件时",
					"CSV、XLS、XLSX 等非 JSON 导入附件不是业务回执，不作为 JSON 解析",
					"直接使用当前命令的结构化回执核对并展示链接",
					"不因非 JSON 附件阻断成功结果展示",
					"links[].name 对应同项 URL 对象",
					"缺名用类型或任务标签，仅在仍无法区分时附 ID，不编造名称",
					"凭据按既有规则脱敏；其余业务对象 URL",
					"URL（含参数顺序及百分号编码）须逐字符完整原样保留，不重建",
					"直接从本条回执复制完整 URL，不重新键入、解码再编码或统一大小写",
					"前一条的 %3d 不决定后一条的编码，如 %3D 不改为 %3d",
					"实际待发送链接与本条回执逐字符比对，有一处不同就修正",
					"只声称原样保留不算核对",
					"缺链仅按本工具明确规则及真实标识回退，否则说明缺口、不臆造",
					"表单管理页不得冒充填写页",
					"交付给填写者且填写入口未就绪时，只说明缺口，不展示或登记管理页链接",
					"即使注明非填写入口，也不将管理页列为可交付内容",
					"只有用户明确要求管理操作或接收者为管理者时，才交付当前可用的管理页",
					"边界：登记或展示失败、能力不可用或结果未确认时，保留业务结果并说明缺口",
					"不为补链接或交付重放业务写入",
					"业务回执或展示请求提交不证明卡片已登记、已渲染或面板已打开",
					"链接有无不证明写入完成、定位成功、访问权限或分享状态",
				} {
					if !strings.Contains(text, required) {
						t.Fatalf("%s %s missing %q: %s", path, surface, required, text)
					}
				}
				for _, forbidden := range []string{"artifact_show_link", "open_in_qwenwork", "仅 Base 列表／搜索结果未返回目标", "当输入含有原始回执文件或请求附件时", "正文可不重复", "不重复卡片 URL", "本次已展示的同对象同有效入口可去重", "[名称](完整URL)"} {
					if strings.Contains(text, forbidden) {
						t.Fatalf("%s %s contains forbidden guidance %q", path, surface, forbidden)
					}
				}
			}
		})
	}
}

// TestCrossPlatformCoverageAitableBaseNavigationFallbackPublished keeps conditional Base fallback on read leaves and their aliases only.
func TestCrossPlatformCoverageAitableBaseNavigationFallbackPublished(t *testing.T) {
	for _, path := range []string{"aitable base list", "aitable base search", "aitable list", "aitable search", "aitable +base-list", "aitable +base-search"} {
		t.Run(path, func(t *testing.T) {
			root := NewRootCommand()
			leaf, _, err := root.Find(strings.Fields(path))
			if err != nil {
				t.Fatal(err)
			}
			full := executeShortcutSchemaQuery(t, "--cli-path", path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			for _, text := range []string{leaf.Long, full["agent_summary"].(string), compact["agent_summary"].(string)} {
				for _, required := range []string{
					"仅 Base 列表／搜索结果未返回目标 url/links 时",
					"可用本次返回的真实 baseId",
					"https://alidocs.dingtalk.com/i/nodes/{baseId}",
					"baseId 按路径段编码一次",
					"已有目标链接优先原样使用，不重建",
					"该许可不适用于表单或其他对象",
					"不证明定位、权限、分享或写入状态",
				} {
					if !strings.Contains(text, required) {
						t.Fatalf("%s: Help/Schema missing %q: %s", path, required, text)
					}
				}
			}
		})
	}
}
