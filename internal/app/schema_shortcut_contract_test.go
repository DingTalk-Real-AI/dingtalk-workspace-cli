// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	publicShortcutCount = 399
	// schemaPublishedShortcutCount counts every delivered *.shortcut_* tool,
	// including the hidden historical minutes.shortcut_minutes_search contract.
	schemaPublishedShortcutCount = 401
	// publiclyDeliveredShortcutCount is the public-catalog subset of that surface.
	publiclyDeliveredShortcutCount = 399
)

func TestDeliverySchemaCoversOrExactlyExcludesEveryPublicShortcutContract(t *testing.T) {
	tools := deliverySchemaAllToolsForHelpFlagTest(t, NewRootCommand())
	public := make([]shortcut.Shortcut, 0, publicShortcutCount)
	for _, candidate := range shortcut.All() {
		if candidate.UserDefined || !shortcut.InPublicCatalog(candidate.Service, candidate.Command) {
			continue
		}
		public = append(public, candidate)
	}
	if got := len(public); got != publicShortcutCount {
		t.Fatalf("public built-in shortcuts = %d, want %d", got, publicShortcutCount)
	}

	deliveredShortcuts := 0
	for canonical := range tools {
		if strings.Contains(canonical, ".shortcut_") {
			deliveredShortcuts++
		}
	}
	if deliveredShortcuts != schemaPublishedShortcutCount {
		t.Fatalf("delivery schema --all shortcut tools = %d, want %d", deliveredShortcuts, schemaPublishedShortcutCount)
	}

	exclusions, err := cli.ReviewedRuntimeSchemaExclusions()
	if err != nil {
		t.Fatal(err)
	}
	excludedPaths := make(map[string]bool, len(exclusions))
	for _, exclusion := range exclusions {
		if !exclusion.Reviewed || strings.TrimSpace(exclusion.Reason) == "" {
			t.Fatalf("unreviewed public command exclusion: %#v", exclusion)
		}
		excludedPaths[exclusion.CLIPath] = true
	}

	excludedShortcuts := 0
	for _, declared := range public {
		declared := declared
		t.Run(declared.Service+"/"+strings.TrimPrefix(declared.Command, "+"), func(t *testing.T) {
			canonical := shortcutSchemaCanonical(declared)
			tool := tools[canonical]
			if tool == nil {
				cliPath := declared.Service + " " + declared.Command
				if !excludedPaths[cliPath] {
					t.Fatalf("delivery schema --all is missing %s (%s) without an exact reviewed exclusion", canonical, cliPath)
				}
				excludedShortcuts++
				return
			}
			assertDeliveryShortcutIdentityAndSelection(t, tool, declared, canonical)
			assertDeliveryShortcutSafetyAndInterface(t, tool, declared, canonical)
			assertDeliveryShortcutParameters(t, tool, declared, canonical)
			assertDeliveryShortcutConstraints(t, tool, declared, canonical)
		})
	}
	if got, want := excludedShortcuts, publicShortcutCount-publiclyDeliveredShortcutCount; got != want {
		t.Fatalf("exactly excluded public shortcuts = %d, want %d", got, want)
	}
}

