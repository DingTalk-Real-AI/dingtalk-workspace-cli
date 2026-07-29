package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMultiSkillAccountSafetyRuleContract pins the multi-account safety rule
// that release run 30437390088 found missing at release time. In the multi
// layout the rule is owned by the dedicated dingtalk-profile skill (dws-shared
// stays a slim routing entry per the 13-sub-skill design); in the mono layout
// the single SKILL.md carries it directly. Losing the rule from either
// embedded layout must fail at PR time instead of at release time.
func TestMultiSkillAccountSafetyRuleContract(t *testing.T) {
	const rule = "禁止选择第一项、最近登录或最近使用账号"
	for _, tc := range []struct {
		mode string
		rel  string
	}{
		{mode: skillSetupModeMulti, rel: filepath.Join("dingtalk-profile", "SKILL.md")},
		{mode: skillSetupModeMono, rel: "SKILL.md"},
	} {
		dir, cleanup, err := materializeEmbeddedSkillSource(tc.mode)
		if err != nil {
			t.Fatalf("materialize embedded %s skill source: %v", tc.mode, err)
		}
		t.Cleanup(cleanup)

		data, err := os.ReadFile(filepath.Join(dir, tc.rel))
		if err != nil {
			t.Fatalf("read embedded %s %s: %v", tc.mode, tc.rel, err)
		}
		if !strings.Contains(string(data), rule) {
			t.Fatalf("embedded %s %s lost the mandatory account safety rule %q", tc.mode, tc.rel, rule)
		}
	}
}
