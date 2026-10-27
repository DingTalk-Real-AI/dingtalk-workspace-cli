// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

// Exercise the delivered leaf queries, including the compact view consumed by
// Agents, rather than only inspecting the source declarations or a subset fixture.
func TestCrossPlatformCoverageDangerousConfirmationFinalSchema(t *testing.T) {
	root := NewRootCommand()
	for _, test := range []struct {
		path   string
		effect string
	}{
		{"calendar event delete", "destructive"},
		{"calendar attendee delete", "write"},
		{"calendar room delete", "write"},
		{"chat group members remove", "write"},
		{"minutes replace-text", "destructive"},
		{"doc permission update", "write"},
	} {
		t.Run(test.path, func(t *testing.T) {
			cmd := exactCommandForTest(root, test.path)
			if cmd == nil || !helpers.HasContractConfirmSafety(cmd) {
				t.Fatalf("%s has no declared runtime confirmation gate", test.path)
			}
			full := executeShortcutSchemaQuery(t, "--cli-path", test.path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", test.path, "--compact")
			provenance := schemaContractMap(full["field_provenance"])
			for field, want := range map[string]string{
				"effect": test.effect, "risk": "high", "confirmation": "user_required", "idempotency": "unknown",
			} {
				if full[field] != want || compact[field] != want {
					t.Errorf("%s: full=%#v compact=%#v, want %q", field, full[field], compact[field], want)
				}
				if p := provenance[field]; p["precedence"] != "contract_final" || p["value"] != want {
					t.Errorf("%s winner differs from final delivery: %#v", field, p)
				}
			}
		})
	}
	// The five destructive controls from #1096 retain their existing contract.
	for _, path := range []string{"aitable base delete", "aitable record delete", "doc delete", "doc block delete", "todo task delete"} {
		t.Run(path, func(t *testing.T) {
			leaf := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			if leaf["effect"] != "destructive" || leaf["confirmation"] != "user_required" {
				t.Fatalf("control safety changed: effect=%#v confirmation=%#v", leaf["effect"], leaf["confirmation"])
			}
		})
	}
}

func TestCrossPlatformCoverageMonoDangerousCalendarPathsAreCanonical(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "skills", "mono", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Git checkouts may use CRLF on Windows; parse the same Markdown section
	// independently of the working tree's line endings.
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	_, section, ok := strings.Cut(text, "## 危险操作确认\n")
	if !ok {
		t.Fatal("mono skill has no dangerous-operation table")
	}
	section, _, _ = strings.Cut(section, "### 确认流程")
	root := NewRootCommand()
	paths := map[string]bool{}
	for _, line := range strings.Split(section, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || strings.TrimSpace(cells[1]) != "`calendar`" {
			continue
		}
		path := "calendar " + strings.Trim(strings.TrimSpace(cells[2]), "`")
		cmd := exactCommandForTest(root, path)
		if cmd == nil || strings.TrimPrefix(cmd.CommandPath(), root.CommandPath()+" ") != path {
			t.Errorf("dangerous-operation table must use a canonical CLI path: %s", path)
		}
		paths[path] = true
	}
	for _, path := range []string{"calendar event delete", "calendar attendee delete", "calendar room delete"} {
		if !paths[path] {
			t.Errorf("dangerous-operation table is missing %s", path)
		}
	}
}
