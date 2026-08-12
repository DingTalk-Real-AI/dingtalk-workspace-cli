// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillprovenance"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func useManagedSkillNames(t *testing.T, names ...string) {
	t.Helper()
	records := make([]skillprovenance.Record, 0, len(names))
	for _, name := range names {
		records = append(records, skillprovenance.Record{Name: name})
	}
	testseam.Swap(t, &skillSetupReadState, func(string) (*skillstate.State, bool, error) {
		return &skillstate.State{ManagedSkills: records}, true, nil
	})
}

// TestCrossPlatformCoverageSkillSetupConfirmPreviewsStaleSkills verifies the
// confirmation prompt lists stale dingtalk-* / dws-shared directories that a
// full (unfiltered) multi install will back up and remove, and that a
// filtered install previews nothing extra.
func TestCrossPlatformCoverageSkillSetupConfirmPreviewsStaleSkills(t *testing.T) {
	testseam.Swap(t, &skillSetupInteractive, func() bool { return false })

	dest := filepath.Join(t.TempDir(), ".claude", "skills")
	stale := filepath.Join(dest, "dingtalk-old")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	useManagedSkillNames(t, "dingtalk-old")

	var out bytes.Buffer
	ok, err := confirmSkillSetup(&out, skillSetupModeMulti, "src", []string{dest}, []string{"dingtalk-chat"}, false)
	if err == nil || ok || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("confirmSkillSetup = (%v, %v), want non-interactive confirmation error", ok, err)
	}
	if !strings.Contains(out.String(), "将备份并移除过期 skill") {
		t.Fatalf("full install preview must list stale skills, got %q", out.String())
	}
	if !strings.Contains(out.String(), filepath.Join(dest, "dingtalk-old")) {
		t.Fatalf("preview must name the stale directory, got %q", out.String())
	}

	out.Reset()
	ok, err = confirmSkillSetup(&out, skillSetupModeMulti, "src", []string{dest}, []string{"dingtalk-chat"}, true)
	if err == nil || ok {
		t.Fatalf("filtered confirmSkillSetup = (%v, %v), want non-interactive confirmation error", ok, err)
	}
	if strings.Contains(out.String(), "将备份并移除过期 skill") {
		t.Fatalf("filtered install must stay additive in the preview, got %q", out.String())
	}
}

func TestCrossPlatformCoverageSkillSetupUnifiedOwnership(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dingtalk-custom")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if isManagedDWSMultiSkillDir(dir) {
		t.Fatal("an unregistered dingtalk-* directory must not be treated as DWS-owned")
	}
	managed := map[string]bool{"dingtalk-custom": true}
	if !isManagedDWSMultiSkillDir(dir, managed) {
		t.Fatal("unified metadata must prove ownership")
	}
	legacy := filepath.Join(t.TempDir(), legacySharedSkill)
	if !isManagedDWSMultiSkillDir(legacy) {
		t.Fatal("the exact legacy dws-shared name must remain managed")
	}

}

func TestCrossPlatformCoveragePublishManagedSkillFailurePaths(t *testing.T) {
	src := writeMultiSkillSource(t, []string{"dingtalk-a"})
	skillSrc := filepath.Join(src, "dingtalk-a")
	failure := errors.New("publish denied")

	t.Run("mkdir", func(t *testing.T) {
		testseam.Swap(t, &skillSetupPublishTemp, func(string, string) (string, error) { return "", failure })
		err := publishDWSManagedSkillDir(skillSrc, filepath.Join(t.TempDir(), "dingtalk-a"))
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "staging") {
			t.Fatalf("mkdir error = %v", err)
		}
	})

	t.Run("copy", func(t *testing.T) {
		parent := t.TempDir()
		testseam.Swap(t, &skillSetupCopyDir, func(string, string) error { return failure })
		err := publishDWSManagedSkillDir(skillSrc, filepath.Join(parent, "dingtalk-a"))
		if !errors.Is(err, failure) {
			t.Fatalf("copy error = %v", err)
		}
		if entries, readErr := os.ReadDir(parent); readErr != nil || len(entries) != 0 {
			t.Fatalf("copy failure retained staging: %v, err=%v", entries, readErr)
		}
	})

	t.Run("rename", func(t *testing.T) {
		parent := t.TempDir()
		testseam.Swap(t, &skillSetupPublishRename, func(string, string) error { return failure })
		err := publishDWSManagedSkillDir(skillSrc, filepath.Join(parent, "dingtalk-a"))
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "发布 Skill") {
			t.Fatalf("rename error = %v", err)
		}
		if entries, readErr := os.ReadDir(parent); readErr != nil || len(entries) != 0 {
			t.Fatalf("rename failure retained staging: %v, err=%v", entries, readErr)
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		renameErr := errors.New("rename denied")
		cleanupErr := errors.New("cleanup denied")
		testseam.Swap(t, &skillSetupPublishRename, func(string, string) error { return renameErr })
		testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return cleanupErr })
		err := publishDWSManagedSkillDir(skillSrc, filepath.Join(t.TempDir(), "dingtalk-a"))
		if !errors.Is(err, renameErr) || !errors.Is(err, cleanupErr) {
			t.Fatalf("cleanup error = %v", err)
		}
	})
}

