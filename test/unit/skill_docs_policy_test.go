package unit_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSkillDocsDoNotRecommendRetiredCommands(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	skillsDir := filepath.Join(root, "skills")
	retiredCommands := []string{
		"chat file upload",
		"conference start",
		"conference get-id",
		"conference member invite",
		"conference share",
		"dingtalk-conference",
	}
	allowedContext := []string{
		"已下线",
		"下线",
		"不支持",
		"不要",
		"无需",
		"当前 CLI 不支持",
		"兼容提示",
		"不可用",
		"钉钉客户端",
	}

	var violations []string
	err := filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(content), "\n") {
			for _, retired := range retiredCommands {
				if !strings.Contains(line, retired) {
					continue
				}
				if hasAny(line, allowedContext) {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s:%d recommends retired command %q: %s", rel, i+1, retired, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("skill docs recommend retired commands:\n%s", strings.Join(violations, "\n"))
	}
}

func hasAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
