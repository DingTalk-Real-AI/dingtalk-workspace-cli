package unit_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestChatSkillPythonScripts(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	cmd := exec.Command(
		python,
		"test/scripts/chat_skill_scripts_test.py",
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("chat skill Python tests failed: %v\n%s", err, output)
	}
}