// TestCrossPlatformCoverageSkillSetupCleanupHomeFailure verifies that
// cleanupMutualExclusion keeps every victim in place with a warning when
// $HOME cannot be resolved, instead of destroying anything.
func TestCrossPlatformCoverageSkillSetupCleanupHomeFailure(t *testing.T) {
	dest := filepath.Join(t.TempDir(), ".agents", "skills")
	victim := filepath.Join(dest, "dws")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "SKILL.md"), []byte("mono"), 0o644); err != nil {
		t.Fatal(err)
	}
	homeErr := errors.New("home boom")
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return "", homeErr })

	var out, errOut bytes.Buffer
	cleanupMutualExclusion(dest, skillSetupModeMulti, &out, &errOut)
	if !strings.Contains(errOut.String(), "无法解析 HOME，跳过删除") {
		t.Fatalf("expected HOME warning on errOut, got %q", errOut.String())
	}
	if _, err := os.Stat(filepath.Join(victim, "SKILL.md")); err != nil {
		t.Fatalf("victim must survive the HOME failure: %v", err)
	}
}

func TestCrossPlatformCoverageSkillSetupBackupFailureSkipsWholeTarget(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	copyCalls := 0
	testseam.Swap(t, &skillSetupCopyDir, func(string, string) error {
		copyCalls++
		return nil
	})
	failure := errors.New("backup boom")
	testseam.Swap(t, &skillSetupBackupAndRemove, func(_ string, dir string) (string, error) {
		if filepath.Base(dir) == "dws" {
			return "", failure
		}
		return "", nil
	})

	dest := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(filepath.Join(dest, "dws"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := writeMultiSkillSource(t, []string{"dingtalk-a", "dingtalk-shared"})
	var out, errOut bytes.Buffer
	installed, skipped, err := installMultiSkillToHomes(src, []string{"dingtalk-a", "dingtalk-shared"}, []string{dest}, &out, &errOut, false)
	if err != nil || installed != 0 || skipped != 2 {
		t.Fatalf("install = (%d, %d, %v), want (0, 2, nil)", installed, skipped, err)
	}
	if copyCalls != 2 {
		t.Fatalf("backup failure staged %d new Skills, want 2", copyCalls)
	}
	if !strings.Contains(errOut.String(), "跳过整个 Agent 目标") {
		t.Fatalf("missing whole-target warning: %q", errOut.String())
	}
}

func TestCrossPlatformCoverageSkillSetupCleanupMutualExclusionBackupFailure(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".agents", "skills")
	victim := filepath.Join(dest, "dws")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("backup boom")
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupBackupAndRemove, func(_ string, dir string) (string, error) {
		if dir != victim {
			t.Fatalf("backup victim = %q, want %q", dir, victim)
		}
		return "", failure
	})

	var out, errOut bytes.Buffer
	err := cleanupMutualExclusion(dest, skillSetupModeMulti, &out, &errOut)
	if !errors.Is(err, failure) {
		t.Fatalf("cleanup error = %v, want %v", err, failure)
	}
	if out.Len() != 0 || !strings.Contains(errOut.String(), "互斥清理失败") {
		t.Fatalf("cleanup output = %q / %q", out.String(), errOut.String())
	}
}

