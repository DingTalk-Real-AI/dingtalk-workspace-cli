//go:build windows

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageSkillSetupWindowsJunctionReadable(t *testing.T) {
	target := filepath.Join(t.TempDir(), "canonical", "dingtalk-chat")
	link := filepath.Join(t.TempDir(), "agent", "dingtalk-chat")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("chat"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := createSkillSetupDirLink(target, link); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(link)

	info, err := os.Stat(link)
	if err != nil || !info.IsDir() {
		t.Fatalf("junction stat = %#v, %v", info, err)
	}
	body, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil || string(body) != "chat" {
		t.Fatalf("junction Skill read = %q, %v", body, err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
		t.Fatalf("junction removal changed canonical target: %v", err)
	}
}
