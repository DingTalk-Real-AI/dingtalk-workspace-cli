// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillstate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageStartupDetectsNestedUpgradeLayoutWithoutMutation(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupAgentHomes, []string{".agents/skills", ".codex/skills"})
	nested := filepath.Join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("old nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := detectNestedMultiSkillLayout()
	if err != nil || !found {
		t.Fatalf("detect nested layout = (%v, %v), want (true, nil)", found, err)
	}
	data, err := os.ReadFile(filepath.Join(nested, "SKILL.md"))
	if err != nil || string(data) != "old nested" {
		t.Fatalf("detection changed nested Skill: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "dingtalk-chat")); !os.IsNotExist(err) {
		t.Fatalf("detection unexpectedly installed a canonical Skill: %v", err)
	}
}

func TestCrossPlatformCoverageStartupDetectionIgnoresMonoAndReportsReadFailure(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupAgentHomes, []string{".agents/skills"})
	mono := filepath.Join(home, ".agents", "skills", "dws")
	if err := os.MkdirAll(mono, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("valid mono"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := detectNestedMultiSkillLayout()
	if err != nil || found {
		t.Fatalf("valid mono detection = (%v, %v), want (false, nil)", found, err)
	}

	failure := errors.New("HOME failure")
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return "", failure })
	found, err = detectNestedMultiSkillLayout()
	if found || !errors.Is(err, failure) {
		t.Fatalf("HOME failure detection = (%v, %v)", found, err)
	}
}

func TestCrossPlatformCoverageStartupDetectionSkipsExplicitSkillManagers(t *testing.T) {
	upgradeCmd := &cobra.Command{Use: "upgrade"}
	if shouldDetectNestedSkillLayout(upgradeCmd) {
		t.Fatal("upgrade must manage its own Skill lifecycle")
	}
	skillCmd := &cobra.Command{Use: "skill"}
	setupCmd := &cobra.Command{Use: "setup"}
	skillCmd.AddCommand(setupCmd)
	if shouldDetectNestedSkillLayout(setupCmd) {
		t.Fatal("skill setup must manage its own Skill lifecycle")
	}
	if !shouldDetectNestedSkillLayout(&cobra.Command{Use: "version"}) || !shouldDetectNestedSkillLayout(nil) {
		t.Fatal("ordinary commands must trigger read-only detection")
	}
}

func TestCrossPlatformCoverageStartupWarningIsReadOnlyAndDoesNotBypassConfirmation(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	testseam.Swap(t, &skillSetupAgentHomes, []string{".codex/skills"})
	nested := filepath.Join(home, ".codex", "skills", "dws", "multi", "dingtalk-chat")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("old nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Any accidental return to startup mutation must fail this test before it
	// can touch the fixture.
	testseam.Swap(t, &skillSetupCopyDir, func(string, string) error {
		t.Fatal("ordinary command attempted to copy Skills")
		return nil
	})
	testseam.Swap(t, &skillSetupBackupAndRemove, func(string, string) (string, error) {
		t.Fatal("ordinary command attempted to back up or remove Skills")
		return "", nil
	})
	testseam.Swap(t, &skillSetupWriteState, func(string, skillstate.State) error {
		t.Fatal("ordinary command attempted to write Skill state")
		return nil
	})

	root := newRootCommandWithEngine(context.Background(), nil, false, true)
	cmd := &cobra.Command{Use: "version"}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if err := root.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	warning := stderr.String()
	if !strings.Contains(warning, "dws skill setup --mode multi") {
		t.Fatalf("safe migration hint missing: %q", warning)
	}
	if strings.Contains(warning, "--yes") {
		t.Fatalf("migration hint bypasses confirmation: %q", warning)
	}
	data, err := os.ReadFile(filepath.Join(nested, "SKILL.md"))
	if err != nil || string(data) != "old nested" {
		t.Fatalf("ordinary command changed nested Skill: data=%q err=%v", data, err)
	}
}