func TestDeliveryShortcutProgressiveQueriesReturnCompleteContracts(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "chat +messages-read-status")
	if got, want := schemaContractString(leaf["canonical_path"]), "chat.shortcut_messages_read_status"; got != want {
		t.Fatalf("shortcut leaf canonical_path = %q, want %q", got, want)
	}
	if got, want := schemaContractString(leaf["confirmation"]), "user_required"; got != want {
		t.Fatalf("shortcut leaf confirmation = %q, want %q", got, want)
	}
	conversationID := schemaContractMap(leaf["parameters"])["conversation-id"]
	if required, _ := conversationID["required"].(bool); required {
		t.Fatal("public --conversation-id must stay optional when hidden siblings still satisfy the declared exactly_one group")
	}
	wantMessagesConstraints := map[string]any{
		"require_one_of":     [][]string{{"conversation-id", "group", "id"}},
		"mutually_exclusive": [][]string{{"conversation-id", "group", "id"}},
	}
	if got := leaf["constraints"]; !schemaContractJSONEqual(got, wantMessagesConstraints) {
		t.Fatalf("shortcut leaf constraints = %#v, want %#v", got, wantMessagesConstraints)
	}

	constrainedLeaf := executeShortcutSchemaQuery(t, "--cli-path", "calendar +freebusy")
	wantConstraints := map[string]any{
		"require_one_of": [][]string{{"users", "rooms"}},
	}
	if got := constrainedLeaf["constraints"]; !schemaContractJSONEqual(got, wantConstraints) {
		t.Fatalf("shortcut leaf constraints = %#v, want %#v", got, wantConstraints)
	}

	product := executeShortcutSchemaQuery(t, "chat")
	productPayload, _ := product["product"].(map[string]any)
	if got, want := int(product["count"].(float64)), 217; got != want {
		t.Fatalf("schema chat count = %d, want %d", got, want)
	}
	summaries := schemaContractObjectSlice(productPayload["tools"])
	shortcutCount := 0
	summaryByCLIPath := make(map[string]map[string]any, len(summaries))
	for _, summary := range summaries {
		summaryByCLIPath[schemaContractString(summary["cli_path"])] = summary
		if strings.HasPrefix(schemaContractString(summary["canonical_path"]), "chat.shortcut_") {
			shortcutCount++
		}
	}
	if shortcutCount != 98 {
		t.Fatalf("schema chat shortcut summaries = %d, want 98", shortcutCount)
	}
	for _, cliPath := range missingChatCatalogCoveragePaths() {
		if summaryByCLIPath[cliPath] == nil {
			t.Fatalf("schema chat missing expected catalog tool %q", cliPath)
		}
	}
	assertSchemaSummarySafety(t, summaryByCLIPath, "chat clear-messages", "destructive", "high", "user_required")
	assertSchemaSummarySafety(t, summaryByCLIPath, "chat data-auth cross-org", "write", "high", "user_required")
	assertSchemaSummarySafety(t, summaryByCLIPath, "chat group share-invite", "write", "medium", "user_required")
	assertChatCatalogCompleteLeafContracts(t)
}

func assertSchemaSummarySafety(
	t testing.TB,
	summaries map[string]map[string]any,
	cliPath string,
	effect string,
	risk string,
	confirmation string,
) {
	t.Helper()
	summary := summaries[cliPath]
	if summary == nil {
		t.Fatalf("schema chat missing expected catalog tool %q", cliPath)
	}
	if got := schemaContractString(summary["effect"]); got != effect {
		t.Fatalf("%s effect = %q, want %q", cliPath, got, effect)
	}
	if got := schemaContractString(summary["risk"]); got != risk {
		t.Fatalf("%s risk = %q, want %q", cliPath, got, risk)
	}
	if got := schemaContractString(summary["confirmation"]); got != confirmation {
		t.Fatalf("%s confirmation = %q, want %q", cliPath, got, confirmation)
	}
}

