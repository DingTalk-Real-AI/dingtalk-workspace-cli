// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSheetWhiteboardMarkdownRoutes(t *testing.T) {
	root := NewRootCommand()
	tools := deliverySchemaAllToolsForHelpFlagTest(t, root)
	assertMarkdownLarkTasksRouteWithoutDuplicateShortcuts(t, root, tools)
	assertMarkdownDriveRoutesStayCrossProduct(t, root, tools)
	assertWhiteboardPublicShortcutsStayAvailableInSchema(t, root, tools)
}

func assertMarkdownLarkTasksRouteWithoutDuplicateShortcuts(t *testing.T, root *cobra.Command, tools map[string]map[string]any) {
	t.Helper()
	registered := 0
	for _, item := range shortcut.All() {
		if item.Service == "markdown" {
			registered++
		}
	}
	if registered != 0 {
		t.Fatalf("registered Markdown Shortcuts=%d, want 0: existing composite leaves own these workflows", registered)
	}

	type route struct {
		canonical    string
		confirmation string
		flags        []string
	}
	routes := map[string]route{
		"create": {
			canonical: "markdown.create", confirmation: "not_required",
			flags: []string{"content", "file", "folder", "name", "space-id", "workspace"},
		},
		"fetch": {
			canonical: "markdown.fetch", confirmation: "not_required",
			flags: []string{"node", "output", "space-id", "workspace"},
		},
		"overwrite": {
			canonical: "markdown.overwrite", confirmation: "user_required",
			flags: []string{"content", "dry-run", "file", "name", "node", "space-id", "workspace"},
		},
		"patch": {
			canonical: "markdown.patch", confirmation: "user_required",
			flags: []string{"content", "dry-run", "node", "pattern", "regex", "space-id", "workspace"},
		},
		"diff": {
			canonical: "markdown.diff", confirmation: "not_required",
			flags: []string{"context", "file", "node", "version", "version2"},
		},
	}

	group := mustFindCommand(t, root, "markdown")
	children := map[string]bool{}
	for _, child := range group.Commands() {
		children[child.Name()] = true
	}
	if len(children) != len(routes) {
		t.Fatalf("Markdown ordinary leaves=%v, want exactly five routed workflows", children)
	}

	for name, want := range routes {
		leaf := mustFindCommand(t, root, "markdown", name)
		if leaf.Hidden || !leaf.Runnable() {
			t.Errorf("markdown %s hidden/runnable=%v/%v, want false/true", name, leaf.Hidden, leaf.Runnable())
		}
		if !children[name] {
			t.Errorf("markdown %s is not mounted on the ordinary product group", name)
		}
		for _, flag := range want.flags {
			if leaf.Flags().Lookup(flag) == nil {
				t.Errorf("markdown %s is missing routed flag --%s", name, flag)
			}
		}
		if shortcut.InPublicCatalog("markdown", "+"+name) {
			t.Errorf("markdown +%s unexpectedly entered the public Shortcut catalog", name)
		}

		meta, ok := cli.ResolveMeta("markdown " + name)
		if !ok {
			t.Errorf("markdown %s missing from assembled Schema", name)
			continue
		}
		if meta.Identity.Canonical != want.canonical || meta.Identity.CLIPath != "markdown "+name {
			t.Errorf("markdown %s identity=%#v, want canonical=%q cli_path=%q", name, meta.Identity, want.canonical, "markdown "+name)
		}
		if meta.Safety.Confirmation != want.confirmation {
			t.Errorf("markdown %s confirmation=%q, want %q", name, meta.Safety.Confirmation, want.confirmation)
		}

		tool := tools[want.canonical]
		if tool == nil {
			t.Errorf("markdown %s missing from full delivery Schema", name)
			continue
		}
		if got := schemaContractString(tool["availability"]); got != "available" {
			t.Errorf("markdown %s availability=%q, want available", name, got)
		}
		if got := schemaContractString(tool["interface_mode"]); got != "composite" {
			t.Errorf("markdown %s interface_mode=%q, want composite", name, got)
		}
		if got := schemaContractString(tool["interface_reason"]); got == "" {
			t.Errorf("markdown %s is missing the reviewed composite routing reason", name)
		}
	}
}

func assertMarkdownDriveRoutesStayCrossProduct(t *testing.T, root *cobra.Command, tools map[string]map[string]any) {
	t.Helper()
	driveShortcuts := map[string]string{
		"+copy":             "drive.shortcut_copy",
		"+delete":           "drive.shortcut_delete",
		"+find-file":        "drive.shortcut_find_file",
		"+list":             "drive.shortcut_list",
		"+move":             "drive.shortcut_move",
		"+publish-get":      "drive.shortcut_publish_get",
		"+recycle-restore":  "drive.shortcut_recycle_restore",
		"+rename":           "drive.shortcut_rename",
		"+version-download": "drive.shortcut_version_download",
		"+version-get":      "drive.shortcut_version_get",
		"+version-history":  "drive.shortcut_version_history",
		"+version-revert":   "drive.shortcut_version_revert",
	}
	for name, canonical := range driveShortcuts {
		leaf := mustFindCommand(t, root, "drive", name)
		if leaf.Hidden || !leaf.Runnable() {
			t.Errorf("drive %s hidden/runnable=%v/%v, want false/true", name, leaf.Hidden, leaf.Runnable())
		}
		if !shortcut.InPublicCatalog("drive", name) {
			t.Errorf("drive %s is not in the public Shortcut catalog", name)
		}
		assertMarkdownCrossProductRoute(t, tools, "drive "+name, canonical)
	}

	ordinaryRoutes := map[string]string{
		"drive permission list": "drive.list_permission",
		"drive pull":            "drive.folder_pull",
		"drive push":            "drive.folder_push",
		"drive status":          "drive.folder_status",
		"drive sync":            "drive.folder_sync",
		"wiki node list":        "wiki.list_nodes",
	}
	for cliPath, canonical := range ordinaryRoutes {
		assertMarkdownCrossProductRoute(t, tools, cliPath, canonical)
	}
}

func assertMarkdownCrossProductRoute(t *testing.T, tools map[string]map[string]any, cliPath, canonical string) {
	t.Helper()
	meta, ok := cli.ResolveMeta(cliPath)
	if !ok {
		t.Errorf("cross-product route %q is missing from assembled Schema", cliPath)
		return
	}
	if meta.Identity.Canonical != canonical || meta.Identity.CLIPath != cliPath {
		t.Errorf("cross-product route %q identity=%#v, want canonical=%q", cliPath, meta.Identity, canonical)
	}
	tool := tools[canonical]
	if tool == nil {
		t.Errorf("cross-product route %q is missing from full delivery Schema", cliPath)
		return
	}
	if got := schemaContractString(tool["availability"]); got != "available" {
		t.Errorf("cross-product route %q availability=%q, want available", cliPath, got)
	}
}
