package unit_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAITableImportViaTaskSkillHelper(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not available")
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	command := exec.Command(python, filepath.Join(root, "test", "skill_scripts", "aitable_import_via_task_test.py"))
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("AITable import Skill helper tests failed: %v\n%s", err, output)
	}
}