func assertChatCatalogCompleteLeafContracts(t testing.TB) {
	t.Helper()
	for _, cliPath := range []string{
		"chat clear-messages",
		"chat clear-red-point",
		"chat hide",
		"chat mark-read",
		"chat mark-unread",
		"chat mute-at-all",
		"chat mute-red-envelope",
	} {
		leaf := executeShortcutSchemaQuery(t, "--cli-path", cliPath)
		assertSchemaLeafParameterRequired(t, leaf, cliPath, "conversation-id", false)
		assertSchemaLeafConstraints(t, leaf, cliPath, map[string]any{
			"require_one_of":     [][]string{{"conversation-id", "id", "chat"}},
			"mutually_exclusive": [][]string{{"conversation-id", "id", "chat"}},
		})
	}

	markRead := executeShortcutSchemaQuery(t, "--cli-path", "chat mark-read")
	assertSchemaLeafParameterRequired(t, markRead, "chat mark-read", "message-id", true)

	chmod := executeShortcutSchemaQuery(t, "--cli-path", "chat chmod")
	assertSchemaLeafConstraints(t, chmod, "chat chmod", map[string]any{
		"require_one_of":     [][]string{{"conversation-id", "open-dingtalk-id", "user", "permParam"}},
		"mutually_exclusive": [][]string{{"conversation-id", "open-dingtalk-id", "user"}},
	})
	assertChatGrantParameterFacts(t, chmod, "chat chmod")

	crossOrg := executeShortcutSchemaQuery(t, "--cli-path", "chat data-auth cross-org")
	assertSchemaLeafConstraints(t, crossOrg, "chat data-auth cross-org", map[string]any{
		"require_one_of":     [][]string{{"target-org-id", "all"}},
		"mutually_exclusive": [][]string{{"target-org-id", "all"}},
	})
	assertChatGrantParameterFacts(t, crossOrg, "chat data-auth cross-org")

	shareInvite := executeShortcutSchemaQuery(t, "--cli-path", "chat group share-invite")
	assertSchemaLeafConstraints(t, shareInvite, "chat group share-invite", map[string]any{
		"require_one_of":     [][]string{{"target", "receiver"}},
		"mutually_exclusive": [][]string{{"target", "receiver"}},
	})

	auditJoin := executeShortcutSchemaQuery(t, "--cli-path", "chat group audit-join-validation")
	assertSchemaLeafParameterEnum(t, auditJoin, "chat group audit-join-validation", "status", []string{"AuditApprove", "AuditDelete"})
}

func assertSchemaLeafParameterRequired(t testing.TB, leaf map[string]any, cliPath, name string, want bool) {
	t.Helper()
	parameters := schemaContractMap(leaf["parameters"])
	parameter := parameters[name]
	if parameter == nil {
		t.Fatalf("%s missing --%s parameter: %#v", cliPath, name, parameters)
	}
	if got, _ := parameter["required"].(bool); got != want {
		t.Fatalf("%s --%s required = %#v, want %v", cliPath, name, parameter["required"], want)
	}
}

func assertSchemaLeafParameterEnum(t testing.TB, leaf map[string]any, cliPath, name string, want []string) {
	t.Helper()
	parameters := schemaContractMap(leaf["parameters"])
	parameter := parameters[name]
	if parameter == nil {
		t.Fatalf("%s missing --%s parameter: %#v", cliPath, name, parameters)
	}
	if got := schemaContractStringSlice(parameter["enum"]); !schemaContractJSONEqual(got, want) {
		t.Fatalf("%s --%s enum = %#v, want %#v", cliPath, name, got, want)
	}
}

func assertSchemaLeafConstraints(t testing.TB, leaf map[string]any, cliPath string, want map[string]any) {
	t.Helper()
	if got := leaf["constraints"]; !schemaContractJSONEqual(got, want) {
		t.Fatalf("%s constraints = %#v, want %#v", cliPath, got, want)
	}
}

func assertChatGrantParameterFacts(t testing.TB, leaf map[string]any, cliPath string) {
	t.Helper()
	parameters := schemaContractMap(leaf["parameters"])
	grantType := parameters["grant-type"]
	if grantType == nil {
		t.Fatalf("%s missing --grant-type parameter: %#v", cliPath, parameters)
	}
	wantEnum := []string{"once", "session", "timed", "permanent"}
	if got := schemaContractStringSlice(grantType["enum"]); !schemaContractJSONEqual(got, wantEnum) {
		t.Fatalf("%s --grant-type enum = %#v, want %#v", cliPath, got, wantEnum)
	}
	if got := schemaContractString(parameters["session-id"]["required_when"]); got != "grant-type is session" {
		t.Fatalf("%s --session-id required_when = %q, want grant-type is session", cliPath, got)
	}
	if got := schemaContractString(parameters["ttl"]["required_when"]); got != "grant-type is timed" {
		t.Fatalf("%s --ttl required_when = %q, want grant-type is timed", cliPath, got)
	}
}

