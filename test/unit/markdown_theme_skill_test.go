// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package unit_test

import (
	"os"
	"strings"
	"testing"
)

func TestMarkdownThemeContractIsPublishedOnMonoAndMultiSkills(t *testing.T) {
	paths := []string{
		"../../skills/mono/references/products/markdown.md",
		"../../skills/multi/dingtalk-misc/references/markdown.md",
	}
	required := []string{
		"### 主题选择（仅 create / overwrite）",
		"Agent 不手写或手改 Front Matter",
		"用户通常不必主动选择主题",
		"创作完整新文档或执行完整重写时",
		"原样搬运已有文件",
		"用户明确选择始终优先",
		"没有明显倾向时使用 `default`",
		"单个颜色词、emoji、代码块或表格不能单独决定主题",
		"`default` | 混合用途或意图不明确",
		"`songyan` | 冷静克制、留白多",
		"`taiying` | 暖色亲和、有书卷感",
		"`sujian` | 简洁中性、信息效率高",
		"`juxia` | 实际视觉偏红、表达强",
		"`qingya` | 清新现代、结构化",
		"整篇恰好一个 H1 作为大标题",
		"章节从 H2 开始",
		"子章节使用 H3/H4",
		"H2 标题不得手写章节编号或序号前缀",
		"这两个主题会为 H2 自动添加编号元素",
		"手写编号会造成重复",
		"未经授权重排标题",
		"显式 `--theme default` 会写入 `default`",
		"不传则完全不处理主题",
		"不属于 `fetch`、`diff` 或 `patch`",
		"没有独立的 set-theme 命令",
		"0600 临时上传副本",
		"不修改 `--file` 或 `--content @file` 指向的用户文件",
		"after 中显示主题处理后的最终完整内容",
		"不会把远程旧 Front Matter 合并进新内容",
		"必须保持完全相同的 `--theme`",
		"主题有任何变化都要重新 dry-run",
	}

	contents := make([]string, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		contents = append(contents, content)
		for _, phrase := range required {
			if !strings.Contains(content, phrase) {
				t.Errorf("%s is missing theme contract %q", path, phrase)
			}
		}
		if got := strings.Count(content, "--theme string"); got != 2 {
			t.Errorf("%s publishes --theme in %d flag blocks, want create and overwrite only", path, got)
		}
	}

	monoSection := markdownThemeSkillSection(t, contents[0])
	multiSection := markdownThemeSkillSection(t, contents[1])
	if monoSection != multiSection {
		t.Fatal("mono and multi Markdown theme contracts diverged")
	}
}

func markdownThemeSkillSection(t *testing.T, content string) string {
	t.Helper()
	_, after, ok := strings.Cut(content, "### 主题选择（仅 create / overwrite）")
	if !ok {
		t.Fatal("Markdown theme section is missing")
	}
	section, _, ok := strings.Cut(after, "## 比较 Markdown 差异")
	if !ok {
		t.Fatal("Markdown theme section has no stable end marker")
	}
	return strings.TrimSpace(section)
}
