package app

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type skillSetupFileInfo struct {
	name string
	mode os.FileMode
}

func (i skillSetupFileInfo) Name() string       { return i.name }
func (i skillSetupFileInfo) Size() int64        { return 0 }
func (i skillSetupFileInfo) Mode() os.FileMode  { return i.mode }
func (i skillSetupFileInfo) ModTime() time.Time { return time.Time{} }
func (i skillSetupFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i skillSetupFileInfo) Sys() any           { return nil }

func skillSetupCoverageCommand(t *testing.T, mode string, yes bool) *cobra.Command {
	t.Helper()
	cmd := newSkillSetupCommand()
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(cmd)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	_ = cmd.Flags().Set("mode", mode)
	_ = cmd.Flags().Set("yes", map[bool]string{true: "true", false: "false"}[yes])
	return cmd
}

func TestCrossPlatformCoverageSkillSetupHighLevelRemainingCoverage(t *testing.T) {
	oldMode := skillSetupResolveMode
	oldSource := skillSetupResolveSource
	oldTargets := skillSetupResolveTargets
	oldList := skillSetupListMulti
	oldFilter := skillSetupFilterMulti
	oldConfirm := skillSetupConfirmPlan
	oldExecute := skillSetupExecutePlan
	t.Cleanup(func() {
		skillSetupResolveMode = oldMode
		skillSetupResolveSource = oldSource
		skillSetupResolveTargets = oldTargets
		skillSetupListMulti = oldList
		skillSetupFilterMulti = oldFilter
		skillSetupConfirmPlan = oldConfirm
		skillSetupExecutePlan = oldExecute
	})
	fail := errors.New("failure")
	skillSetupResolveMode = func(mode string, _ bool, _ io.Writer) (string, error) { return mode, nil }
	skillSetupResolveSource = func(string, string) (string, func(), error) { return "source", func() {}, nil }
	skillSetupResolveTargets = func(string, string) ([]string, error) { return []string{filepath.Join(t.TempDir(), "dest")}, nil }
	skillSetupFilterMulti = func(all, _, _ []string) ([]string, error) { return all, nil }

	skillSetupListMulti = func(string) ([]string, error) { return nil, fail }
	if err := skillSetupCoverageCommand(t, skillSetupModeMulti, true).RunE(skillSetupCoverageCommand(t, skillSetupModeMulti, true), nil); err == nil {
		t.Fatal("multi list failure should propagate")
	}
	skillSetupListMulti = func(string) ([]string, error) { return nil, nil }
	cmd := skillSetupCoverageCommand(t, skillSetupModeMulti, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("empty multi source should fail")
	}
	skillSetupListMulti = func(string) ([]string, error) { return []string{"dingtalk-shared", "dingtalk-doc"}, nil }
	skillSetupFilterMulti = func([]string, []string, []string) ([]string, error) { return nil, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMulti, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("multi filter failure should propagate")
	}
	skillSetupFilterMulti = func(all, _, _ []string) ([]string, error) { return all, nil }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMulti, true)
	cmd.Flags().Bool("dry-run", true, "")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	skillSetupConfirmPlan = func(io.Writer, *skillSetupPlan) (bool, error) { return false, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, false)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("confirmation failure should propagate")
	}
	skillSetupConfirmPlan = func(io.Writer, *skillSetupPlan) (bool, error) { return false, nil }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, false)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	skillSetupResolveMode = func(string, bool, io.Writer) (string, error) { return "unknown", nil }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("unknown resolved mode should fail")
	}
	skillSetupResolveMode = func(mode string, _ bool, _ io.Writer) (string, error) { return mode, nil }
	skillSetupExecutePlan = func(*skillSetupPlan, io.Writer, io.Writer) (int, int, error) { return 0, 0, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("mono install failure should propagate")
	}
	skillSetupExecutePlan = func(*skillSetupPlan, io.Writer, io.Writer) (int, int, error) { return 1, 0, nil }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, true)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	skillSetupExecutePlan = func(*skillSetupPlan, io.Writer, io.Writer) (int, int, error) { return 0, 0, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMulti, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("multi install failure should propagate")
	}
}

