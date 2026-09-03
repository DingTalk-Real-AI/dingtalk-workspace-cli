package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

func TestCrossPlatformCoverageSkillSetupRollbackRetainsConcurrentReplacement(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".agents", "skills")
	first := filepath.Join(base, "dingtalk-first")
	second := filepath.Join(base, "dingtalk-second")
	writeSkillSetupFile(t, first, "old first")
	writeSkillSetupFile(t, second, "old second")
	backups, err := backupSkillSetupTarget(home, []skillSetupBackup{{Path: first}, {Path: second}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	stageRoot := filepath.Join(base, ".stage")
	stagedFirst := filepath.Join(stageRoot, "dingtalk-first")
	stagedSecond := filepath.Join(stageRoot, "dingtalk-second")
	writeSkillSetupFile(t, stagedFirst, "new first")
	writeSkillSetupFile(t, stagedSecond, "new second")

	failure := errors.New("injected second setup publication failure")
	originalPublish := skillSetupPublishPath
	calls := 0
	testseam.Swap(t, &skillSetupPublishPath, func(staged, destination string) (upgrade.SkillPathPublication, error) {
		calls++
		if calls == 2 {
			if err := os.RemoveAll(first); err != nil {
				t.Fatal(err)
			}
			writeSkillSetupFile(t, first, "concurrent")
			return upgrade.SkillPathPublication{}, failure
		}
		return originalPublish(staged, destination)
	})

	err = publishSkillSetupTarget([]skillSetupStagedDir{
		{staged: stagedFirst, dest: first},
		{staged: stagedSecond, dest: second},
	}, backups)
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "拒绝删除非本事务") {
		t.Fatalf("setup transaction error = %v", err)
	}
	assertSkillSetupFile(t, first, "concurrent")
	assertSkillSetupFile(t, second, "old second")
	var firstBackup string
	for _, item := range backups {
		if item.original == first {
			firstBackup = item.backup
			break
		}
	}
	if firstBackup == "" {
		t.Fatal("first backup was not recorded")
	}
	assertSkillSetupFile(t, firstBackup, "old first")
}

func writeSkillSetupFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertSkillSetupFile(t *testing.T, dir, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil || string(content) != want {
		t.Fatalf("Skill content at %s = %q, %v; want %q", dir, content, err, want)
	}
}
