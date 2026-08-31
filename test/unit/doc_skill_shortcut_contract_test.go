package unit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDocSkillPinsRequiredShortcutArguments(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	path := filepath.Join(root, "skills", "multi", "dingtalk-doc", "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(content)

	for _, required := range []string{
		"dws doc +create --name <标题> --content <文本\\|-\\|@文件> [--folder <ID>\\|--workspace <ID>]",
		"dws doc +import --file <相对路径> [--folder <ID>\\|--workspace <ID>]",
		"指定位置复用真实 ID，二者互斥",
		"未指定才由 Runtime 取默认根",
		"dws doc +comment-create --node <ID或URL> --content <文字> [--selection <原文>]",
		"+review --node <ID或URL>",
		"node/content 必填",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("%s missing required shortcut contract %q", path, required)
		}
	}
}
