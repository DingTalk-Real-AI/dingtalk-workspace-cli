package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/charmbracelet/huh"
)

func TestCrossPlatformCoverageSkillSetupPlanPreviewDeclineAndExecutionMatch(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".claude", "skills")
	source := writeMultiSkillSource(t, []string{"dingtalk-a", "dingtalk-shared"})
	for _, name := range []string{"dws", "dingtalk-a", "dingtalk-stale"} {
		if err := os.MkdirAll(filepath.Join(dest, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	useManagedSkillNames(t, "dingtalk-stale")

	testseam.Swap(t, &skillSetupResolveMode, func(mode string, _ bool, _ io.Writer) (string, error) { return mode, nil })
	testseam.Swap(t, &skillSetupResolveSource, func(string, string) (string, func(), error) { return source, func() {}, nil })
	testseam.Swap(t, &skillSetupResolveTargets, func(string, string) ([]string, error) { return []string{dest}, nil })
	testseam.Swap(t, &skillSetupListMulti, func(string) ([]string, error) {
		return []string{"dingtalk-a", "dingtalk-shared"}, nil
	})
	testseam.Swap(t, &skillSetupFilterMulti, filterMultiSkillNames)
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupInteractive, func() bool { return true })
	testseam.Swap(t, &skillSetupRunForm, func(*huh.Form) error { return nil })
	testseam.Swap(t, &skillSetupWriteState, func(string, skillstate.State) error { return nil })

	wantBackups := []string{
		filepath.Join(dest, "dingtalk-a"),
		filepath.Join(dest, "dingtalk-stale"),
		filepath.Join(dest, "dws"),
	}

	// Dry-run must disclose every exact path and perform no backup or copy.
	backupCalls, copyCalls := []string{}, 0
	testseam.Swap(t, &skillSetupBackupAndRemove, func(_ string, path string) (string, error) {
		backupCalls = append(backupCalls, path)
		return "backup", nil
	})
	testseam.Swap(t, &skillSetupCopyDir, func(string, string) error { copyCalls++; return nil })
	testseam.Swap(t, &skillSetupWriteFile, func(string, []byte, os.FileMode) error { return nil })
	testseam.Swap(t, &skillSetupPublishRename, func(src, dest string) error {
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
		return os.Rename(src, dest)
	})
	dryRunCmd := skillSetupCoverageCommand(t, skillSetupModeMulti, false)
	var dryRunOut bytes.Buffer
	dryRunCmd.SetOut(&dryRunOut)
	if err := dryRunCmd.Root().PersistentFlags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	if err := dryRunCmd.RunE(dryRunCmd, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range wantBackups {
		if strings.Count(dryRunOut.String(), path) != 1 {
			t.Fatalf("dry-run path %s count != 1:\n%s", path, dryRunOut.String())
		}
	}
	if len(backupCalls) != 0 || copyCalls != 0 {
		t.Fatalf("dry-run mutated backup=%v copy=%d", backupCalls, copyCalls)
	}

	// The real confirmation renderer discloses the same paths. Its default
	// negative answer must leave backup and copy at zero calls.
	declineCmd := skillSetupCoverageCommand(t, skillSetupModeMulti, false)
	var declineOut bytes.Buffer
	declineCmd.SetOut(&declineOut)
	if err := declineCmd.RunE(declineCmd, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range wantBackups {
		if strings.Count(declineOut.String(), path) != 1 {
			t.Fatalf("confirmation path %s count != 1:\n%s", path, declineOut.String())
		}
	}
	if len(backupCalls) != 0 || copyCalls != 0 {
		t.Fatalf("declined confirmation mutated backup=%v copy=%d", backupCalls, copyCalls)
	}

	// Explicit confirmation executes exactly the paths rendered from the plan.
	var confirmedPlan *skillSetupPlan
	testseam.Swap(t, &skillSetupConfirmPlan, func(out io.Writer, plan *skillSetupPlan) (bool, error) {
		confirmedPlan = plan
		renderSkillSetupPlan(out, plan)
		return true, nil
	})
	confirmCmd := skillSetupCoverageCommand(t, skillSetupModeMulti, false)
	if err := confirmCmd.RunE(confirmCmd, nil); err != nil {
		t.Fatal(err)
	}
	var planned []string
	for _, target := range confirmedPlan.Targets {
		for _, backup := range target.Backups {
			planned = append(planned, backup.Path)
		}
	}
	if !reflect.DeepEqual(planned, wantBackups) || !reflect.DeepEqual(backupCalls, wantBackups) {
		t.Fatalf("planned=%v executed=%v want=%v", planned, backupCalls, wantBackups)
	}
	if copyCalls != 2 {
		t.Fatalf("copy calls = %d, want 2", copyCalls)
	}

	// A filtered multi plan replaces only selected same-name skills and leaves
	// unselected siblings out of the backup set.
	filtered, err := buildSkillSetupPlan(skillSetupModeMulti, source, []string{dest}, []string{"dingtalk-a"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var filteredPaths []string
	for _, backup := range filtered.Targets[0].Backups {
		filteredPaths = append(filteredPaths, backup.Path)
	}
	if !reflect.DeepEqual(filteredPaths, []string{filepath.Join(dest, "dingtalk-a"), filepath.Join(dest, "dws")}) {
		t.Fatalf("filtered backups = %v", filteredPaths)
	}
}

func TestCrossPlatformCoverageSkillSetupMonoPlanIncludesSameNameTarget(t *testing.T) {
	dest := filepath.Join(t.TempDir(), ".agents", "skills", "dws")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := buildSkillSetupPlan(skillSetupModeMono, "source", []string{dest}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 || len(plan.Targets[0].Backups) != 1 || plan.Targets[0].Backups[0].Path != dest || plan.Targets[0].Backups[0].Reason != skillSetupBackupReplace {
		t.Fatalf("mono plan = %#v", plan)
	}
}

func TestCrossPlatformCoverageSkillSetupGenericCleanupDerivesHomeFromConcreteTarget(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".codex", "skills")
	genericMono := filepath.Join(home, ".agents", "skills", "dws")
	if err := os.MkdirAll(genericMono, 0o755); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) {
		return "", errors.New("transient HOME failure")
	})

	plan, err := buildSkillSetupPlan(skillSetupModeMulti, "source", []string{dest}, []string{"dingtalk-chat"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 2 || !plan.Targets[1].CleanupOnly || plan.Targets[1].Destination != filepath.Dir(genericMono) {
		t.Fatalf("generic cleanup target = %#v", plan.Targets)
	}
	if len(plan.Targets[1].Backups) != 1 || plan.Targets[1].Backups[0].Path != genericMono {
		t.Fatalf("generic cleanup backups = %#v", plan.Targets[1].Backups)
	}
	var preview bytes.Buffer
	renderSkillSetupPlan(&preview, plan)
	if !strings.Contains(preview.String(), "仅迁移旧的通用 DWS 副本") {
		t.Fatalf("generic cleanup preview missing: %s", preview.String())
	}

	t.Run("managed multi and scan failure", func(t *testing.T) {
		managedDir := filepath.Join(home, ".agents", "skills", "dingtalk-chat")
		if err := os.MkdirAll(managedDir, 0o755); err != nil {
			t.Fatal(err)
		}
		target, targetErr := genericSkillCleanupTarget([]string{dest}, map[string]bool{"dingtalk-chat": true})
		if targetErr != nil || target == nil || len(target.Backups) != 2 {
			t.Fatalf("managed generic cleanup = %#v, %v", target, targetErr)
		}
		failure := errors.New("generic scan failure")
		testseam.Swap(t, &skillSetupReadDir, func(string) ([]os.DirEntry, error) { return nil, failure })
		if _, targetErr := genericSkillCleanupTarget([]string{dest}, nil); !errors.Is(targetErr, failure) {
			t.Fatalf("generic scan error = %v", targetErr)
		}
		if _, planErr := buildSkillSetupPlan(skillSetupModeMulti, "source", []string{dest}, []string{"dingtalk-chat"}, true); !errors.Is(planErr, failure) {
			t.Fatalf("generic cleanup plan error = %v", planErr)
		}
	})
}

func TestCrossPlatformCoverageSkillSetupCleanupOnlyExecutionBranches(t *testing.T) {
	failure := errors.New("cleanup failure")
	cleanup := skillSetupTargetPlan{Destination: "generic", CleanupOnly: true, Backups: []skillSetupBackup{{Path: "old"}}}

	t.Run("prior skip suppresses cleanup", func(t *testing.T) {
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return t.TempDir(), nil })
		plan := &skillSetupPlan{Mode: skillSetupModeMono, Source: "missing", Targets: []skillSetupTargetPlan{{Destination: "install"}, cleanup}}
		installed, skipped, err := executeSkillSetupPlan(plan, io.Discard, io.Discard)
		if err != nil || installed != 0 || skipped != 1 {
			t.Fatalf("cleanup after skip = (%d, %d, %v)", installed, skipped, err)
		}
	})

	t.Run("home failure keeps generic copy", func(t *testing.T) {
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return "", failure })
		var stderr bytes.Buffer
		_, skipped, err := executeSkillSetupPlan(&skillSetupPlan{Mode: skillSetupModeMono, Targets: []skillSetupTargetPlan{cleanup}}, io.Discard, &stderr)
		if err != nil || skipped != 1 || !strings.Contains(stderr.String(), "保留通用 Skill 副本") {
			t.Fatalf("cleanup HOME failure = (%d, %v, %q)", skipped, err, stderr.String())
		}
	})

	t.Run("backup failure is reported", func(t *testing.T) {
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return t.TempDir(), nil })
		testseam.Swap(t, &skillSetupBackupAndRemove, func(string, string) (string, error) { return "", failure })
		var stderr bytes.Buffer
		_, skipped, err := executeSkillSetupPlan(&skillSetupPlan{Mode: skillSetupModeMono, Targets: []skillSetupTargetPlan{cleanup}}, io.Discard, &stderr)
		if err != nil || skipped != 1 || !strings.Contains(stderr.String(), "迁移失败") {
			t.Fatalf("cleanup backup failure = (%d, %v, %q)", skipped, err, stderr.String())
		}
	})
}

