// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiWikiSkillDoesNotRequireYesForAppend(t *testing.T) {
	path := filepath.Join("..", "..", "skills", "multi", "dingtalk-wiki", "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read wiki skill: %v", err)
	}
	text := string(content)
	if strings.Contains(text, "--mode overwrite|append --content-file <tmp.md> --yes") {
		t.Fatal("wiki skill still applies --yes to append and overwrite indiscriminately")
	}
	if !strings.Contains(text, "--mode append --content-file <tmp.md>") ||
		!strings.Contains(text, "--mode overwrite --content-file <tmp.md> --yes") {
		t.Fatal("wiki skill must publish separate append and confirmed overwrite examples")
	}
}
