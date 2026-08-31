package unit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSheetSkillRoutesImportTemplatesAndValidation(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	rootReferences := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "sheet.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "sheet.md"),
	}
	for _, path := range rootReferences {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, required := range []string{
			"dws sheet import create",
			"dws drive download",
			"本地 xlsx/xls",
			"Drive 中的 xlsx/xls",
			"当前 `sheet import` 不支持 xlsm/csv",
			"dws sheet template list",
			"dws sheet template search",
			"dws sheet template apply",
			"唯一的真实 `templateId`",
			"`details.status` 为 `unknown` 或 `partial_success`",
			"不得整体重跑 `create-with-data`",
			"不得自动删除已写数据",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing Sheet route contract %q", path, required)
			}
		}
		if strings.Contains(text, "本地或 Drive 中的 xlsx/xls/xlsm/csv 文件") {
			t.Errorf("%s still collapses import and local-analysis intents", path)
		}
	}

	writeReferences := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "sheet", "sheet-write-data.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "sheet", "sheet-write-data.md"),
	}
	for _, path := range writeReferences {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, required := range []string{
			"不传表示保留原规则",
			`{"dataValidation":{"type":"none"}}`,
			"`dropdown` 或 `checkbox`",
			"`options` 与 `sourceRange` 必须且只能传一个",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing dataValidation contract %q", path, required)
			}
		}
	}

	chartReferences := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "sheet", "sheet-chart.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "sheet", "sheet-chart.md"),
	}
	for _, path := range chartReferences {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, required := range []string{
			"`axisMin` / `axisMax` 为 `number` / `null`",
			"`splitLine:boolean`",
			"`minorSplitLine:boolean`",
			"`axisLabel:boolean`",
			"`axisLine:boolean`",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing chart axis contract %q", path, required)
			}
		}
		if strings.Contains(text, "number|null") {
			t.Errorf("%s contains an unescaped GFM table separator", path)
		}
	}
}