func TestCrossPlatformCoverageSkillSetupMigratesLegacySharedAfterReplacement(t *testing.T) {
	setTestHome(t, t.TempDir())
	src := writeMultiSkillSource(t, []string{multiSharedSkill, "dingtalk-chat"})
	home := filepath.Join(t.TempDir(), "skills")
	legacyPath := filepath.Join(home, legacyMultiSharedSkill)
	customPath := filepath.Join(home, "custom-skill")
	for _, path := range []string{legacyPath, customPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("legacy or custom\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out, errOut bytes.Buffer
	installed, skipped, err := installMultiSkillToHomes(
		src,
		[]string{multiSharedSkill, "dingtalk-chat"},
		[]string{home},
		&out,
		&errOut,
		true,
	)
	if err != nil || installed != 2 || skipped != 0 {
		t.Fatalf("install = %d/%d, err=%v, stderr=%s", installed, skipped, err, errOut.String())
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy shared skill still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, multiSharedSkill, "SKILL.md")); err != nil {
		t.Fatalf("replacement shared skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customPath, "SKILL.md")); err != nil {
		t.Fatalf("unrelated custom skill changed: %v", err)
	}
	if !strings.Contains(out.String(), "已备份并清理过期 skill") {
		t.Fatalf("legacy cleanup was not reported: %s", out.String())
	}

	t.Run("failed replacement preserves legacy", func(t *testing.T) {
		missingSource := t.TempDir()
		failureHome := filepath.Join(t.TempDir(), "skills")
		failureLegacy := filepath.Join(failureHome, legacyMultiSharedSkill)
		if err := os.MkdirAll(failureLegacy, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(failureLegacy, "SKILL.md"), []byte("legacy\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		var failureOut, failureErr bytes.Buffer
		installed, skipped, err := installMultiSkillToHomes(
			missingSource,
			[]string{multiSharedSkill},
			[]string{failureHome},
			&failureOut,
			&failureErr,
			true,
		)
		if err != nil || installed != 0 || skipped != 1 {
			t.Fatalf("failed replacement = %d/%d, err=%v", installed, skipped, err)
		}
		got, readErr := os.ReadFile(filepath.Join(failureLegacy, "SKILL.md"))
		if readErr != nil || string(got) != "legacy\n" {
			t.Fatalf("failed replacement changed the live legacy copy: %q, err=%v", got, readErr)
		}
		if !strings.Contains(failureErr.String(), "Skill staging 失败，保留原集合") {
			t.Fatalf("failed replacement did not report preserved live set: %s", failureErr.String())
		}
	})
}

func TestCrossPlatformCoverageSkillSetupLegacySharedCleanupFailures(t *testing.T) {
	fail := errors.New("legacy cleanup failure")

	t.Run("missing legacy is a no-op", func(t *testing.T) {
		var out, errOut bytes.Buffer
		cleanupLegacyMultiSharedSkill(t.TempDir(), &out, &errOut)
		if out.Len() != 0 || errOut.Len() != 0 {
			t.Fatalf("missing legacy emitted output: stdout=%q stderr=%q", out.String(), errOut.String())
		}
	})

	t.Run("stat failure is reported", func(t *testing.T) {
		testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, fail })
		var out, errOut bytes.Buffer
		cleanupLegacyMultiSharedSkill("dest", &out, &errOut)
		if out.Len() != 0 || !strings.Contains(errOut.String(), "无法检查已退役 Skill 残留") {
			t.Fatalf("stat failure output: stdout=%q stderr=%q", out.String(), errOut.String())
		}
	})

	t.Run("remove failure is reported", func(t *testing.T) {
		testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
			return skillSetupFileInfo{name: filepath.Base(path), mode: os.ModeDir}, nil
		})
		testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return fail })
		var out, errOut bytes.Buffer
		cleanupLegacyMultiSharedSkill("dest", &out, &errOut)
		if out.Len() != 0 || !strings.Contains(errOut.String(), "已退役 Skill 清理失败") {
			t.Fatalf("remove failure output: stdout=%q stderr=%q", out.String(), errOut.String())
		}
	})
}

