//go:build windows

package upgrade

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageWindowsSamePhysicalSkillRootJunction(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "canonical")
	alias := filepath.Join(root, "alias")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createSkillPathDirJunction(canonical, alias); err != nil {
		t.Skipf("junction unavailable: %v", err)
	}
	defer os.Remove(alias)

	if !samePhysicalSkillRoot(alias, canonical) {
		t.Fatal("junction pointing to canonical must be recognized as the same physical root")
	}

	t.Run("relative target", func(t *testing.T) {
		testseam.Swap(t, &upgradeReadlink, func(path string) (string, error) {
			if path == alias {
				return "canonical", nil
			}
			return os.Readlink(path)
		})
		if !samePhysicalSkillRoot(alias, canonical) {
			t.Fatal("relative junction target must resolve to the canonical root")
		}
	})

	t.Run("readlink failure", func(t *testing.T) {
		testseam.Swap(t, &upgradeReadlink, func(string) (string, error) {
			return "", os.ErrPermission
		})
		if samePhysicalSkillRoot(alias, canonical) {
			t.Fatal("unreadable junction target must not be treated as canonical")
		}
	})

	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if samePhysicalSkillRoot(alias, other) {
		t.Fatal("junction pointing to another root must not be treated as canonical")
	}
}

func TestCrossPlatformCoverageWindowsJunctionCleanupSelectors(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "canonical")
	base := filepath.Join(root, "base")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("chat"), 0o644); err != nil {
		t.Fatal(err)
	}

	stalePath := filepath.Join(base, "dingtalk-stale")
	if err := createSkillPathDirJunction(canonical, stalePath); err != nil {
		t.Skipf("junction unavailable: %v", err)
	}
	defer os.Remove(stalePath)
	useUpgradeManagedNames(t, "dingtalk-stale")

	managed, err := managedMultiSkillVictims(base, readManagedSkillNames(t.TempDir()))
	if err != nil || len(managed) != 1 || managed[0] != stalePath {
		t.Fatalf("managed junction victims = %v, %v", managed, err)
	}

	oppositePath := filepath.Join(base, "dingtalk-opposite")
	if err := createSkillPathDirJunction(canonical, oppositePath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(oppositePath)
	useUpgradeManagedNames(t, "dingtalk-opposite")
	victims, err := oppositeModeSkillVictims(base, map[string]bool{}, readManagedSkillNames(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, victim := range victims {
		if victim == oppositePath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("opposite-mode junction victims = %v", victims)
	}
}

func TestCrossPlatformCoverageWindowsUpgradePublishesAndKeepsJunction(t *testing.T) {
	home := withFakeHome(t)
	testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills"})
	for _, parent := range []string{".agents", ".claude"} {
		if err := os.MkdirAll(filepath.Join(home, parent, "skills"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	extract := t.TempDir()
	multiRoot := writeMultiBundle(t, extract, "dingtalk-chat")
	for i := 0; i < 2; i++ {
		result, err := UpgradeSkillLocations(multiRoot)
		if err != nil || len(result.Failed()) != 0 {
			t.Fatalf("UpgradeSkillLocations() iteration %d = %#v, %v", i, result, err)
		}
		link := filepath.Join(home, ".claude", "skills", "dingtalk-chat")
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular == 0 {
			t.Fatalf("upgrade link mode = %v, want junction", info.Mode())
		}
		if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
			t.Fatalf("junction Skill must remain readable: %v", err)
		}
	}
}