func TestDeliveryDocUpdateShortcutPublishesCompleteConditionalContract(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "doc +update")
	if got, want := schemaContractString(leaf["confirmation"]), "not_required"; got != want {
		t.Fatalf("confirmation = %q, want %q", got, want)
	}
	parameters := schemaContractMap(leaf["parameters"])
	if got, want := len(parameters), 14; got != want {
		t.Fatalf("parameter count = %d, want %d: %#v", got, want, parameters)
	}
	if required, _ := parameters["node"]["required"].(bool); !required {
		t.Errorf("--node required = %#v, want true", parameters["node"]["required"])
	}
	if required, _ := parameters["command"]["required"].(bool); !required {
		t.Errorf("--command required = %#v, want true", parameters["command"]["required"])
	}
	wantProperties := map[string]string{
		"node": "node", "doc": "doc", "command": "command", "content": "content", "text": "text", "doc-format": "docFormat",
		"block-id": "blockId", "after-block-id": "afterBlockId", "ref-block": "referenceBlockId", "where": "where", "old": "old", "new": "new",
		"allow-resource-delete": "allowResourceDelete", "expected-revision": "expectedRevision",
	}
	for name, want := range wantProperties {
		if got := schemaContractString(parameters[name]["property"]); got != want {
			t.Errorf("--%s property = %q, want %q", name, got, want)
		}
	}
	wantRequiredWhen := map[string]string{
		"content":        "--command is append, overwrite, block_insert, block_insert_after, or block_replace",
		"block-id":       "--command is block_replace, block_delete, block_copy_insert, or block_copy_insert_after",
		"after-block-id": "--command is block_insert_after or block_copy_insert_after",
		"ref-block":      "--command is block_insert or block_copy_insert",
		"where":          "--command is block_insert or block_copy_insert",
		"old":            "--command=str_replace",
		"new":            "--command=str_replace",
	}
	for name, want := range wantRequiredWhen {
		parameter := parameters[name]
		if required, _ := parameter["required"].(bool); required {
			t.Errorf("--%s required = true, want conditional requirement", name)
		}
		if got := schemaContractString(parameter["required_when"]); got != want {
			t.Errorf("--%s required_when = %q, want %q", name, got, want)
		}
	}
	for _, alias := range []string{"doc", "text"} {
		if _, exists := parameters[alias]; !exists {
			t.Errorf("visible compatibility alias --%s missing from Schema", alias)
		}
	}
	if constraints, exists := leaf["constraints"]; exists && constraints != nil {
		t.Fatalf("enum-discriminated requirements must not be mispublished as relationship constraints: %#v", constraints)
	}
}

func TestDeliveryDocMediaInsertEntrypointsPublishTheSameGuardrails(t *testing.T) {
	wantConstraints := map[string]any{
		"mutually_exclusive": [][]string{{"index", "where"}, {"index", "ref-block"}},
		"require_together":   [][]string{{"where", "ref-block"}},
	}
	for _, cliPath := range []string{"doc +media-insert", "doc media insert"} {
		leaf := executeShortcutSchemaQuery(t, "--cli-path", cliPath)
		wantConfirmation := "not_required"
		if cliPath == "doc +media-insert" {
			wantConfirmation = "user_required"
		}
		if got := schemaContractString(leaf["confirmation"]); got != wantConfirmation {
			t.Errorf("%s confirmation = %q, want %s", cliPath, got, wantConfirmation)
		}
		parameters := schemaContractMap(leaf["parameters"])
		for _, name := range []string{"node", "file"} {
			if required, _ := parameters[name]["required"].(bool); !required {
				t.Errorf("%s --%s required = %#v, want true", cliPath, name, parameters[name]["required"])
			}
		}
		for name, want := range map[string]string{
			"where":     "--ref-block is provided",
			"ref-block": "--where is provided",
		} {
			if got := schemaContractString(parameters[name]["required_when"]); got != want {
				t.Errorf("%s --%s required_when = %q, want %q", cliPath, name, got, want)
			}
		}
		if got := leaf["constraints"]; !schemaContractJSONEqual(got, wantConstraints) {
			t.Errorf("%s constraints = %#v, want %#v", cliPath, got, wantConstraints)
		}
	}
}

