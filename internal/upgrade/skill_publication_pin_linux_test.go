//go:build linux

package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSkillPublicationPinnedIdentity(t *testing.T) {
	for _, kind := range []string{"directory", "file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			staged, destination := filepath.Join(root, "staged"), filepath.Join(root, "destination")
			seed := func(path string) {
				t.Helper()
				var err error
				switch kind {
				case "directory":
					seedUpgradeSkill(t, path, "identical", false)
				case "file":
					err = os.WriteFile(path, []byte("identical"), 0o600)
				case "symlink":
					err = os.Symlink("missing-target", path)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			seed(staged)
			publication, err := PublishSkillPathNoReplace(staged, destination)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ReleaseSkillPathPublications([]SkillPathPublication{publication}) })
			if publication.identityPin == nil {
				t.Fatal("Linux publication must retain an identity handle")
			}
			if err := os.RemoveAll(destination); err != nil {
				t.Fatal(err)
			}
			seed(destination)
			held, err := publication.identityPin.Stat()
			if err != nil {
				t.Fatal(err)
			}
			live, err := os.Lstat(destination)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(held, publication.identity) || os.SameFile(held, live) {
				t.Fatal("held identity must outlive unlink and distinguish an identical replacement")
			}
			if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); err == nil || !strings.Contains(err.Error(), "拒绝删除非本事务") {
				t.Fatalf("replacement rollback = %v", err)
			}
			if _, err := os.Lstat(destination); err != nil {
				t.Fatal("replacement lost:", err)
			}
			if _, err := publication.identityPin.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("handle not released: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageSkillPublicationPinLifetime(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "missing", "rollback-error"} {
		t.Run(outcome, func(t *testing.T) {
			root := t.TempDir()
			staged, destination := filepath.Join(root, "staged"), filepath.Join(root, "destination")
			seedUpgradeSkill(t, staged, "new", false)
			publication, err := PublishSkillPathNoReplace(staged, destination)
			if err != nil {
				t.Fatal(err)
			}
			publications := []SkillPathPublication{publication}
			t.Cleanup(func() { _ = ReleaseSkillPathPublications(publications) })
			if outcome == "commit" {
				err = ReleaseSkillPathPublications(publications)
			} else {
				if outcome == "missing" {
					if err := os.RemoveAll(destination); err != nil {
						t.Fatal(err)
					}
				}
				if outcome == "rollback-error" {
					testseam.Swap(t, &skillPathMkdirTemp, func(string, string) (string, error) { return "", os.ErrPermission })
				}
				err = RollbackSkillPathPublications(publications)
			}
			if outcome == "rollback-error" {
				if !errors.Is(err, os.ErrPermission) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if _, err := publication.identityPin.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("handle not released: %v", err)
			}
			if err := ReleaseSkillPathPublications(publications); err != nil {
				t.Fatal("release must be idempotent:", err)
			}
			if outcome == "commit" {
				if err := RollbackSkillPathPublications(publications); err == nil {
					t.Fatal("released ownership proof must not authorize deletion")
				}
				assertUpgradeSkillContent(t, destination, "new")
			}
		})
	}
}

func TestCrossPlatformCoverageSkillPublicationPinMissingPath(t *testing.T) {
	if _, err := pinSkillPathIdentity(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageUpgradePublicationReleasesPins(t *testing.T) {
	for _, failSecond := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[failSecond], func(t *testing.T) {
			root := t.TempDir()
			var captured []SkillPathPublication
			testseam.Swap(t, &upgradePublishSkillPath, func(source, destination string) (SkillPathPublication, error) {
				if failSecond && len(captured) == 1 {
					return SkillPathPublication{}, os.ErrPermission
				}
				publication, err := PublishSkillPathNoReplace(source, destination)
				if err == nil {
					captured = append(captured, publication)
				}
				return publication, err
			})
			t.Cleanup(func() { _ = ReleaseSkillPathPublications(captured) })
			var staged []stagedSkillDir
			for _, name := range []string{"first", "second"} {
				source, destination := filepath.Join(root, "staged-"+name), filepath.Join(root, name)
				seedUpgradeSkill(t, source, "new", false)
				staged = append(staged, stagedSkillDir{staged: source, dest: destination})
			}
			err := publishStagedSkillSet(root, staged, nil)
			if failSecond {
				if !errors.Is(err, os.ErrPermission) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			for _, publication := range captured {
				if _, err := publication.identityPin.Stat(); !errors.Is(err, os.ErrClosed) {
					t.Fatalf("transaction leaked pin: %v", err)
				}
			}
		})
	}
}
