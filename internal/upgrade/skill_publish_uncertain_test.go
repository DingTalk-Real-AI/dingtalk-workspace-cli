// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// TestCrossPlatformCoverageUncertainPublicationState pins the contract behind
// ErrSkillPathPublicationUncertain: a child-move publication whose mkdir
// claim no longer describes the destination reports the uncertain sentinel
// instead of guessing, keeps the destination in place, and stops upstream
// retry paths (the linked-copy fallback) from displacing the concurrent
// writer's object.
func TestCrossPlatformCoverageUncertainPublicationState(t *testing.T) {

	t.Run("destination replaced after child-move reports uncertain and is retained", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "payload", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		// Force the child-move slow path on every platform (Linux would
		// otherwise publish through the fast-path rename over the claim).
		testseam.Swap(t, &skillPathRename, func(string, string) error { return os.ErrExist })
		// The identity seam simulates the wholesale replacement: the mkdir
		// claim captures "claim", every later read of the destination
		// reports the replacement's "foreign" identity. This keeps the
		// test deterministic on platforms without a stable file identity.
		destinationReads := 0
		testseam.Swap(t, &skillPathFileIdentity, func(path string) string {
			if path != destination {
				return ""
			}
			destinationReads++
			if destinationReads == 1 {
				return "claim"
			}
			return "foreign"
		})
		_, err := PublishSkillPathNoReplace(staged, destination)
		if !errors.Is(err, ErrSkillPathPublicationUncertain) {
			t.Fatalf("replacement after child-move = %v, want uncertain sentinel", err)
		}
		if destinationReads != 2 {
			t.Fatalf("destination identity reads = %d, want 2 (claim capture + ownership check)", destinationReads)
		}
		// Nothing was rolled back over the retained destination.
		assertUpgradeSkillContent(t, destination, "payload")
	})

	t.Run("child-move reports the real claim identity where the platform supports it", func(t *testing.T) {
		dir := t.TempDir()
		if id := skillPathFileIdentity(dir); id == "" {
			t.Skip("platform reports no stable file identity; the claim witness degrades to the fingerprint backstop")
		}
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "destination")
		seedUpgradeSkill(t, source, "payload", false)
		testseam.Swap(t, &skillPathRenameNoReplaceAtomic, func(string, string) error { return errNoReplaceRenameUnsupported })
		testseam.Swap(t, &skillPathRename, func(string, string) error { return os.ErrExist })
		claim, err := renameSkillPathNoReplace(source, destination)
		if err != nil {
			t.Fatal(err)
		}
		if claim == "" {
			t.Fatal("child-move must return the mkdir-claim identity on platforms that report one")
		}
		if live := skillPathFileIdentity(destination); live == "" || live != claim {
			t.Fatalf("destination identity = %q, want the published claim %q", live, claim)
		}
	})

	t.Run("uncertain publication passes through the staged-set transaction", func(t *testing.T) {
		home := t.TempDir()
		old := filepath.Join(home, "skills", "dws")
		staged := filepath.Join(home, "skills", ".stage", "dws")
		seedUpgradeSkill(t, old, "old", false)
		seedUpgradeSkill(t, staged, "new", false)
		testseam.Swap(t, &upgradePublishSkillPath, func(string, string) (SkillPathPublication, error) {
			return SkillPathPublication{}, fmt.Errorf("并发写入: %w", ErrSkillPathPublicationUncertain)
		})
		err := publishStagedSkillSet(home, []stagedSkillDir{{staged: staged, dest: old}}, []string{old})
		if !errors.Is(err, ErrSkillPathPublicationUncertain) {
			t.Fatalf("staged-set publish = %v, want uncertain passthrough", err)
		}
		// The pre-publish victim is restored; the uncertain destination is
		// never retried or overwritten.
		assertUpgradeSkillContent(t, old, "old")
	})

	t.Run("mono copy fallback does not retry an uncertain destination", func(t *testing.T) {
		home := t.TempDir()
		base := filepath.Join(home, ".agents", "skills")
		seedUpgradeSkill(t, filepath.Join(base, "dws"), "old mono", false)
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills"})
		monoRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(monoRoot, "SKILL.md"), []byte("new mono"), 0o644); err != nil {
			t.Fatal(err)
		}
		linkedDest := filepath.Join(home, ".claude", "skills", "dws")
		linkedAttempts := 0
		originalPublish := upgradePublishSkillPath
		testseam.Swap(t, &upgradePublishSkillPath, func(src, dest string) (SkillPathPublication, error) {
			if dest == linkedDest {
				linkedAttempts++
				return SkillPathPublication{}, fmt.Errorf("%w：链接发布状态不确定", ErrSkillPathPublicationUncertain)
			}
			return originalPublish(src, dest)
		})
		result, err := upgradeMonoSkillLocations(home, monoRoot)
		if err != nil {
			t.Fatalf("mono upgrade = %v", err)
		}
		if linkedAttempts != 1 {
			t.Fatalf("linked publish attempts = %d, want exactly 1 (the copy fallback must not retry an uncertain destination)", linkedAttempts)
		}
		recorded := false
		for _, entry := range result.Results {
			if entry.Dir == linkedDest && entry.Status == SkillDirFailed && errors.Is(entry.Err, ErrSkillPathPublicationUncertain) {
				recorded = true
			}
		}
		if !recorded {
			t.Fatalf("results = %#v, want a recorded uncertain failure for %s", result.Results, linkedDest)
		}
	})

	t.Run("multi copy fallback does not retry an uncertain destination", func(t *testing.T) {
		home := t.TempDir()
		base := filepath.Join(home, ".agents", "skills")
		seedUpgradeSkill(t, filepath.Join(base, "dingtalk-a"), "old a", false)
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &knownSkillDirs, []string{".agents/skills", ".claude/skills"})
		multiRoot := writeMultiBundle(t, t.TempDir(), "dingtalk-a")
		linkedBase := filepath.Join(home, ".claude", "skills")
		linkedAttempts := 0
		originalPublish := upgradePublishSkillPath
		testseam.Swap(t, &upgradePublishSkillPath, func(src, dest string) (SkillPathPublication, error) {
			if strings.HasPrefix(dest, linkedBase) {
				linkedAttempts++
				return SkillPathPublication{}, fmt.Errorf("%w：链接发布状态不确定", ErrSkillPathPublicationUncertain)
			}
			return originalPublish(src, dest)
		})
		result, err := upgradeMultiSkillLocations(home, multiRoot, []string{"dingtalk-a"})
		if err != nil {
			t.Fatalf("multi upgrade = %v", err)
		}
		if linkedAttempts != 1 {
			t.Fatalf("linked publish attempts = %d, want exactly 1 (the copy fallback must not retry an uncertain destination)", linkedAttempts)
		}
		recorded := false
		for _, entry := range result.Results {
			if entry.Dir == linkedBase && entry.Status == SkillDirFailed && errors.Is(entry.Err, ErrSkillPathPublicationUncertain) {
				recorded = true
			}
		}
		if !recorded {
			t.Fatalf("results = %#v, want a recorded uncertain failure for %s", result.Results, linkedBase)
		}
	})
}