func TestCrossPlatformCoverageSkillSetupPlanDeduplicatesAndFailsClosed(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(filepath.Join(dest, "dws"), 0o755); err != nil {
		t.Fatal(err)
	}
	// "dws" is synthetic but makes the mutual-exclusion target and selected
	// same-name target overlap, pinning path deduplication in the plan itself.
	plan, err := buildSkillSetupPlan(skillSetupModeMulti, "source", []string{dest}, []string{"dws"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets[0].Backups) != 1 || plan.Targets[0].Backups[0].Path != filepath.Join(dest, "dws") {
		t.Fatalf("deduplicated plan = %#v", plan)
	}

	failure := errors.New("scan denied")
	monoDest := filepath.Join(t.TempDir(), "agent", "dws")
	if err := os.MkdirAll(filepath.Dir(monoDest), 0o755); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, failure })
	if _, err := buildSkillSetupPlan(skillSetupModeMono, "source", []string{monoDest}, nil, false); err == nil || !strings.Contains(err.Error(), "\u68c0\u67e5\u5c06\u88ab\u66ff\u6362") {
		t.Fatalf("replacement stat error = %v", err)
	}

	testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
	testseam.Swap(t, &skillSetupReadDir, func(string) ([]os.DirEntry, error) { return nil, failure })
	if _, err := buildSkillSetupPlan(skillSetupModeMulti, "source", []string{dest}, []string{"dingtalk-a"}, false); err == nil || !strings.Contains(err.Error(), "\u626b\u63cf\u8fc7\u671f") {
		t.Fatalf("stale scan error = %v", err)
	}
}

