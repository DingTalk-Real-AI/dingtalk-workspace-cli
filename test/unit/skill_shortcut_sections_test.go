package unit

import (
	"os"
	"os/exec"
	"testing"
)

func TestCrossPlatformCoverageSkillShortcutSections(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	cmd := exec.Command(python, "test/scripts/skill_shortcut_sections_test.py")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Skill Shortcut section tests failed: %v\n%s", err, output)
	}
}