func TestCrossPlatformCoverageSkillSetupMonoCleanupFailureSkipsWholeTarget(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	copyCalls := 0
	testseam.Swap(t, &skillSetupCopyDir, func(string, string) error {
		copyCalls++
		return nil
	})
	failure := errors.New("multi backup boom")
	testseam.Swap(t, &skillSetupBackupAndRemove, func(_ string, dir string) (string, error) {
		if filepath.Base(dir) == "dingtalk-a" {
			return "", failure
		}
		return "", nil
	})

	base := filepath.Join(home, ".agents", "skills")
	multi := filepath.Join(base, "dingtalk-a")
	if err := os.MkdirAll(multi, 0o755); err != nil {
		t.Fatal(err)
	}
	useManagedSkillNames(t, filepath.Base(multi))
	monoSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(monoSrc, "SKILL.md"), []byte("mono"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	installed, skipped, err := installSkillToHomes(monoSrc, []string{filepath.Join(base, "dws")}, &out, &errOut)
	if err != nil || installed != 0 || skipped != 1 {
		t.Fatalf("install = (%d, %d, %v), want (0, 1, nil)", installed, skipped, err)
	}
	if copyCalls != 1 {
		t.Fatalf("multi cleanup failure staged mono %d times, want 1", copyCalls)
	}
	if _, err := os.Stat(multi); err != nil {
		t.Fatalf("multi leftover must survive backup failure: %v", err)
	}
	if !strings.Contains(errOut.String(), "Skill 备份失败，已执行回滚，跳过整个 Agent 目标") {
		t.Fatalf("missing mono whole-target warning: %q", errOut.String())
	}
}

func TestCrossPlatformCoverageSkillSetupStaleBackupFailureSkipsWholeTarget(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	copyCalls := 0
	testseam.Swap(t, &skillSetupCopyDir, func(string, string) error {
		copyCalls++
		return nil
	})
	failure := errors.New("stale backup boom")
	testseam.Swap(t, &skillSetupBackupAndRemove, func(_ string, dir string) (string, error) {
		if filepath.Base(dir) == "dingtalk-stale" {
			return "", failure
		}
		return "", nil
	})

	dest := filepath.Join(home, ".agents", "skills")
	stale := filepath.Join(dest, "dingtalk-stale")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	useManagedSkillNames(t, filepath.Base(stale))
	src := writeMultiSkillSource(t, []string{"dingtalk-a", "dingtalk-shared"})
	var out, errOut bytes.Buffer
	installed, skipped, err := installMultiSkillToHomes(src, []string{"dingtalk-a", "dingtalk-shared"}, []string{dest}, &out, &errOut, false)
	if err != nil || installed != 0 || skipped != 2 {
		t.Fatalf("install = (%d, %d, %v), want (0, 2, nil)", installed, skipped, err)
	}
	if copyCalls != 2 {
		t.Fatalf("stale backup failure staged %d new Skills, want 2", copyCalls)
	}
	if !strings.Contains(errOut.String(), "Skill 备份失败，已执行回滚，跳过整个 Agent 目标") {
		t.Fatalf("missing stale whole-target warning: %q", errOut.String())
	}
}

func TestCrossPlatformCoverageSkillSetupTransactionFailuresRestoreOldSet(t *testing.T) {
	for _, failureKind := range []string{"later_backup", "later_publish"} {
		failureKind := failureKind
		t.Run(failureKind, func(t *testing.T) {
			home := t.TempDir()
			dest := filepath.Join(home, ".agents", "skills")
			first := filepath.Join(dest, "dingtalk-first")
			second := filepath.Join(dest, "dingtalk-second")
			for path, body := range map[string]string{
				filepath.Join(first, "SKILL.md"):  "old first\n",
				filepath.Join(second, "SKILL.md"): "old second\n",
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			src := writeMultiSkillSource(t, []string{"dingtalk-first", "dingtalk-second"})
			if err := os.WriteFile(filepath.Join(src, "dingtalk-first", "SKILL.md"), []byte("new first\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(src, "dingtalk-second", "SKILL.md"), []byte("new second\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
			failure := errors.New("injected " + failureKind + " failure")
			if failureKind == "later_backup" {
				originalBackup := skillSetupBackupAndRemove
				testseam.Swap(t, &skillSetupBackupAndRemove, func(homeDir, dir string) (string, error) {
					if dir == second {
						return "", failure
					}
					return originalBackup(homeDir, dir)
				})
			} else {
				originalRename := skillSetupPublishRename
				testseam.Swap(t, &skillSetupPublishRename, func(oldPath, newPath string) error {
					if newPath == second && strings.HasPrefix(filepath.Base(filepath.Dir(oldPath)), ".dws-setup-set-") {
						return failure
					}
					return originalRename(oldPath, newPath)
				})
			}

			var out, errOut bytes.Buffer
			installed, skipped, err := installMultiSkillToHomes(
				src,
				[]string{"dingtalk-first", "dingtalk-second"},
				[]string{dest},
				&out,
				&errOut,
				true,
			)
			if err != nil || installed != 0 || skipped != 2 {
				t.Fatalf("transaction failure = (%d, %d, %v), stderr=%s", installed, skipped, err, errOut.String())
			}
			for path, want := range map[string]string{
				filepath.Join(first, "SKILL.md"):  "old first\n",
				filepath.Join(second, "SKILL.md"): "old second\n",
			} {
				got, readErr := os.ReadFile(path)
				if readErr != nil || string(got) != want {
					t.Fatalf("restored %s = %q, err=%v, want %q", path, got, readErr, want)
				}
			}
			entries, readErr := os.ReadDir(dest)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".dws-setup-set-") {
					t.Fatalf("transaction left staging directory %s", entry.Name())
				}
			}
			if !strings.Contains(errOut.String(), "已执行回滚") || !strings.Contains(errOut.String(), failure.Error()) {
				t.Fatalf("transaction failure output = %q", errOut.String())
			}
		})
	}
}

func TestCrossPlatformCoverageSkillSetupTransactionFailureEdges(t *testing.T) {
	failure := errors.New("injected transaction failure")

	t.Run("managed publish success", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{"dingtalk-a"})
		dest := filepath.Join(t.TempDir(), "dingtalk-a")
		if err := publishDWSManagedSkillDir(filepath.Join(src, "dingtalk-a"), dest); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
			t.Fatalf("published Skill missing: %v", err)
		}
	})

	t.Run("staging cleanup failure", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{"dingtalk-a"})
		dest := t.TempDir()
		testseam.Swap(t, &skillSetupCopyDir, func(string, string) error { return failure })
		cleanupErr := errors.New("staging cleanup failure")
		testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return cleanupErr })
		_, _, err := stageSkillSetupTarget(
			&skillSetupPlan{Mode: skillSetupModeMulti, Source: src, MultiSkillNames: []string{"dingtalk-a"}},
			skillSetupTargetPlan{Destination: dest},
		)
		if !errors.Is(err, failure) || !errors.Is(err, cleanupErr) {
			t.Fatalf("staging cleanup error = %v", err)
		}
	})

	t.Run("staging directory failure", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{"dingtalk-a"})
		dest := t.TempDir()
		originalMkdirAll := skillSetupMkdirAll
		testseam.Swap(t, &skillSetupMkdirAll, func(path string, mode os.FileMode) error {
			if filepath.Base(path) == "dingtalk-a" && strings.HasPrefix(filepath.Base(filepath.Dir(path)), ".dws-setup-set-") {
				return failure
			}
			return originalMkdirAll(path, mode)
		})
		_, _, err := stageSkillSetupTarget(
			&skillSetupPlan{Mode: skillSetupModeMulti, Source: src, MultiSkillNames: []string{"dingtalk-a"}},
			skillSetupTargetPlan{Destination: dest},
		)
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "创建 Skill staging 目录失败") {
			t.Fatalf("staging directory error = %v", err)
		}
	})

	t.Run("restore failure aggregation", func(t *testing.T) {
		t.Run("remove published", func(t *testing.T) {
			testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return failure })
			if err := restoreSkillSetupTarget([]string{"published"}, nil); !errors.Is(err, failure) {
				t.Fatalf("remove published error = %v", err)
			}
		})
		t.Run("original still exists", func(t *testing.T) {
			original := t.TempDir()
			err := restoreSkillSetupTarget(nil, []skillSetupBackedUpDir{{original: original, backup: "backup"}})
			if err == nil || !strings.Contains(err.Error(), "恢复目标仍存在") {
				t.Fatalf("existing restore target error = %v", err)
			}
		})
		t.Run("stat", func(t *testing.T) {
			testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, failure })
			err := restoreSkillSetupTarget(nil, []skillSetupBackedUpDir{{original: "original", backup: "backup"}})
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), "检查 Skill 恢复目标失败") {
				t.Fatalf("restore stat error = %v", err)
			}
		})
		t.Run("mkdir", func(t *testing.T) {
			testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
			testseam.Swap(t, &skillSetupMkdirAll, func(string, os.FileMode) error { return failure })
			err := restoreSkillSetupTarget(nil, []skillSetupBackedUpDir{{original: "original", backup: "backup"}})
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), "创建 Skill 恢复目录失败") {
				t.Fatalf("restore mkdir error = %v", err)
			}
		})
		t.Run("rename", func(t *testing.T) {
			testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
			testseam.Swap(t, &skillSetupMkdirAll, func(string, os.FileMode) error { return nil })
			testseam.Swap(t, &skillSetupPublishRename, func(string, string) error { return failure })
			err := restoreSkillSetupTarget(nil, []skillSetupBackedUpDir{{original: "original", backup: "backup"}})
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), "恢复原 Skill 失败") {
				t.Fatalf("restore rename error = %v", err)
			}
		})
	})

	t.Run("backup rollback failure", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &skillSetupBackupAndRemove, func(string, string) (string, error) {
			calls++
			if calls == 1 {
				return "backup", nil
			}
			return "", failure
		})
		testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
		testseam.Swap(t, &skillSetupMkdirAll, func(string, os.FileMode) error { return nil })
		restoreErr := errors.New("restore failure")
		testseam.Swap(t, &skillSetupPublishRename, func(string, string) error { return restoreErr })
		_, err := backupSkillSetupTarget("home", []skillSetupBackup{{Path: "first"}, {Path: "second"}}, io.Discard)
		if !errors.Is(err, failure) || !errors.Is(err, restoreErr) {
			t.Fatalf("backup rollback error = %v", err)
		}
	})

	t.Run("publish rollback failure", func(t *testing.T) {
		testseam.Swap(t, &skillSetupPublishRename, func(string, string) error { return failure })
		testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
		testseam.Swap(t, &skillSetupMkdirAll, func(string, os.FileMode) error { return nil })
		err := publishSkillSetupTarget(
			[]skillSetupStagedDir{{staged: "staged", dest: "dest"}},
			[]skillSetupBackedUpDir{{original: "dest", backup: "backup"}},
		)
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "回滚不完整") {
			t.Fatalf("publish rollback error = %v", err)
		}
	})

	t.Run("execute cleanup errors", func(t *testing.T) {
		newPlan := func(t *testing.T) *skillSetupPlan {
			t.Helper()
			src := writeMultiSkillSource(t, []string{"dingtalk-a"})
			return &skillSetupPlan{
				Mode:            skillSetupModeMulti,
				Source:          src,
				MultiSkillNames: []string{"dingtalk-a"},
				Targets:         []skillSetupTargetPlan{{Destination: t.TempDir()}},
			}
		}

		t.Run("after backup failure", func(t *testing.T) {
			plan := newPlan(t)
			plan.Targets[0].Backups = []skillSetupBackup{{Path: filepath.Join(plan.Targets[0].Destination, "old")}}
			testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return t.TempDir(), nil })
			testseam.Swap(t, &skillSetupBackupAndRemove, func(string, string) (string, error) { return "", failure })
			cleanupErr := errors.New("cleanup after backup failure")
			testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return cleanupErr })
			var stderr bytes.Buffer
			_, skipped, err := executeSkillSetupPlan(plan, io.Discard, &stderr)
			if err != nil || skipped != 1 || !strings.Contains(stderr.String(), cleanupErr.Error()) {
				t.Fatalf("backup cleanup = skipped %d, err %v, stderr %q", skipped, err, stderr.String())
			}
		})

		t.Run("after publish failure", func(t *testing.T) {
			plan := newPlan(t)
			originalRename := skillSetupPublishRename
			testseam.Swap(t, &skillSetupPublishRename, func(oldPath, newPath string) error {
				if strings.HasPrefix(filepath.Base(filepath.Dir(oldPath)), ".dws-setup-set-") {
					return failure
				}
				return originalRename(oldPath, newPath)
			})
			originalRemoveAll := skillSetupRemoveAll
			cleanupErr := errors.New("cleanup after publish failure")
			testseam.Swap(t, &skillSetupRemoveAll, func(path string) error {
				if strings.HasPrefix(filepath.Base(path), ".dws-setup-set-") {
					return cleanupErr
				}
				return originalRemoveAll(path)
			})
			var stderr bytes.Buffer
			_, skipped, err := executeSkillSetupPlan(plan, io.Discard, &stderr)
			if err != nil || skipped != 1 || !strings.Contains(stderr.String(), cleanupErr.Error()) {
				t.Fatalf("publish cleanup = skipped %d, err %v, stderr %q", skipped, err, stderr.String())
			}
		})

		t.Run("after success", func(t *testing.T) {
			plan := newPlan(t)
			originalRemoveAll := skillSetupRemoveAll
			cleanupErr := errors.New("cleanup after success")
			testseam.Swap(t, &skillSetupRemoveAll, func(path string) error {
				if strings.HasPrefix(filepath.Base(path), ".dws-setup-set-") {
					return cleanupErr
				}
				return originalRemoveAll(path)
			})
			var stderr bytes.Buffer
			installed, skipped, err := executeSkillSetupPlan(plan, io.Discard, &stderr)
			if err != nil || installed != 1 || skipped != 0 || !strings.Contains(stderr.String(), cleanupErr.Error()) {
				t.Fatalf("success cleanup = installed %d, skipped %d, err %v, stderr %q", installed, skipped, err, stderr.String())
			}
		})
	})
}