func TestCrossPlatformCoverageSkillSetupInstallWrappersFailOnPlanErrors(t *testing.T) {
	t.Run("multi mono-leftover stat failure", func(t *testing.T) {
		failure := errors.New("stat denied")
		testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) {
			return nil, failure
		})

		installed, skipped, err := installMultiSkillToHomes(
			"source",
			[]string{"dingtalk-a"},
			[]string{filepath.Join(t.TempDir(), "skills")},
			io.Discard,
			io.Discard,
			true,
		)
		if installed != 0 || skipped != 1 || !errors.Is(err, failure) {
			t.Fatalf("installMultiSkillToHomes = (%d, %d, %v), want (0, 1, %v)", installed, skipped, err, failure)
		}
	})

	t.Run("mono multi-leftover scan failure", func(t *testing.T) {
		failure := errors.New("scan denied")
		testseam.Swap(t, &skillSetupReadDir, func(string) ([]os.DirEntry, error) {
			return nil, failure
		})

		installed, skipped, err := installSkillToHomes(
			"source",
			[]string{filepath.Join(t.TempDir(), "skills", "dws")},
			io.Discard,
			io.Discard,
		)
		if installed != 0 || skipped != 1 || !errors.Is(err, failure) {
			t.Fatalf("installSkillToHomes = (%d, %d, %v), want (0, 1, %v)", installed, skipped, err, failure)
		}
	})
}