func TestDeliveryDocShortcutAndLeafSafetyMatchesPublishedContracts(t *testing.T) {
	tests := []struct {
		name         string
		effect       string
		risk         string
		confirmation string
		cliPaths     []string
	}{
		{name: "content update shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +update"}},
		{name: "content update leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc update"}},
		{name: "copy shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +copy"}},
		{name: "copy leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc copy"}},
		{name: "move shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +move"}},
		{name: "move leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc move"}},
		{name: "comment create shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +comment-create"}},
		{name: "comment create leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc comment create"}},
		{name: "comment reply shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +comment-reply"}},
		{name: "comment reply leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc comment reply"}},
		{name: "version save shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +version-save"}},
		{name: "version save leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc version save"}},
		{name: "version revert shortcut", effect: "destructive", risk: "high", confirmation: "user_required", cliPaths: []string{"doc +version-revert"}},
		{name: "version revert leaf", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc version revert"}},
		{name: "media insert shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +media-insert"}},
		{name: "media insert leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc media insert"}},
		{name: "cover set shortcut", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc +cover-set"}},
		{name: "cover set leaf", effect: "write", risk: "low", confirmation: "not_required", cliPaths: []string{"doc style cover set"}},
		{name: "cover clear shortcut", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc +cover-clear"}},
		{name: "cover clear leaf", effect: "write", risk: "low", confirmation: "not_required", cliPaths: []string{"doc style cover clear"}},
		{name: "background clear shortcut", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +background-delete"}},
		{name: "background clear leaf", effect: "write", risk: "low", confirmation: "not_required", cliPaths: []string{"doc style background clear"}},
		{name: "permission grant", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +access-grant"}},
		{name: "permission add leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc permission add"}},
		{name: "permission change", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc +access-change"}},
		{name: "permission update leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc permission update"}},
		{name: "permission revoke", effect: "destructive", risk: "high", confirmation: "user_required", cliPaths: []string{"doc +access-revoke"}},
		{name: "permission remove leaf", effect: "write", risk: "medium", confirmation: "not_required", cliPaths: []string{"doc permission remove"}},
		{name: "comment delete", effect: "destructive", risk: "high", confirmation: "user_required", cliPaths: []string{"doc +comment-delete"}},
		{name: "comment delete leaf", effect: "write", risk: "medium", confirmation: "user_required", cliPaths: []string{"doc comment delete"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, cliPath := range test.cliPaths {
				leaf := executeShortcutSchemaQuery(t, "--cli-path", cliPath)
				if got := schemaContractString(leaf["effect"]); got != test.effect {
					t.Errorf("%s effect = %q, want %q", cliPath, got, test.effect)
				}
				if got := schemaContractString(leaf["risk"]); got != test.risk {
					t.Errorf("%s risk = %q, want %q", cliPath, got, test.risk)
				}
				if got := schemaContractString(leaf["confirmation"]); got != test.confirmation {
					t.Errorf("%s confirmation = %q, want %q", cliPath, got, test.confirmation)
				}
			}
		})
	}
}

func TestDeliveryDocFetchPublishesScopeContract(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "doc +fetch")
	parameters := schemaContractMap(leaf["parameters"])
	wantRequiredWhen := map[string]string{
		"keyword":        "--scope=keyword",
		"start-block-id": "--scope=range or --scope=section",
		"tags":           "--scope=tags",
	}
	for name, want := range wantRequiredWhen {
		if got := schemaContractString(parameters[name]["required_when"]); got != want {
			t.Errorf("--%s required_when = %q, want %q", name, got, want)
		}
	}
	constraints, _ := leaf["constraints"].(map[string]any)
	if got := constraints["require_one_of"]; !schemaContractJSONEqual(got, [][]string{{"node", "query"}}) {
		t.Fatalf("fetch require_one_of = %#v", got)
	}
	if got := constraints["mutually_exclusive"]; !schemaContractJSONEqual(got, [][]string{{"node", "query"}}) {
		t.Fatalf("fetch mutually_exclusive = %#v", got)
	}
}

