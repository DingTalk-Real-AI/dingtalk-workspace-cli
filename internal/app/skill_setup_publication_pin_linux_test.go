//go:build linux

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
)

func TestCrossPlatformCoverageSkillSetupPublicationReleasesPins(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	writeSkillSetupFile(t, source, "new")
	var captured upgrade.SkillPathPublication
	testseam.Swap(t, &skillSetupPublishPath, func(source, destination string) (upgrade.SkillPathPublication, error) {
		publication, err := upgrade.PublishSkillPathNoReplace(source, destination)
		captured = publication
		return publication, err
	})
	t.Cleanup(func() { _ = upgrade.ReleaseSkillPathPublications([]upgrade.SkillPathPublication{captured}) })
	if err := publishSkillSetupTarget([]skillSetupStagedDir{{staged: source, dest: destination}}, nil); err != nil {
		t.Fatal(err)
	}
	// 成功提交后的记录已释放句柄，不能再用旧记录删除已交付的内容。
	if err := upgrade.RollbackSkillPathPublications([]upgrade.SkillPathPublication{captured}); err == nil || !strings.Contains(err.Error(), "拒绝删除非本事务") {
		t.Fatalf("committed publication still owns deletion: %v", err)
	}
	assertSkillSetupFile(t, destination, "new")
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
