package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCrossPlatformCoverageSkillPathIdentityHelpers exercises the exported
// identity helpers that the app-level staging cleanup relies on. The full-suite
// coverage shards instrument each changed package independently, so the
// upgrade package must cover these wrappers from its own tests.
func TestCrossPlatformCoverageSkillPathIdentityHelpers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	identity := SkillPathFileIdentity(dir)
	if identity == "" {
		t.Skipf("platform does not report stable file identities")
	}

	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !SkillPathIdentityProven(info, info, identity, identity) {
		t.Fatal("SkillPathIdentityProven rejected the live identity of the same object")
	}
}