func TestCrossPlatformCoverageSkillSetupMergedEventPlanEdges(t *testing.T) {
	t.Run("legacy shared stat failure", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "skills")
		failure := errors.New("legacy stat denied")
		testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(dest, legacySharedSkill) {
				return nil, failure
			}
			return nil, os.ErrNotExist
		})

		_, err := buildSkillSetupPlan(
			skillSetupModeMulti,
			"source",
			[]string{dest},
			[]string{multiSharedSkill},
			true,
		)
		if !errors.Is(err, failure) {
			t.Fatalf("legacy shared stat error = %v, want %v", err, failure)
		}
	})

	t.Run("migration plan retains unrelated backups", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "skills")
		unrelated := filepath.Join(dest, "dws")
		plan := &skillSetupPlan{Targets: []skillSetupTargetPlan{
			{
				Destination: dest,
				Backups: []skillSetupBackup{
					{Path: filepath.Join(dest, multiEventSkill)},
					{Path: filepath.Join(dest, multiMiscSkill)},
					{Path: unrelated},
				},
			},
		}}

		configureEventMiscMigrationPlan(plan, []string{dest}, true)
		if len(plan.Targets[0].Backups) != 1 || plan.Targets[0].Backups[0].Path != unrelated {
			t.Fatalf("migration backups = %#v, want only %s", plan.Targets[0].Backups, unrelated)
		}
	})

	t.Run("no migration targets delegates filtered install", func(t *testing.T) {
		called := false
		testseam.Swap(t, &skillSetupInstallMulti, func(_ string, _ []string, _ []string, _ io.Writer, _ io.Writer, filtered bool) (int, int, error) {
			called = true
			if !filtered {
				t.Fatal("filtered flag was not forwarded")
			}
			return 1, 2, nil
		})

		installed, skipped, err := installMultiSkillsWithEventMigration(
			"source", []string{"dingtalk-a"}, []string{"dest"}, nil, true, io.Discard, io.Discard,
		)
		if err != nil || installed != 1 || skipped != 2 || !called {
			t.Fatalf("delegated install = (%d, %d, %v), called=%v", installed, skipped, err, called)
		}
	})

	t.Run("migration cleanup scan failure", func(t *testing.T) {
		failure := errors.New("cleanup stat denied")
		testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) {
			return nil, failure
		})
		dest := filepath.Join(t.TempDir(), "skills")

		installed, skipped, err := installMultiSkillsWithEventMigration(
			"source",
			[]string{multiEventSkill, multiSharedSkill},
			[]string{dest},
			[]string{dest},
			true,
			io.Discard,
			io.Discard,
		)
		if installed != 0 || skipped != 2 || !errors.Is(err, failure) {
			t.Fatalf("cleanup scan failure = (%d, %d, %v), want (0, 2, %v)", installed, skipped, err, failure)
		}
	})
}

func TestCrossPlatformCoverageSkillSetupEmptyCleanupPlansAreNoOps(t *testing.T) {
	dest := t.TempDir()
	var out, errOut bytes.Buffer
	if err := cleanupMutualExclusion(dest, skillSetupModeMulti, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := removeStaleMultiSkills(dest, []string{"dingtalk-a"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("empty cleanup output = %q / %q", out.String(), errOut.String())
	}
}

func TestCrossPlatformCoverageSkillSetupSameNameBackupFailureSkipsTarget(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".agents", "skills")
	plan := &skillSetupPlan{
		Mode:            skillSetupModeMulti,
		Source:          "source",
		MultiSkillNames: []string{"dingtalk-a"},
		Targets: []skillSetupTargetPlan{{
			Destination: dest,
			Backups: []skillSetupBackup{{
				Path:   filepath.Join(dest, "dingtalk-a"),
				Reason: skillSetupBackupReplace,
			}},
		}},
	}
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupBackupAndRemove, func(string, string) (string, error) {
		return "", errors.New("backup denied")
	})
	copyCalls := 0
	testseam.Swap(t, &skillSetupCopyDir, func(string, string) error { copyCalls++; return nil })
	var out, errOut bytes.Buffer
	installed, skipped, err := executeSkillSetupPlan(plan, &out, &errOut)
	if err != nil || installed != 0 || skipped != 1 || copyCalls != 1 {
		t.Fatalf("same-name failure = (%d, %d, %v), copy=%d", installed, skipped, err, copyCalls)
	}
	if !strings.Contains(errOut.String(), "Skill 备份失败，已执行回滚，跳过整个 Agent 目标") {
		t.Fatalf("same-name warning = %q", errOut.String())
	}
}
