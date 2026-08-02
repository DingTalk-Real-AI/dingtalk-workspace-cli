package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMultiSkillSharedContractKeepsAccountSafetyRule pins the multi-account
// safety rule that release run 30437390088 found missing: the MultiSkill e2e
// contract asserts the exact phrase below inside the installed
// dws-shared/SKILL.md, so removing it from the embedded skill source must
// fail at PR time instead of at release time.
func TestMultiSkillSharedContractKeepsAccountSafetyRule(t *testing.T) {
	dir, cleanup, err := materializeEmbeddedSkillSource(skillSetupModeMulti)
	if err != nil {
		t.Fatalf("materialize embedded multi skill source: %v", err)
	}
	t.Cleanup(cleanup)

	data, err := os.ReadFile(filepath.Join(dir, "dws-shared", "SKILL.md"))
	if err != nil {
		t.Fatalf("read embedded dws-shared/SKILL.md: %v", err)
	}
	const rule = "禁止选择第一项、最近登录或最近使用账号"
	if !strings.Contains(string(data), rule) {
		t.Fatalf("embedded dws-shared/SKILL.md lost the mandatory account safety rule %q", rule)
	}
}
