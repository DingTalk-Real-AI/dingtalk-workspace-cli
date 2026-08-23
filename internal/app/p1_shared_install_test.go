package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeMultiSkillSrc creates a fake multi skill source tree with the given
// subdir names, each containing a minimal SKILL.md.
func writeMultiSkillSrc(t *testing.T, names ...string) string {
	t.Helper()
	src := t.TempDir()
	for _, n := range names {
		dir := filepath.Join(src, n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// dingtalk-shared must ship even when --skill narrows the set to a single product.
func TestP1SharedAlwaysIncludedWithSkillFilter(t *testing.T) {
	src := writeMultiSkillSrc(t, "dingtalk-shared", "dingtalk-aitable", "dingtalk-calendar")
	all, err := listMultiSkillNames(src)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(all, "dingtalk-shared") {
		t.Fatalf("listMultiSkillNames did not enumerate dingtalk-shared: %v", all)
	}
	filtered, err := filterMultiSkillNames(all, []string{"aitable"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if contains(filtered, "dingtalk-shared") {
		t.Fatalf("precondition: filter should drop dingtalk-shared for -s aitable: %v", filtered)
	}
	final := ensureMandatorySharedSkill(filtered, all)
	if !contains(final, "dingtalk-shared") {
		t.Fatalf("ensureMandatorySharedSkill must re-add dingtalk-shared: %v", final)
	}

	// Actually install with the filtered+mandatory set and assert dingtalk-shared landed.
	dest := t.TempDir()
	var out, errOut bytes.Buffer
	if _, _, err := installMultiSkillToHomes(src, final, []string{dest}, &out, &errOut, true); err != nil {
		t.Fatalf("install: %v (%s)", err, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dest, "dingtalk-shared", "SKILL.md")); err != nil {
		t.Fatalf("dingtalk-shared not installed with -s aitable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "dingtalk-aitable", "SKILL.md")); err != nil {
		t.Fatalf("dingtalk-aitable not installed: %v", err)
	}
}

// When the source has no dingtalk-shared (older layout), nothing is forced.
func TestP1SharedNoopWhenAbsent(t *testing.T) {
	src := writeMultiSkillSrc(t, "dingtalk-aitable")
	all, err := listMultiSkillNames(src)
	if err != nil {
		t.Fatal(err)
	}
	final := ensureMandatorySharedSkill([]string{"dingtalk-aitable"}, all)
	if contains(final, "dingtalk-shared") {
		t.Fatalf("must not invent dingtalk-shared when source lacks it: %v", final)
	}
}