func TestDeliveryDocCommentExportImportContractsAreCanonical(t *testing.T) {
	comment := executeShortcutSchemaQuery(t, "--cli-path", "doc +comment-create")
	commentParameters := schemaContractMap(comment["parameters"])
	for _, name := range []string{"node", "content", "selection", "block-id", "start", "end", "selected-text", "mention"} {
		if _, ok := commentParameters[name]; !ok {
			t.Errorf("comment-create missing --%s: %#v", name, commentParameters)
		}
	}
	for name, want := range map[string]string{"node": "node", "mention": "mention"} {
		if got := schemaContractString(commentParameters[name]["property"]); got != want {
			t.Errorf("comment-create --%s property = %q, want %q", name, got, want)
		}
	}

	export := executeShortcutSchemaQuery(t, "--cli-path", "doc +export")
	exportFormat := schemaContractMap(export["parameters"])["export-format"]
	if required, _ := exportFormat["required"].(bool); required {
		t.Fatalf("export --export-format required = true, want compatibility default")
	}
	if defaultValue := schemaContractString(exportFormat["default"]); defaultValue != "docx" {
		t.Fatalf("export --export-format default = %q, want docx", defaultValue)
	}

	importLeaf := executeShortcutSchemaQuery(t, "--cli-path", "doc +import")
	constraints, _ := importLeaf["constraints"].(map[string]any)
	if got := constraints["require_one_of"]; got != nil {
		t.Fatalf("import require_one_of = %#v, want no required target group", got)
	}
	if got := constraints["mutually_exclusive"]; !schemaContractJSONEqual(got, [][]string{{"folder", "workspace"}}) {
		t.Fatalf("import mutually_exclusive = %#v, want folder/workspace", got)
	}
}

func executeShortcutSchemaQuery(t testing.TB, args ...string) map[string]any {
	t.Helper()
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"schema"}, append(args, "--format", "json")...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute dws schema %q: %v; stderr=%s", strings.Join(args, " "), err, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode dws schema %q: %v", strings.Join(args, " "), err)
	}
	return payload
}

func shortcutSchemaCanonical(declared shortcut.Shortcut) string {
	name := strings.ReplaceAll(strings.TrimPrefix(declared.Command, "+"), "-", "_")
	return declared.Service + ".shortcut_" + name
}

func assertDeliveryShortcutIdentityAndSelection(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	if got, want := schemaContractString(tool["canonical_path"]), canonical; got != want {
		t.Errorf("canonical_path = %q, want %q", got, want)
	}
	if got, want := schemaContractString(tool["primary_cli_path"]), declared.Service+" "+declared.Command; got != want {
		t.Errorf("%s primary_cli_path = %q, want %q", canonical, got, want)
	}
	if got, want := schemaContractString(tool["agent_summary"]), declared.Description; got != want {
		t.Errorf("%s agent_summary = %q, want %q", canonical, got, want)
	}
	if got, want := schemaContractStringSlice(tool["use_when"]), []string{declared.Intent}; !schemaContractJSONEqual(got, want) {
		t.Errorf("%s use_when = %#v, want %#v", canonical, got, want)
	}
	if len(schemaContractStringSlice(tool["avoid_when"])) == 0 {
		t.Errorf("%s has no reviewed avoid_when", canonical)
	}
	examples := schemaContractStringSlice(tool["examples"])
	if len(examples) == 0 || len(examples) > 2 {
		t.Errorf("%s examples = %d, want 1..2", canonical, len(examples))
	}
	for _, example := range examples {
		if strings.Contains(example, "--yes") {
			t.Errorf("%s stores unsafe example %q", canonical, example)
		}
		if !strings.HasPrefix(example, "dws "+declared.Service+" "+declared.Command) {
			t.Errorf("%s example does not use its primary path: %q", canonical, example)
		}
	}
}

