package app

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	oldConfirm := skillSetupConfirm
	oldMono := skillSetupInstallMono
	oldMulti := skillSetupInstallMulti
	t.Cleanup(func() {
		skillSetupResolveMode = oldMode
		skillSetupResolveSource = oldSource
		skillSetupResolveTargets = oldTargets
		skillSetupListMulti = oldList
		skillSetupFilterMulti = oldFilter
		skillSetupConfirm = oldConfirm
		skillSetupInstallMono = oldMono
		skillSetupInstallMulti = oldMulti
	})
	fail := errors.New("failure")
	skillSetupResolveMode = func(mode string, _ bool, _ io.Writer) (string, error) { return mode, nil }
	skillSetupResolveSource = func(string, string) (string, func(), error) { return "source", func() {}, nil }
	skillSetupResolveTargets = func(string, string) ([]string, error) { return []string{"dest"}, nil }
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
	skillSetupListMulti = func(string) ([]string, error) { return []string{"dws-shared", "dingtalk-doc"}, nil }
	skillSetupFilterMulti = func([]string, []string, []string) ([]string, error) { return nil, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMulti, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("multi filter failure should propagate")
	}
	skillSetupFilterMulti = func(all, _, _ []string) ([]string, error) { return all, nil }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMulti, true)
	_ = cmd.Root().PersistentFlags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	skillSetupConfirm = func(io.Writer, string, string, []string, []string) (bool, error) { return false, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, false)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("confirmation failure should propagate")
	}
	skillSetupConfirm = func(io.Writer, string, string, []string, []string) (bool, error) { return false, nil }
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
	skillSetupInstallMono = func(string, []string, io.Writer, io.Writer) (int, int, error) { return 0, 0, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("mono install failure should propagate")
	}
	skillSetupInstallMono = func(string, []string, io.Writer, io.Writer) (int, int, error) { return 1, 0, nil }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMono, true)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	skillSetupInstallMulti = func(string, []string, []string, io.Writer, io.Writer) (int, int, error) { return 0, 0, fail }
	cmd = skillSetupCoverageCommand(t, skillSetupModeMulti, true)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("multi install failure should propagate")
	}
}

func TestCrossPlatformCoverageSkillSetupLowLevelRemainingCoverage(t *testing.T) {
	oldRunForm, oldInteractive := skillSetupRunForm, skillSetupInteractive
	oldReadDir, oldStat := skillSetupReadDir, skillSetupStat
	oldExecutable, oldGetwd, oldHome := skillSetupExecutable, skillSetupGetwd, skillSetupUserHomeDir
	oldRemove, oldMkdir := skillSetupRemoveAll, skillSetupMkdirAll
	oldCopyDir, oldWalk, oldRel := skillSetupCopyDir, skillSetupWalk, skillSetupRel
	oldReadlink, oldOpen, oldOpenFile, oldCopy := skillSetupReadlink, skillSetupOpen, skillSetupOpenFile, skillSetupCopy
	t.Cleanup(func() {
		skillSetupRunForm, skillSetupInteractive = oldRunForm, oldInteractive
		skillSetupReadDir, skillSetupStat = oldReadDir, oldStat
		skillSetupExecutable, skillSetupGetwd, skillSetupUserHomeDir = oldExecutable, oldGetwd, oldHome
		skillSetupRemoveAll, skillSetupMkdirAll = oldRemove, oldMkdir
		skillSetupCopyDir, skillSetupWalk, skillSetupRel = oldCopyDir, oldWalk, oldRel
		skillSetupReadlink, skillSetupOpen, skillSetupOpenFile, skillSetupCopy = oldReadlink, oldOpen, oldOpenFile, oldCopy
	})
	fail := errors.New("failure")

	skillSetupInteractive = func() bool { return true }
	skillSetupRunForm = func(*huh.Form) error { return fail }
	if _, err := resolveSkillSetupMode("", false, io.Discard); err == nil {
		t.Fatal("interactive mode failure should propagate")
	}
	skillSetupRunForm = func(*huh.Form) error { return nil }
	if got, err := resolveSkillSetupMode("", false, io.Discard); err != nil || got != skillSetupModeMono {
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
	if _, err := confirmSkillSetup(&out, skillSetupModeMulti, "src", []string{monoDest}, []string{"dingtalk-doc"}); err == nil {
		t.Fatal("confirmation form failure should propagate")
	}
	skillSetupRunForm = func(*huh.Form) error { return nil }
	if ok, err := confirmSkillSetup(&out, skillSetupModeMono, "src", []string{monoDest}, nil); err != nil || ok {
		t.Fatalf("EOF confirmation = %v, %v", ok, err)
	}
	skillSetupRemoveAll = func(string) error { return fail }
	cleanupMutualExclusion(monoDest, skillSetupModeMono, &out, &errOut)

	skillSetupCopyDir = func(string, string) error { return fail }
	skillSetupRemoveAll = func(string) error { return fail }
	_, skipped, _ := installSkillToHomes("src", []string{"a"}, &out, &errOut)
	if skipped != 1 {
		t.Fatal("mono remove failure not skipped")
	}
	skillSetupRemoveAll = func(string) error { return nil }
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
	_, skipped, _ = installMultiSkillToHomes("src", []string{"one", "two"}, []string{"dest"}, &out, &errOut)
	if skipped != 2 {
		t.Fatal("multi mkdir failure count mismatch")
	}
	skillSetupMkdirAll = func(string, os.FileMode) error { return nil }
	skillSetupRemoveAll = func(string) error { return fail }
	_, skipped, _ = installMultiSkillToHomes("src", []string{"one"}, []string{"dest"}, &out, &errOut)
	if skipped != 1 {
		t.Fatal("multi remove failure count mismatch")
	}
	skillSetupRemoveAll = func(string) error { return nil }
	_, skipped, _ = installMultiSkillToHomes("src", []string{"one"}, []string{"dest"}, &out, &errOut)
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
