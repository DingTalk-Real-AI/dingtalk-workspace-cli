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
	publicShortcutCount = 438
	// schemaPublishedShortcutCount counts every delivered *.shortcut_* tool,
	// including reviewed hidden compatibility and unavailable contracts.
	schemaPublishedShortcutCount = 495
	// publiclyDeliveredShortcutCount is the public-catalog subset of that surface.
	publiclyDeliveredShortcutCount = 438
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

func TestDeliveryTodoReminderPreservesHistoricalConstraintBoundary(t *testing.T) {
	tool := executeShortcutSchemaQuery(t, "--cli-path", "todo +reminder")
	want := map[string][][]string{
		"require_one_of":     {{"clear", "base-time"}},
		"mutually_exclusive": {{"clear", "base-time"}},
	}
	if got := tool["constraints"]; !schemaContractJSONEqual(got, want) {
		t.Fatalf("todo +reminder constraints = %s, want %s", mustShortcutJSON(got), mustShortcutJSON(want))
	}
	parameters := schemaContractMap(tool["parameters"])
	for _, name := range []string{"base-time", "due-date-offset", "at"} {
		description := schemaContractString(parameters[name]["description"])
		if !strings.Contains(description, "无关时间参数兼容忽略") {
			t.Fatalf("todo +reminder --%s compatibility boundary missing from final Schema: %q", name, description)
		}
	}
}