// TestCrossPlatformCoverageSkillSetupInstallHomeFailureSkips verifies both
// install paths skip (never destroy) every target when $HOME cannot be
// resolved for the pre-refresh backup.
func TestCrossPlatformCoverageSkillSetupInstallHomeFailureSkips(t *testing.T) {
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return "", errors.New("home boom") })

	monoSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(monoSrc, "SKILL.md"), []byte("# mono"), 0o644); err != nil {
		t.Fatal(err)
	}
	monoDest := filepath.Join(t.TempDir(), "agent", "dws")
	if err := os.MkdirAll(monoDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monoDest, "SKILL.md"), []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	installed, skipped, err := installSkillToHomes(monoSrc, []string{monoDest}, &out, &errOut)
	if err != nil || installed != 0 || skipped != 1 {
		t.Fatalf("mono install = (%d, %d, %v), want (0, 1, nil)", installed, skipped, err)
	}
	if !strings.Contains(errOut.String(), "无法解析 HOME，跳过刷新") {
		t.Fatalf("expected HOME skip warning, got %q", errOut.String())
	}
	if _, err := os.Stat(filepath.Join(monoDest, "SKILL.md")); err != nil {
		t.Fatalf("existing mono dir must be preserved: %v", err)
	}

	multiSrc := writeMultiSkillSource(t, []string{"dingtalk-a"})
	multiDest := filepath.Join(t.TempDir(), ".claude", "skills")
	if err := os.MkdirAll(filepath.Join(multiDest, "dingtalk-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	installed, skipped, err = installMultiSkillToHomes(multiSrc, []string{"dingtalk-a"}, []string{multiDest}, &out, &errOut, true)
	if err != nil || installed != 0 || skipped != 1 {
		t.Fatalf("multi install = (%d, %d, %v), want (0, 1, nil)", installed, skipped, err)
	}
	if !strings.Contains(errOut.String(), "无法解析 HOME，跳过整个 Agent 目标") {
		t.Fatalf("expected multi HOME skip warning, got %q", errOut.String())
	}
	if _, err := os.Stat(filepath.Join(multiDest, "dingtalk-a")); err != nil {
		t.Fatalf("existing sub skill must be preserved: %v", err)
	}
}

// TestCrossPlatformCoverageSkillSetupRemoveStaleMultiSkillsEdges covers
// removeStaleMultiSkills and its preview companion staleMultiSkillVictims:
// scan failures, the HOME failure, backup failures, and the success path.
func TestCrossPlatformCoverageSkillSetupRemoveStaleMultiSkillsEdges(t *testing.T) {
	dest := filepath.Join(t.TempDir(), ".cursor", "skills")
	keep := []string{"dingtalk-chat"}
	entries := map[string]bool{ // dir entries; README below is a plain file
		"dingtalk-chat":  true, // kept (in bundle)
		"dingtalk-stale": true, // stale product skill
		"dws-shared":     true, // legacy shared name is stale too
		"other-skill":    true, // non-DWS, must survive
	}
	for name := range entries {
		if err := os.MkdirAll(filepath.Join(dest, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	useManagedSkillNames(t, "dingtalk-stale")
	if err := os.WriteFile(filepath.Join(dest, "README"), []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer

	// Non-ENOENT scan failure warns; ENOENT is silent.
	testseam.Swap(t, &skillSetupReadDir, func(string) ([]os.DirEntry, error) { return nil, errors.New("scan boom") })
	removeStaleMultiSkills(dest, keep, &out, &errOut)
	if !strings.Contains(errOut.String(), "过期 skill 扫描失败") {
		t.Fatalf("expected scan warning, got %q", errOut.String())
	}
	errOut.Reset()
	testseam.Swap(t, &skillSetupReadDir, func(string) ([]os.DirEntry, error) { return nil, os.ErrNotExist })
	removeStaleMultiSkills(dest, keep, &out, &errOut)
	if errOut.Len() != 0 {
		t.Fatalf("ENOENT scan must be silent, got %q", errOut.String())
	}
	testseam.Swap(t, &skillSetupReadDir, os.ReadDir)

	// The preview companion sees the same victims and skips files/kept/non-DWS.
	victims := staleMultiSkillVictims(dest, keep)
	wantVictims := []string{filepath.Join(dest, "dingtalk-stale"), filepath.Join(dest, "dws-shared")}
	if len(victims) != len(wantVictims) {
		t.Fatalf("staleMultiSkillVictims = %v, want %v", victims, wantVictims)
	}

	// HOME failure keeps every stale directory with a warning.
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return "", errors.New("home boom") })
	errOut.Reset()
	removeStaleMultiSkills(dest, keep, &out, &errOut)
	if !strings.Contains(errOut.String(), "无法解析 HOME，跳过删除") {
		t.Fatalf("expected HOME warning, got %q", errOut.String())
	}
	for name := range entries {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Fatalf("entry %s must survive the HOME failure: %v", name, err)
		}
	}
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return t.TempDir(), nil })

	// Backup failure keeps the stale directory with a warning.
	testseam.Swap(t, &skillSetupBackupAndRemove, func(string, string) (string, error) { return "", errors.New("backup boom") })
	errOut.Reset()
	removeStaleMultiSkills(dest, keep, &out, &errOut)
	if !strings.Contains(errOut.String(), "过期 skill 清理失败（保留原目录") {
		t.Fatalf("expected backup failure warning, got %q", errOut.String())
	}
	for _, stale := range wantVictims {
		if _, err := os.Stat(stale); err != nil {
			t.Fatalf("stale dir must survive the backup failure: %v", err)
		}
	}

	// Success: both stale dirs are backed up and reported; the rest survives.
	testseam.Swap(t, &skillSetupBackupAndRemove, func(_, dir string) (string, error) { return filepath.Join(t.TempDir(), "backup"), nil })
	out.Reset()
	removeStaleMultiSkills(dest, keep, &out, &errOut)
	if count := strings.Count(out.String(), "已备份并清理过期 skill"); count != len(wantVictims) {
		t.Fatalf("expected %d stale cleanup lines, got %d (out=%q)", len(wantVictims), count, out.String())
	}
	if _, err := os.Stat(filepath.Join(dest, "other-skill")); err != nil {
		t.Fatalf("non-DWS dir must survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "dingtalk-chat")); err != nil {
		t.Fatalf("bundle skill must survive: %v", err)
	}
}