func TestCrossPlatformCoverageSkillSetupLowLevelRemainingCoverage(t *testing.T) {
	oldRunForm, oldInteractive := skillSetupRunForm, skillSetupInteractive
	oldReadDir, oldStat := skillSetupReadDir, skillSetupStat
	oldExecutable, oldGetwd, oldHome := skillSetupExecutable, skillSetupGetwd, skillSetupUserHomeDir
	oldRemove, oldMkdir := skillSetupRemoveAll, skillSetupMkdirAll
	oldBackup := skillSetupBackupAndRemove
	oldCopyDir, oldWalk, oldRel := skillSetupCopyDir, skillSetupWalk, skillSetupRel
	oldMkdirTemp, oldRename := skillSetupMkdirTemp, skillSetupRename
	oldReadlink, oldOpen, oldOpenFile, oldCopy := skillSetupReadlink, skillSetupOpen, skillSetupOpenFile, skillSetupCopy
	t.Cleanup(func() {
		skillSetupRunForm, skillSetupInteractive = oldRunForm, oldInteractive
		skillSetupReadDir, skillSetupStat = oldReadDir, oldStat
		skillSetupExecutable, skillSetupGetwd, skillSetupUserHomeDir = oldExecutable, oldGetwd, oldHome
		skillSetupRemoveAll, skillSetupMkdirAll = oldRemove, oldMkdir
		skillSetupBackupAndRemove = oldBackup
		skillSetupCopyDir, skillSetupWalk, skillSetupRel = oldCopyDir, oldWalk, oldRel
		skillSetupMkdirTemp, skillSetupRename = oldMkdirTemp, oldRename
		skillSetupReadlink, skillSetupOpen, skillSetupOpenFile, skillSetupCopy = oldReadlink, oldOpen, oldOpenFile, oldCopy
	})
	fail := errors.New("failure")

	skillSetupInteractive = func() bool { return true }
	skillSetupRunForm = func(*huh.Form) error { return fail }
	if _, err := resolveSkillSetupMode("", false, io.Discard); err == nil {
		t.Fatal("interactive mode failure should propagate")
	}
	skillSetupRunForm = func(*huh.Form) error { return nil }
	if got, err := resolveSkillSetupMode("", false, io.Discard); err != nil || got != skillSetupModeMulti {
		t.Fatalf("interactive default choice = %q, %v", got, err)
	}

	source := writeMultiSkillSource(t, []string{"dingtalk-doc"})
	if err := os.WriteFile(filepath.Join(source, "README"), []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := listMultiSkillNames(source); err != nil || len(got) != 1 {
		t.Fatalf("listed multi skills = %#v, %v", got, err)
	}
	t.Setenv("DWS_SKILL_SOURCE", source)
	if got, err := resolveSkillSetupSource("", skillSetupModeMulti); err != nil || got != source {
		t.Fatalf("environment skill source = %q, %v", got, err)
	}
	t.Setenv("DWS_SKILL_SOURCE", "")
	legacyRoot := t.TempDir()
	legacyMono := filepath.Join(legacyRoot, "skills", skillSetupModeMono)
	if err := os.MkdirAll(legacyMono, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyMono, "SKILL.md"), []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillSetupExecutable = func() (string, error) { return filepath.Join(legacyRoot, "dws"), nil }
	skillSetupStat = oldStat
	if got, err := resolveSkillSetupSource("", skillSetupModeMono); err != nil || got != legacyMono {
		t.Fatalf("legacy executable source = %q, %v", got, err)
	}
	t.Setenv("DWS_SKILL_SOURCE", "env-source")
	if got := skillSourceCandidates("", skillSetupModeMono); len(got) < 2 {
		t.Fatalf("environment source candidates = %#v", got)
	}
	t.Setenv("DWS_SKILL_SOURCE", "")
	skillSetupExecutable = func() (string, error) { return "", fail }
	skillSetupGetwd = func() (string, error) { return "", fail }
	skillSetupUserHomeDir = func() (string, error) { return "", fail }
	if _, err := resolveSkillSetupSource("", skillSetupModeMono); err == nil {
		t.Fatal("missing fallback source should fail")
	}
	_ = skillSourceCandidates("explicit", skillSetupModeMono)

	skillSetupReadDir = func(string) ([]os.DirEntry, error) { return nil, fail }
	if isSkillSourceRoot("missing", skillSetupModeMulti) {
		t.Fatal("unreadable multi source accepted")
	}
	skillSetupUserHomeDir = func() (string, error) { return "", fail }
	if _, err := resolveSkillSetupTargets("all", skillSetupModeMono); err == nil {
		t.Fatal("HOME failure should propagate")
	}

	monoDest := filepath.Join(t.TempDir(), "skills", "dws")
	multiRoot := filepath.Dir(monoDest)
	if err := os.MkdirAll(filepath.Join(multiRoot, "dingtalk-doc"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillSetupReadDir, skillSetupStat = oldReadDir, oldStat
	var out, errOut bytes.Buffer
	skillSetupRunForm = func(*huh.Form) error { return fail }
	if _, err := confirmSkillSetup(&out, skillSetupModeMulti, "src", []string{monoDest}, []string{"dingtalk-doc"}, false); err == nil {
		t.Fatal("confirmation form failure should propagate")
	}
	skillSetupRunForm = func(*huh.Form) error { return nil }
	if ok, err := confirmSkillSetup(&out, skillSetupModeMono, "src", []string{monoDest}, nil, false); err != nil || ok {
		t.Fatalf("EOF confirmation = %v, %v", ok, err)
	}
	skillSetupUserHomeDir = func() (string, error) { return t.TempDir(), nil }
	skillSetupBackupAndRemove = func(string, string) (string, error) { return "", fail }
	cleanupMutualExclusion(monoDest, skillSetupModeMono, &out, &errOut)

	skillSetupCopyDir = func(string, string) error { return fail }
	skillSetupBackupAndRemove = func(string, string) (string, error) { return "", fail }
	_, skipped, _ := installSkillToHomes("src", []string{"a"}, &out, &errOut)
	if skipped != 1 {
		t.Fatal("mono backup failure not skipped")
	}
	skillSetupBackupAndRemove = func(string, string) (string, error) { return "", nil }
	skillSetupMkdirAll = func(string, os.FileMode) error { return fail }
	_, skipped, _ = installSkillToHomes("src", []string{"b"}, &out, &errOut)
	if skipped != 1 {
		t.Fatal("mono mkdir failure not skipped")
	}
	skillSetupMkdirAll = func(string, os.FileMode) error { return nil }
	_, skipped, _ = installSkillToHomes("src", []string{"c"}, &out, &errOut)
	if skipped != 1 {
		t.Fatal("mono copy failure not skipped")
	}

	skillSetupMkdirAll = func(string, os.FileMode) error { return fail }
	_, skipped, _ = installMultiSkillToHomes("src", []string{"one", "two"}, []string{filepath.Join(t.TempDir(), "dest")}, &out, &errOut, true)
	if skipped != 2 {
		t.Fatal("multi mkdir failure count mismatch")
	}
	skillSetupMkdirAll = func(string, os.FileMode) error { return nil }
	skillSetupBackupAndRemove = func(string, string) (string, error) { return "", fail }
	_, skipped, _ = installMultiSkillToHomes("src", []string{"one"}, []string{filepath.Join(t.TempDir(), "dest")}, &out, &errOut, true)
	if skipped != 1 {
		t.Fatal("multi backup failure count mismatch")
	}
	skillSetupBackupAndRemove = func(string, string) (string, error) { return "", nil }
	_, skipped, _ = installMultiSkillToHomes("src", []string{"one"}, []string{filepath.Join(t.TempDir(), "dest")}, &out, &errOut, true)
	if skipped != 1 {
		t.Fatal("multi copy failure count mismatch")
	}

	skillSetupWalk = func(string, filepath.WalkFunc) error { return fail }
	if err := copyDir("src", "dst"); !errors.Is(err, fail) {
		t.Fatalf("walk failure = %v", err)
	}
	skillSetupWalk = func(_ string, fn filepath.WalkFunc) error {
		return fn("path", skillSetupFileInfo{name: "path"}, nil)
	}
	skillSetupRel = func(string, string) (string, error) { return "", fail }
	if err := copyDir("src", "dst"); !errors.Is(err, fail) {
		t.Fatalf("relative-path failure = %v", err)
	}
	skillSetupRel = func(string, string) (string, error) { return "file", nil }
	skillSetupWalk = func(_ string, fn filepath.WalkFunc) error {
		return fn("link", skillSetupFileInfo{name: "link", mode: os.ModeSymlink}, nil)
	}
	skillSetupReadlink = func(string) (string, error) { return "", fail }
	if err := copyDir("src", "dst"); !errors.Is(err, fail) {
		t.Fatalf("readlink failure = %v", err)
	}
	for _, target := range []string{"relative-target", "/absolute-target"} {
		skillSetupReadlink = func(string) (string, error) { return target, nil }
		_ = copyDir("src", "dst")
	}

	skillSetupMkdirAll = func(string, os.FileMode) error { return fail }
	if err := copyFileContent("src", "dst", 0o600); !errors.Is(err, fail) {
		t.Fatalf("copy mkdir failure = %v", err)
	}
	skillSetupMkdirAll = func(string, os.FileMode) error { return nil }
	skillSetupOpen = func(string) (*os.File, error) { return nil, fail }
	if err := copyFileContent("src", "dst", 0o600); !errors.Is(err, fail) {
		t.Fatalf("copy open failure = %v", err)
	}
	in, err := os.CreateTemp(t.TempDir(), "in")
	if err != nil {
		t.Fatal(err)
	}
	skillSetupOpen = func(string) (*os.File, error) { return in, nil }
	skillSetupOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, fail }
	if err := copyFileContent("src", "dst", 0o600); !errors.Is(err, fail) {
		t.Fatalf("copy output-open failure = %v", err)
	}
	outFile, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	skillSetupOpen = func(string) (*os.File, error) { return in, nil }
	skillSetupOpenFile = func(string, int, os.FileMode) (*os.File, error) { return outFile, nil }
	skillSetupCopy = func(io.Writer, io.Reader) (int64, error) { return 0, fail }
	if err := copyFileContent("src", "dst", 0o600); !errors.Is(err, fail) {
		t.Fatalf("copy content failure = %v", err)
	}

	closed, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if isCharDevice(closed) {
		t.Fatal("closed file is not a character device")
	}
	_ = fs.ValidPath("path")
}

func TestCrossPlatformCoverageSkillSetupEventMigrationFailureBranches(t *testing.T) {
	fail := errors.New("injected failure")
	validSkill := func(name string) []byte {
		return []byte("---\nname: " + name + "\ndescription: valid migration skill\n---\n\n# Skill\n")
	}

	t.Run("folded discovery rejects directory reference", func(t *testing.T) {
		dest := t.TempDir()
		miscRoot := filepath.Join(dest, multiMiscSkill)
		if err := os.MkdirAll(miscRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(miscRoot, "SKILL.md"), []byte("dws event\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
			if strings.HasSuffix(path, filepath.Join("references", "event.md")) {
				return skillSetupFileInfo{name: "event.md", mode: os.ModeDir}, nil
			}
			return os.Stat(path)
		})
		if got := findFoldedEventMiscTargets([]string{dest}); len(got) != 0 {
			t.Fatalf("directory event reference accepted: %#v", got)
		}
	})

	t.Run("migration root validation failures", func(t *testing.T) {
		if err := validateMigrationSkillRoot(filepath.Join(t.TempDir(), "missing"), multiEventSkill, nil); err == nil {
			t.Fatal("missing SKILL.md succeeded")
		}

		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "SKILL.md"), validSkill(multiEventSkill), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "references", "directory.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := validateMigrationSkillRoot(root, multiEventSkill, []string{filepath.Join("references", "directory.md")}); err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("directory required file = %v", err)
		}

		missing := filepath.Join("references", "missing.md")
		testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
			if path == filepath.Join(root, missing) {
				return skillSetupFileInfo{name: "missing.md"}, nil
			}
			return os.Stat(path)
		})
		if err := validateMigrationSkillRoot(root, multiEventSkill, []string{missing}); err == nil || !strings.Contains(err.Error(), "无法读取") {
			t.Fatalf("unreadable required file = %v", err)
		}
	})

	t.Run("frontmatter validation branches", func(t *testing.T) {
		validWithIgnoredLine := []byte("---\nignored line\nname: dingtalk-event\ndescription: valid\n---\n\nbody\n")
		if name, err := parseMigrationSkillFrontmatter(validWithIgnoredLine); err != nil || name != multiEventSkill {
			t.Fatalf("ignored frontmatter line = %q, %v", name, err)
		}
		for name, body := range map[string][]byte{
			"duplicate name": []byte("---\nname: one\nname: two\ndescription: valid\n---\nbody\n"),
			"unclosed":       []byte("---\nname: one\ndescription: valid\nbody\n"),
			"missing name":   []byte("---\ndescription: valid\n---\nbody\n"),
			"missing desc":   []byte("---\nname: one\n---\nbody\n"),
			"empty body":     []byte("---\nname: one\ndescription: valid\n---\n  \n"),
		} {
			t.Run(name, func(t *testing.T) {
				if _, err := parseMigrationSkillFrontmatter(body); err == nil {
					t.Fatal("invalid frontmatter succeeded")
				}
			})
		}
	})

	t.Run("clean misc validation branches", func(t *testing.T) {
		if err := validateCleanEventMiscRoot(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("missing misc root succeeded")
		}

		routed := t.TempDir()
		if err := os.WriteFile(filepath.Join(routed, "SKILL.md"), append(validSkill(multiMiscSkill), []byte("dws event\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := validateCleanEventMiscRoot(routed); err == nil || !strings.Contains(err.Error(), "仍包含个人 Event 路由") {
			t.Fatalf("routed misc = %v", err)
		}

		clean := t.TempDir()
		if err := os.WriteFile(filepath.Join(clean, "SKILL.md"), validSkill(multiMiscSkill), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := validateCleanEventMiscRoot(clean); err != nil {
			t.Fatalf("missing references should be clean: %v", err)
		}

		testseam.Swap(t, &skillSetupReadDir, func(string) ([]os.DirEntry, error) { return nil, fail })
		if err := validateCleanEventMiscRoot(clean); !errors.Is(err, fail) {
			t.Fatalf("read-dir failure = %v", err)
		}
	})

	t.Run("ordinary install error", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiMiscSkill})
		copyCalls := 0
		testseam.Swap(t, &skillSetupCopyDir, func(string, string) error { copyCalls++; return fail })
		migration := filepath.Join(t.TempDir(), "migration")
		ordinary := filepath.Join(t.TempDir(), "ordinary")
		if _, _, err := installMultiSkillsWithEventMigration(src, []string{multiEventSkill}, []string{migration, ordinary}, []string{migration}, true, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "未执行迁移") {
			t.Fatalf("ordinary install failure = %v", err)
		}
		if copyCalls == 0 {
			t.Fatal("ordinary staging failure seam was not exercised")
		}
	})

	t.Run("canonical ordinary install error", func(t *testing.T) {
		testseam.Swap(t, &skillSetupInstallMulti, func(string, []string, []string, io.Writer, io.Writer, bool) (int, int, error) {
			return 0, 0, fail
		})
		home := t.TempDir()
		canonical := filepath.Join(home, ".agents", "skills")
		universalMigration := filepath.Join(home, ".codex", "skills")
		if _, _, err := installMultiSkillsWithEventMigration(
			"src",
			[]string{multiEventSkill},
			[]string{canonical, universalMigration},
			[]string{universalMigration},
			true,
			io.Discard,
			io.Discard,
		); !errors.Is(err, fail) {
			t.Fatalf("canonical ordinary install failure = %v, want %v", err, fail)
		}
	})

	t.Run("prerequisite install error", func(t *testing.T) {
		testseam.Swap(t, &skillSetupInstallMulti, func(string, []string, []string, io.Writer, io.Writer, bool) (int, int, error) {
			return 0, 0, fail
		})
		migration := filepath.Join(t.TempDir(), "migration")
		if _, _, err := installMultiSkillsWithEventMigration("src", []string{multiEventSkill, multiMiscSkill, multiSharedSkill}, []string{migration}, []string{migration}, true, io.Discard, io.Discard); !errors.Is(err, fail) {
			t.Fatalf("prerequisite install failure = %v", err)
		}
	})

	t.Run("preparation cleanup and staged misc validation", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiMiscSkill})
		dest := t.TempDir()
		testseam.Swap(t, &skillSetupCopyDir, func(string, string) error { return fail })
		cleanupFail := errors.New("cleanup failure")
		testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return cleanupFail })
		if _, err := prepareEventMiscMigration(src, dest); err == nil || !errors.Is(err, fail) || !errors.Is(err, cleanupFail) {
			t.Fatalf("joined preparation cleanup error = %v", err)
		}
	})

	t.Run("invalid staged misc root", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiMiscSkill})
		if err := os.WriteFile(filepath.Join(src, multiMiscSkill, "SKILL.md"), validSkill(multiEventSkill), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareEventMiscMigration(src, t.TempDir()); err == nil || !strings.Contains(err.Error(), "staging 验证失败") {
			t.Fatalf("invalid staged misc = %v", err)
		}
	})

	t.Run("later staging failure joins cleanup error", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiMiscSkill})
		first := filepath.Join(t.TempDir(), "a")
		second := filepath.Join(t.TempDir(), "b")
		if err := os.MkdirAll(first, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(second, 0o755); err != nil {
			t.Fatal(err)
		}
		originalMkdirTemp := skillSetupMkdirTemp
		calls := 0
		testseam.Swap(t, &skillSetupMkdirTemp, func(dir, pattern string) (string, error) {
			calls++
			if calls == 2 {
				return "", fail
			}
			return originalMkdirTemp(dir, pattern)
		})
		cleanupFail := errors.New("stage cleanup failure")
		testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return cleanupFail })
		if _, err := migrateEventMiscAtomically(src, []string{second, first}, io.Discard, io.Discard); err == nil || !errors.Is(err, fail) || !errors.Is(err, cleanupFail) {
			t.Fatalf("later preparation failure = %v", err)
		}
	})

	t.Run("successful migration reports cleanup warning", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiMiscSkill})
		home := filepath.Join(t.TempDir(), "skills")
		writeFoldedEventMisc(t, home)
		cleanupFail := errors.New("final cleanup failure")
		testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return cleanupFail })
		var stderr bytes.Buffer
		installed, err := migrateEventMiscAtomically(src, []string{home}, io.Discard, &stderr)
		if err != nil || installed != 2 || !strings.Contains(stderr.String(), cleanupFail.Error()) {
			t.Fatalf("successful migration cleanup warning: installed=%d err=%v stderr=%s", installed, err, stderr.String())
		}
	})

	t.Run("commit rollback and cleanup failures are joined", func(t *testing.T) {
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiMiscSkill})
		root := t.TempDir()
		first := filepath.Join(root, "a", "skills")
		second := filepath.Join(root, "b", "skills")
		for _, home := range []string{first, second} {
			writeFoldedEventMisc(t, home)
			writeOldStandaloneEvent(t, home)
		}
		commitFail := errors.New("second commit failure")
		rollbackFail := errors.New("first rollback failure")
		originalRename := skillSetupRename
		testseam.Swap(t, &skillSetupRename, func(oldPath, newPath string) error {
			if filepath.Base(oldPath) == "new-misc" && newPath == filepath.Join(second, multiMiscSkill) {
				return commitFail
			}
			if filepath.Base(oldPath) == "old-misc" && newPath == filepath.Join(first, multiMiscSkill) {
				return rollbackFail
			}
			return originalRename(oldPath, newPath)
		})
		cleanupFail := errors.New("post-rollback cleanup failure")
		testseam.Swap(t, &skillSetupRemoveAll, func(string) error { return cleanupFail })
		if _, err := migrateEventMiscAtomically(src, []string{second, first}, io.Discard, io.Discard); err == nil || !errors.Is(err, commitFail) || !errors.Is(err, rollbackFail) || !errors.Is(err, cleanupFail) {
			t.Fatalf("joined commit/rollback/cleanup error = %v", err)
		}
	})

	t.Run("commit preflight and rollback aggregation", func(t *testing.T) {
		migration := &eventMiscMigration{dest: "dest", eventPath: "event", miscPath: "misc"}
		testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, fail })
		if err := commitEventMiscMigration(migration); !errors.Is(err, fail) {
			t.Fatalf("event stat failure = %v", err)
		}

		testseam.Swap(t, &skillSetupStat, func(path string) (os.FileInfo, error) {
			if path == migration.eventPath {
				return skillSetupFileInfo{name: "event", mode: os.ModeDir}, nil
			}
			return nil, fail
		})
		if err := commitEventMiscMigration(migration); !errors.Is(err, fail) {
			t.Fatalf("misc stat failure = %v", err)
		}

		testseam.Swap(t, &skillSetupStat, func(string) (os.FileInfo, error) { return nil, os.ErrNotExist })
		if err := commitEventMiscMigration(migration); err == nil || !strings.Contains(err.Error(), "已不存在") {
			t.Fatalf("missing misc = %v", err)
		}

		migration.newMiscEnabled = true
		testseam.Swap(t, &skillSetupRename, func(string, string) error { return fail })
		if err := rollbackEventMiscMigrations([]*eventMiscMigration{migration}); !errors.Is(err, fail) {
			t.Fatalf("rollback aggregation = %v", err)
		}
	})
}
