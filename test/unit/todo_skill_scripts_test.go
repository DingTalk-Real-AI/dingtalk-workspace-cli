package unit

import (
	"os"
	"os/exec"
	"testing"
)

func TestCrossPlatformCoverageTodoSkillScripts(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	cmd := exec.Command(python, "test/scripts/todo_skill_scripts_test.py")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("todo skill script tests failed: %v\n%s", err, output)
	}
}