func assertDeliveryShortcutSafetyAndInterface(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	safety := shortcut.EffectiveSafety(declared)
	wantEffect, wantRisk := safety.Effect, safety.Risk
	wantConfirmation, wantIdempotency := safety.Confirmation, safety.Idempotency
	for field, want := range map[string]string{
		"effect":         wantEffect,
		"risk":           wantRisk,
		"confirmation":   wantConfirmation,
		"idempotency":    wantIdempotency,
		"interface_mode": "composite",
		"availability":   "available",
	} {
		if got := schemaContractString(tool[field]); got != want {
			t.Errorf("%s %s = %q, want %q", canonical, field, got, want)
		}
	}
	if strings.TrimSpace(schemaContractString(tool["interface_reason"])) == "" {
		t.Errorf("%s has no reviewed composite interface reason", canonical)
	}
}

func assertDeliveryShortcutParameters(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	parameters := schemaContractMap(tool["parameters"])
	publicFlags := make([]shortcut.Flag, 0, len(declared.Flags))
	for _, flag := range declared.Flags {
		if !flag.Hidden {
			publicFlags = append(publicFlags, flag)
			if flag.AliasesVisible {
				for _, alias := range flag.Aliases {
					aliasFlag := flag
					aliasFlag.Name = alias
					aliasFlag.Default = ""
					aliasFlag.Aliases = nil
					publicFlags = append(publicFlags, aliasFlag)
				}
			}
		}
	}
	if got, want := len(parameters), len(publicFlags); got != want {
		t.Errorf("%s parameters = %d, want %d", canonical, got, want)
	}
	for _, flag := range publicFlags {
		parameter := parameters[flag.Name]
		if parameter == nil {
			t.Errorf("%s is missing parameter --%s", canonical, flag.Name)
			continue
		}
		flagType := flag.Type
		if flagType == "" {
			flagType = shortcut.FlagString
		}
		wantType := map[shortcut.FlagType]string{
			shortcut.FlagString:      "string",
			shortcut.FlagBool:        "boolean",
			shortcut.FlagInt:         "integer",
			shortcut.FlagStringSlice: "array",
		}[flagType]
		if got := schemaContractString(parameter["type"]); got != wantType {
			t.Errorf("%s --%s type = %q, want %q", canonical, flag.Name, got, wantType)
		}
		if got, _ := parameter["required"].(bool); got != shortcutSchemaRequired(declared, flag.Name) {
			t.Errorf("%s --%s required = %t, want %t", canonical, flag.Name, got, shortcutSchemaRequired(declared, flag.Name))
		}
		if got, want := schemaContractString(parameter["default"]), shortcutSchemaDefault(flag); got != want {
			t.Errorf("%s --%s default = %q, want %q", canonical, flag.Name, got, want)
		}
		gotEnum := schemaContractStringSlice(parameter["enum"])
		if len(flag.Enum) == 0 {
			if len(gotEnum) != 0 {
				t.Errorf("%s --%s enum = %#v, want empty", canonical, flag.Name, gotEnum)
			}
		} else if !schemaContractJSONEqual(gotEnum, flag.Enum) {
			t.Errorf("%s --%s enum = %#v, want %#v", canonical, flag.Name, gotEnum, flag.Enum)
		}
	}
}

func shortcutSchemaDefault(flag shortcut.Flag) string {
	value := strings.TrimSpace(flag.Default)
	switch flag.Type {
	case shortcut.FlagBool:
		if value != "true" {
			return ""
		}
	case shortcut.FlagInt:
		if value == "0" {
			return ""
		}
	case shortcut.FlagStringSlice:
		if value != "" {
			return "[" + value + "]"
		}
	}
	return value
}

