package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSkillUpgradeAlwaysRestoresOfficialBundle(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", "")
	home := withFakeHome(t)
	testseam.Swap(t, &knownSkillDirs, []string{".agents/skills"})
	testseam.Swap(t, &upgradeNow, func() time.Time { return time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC) })
	base := filepath.Join(home, ".agents", "skills")
	for _, name := range []string{"dingtalk-a", "dingtalk-shared"} {
		if err := os.MkdirAll(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, name, "SKILL.md"), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := skillstate.Write(home, skillstate.State{OfficialSkills: []string{"dingtalk-a", "dingtalk-b", "dingtalk-shared"}}); err != nil {
		t.Fatal(err)
	}
	multi := writeMultiBundle(t, t.TempDir(), "dingtalk-a", "dingtalk-b", "dingtalk-c", "dingtalk-shared")
	result, err := UpgradeSkillLocationsWithOptions(multi, SkillUpgradeOptions{Version: "1.1.0"})
	if err != nil || len(result.Failed()) != 0 {
		t.Fatalf("full refresh = %#v, %v", result, err)
	}
	for _, name := range []string{"dingtalk-a", "dingtalk-b", "dingtalk-c", "dingtalk-shared"} {
		if _, err := os.Stat(filepath.Join(base, name, "SKILL.md")); err != nil {
			t.Fatalf("official Skill %s was not restored: %v", name, err)
		}
	}
	state, readable, err := skillstate.Read(home)
	wantOfficial := []string{"dingtalk-a", "dingtalk-b", "dingtalk-c", "dingtalk-shared"}
	if err != nil || !readable || !reflect.DeepEqual(state.OfficialSkills, wantOfficial) || !reflect.DeepEqual(state.UpdatedSkills, wantOfficial) {
		t.Fatalf("state = %#v, %v, %v", state, readable, err)
	}
	if len(state.ManagedSkills) != len(wantOfficial) {
		t.Fatalf("managed provenance = %#v", state.ManagedSkills)
	}
	for _, provenance := range state.ManagedSkills {
		if provenance.Version != "1.1.0" || provenance.Source != skillprovenance.SourceUpgrade || !strings.HasPrefix(provenance.Digest, "sha256:") {
			t.Fatalf("provenance = %#v", provenance)
		}
	}
}

func TestCrossPlatformCoverageFullSkillUpgradeIgnoresOldStateAndReportsWriteFailure(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", "")
	testseam.Swap(t, &knownSkillDirs, []string{".agents/skills"})
	multi := writeMultiBundle(t, t.TempDir(), "dingtalk-a", "dingtalk-b", "dingtalk-shared")
	home := withFakeHome(t)
	statePath := skillstate.Path(home)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := UpgradeSkillLocationsWithOptions(multi, SkillUpgradeOptions{})
	if err != nil || len(result.Succeeded()) != 1 {
		t.Fatalf("corrupt old state should not block full refresh = %#v, %v", result, err)
	}
	home2 := t.TempDir()
	testseam.Swap(t, &upgradeUserHomeDir, func() (string, error) { return home2, nil })
	testseam.Swap(t, &upgradeWriteSkillState, func(string, skillstate.State) error { return errors.New("denied") })
	result, err = UpgradeSkillLocationsWithOptions(multi, SkillUpgradeOptions{})
	if err == nil || !strings.Contains(err.Error(), "状态未写入") || len(result.Succeeded()) != 1 {
		t.Fatalf("write failure = %#v, %v", result, err)
	}
}