func TestDeliveryShortcutProgressiveQueriesReturnCompleteContracts(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "chat +messages-read-status")
	if got, want := schemaContractString(leaf["canonical_path"]), "chat.shortcut_messages_read_status"; got != want {
		t.Fatalf("shortcut leaf canonical_path = %q, want %q", got, want)
	}
	if got, want := schemaContractString(leaf["confirmation"]), "not_required"; got != want {
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
	if got, want := int(product["count"].(float64)), 233; got != want {
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

func TestChatPersonalEmotionSchemaDeclaresUnpinnedIMAdapter(t *testing.T) {
	for _, tc := range []struct {
		cliPath string
		params  map[string]string
	}{
		{
			cliPath: "chat emotion list",
		},
		{
			cliPath: "chat emotion send",
			params: map[string]string{
				"media-id":         "mediaId",
				"emotion-id":       "emotionId",
				"group":            "openConversationId",
				"open-dingtalk-id": "receiverOpenDingTalkId",
				"idempotency-key":  "uuid",
			},
		},
		{
			cliPath: "chat emotion favorite",
			params: map[string]string{
				"media-id":               "mediaId",
				"file-path":              "",
				"name":                   "name",
				"source-conversation-id": "sourceConversationId",
				"source-message-id":      "sourceMessageId",
			},
		},
	} {
		t.Run(tc.cliPath, func(t *testing.T) {
			leaf := executeShortcutSchemaQuery(t, "--cli-path", tc.cliPath)
			if got := schemaContractString(leaf["interface_mode"]); got != "composite" {
				t.Fatalf("%s interface_mode = %q, want composite", tc.cliPath, got)
			}
			reason := schemaContractString(leaf["interface_reason"])
			if !strings.Contains(reason, "Reviewed unpinned remote adapter") {
				t.Fatalf("%s interface_reason = %q", tc.cliPath, reason)
			}
			parameters := schemaContractMap(leaf["parameters"])
			for name, want := range tc.params {
				parameter := parameters[name]
				if parameter == nil {
					t.Fatalf("%s missing --%s parameter: %#v", tc.cliPath, name, parameters)
				}
				if got := schemaContractString(parameter["property"]); got != want {
					t.Fatalf("%s --%s property = %q, want %q", tc.cliPath, name, got, want)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAITableTableBootstrapPublishesResultContract(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "aitable +table-bootstrap")
	result, _ := leaf["result"].(map[string]any)
	if got, want := schemaContractStringSlice(result["outcomes"]), []string{"success", "failure"}; !schemaContractJSONEqual(got, want) {
		t.Fatalf("aitable +table-bootstrap outcomes = %#v, want %#v", got, want)
	}
	dataSchema, _ := result["data_schema"].(map[string]any)
	properties := schemaContractMap(dataSchema["properties"])
	status := properties["status"]
	if got, want := schemaContractStringSlice(status["enum"]), []string{"success", "planned", "partial_success", "unknown"}; !schemaContractJSONEqual(got, want) {
		t.Fatalf("aitable +table-bootstrap status enum = %#v, want %#v", got, want)
	}
	for _, property := range []string{"contractVersion", "operation", "executed", "retryable", "plan", "completedSteps", "verification", "checkpoint", "knownSideEffects", "result"} {
		if properties[property] == nil {
			t.Errorf("aitable +table-bootstrap final Result data_schema is missing %q", property)
		}
	}
}

func TestDeliveryWikiSpaceSearchDeclaresCompatibilityAdapter(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "wiki +space-search")
	if got := schemaContractString(leaf["interface_mode"]); got != "composite" {
		t.Fatalf("wiki +space-search interface_mode = %q, want composite", got)
	}
	reason := schemaContractString(leaf["interface_reason"])
	for _, fragment := range []string{"query/limit", "search_wikiSpaces.keyword/pageSize", "versioned Schema migration"} {
		if !strings.Contains(reason, fragment) {
			t.Fatalf("wiki +space-search interface_reason = %q, want fragment %q", reason, fragment)
		}
	}
	parameters := schemaContractMap(leaf["parameters"])
	for name, want := range map[string]string{"query": "query", "limit": "limit"} {
		parameter := parameters[name]
		if parameter == nil {
			t.Fatalf("wiki +space-search missing --%s parameter: %#v", name, parameters)
		}
		if got := schemaContractString(parameter["property"]); got != want {
			t.Fatalf("wiki +space-search --%s property = %q, want compatibility value %q", name, got, want)
		}
	}
}

func TestCrossPlatformCoverageWikiSpaceCreatePublishesVerifiedTypeResult(t *testing.T) {
	leaf := executeShortcutSchemaQuery(t, "--cli-path", "wiki +space-create")
	result, _ := leaf["result"].(map[string]any)
	if got, want := schemaContractStringSlice(result["outcomes"]), []string{"success", "partial_failure"}; !schemaContractJSONEqual(got, want) {
		t.Fatalf("wiki +space-create outcomes = %#v, want %#v", got, want)
	}
	dataSchema, _ := result["data_schema"].(map[string]any)
	properties := schemaContractMap(dataSchema["properties"])
	for _, property := range []string{"success", "workspaceId", "space", "spaceType", "spaceTypeVerified", "spaceTypeEvidence"} {
		if properties[property] == nil {
			t.Errorf("wiki +space-create Result data_schema is missing %q", property)
		}
	}
	if got, want := schemaContractStringSlice(properties["spaceType"]["enum"]), []string{"orgWikiSpace", "myWikiSpace"}; !schemaContractJSONEqual(got, want) {
		t.Fatalf("wiki +space-create spaceType enum = %#v, want %#v", got, want)
	}
}

func TestAllShortcutsWikiSchemaExamplesIncludeRequiredParameters(t *testing.T) {
	tools := deliverySchemaAllToolsForHelpFlagTest(t, NewRootCommand())
	checked := 0
	for _, declared := range shortcut.All() {
		if declared.Service != "wiki" || declared.UserDefined || !shortcut.InPublicCatalog(declared.Service, declared.Command) {
			continue
		}
		checked++
		canonical := shortcutSchemaCanonical(declared)
		tool := tools[canonical]
		if tool == nil {
			t.Fatalf("delivery schema --all is missing %s", canonical)
		}
		examples := schemaContractStringSlice(tool["examples"])
		if len(examples) == 0 {
			t.Fatalf("%s has no delivered examples", canonical)
		}
		for _, example := range examples {
			argv, err := cli.ParseAgentExampleArgv(example)
			if err != nil {
				t.Fatalf("%s example %q is not valid argv: %v", canonical, example, err)
			}
			for _, flag := range declared.Flags {
				if !flag.Required {
					continue
				}
				names := append([]string{flag.Name}, flag.Aliases...)
				if !schemaExampleHasLongFlag(argv, names...) {
					t.Errorf("%s example %q is missing required --%s", canonical, example, flag.Name)
				}
			}
		}
	}
	if checked != 20 {
		t.Fatalf("checked Wiki shortcut examples = %d, want 20", checked)
	}
}

func TestAllShortcutsAITableDatasourceExamplesSourceConfigHasRequiredMembers(t *testing.T) {
	tools := deliverySchemaAllToolsForHelpFlagTest(t, NewRootCommand())
	requiredSourceConfigMembers := []string{"processCode", "name", "iconUrl", "url"}
	checked := 0
	for _, declared := range shortcut.All() {
		if declared.Service != "aitable" || declared.UserDefined || !shortcut.InPublicCatalog(declared.Service, declared.Command) {
			continue
		}
		if !strings.HasPrefix(declared.Command, "+datasource-") {
			continue
		}
		if declared.Command != "+datasource-create" && declared.Command != "+datasource-update" && declared.Command != "+datasource-get-fields" {
			continue
		}
		checked++
		canonical := shortcutSchemaCanonical(declared)
		tool := tools[canonical]
		if tool == nil {
			t.Fatalf("delivery schema --all is missing %s", canonical)
		}
		examples := schemaContractStringSlice(tool["examples"])
		if len(examples) == 0 {
			t.Fatalf("%s has no delivered examples", canonical)
		}
		for _, example := range examples {
			if !strings.Contains(example, "--source-config") {
				continue
			}
			argv, err := cli.ParseAgentExampleArgv(example)
			if err != nil {
				t.Fatalf("%s example %q is not valid argv: %v", canonical, example, err)
			}
			sourceConfig := schemaExampleFlagValue(argv, "source-config")
			if sourceConfig == "" {
				t.Errorf("%s example %q contains --source-config but has no value", canonical, example)
				continue
			}
			var cfg map[string]any
			if err := json.Unmarshal([]byte(sourceConfig), &cfg); err != nil {
				t.Errorf("%s example %q has invalid source-config JSON: %v", canonical, example, err)
				continue
			}
			for _, member := range requiredSourceConfigMembers {
				if _, ok := cfg[member]; !ok {
					t.Errorf("%s example %q source-config is missing required member %q", canonical, example, member)
				}
			}
		}
	}
	if checked != 3 {
		t.Fatalf("checked aitable datasource source-config examples = %d, want 3", checked)
	}
}

func schemaExampleHasLongFlag(argv []string, names ...string) bool {
	for _, argument := range argv {
		for _, name := range names {
			if argument == "--"+name || strings.HasPrefix(argument, "--"+name+"=") {
				return true
			}
		}
	}
	return false
}

func schemaExampleFlagValue(argv []string, name string) string {
	prefix := "--" + name + "="
	for _, argument := range argv {
		if argument == "--"+name {
			continue
		}
		if strings.HasPrefix(argument, prefix) {
			return strings.TrimPrefix(argument, prefix)
		}
	}
	// Value may be in the next argv entry: `--flag value` form.
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == "--"+name {
			return argv[i+1]
		}
	}
	return ""
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
	assertSchemaLeafParameterRequired(t, auditJoin, "chat group audit-join-validation", "conversation-id", true)
	assertSchemaLeafParameterEnum(t, auditJoin, "chat group audit-join-validation", "status", []string{"AuditApprove", "AuditDelete"})
	if parameters := schemaContractMap(auditJoin["parameters"]); parameters["group"] != nil {
		t.Fatalf("chat group audit-join-validation publishes hidden --group alias: %#v", parameters["group"])
	}
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
	if got, want := schemaContractString(leaf["confirmation"]), "user_required"; got != want {
		t.Fatalf("confirmation = %q, want %q", got, want)
	}
	parameters := schemaContractMap(leaf["parameters"])
	if got, want := len(parameters), 13; got != want {
		t.Fatalf("parameter count = %d, want %d: %#v", got, want, parameters)
	}
	if required, _ := parameters["node"]["required"].(bool); !required {
		t.Errorf("--node required = %#v, want true", parameters["node"]["required"])
	}
	if required, _ := parameters["command"]["required"].(bool); required {
		t.Errorf("--command required = true, want runtime custom validation")
	}
	wantProperties := map[string]string{
		"node": "node", "doc": "node", "command": "command", "content": "content", "text": "content", "doc-format": "docFormat",
		"block-id": "blockId", "after-block-id": "afterBlockId", "before-block-id": "beforeBlockId", "heading-level": "headingLevel", "old": "old", "new": "new",
		"expected-revision": "expectedRevision",
	}
	for name, want := range wantProperties {
		if got := schemaContractString(parameters[name]["property"]); got != want {
			t.Errorf("--%s property = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"content", "block-id", "after-block-id", "before-block-id", "heading-level", "old", "new"} {
		parameter := parameters[name]
		if required, _ := parameter["required"].(bool); required {
			t.Errorf("--%s required = true, want runtime custom validation", name)
		}
		if got := schemaContractString(parameter["required_when"]); got != "" {
			t.Errorf("--%s required_when = %q, want compatibility-safe custom validation", name, got)
		}
	}
	if got, want := schemaContractStringSlice(parameters["command"]["enum"]), []string{"append", "overwrite", "block_insert_before", "block_insert_after", "block_replace", "block_delete", "str_replace", "block_copy_insert_after"}; !schemaContractJSONEqual(got, want) {
		t.Errorf("--command enum = %#v, want %#v", got, want)
	}
	if constraints, exists := leaf["constraints"]; exists && constraints != nil {
		t.Fatalf("enum-discriminated requirements must not be mispublished as relationship constraints: %#v", constraints)
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
	if requireOneOf, ok := constraints["require_one_of"]; ok && !schemaContractJSONEqual(requireOneOf, [][]string{}) {
		t.Fatalf("import unexpectedly requires a target: %#v", constraints)
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
