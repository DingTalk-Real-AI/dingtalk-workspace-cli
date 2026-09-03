package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// TestCrossPlatformCoverageSkillSetupEventMigrationRetiresUniversalFoldedCopies
// pins the P0 fix: when a beta.6 folded dingtalk-misc (carrying the personal
// Event route) exists only in a UNIVERSAL agent home (~/.codex/skills), the
// Event/misc migration must not leave physical duplicates there. Universal
// homes read ~/.agents/skills directly, so their old folded copies are retired
// (backed up) and the split standalone event + clean misc land in canonical.
func TestCrossPlatformCoverageSkillSetupEventMigrationRetiresUniversalFoldedCopies(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })

	codexSkills := filepath.Join(home, ".codex", "skills")
	writeFoldedEventMisc(t, codexSkills)
	// beta.6 physical copies alongside the folded misc.
	for _, name := range []string{multiEventSkill, multiSharedSkill} {
		if err := os.MkdirAll(filepath.Join(codexSkills, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(codexSkills, name, "SKILL.md"), []byte("beta6 "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	canonical := filepath.Join(home, ".agents", "skills")
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})

	stdout, stderr, err := executeMultiSkillSetupTest(t, src, []string{canonical, codexSkills}, "--skill", "event", "--yes")
	if err != nil {
		t.Fatalf("setup failed: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}

	// P0: the universal home must not retain any physical dingtalk-* copy.
	for _, name := range []string{multiEventSkill, multiMiscSkill, multiSharedSkill} {
		if _, err := os.Stat(filepath.Join(codexSkills, name)); !os.IsNotExist(err) {
			t.Fatalf("universal .codex/skills still has physical %s (stat err=%v)", name, err)
		}
	}
	// canonical owns the new standalone event + clean misc (+ shared).
	for _, name := range []string{multiEventSkill, multiMiscSkill, multiSharedSkill} {
		if _, err := os.Stat(filepath.Join(canonical, name, "SKILL.md")); err != nil {
			t.Fatalf("canonical missing %s: %v", name, err)
		}
	}
	if err := validateCleanEventMiscRoot(filepath.Join(canonical, multiMiscSkill)); err != nil {
		t.Fatalf("canonical misc still contains folded Event content: %v", err)
	}
	if _, _, err := executeMultiSkillSetupTest(t, src, []string{canonical, codexSkills}, "--skill", "event", "--yes"); err != nil {
		t.Fatalf("idempotent rerun failed: %v", err)
	}
}

func TestCrossPlatformCoverageSkillSetupUnrelatedSelectionPreservesUniversalFoldedPair(t *testing.T) {
	home := t.TempDir()
	testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
	canonical := filepath.Join(home, ".agents", "skills")
	codexSkills := filepath.Join(home, ".codex", "skills")
	writeFoldedEventMisc(t, codexSkills)
	writeOldStandaloneEvent(t, codexSkills)
	chat := filepath.Join(codexSkills, "dingtalk-chat")
	if err := os.MkdirAll(chat, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chat, "SKILL.md"), []byte("keep chat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill, "dingtalk-doc"})
	stdout, stderr, err := executeMultiSkillSetupTest(t, src, []string{canonical, codexSkills}, "--skill", "doc", "--yes")
	if err != nil {
		t.Fatalf("selective doc install failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if strings.Contains(stdout, "Event 原子迁移") {
		t.Fatalf("unrelated selection migrated Event/misc: %s", stdout)
	}
	assertOldEventMiscPair(t, codexSkills)
	if body, err := os.ReadFile(filepath.Join(chat, "SKILL.md")); err != nil || string(body) != "keep chat\n" {
		t.Fatalf("unselected chat changed: body=%q err=%v", body, err)
	}
}

func TestCrossPlatformCoverageSkillSetupUniversalRetirementFailures(t *testing.T) {
	failure := errors.New("retirement denied")

	t.Run("home", func(t *testing.T) {
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return "", failure })
		codex := filepath.Join(t.TempDir(), ".codex", "skills")
		if err := retireMigratedUniversalSkills([]string{codex}, []string{multiEventSkill}, io.Discard); !errors.Is(err, failure) {
			t.Fatalf("home failure = %v, want %v", err, failure)
		}
	})

	t.Run("deduplicate", func(t *testing.T) {
		home := t.TempDir()
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
		codex := filepath.Join(home, ".codex", "skills")
		if err := retireMigratedUniversalSkills(
			[]string{codex, codex},
			[]string{multiEventSkill, multiEventSkill},
			io.Discard,
		); err != nil {
			t.Fatalf("deduplicated retirement failed: %v", err)
		}
	})

	// Retiring an obsolete universal copy installs nothing, so its failure is
	// surfaced as a warning and the successful installation is kept.
	t.Run("backup failure warns without failing the command", func(t *testing.T) {
		home := t.TempDir()
		testseam.Swap(t, &skillSetupUserHomeDir, func() (string, error) { return home, nil })
		codex := filepath.Join(home, ".codex", "skills")
		writeFoldedEventMisc(t, codex)
		src := writeMultiSkillSource(t, []string{multiEventSkill, multiSharedSkill, multiMiscSkill})
		testseam.Swap(t, &skillSetupBackupAndRemove, func(string, string) (string, error) {
			return "", failure
		})
		_, stderr, err := executeMultiSkillSetupTest(
			t,
			src,
			[]string{filepath.Join(home, ".agents", "skills"), codex},
			"--skill", "event", "--yes",
		)
		if err != nil {
			t.Fatalf("retirement backup failure must not fail the command: %v", err)
		}
		if !strings.Contains(stderr, "退役 universal Agent") {
			t.Fatalf("retirement backup failure must be reported, stderr = %q", stderr)
		}
	})
}