func shortcutSchemaRequired(declared shortcut.Shortcut, flagName string) bool {
	for _, flag := range declared.Flags {
		if flag.Name == flagName && flag.Required {
			return true
		}
		if flag.Required && flag.AliasesVisible {
			for _, alias := range flag.Aliases {
				if alias == flagName {
					return true
				}
			}
		}
	}
	public := make(map[string]bool, len(declared.Flags))
	for _, flag := range declared.Flags {
		if !flag.Hidden {
			public[flag.Name] = true
		}
	}
	for _, constraint := range declared.Constraints {
		if constraint.Kind != shortcut.ConstraintAtLeastOne && constraint.Kind != shortcut.ConstraintExactlyOne {
			continue
		}
		visible := make([]string, 0, len(constraint.Flags))
		for _, constrained := range constraint.Flags {
			if public[constrained] {
				visible = append(visible, constrained)
			}
		}
		// Match AnnotateConstraints: only collapse to required when the projected
		// group has a single member (no remaining hidden siblings).
		flags := visible
		if len(visible) < len(constraint.Flags) {
			flags = append([]string(nil), constraint.Flags...)
		}
		if len(flags) == 1 && flags[0] == flagName {
			return true
		}
	}
	return false
}

func assertDeliveryShortcutConstraints(
	t testing.TB,
	tool map[string]any,
	declared shortcut.Shortcut,
	canonical string,
) {
	t.Helper()
	public := make(map[string]bool, len(declared.Flags))
	for _, flag := range declared.Flags {
		if !flag.Hidden {
			public[flag.Name] = true
		}
	}
	want := map[string][][]string{}
	for _, constraint := range declared.Constraints {
		visible := make([]string, 0, len(constraint.Flags))
		for _, flagName := range constraint.Flags {
			if public[flagName] {
				visible = append(visible, flagName)
			}
		}
		// Match AnnotateConstraints declare≡execute projection: keep the full
		// declared group when any hidden sibling remains.
		flags := visible
		if len(visible) < len(constraint.Flags) {
			flags = append([]string(nil), constraint.Flags...)
		}
		switch constraint.Kind {
		case shortcut.ConstraintAtLeastOne:
			if len(flags) > 1 {
				want["require_one_of"] = append(want["require_one_of"], flags)
			}
		case shortcut.ConstraintExactlyOne:
			if len(flags) > 1 {
				want["require_one_of"] = append(want["require_one_of"], flags)
				want["mutually_exclusive"] = append(want["mutually_exclusive"], flags)
			}
		case shortcut.ConstraintMutuallyExclusive:
			if len(flags) > 1 {
				want["mutually_exclusive"] = append(want["mutually_exclusive"], flags)
			}
		case shortcut.ConstraintRequireTogether:
			if len(flags) > 1 {
				want["require_together"] = append(want["require_together"], flags)
			}
		case shortcut.ConstraintCustom:
			for _, flagName := range flags {
				description := schemaContractString(schemaContractMap(tool["parameters"])[flagName]["description"])
				for _, evidence := range shortcutCustomConstraintEvidence(constraint.Description) {
					if !strings.Contains(description, evidence) {
						t.Errorf("%s --%s description does not publish custom constraint evidence %q: %q", canonical, flagName, evidence, description)
					}
				}
			}
		default:
			t.Errorf("%s has unsupported declared shortcut constraint %q", canonical, constraint.Kind)
		}
	}
	if len(want) == 0 {
		if got := tool["constraints"]; got != nil {
			t.Errorf("%s constraints = %#v, want omitted", canonical, got)
		}
		return
	}
	if got := tool["constraints"]; !schemaContractJSONEqual(got, want) {
		t.Errorf("%s constraints = %s, want %s", canonical, mustShortcutJSON(got), mustShortcutJSON(want))
	}
}

func shortcutCustomConstraintEvidence(description string) []string {
	// Custom constraints are prose rather than a typed wire contract. Require
	// their decision-relevant facts to survive in the delivered parameter
	// description while allowing the renderer to reorder connective wording.
	probes := []string{
		"原文=>替换",
		"不能为空",
		"不能重复",
		"大于 0",
		"工作目录",
		"相对路径",
		"绝对路径",
		"..",
		"最多 15 个字符",
		"能力矩阵",
	}
	evidence := make([]string, 0, len(probes))
	for _, probe := range probes {
		if strings.Contains(description, probe) {
			evidence = append(evidence, probe)
		}
	}
	if len(evidence) > 0 {
		return evidence
	}
	return []string{strings.TrimSpace(description)}
}

func mustShortcutJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(encoded)
}
