package unit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDriveSkillPinsDeleteNodeArgument(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	path := filepath.Join(root, "skills", "multi", "dingtalk-drive", "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	required := "dws drive +delete --node <dentryUuid>"
	if !strings.Contains(string(content), required) {
		t.Fatalf("%s missing required shortcut contract %q", path, required)
	}
}
