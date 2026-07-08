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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeKnowledgeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	faq := `# 安装

dws 通过 brew install dws 安装，升级用 dws upgrade。

# 缓存问题

服务发现已下线。出现命令缺失或 endpoint_not_resolved 时，检查静态端点目录并升级到包含该能力的 dws 版本。
`
	bot := `# 机器人建联

用 dws devapp robot create 建号，dws devapp robot connect 建联。
卡片停在数据加载中说明回复超过了 AI 助理应答窗口。
`
	if err := os.WriteFile(filepath.Join(dir, "faq.md"), []byte(faq), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bot.md"), []byte(bot), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-knowledge files are ignored.
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte{0x89, 0x50}, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestKnowledgeBaseLoadAndChunk(t *testing.T) {
	kb, err := loadKnowledgeBase(writeKnowledgeDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// faq.md has 2 heading-chunks, bot.md has 1.
	if len(kb.chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(kb.chunks))
	}
}

func TestKnowledgeAugmentPicksRelevantChunk(t *testing.T) {
	kb, err := loadKnowledgeBase(writeKnowledgeDir(t))
	if err != nil {
		t.Fatal(err)
	}
	got := kb.augment("机器人卡在数据加载中怎么办")
	if !strings.Contains(got, "AI 助理应答窗口") {
		t.Fatalf("augment missed the bot chunk:\n%s", got)
	}
	if !strings.Contains(got, "用户问题：机器人卡在数据加载中怎么办") {
		t.Fatalf("augment must keep the original question:\n%s", got)
	}
	if !strings.Contains(got, "bot.md") {
		t.Fatalf("augment must cite the source file:\n%s", got)
	}
}

func TestKnowledgeAugmentEnglishTerms(t *testing.T) {
	kb, err := loadKnowledgeBase(writeKnowledgeDir(t))
	if err != nil {
		t.Fatal(err)
	}
	got := kb.augment("endpoint_not_resolved 是干什么的")
	if !strings.Contains(got, "静态端点目录") {
		t.Fatalf("augment missed the cache chunk:\n%s", got)
	}
}

func TestKnowledgeAugmentNoMatchPassesThrough(t *testing.T) {
	kb, err := loadKnowledgeBase(writeKnowledgeDir(t))
	if err != nil {
		t.Fatal(err)
	}
	q := "qqqq zzzz xxxx"
	if got := kb.augment(q); got != q {
		t.Fatalf("no-match should pass through, got:\n%s", got)
	}
}

func TestKnowledgeBaseEmptyDirErrors(t *testing.T) {
	if _, err := loadKnowledgeBase(t.TempDir()); err == nil {
		t.Fatal("empty dir should error so a typo'd --knowledge-dir is loud")
	}
}

func TestKnowledgeNilBasePassesThrough(t *testing.T) {
	var kb *knowledgeBase
	if got := kb.augment("你好"); got != "你好" {
		t.Fatalf("nil kb should pass through, got %q", got)
	}
}

func TestIsKnowledgeTextExt(t *testing.T) {
	for _, e := range []string{".md", ".MD", ".markdown", ".mdx", ".txt", ".text"} {
		if !isKnowledgeTextExt(e) {
			t.Fatalf("%q should be an indexable text ext", e)
		}
	}
	for _, e := range []string{".pdf", ".docx", ".json", ".png", ""} {
		if isKnowledgeTextExt(e) {
			t.Fatalf("%q should NOT be indexable", e)
		}
	}
}

func TestKnowledgeBaseIndexesMarkdownFamily(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.markdown"), []byte("# 标题\n\n内容关于考勤制度的说明。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.text"), []byte("纯文本资料一段。"), 0o644); err != nil {
		t.Fatal(err)
	}
	kb, err := loadKnowledgeBase(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(kb.chunks) == 0 {
		t.Fatal(".markdown/.text should be indexed")
	}
}

func TestKnowledgeBaseNoTextFilesError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.pdf"), []byte("%PDF-1.7"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadKnowledgeBase(dir)
	if err == nil || !strings.Contains(err.Error(), "没有可用的文本内容") {
		t.Fatalf("a dir with only non-text files should error clearly, got %v", err)
	}
}
