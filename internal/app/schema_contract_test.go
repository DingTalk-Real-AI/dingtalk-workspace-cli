// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	fullSchemaSnapshotOnce sync.Once
	fullSchemaSnapshot     cli.SchemaCatalogSnapshot
	fullSchemaSnapshotErr  error
)

// fullSchemaSnapshotForTest runs the complete source-to-delivery invariant
// once per test binary. The returned snapshot is a shared read-only fixture;
// callers still build their own Cobra root when they need executable command
// pointers. This preserves every full-Catalog assertion without paying the
// multi-gigabyte generation/validation cost three times under -race.
func fullSchemaSnapshotForTest(t testing.TB) cli.SchemaCatalogSnapshot {
	t.Helper()
	fullSchemaSnapshotOnce.Do(func() {
		resolved, err := cli.ResolveSchemaBuild(NewRootCommand())
		if err != nil {
			fullSchemaSnapshotErr = err
			return
		}
		fullSchemaSnapshot, fullSchemaSnapshotErr = cli.BuildSchemaCatalogSnapshot(resolved, cli.SchemaCatalogBuildOptions{})
	})
	if fullSchemaSnapshotErr != nil {
		t.Fatalf("build shared final Schema snapshot: %v", fullSchemaSnapshotErr)
	}
	return fullSchemaSnapshot
}

func TestEmbeddedSchemaContractMapsToExecutableTree(t *testing.T) {
	root := NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	expected := make(map[string]bool)
	for _, command := range effective.Commands {
		if command.Visibility == cli.SchemaVisibilityPublic {
			expected[command.CanonicalPath] = true
		}
	}

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"schema", "--all", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute embedded schema --all: %v; stderr=%s", err, stderr.String())
	}
	var payload struct {
		Products []struct {
			Tools []struct {
				CanonicalPath string `json:"canonical_path"`
			} `json:"tools"`
		} `json:"products"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode embedded schema --all: %v", err)
	}
	actual := make(map[string]bool)
	var duplicates []string
	for _, product := range payload.Products {
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(tool.CanonicalPath)
			if canonical == "" {
				t.Fatal("embedded schema --all contains an empty canonical path")
			}
			if actual[canonical] {
				duplicates = append(duplicates, canonical)
			}
			actual[canonical] = true
		}
	}
	if len(duplicates) > 0 {
		sort.Strings(duplicates)
		t.Fatalf("embedded schema --all contains duplicate canonicals: %v", duplicates)
	}

	var missing, extra []string
	for canonical := range expected {
		if !actual[canonical] {
			missing = append(missing, canonical)
		}
	}
	for canonical := range actual {
		if !expected[canonical] {
			extra = append(extra, canonical)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		sort.Strings(missing)
		sort.Strings(extra)
		t.Fatalf("embedded Schema canonical set differs from EffectiveCommandRegistry: missing=%v extra=%v", missing, extra)
	}
}

func TestGeneratedSchemaContractMapsToExecutableTree(t *testing.T) {
	root := NewRootCommand()
	snapshot := fullSchemaSnapshotForTest(t)
	bindings, err := cli.EmbeddedSchemaParameterBindings()
	if err != nil {
		t.Fatalf("EmbeddedSchemaParameterBindings() error = %v", err)
	}
	if len(snapshot.Tools) == 0 {
		t.Fatal("generated Schema Catalog contains no tools")
	}
	for canonicalPath := range bindings {
		if _, ok := snapshot.Tools[canonicalPath]; !ok {
			t.Errorf("parameter bindings reference canonical %q that is absent from the final generated Schema", canonicalPath)
		}
	}

	for canonicalPath, definition := range snapshot.Tools {
		cliPath := schemaContractString(definition["primary_cli_path"])
		if cliPath == "" {
			cliPath = schemaContractString(definition["cli_path"])
		}
		command := exactCommandForTest(root, cliPath)
		if command == nil {
			for _, alias := range schemaContractStringSlice(definition["aliases"]) {
				if command = exactCommandForTest(root, alias); command != nil {
					break
				}
			}
		}
		if command == nil {
			t.Errorf("%s has no executable CLI path %q", canonicalPath, cliPath)
			continue
		}
		for parameterName, rawParameter := range schemaContractMap(definition["parameters"]) {
			flag := schemaContractCommandFlag(command, parameterName)
			if flag == nil {
				t.Errorf("%s maps parameter %q to missing flag on %q", canonicalPath, parameterName, command.CommandPath())
				continue
			}
			if got, want := schemaContractFlagDefault(flag), schemaContractString(rawParameter["default"]); want != got {
				t.Errorf("%s parameter %q default = %q, Cobra --help default = %q", canonicalPath, parameterName, want, got)
			}
		}
		for flagName, propertyName := range bindings[canonicalPath] {
			flag := schemaContractCommandFlag(command, flagName)
			if flag == nil || flag.Hidden {
				t.Errorf("%s binding --%s references a missing or hidden public flag", canonicalPath, flagName)
				continue
			}
			parameter := schemaContractMap(definition["parameters"])[flagName]
			if parameter == nil || schemaContractString(parameter["property"]) != propertyName {
				t.Errorf("%s binding --%s -> %s is absent from generated Catalog", canonicalPath, flagName, propertyName)
			}
		}
	}
}

func TestRuntimeSchemaParameterMetadataMapsToGeneratedCatalog(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)

	for canonicalPath, metadata := range cli.RuntimeSchemaParameterMetadataDefinitions() {
		tool := snapshot.Tools[canonicalPath]
		if tool == nil {
			t.Errorf("parameter metadata references unknown tool %q", canonicalPath)
			continue
		}
		parameters, _ := tool["parameters"].(map[string]any)
		parameter := func(flagName string) map[string]any {
			value, _ := parameters[flagName].(map[string]any)
			if value == nil {
				t.Errorf("%s parameter metadata references unknown flag --%s", canonicalPath, flagName)
			}
			return value
		}
		for _, flagName := range metadata.Inherited {
			parameter(flagName)
		}
		for _, flagName := range metadata.Required {
			if value := parameter(flagName); value != nil {
				assertRuntimeSchemaMetadataCandidate(t, canonicalPath, flagName, value, "required", true)
			}
		}
		for flagName, want := range metadata.RequiredWhen {
			if value := parameter(flagName); value != nil {
				assertRuntimeSchemaMetadataCandidate(t, canonicalPath, flagName, value, "required_when", want)
			}
		}
		for flagName, want := range metadata.Formats {
			if value := parameter(flagName); value != nil {
				assertRuntimeSchemaMetadataCandidate(t, canonicalPath, flagName, value, "format", want)
			}
		}
		for flagName, want := range metadata.Examples {
			if value := parameter(flagName); value != nil {
				assertRuntimeSchemaMetadataCandidate(t, canonicalPath, flagName, value, "example", want)
			}
		}
		for flagName, want := range metadata.Enums {
			if value := parameter(flagName); value != nil {
				assertRuntimeSchemaMetadataCandidate(t, canonicalPath, flagName, value, "enum", want)
			}
		}
	}
}

// assertRuntimeSchemaMetadataCandidate verifies the field-level resolver
// contract without assuming which source wins. Typed runtime metadata must
// remain visible as a candidate, resolution must select exactly one candidate,
// and the final payload/envelope must agree with that selected candidate.
func assertRuntimeSchemaMetadataCandidate(t testing.TB, canonicalPath, flagName string, parameter map[string]any, field string, typedValue any) {
	t.Helper()
	fieldProvenance, _ := parameter["field_provenance"].(map[string]any)
	provenance, _ := fieldProvenance[field].(map[string]any)
	if provenance == nil {
		t.Errorf("%s --%s %s has no final resolver provenance", canonicalPath, flagName, field)
		return
	}

	foundTypedCandidate := false
	var selected map[string]any
	selectedCount := 0
	for _, candidate := range schemaContractObjectSlice(provenance["candidates"]) {
		if candidate["source"] == "typed_parameter_metadata" && schemaContractJSONEqual(candidate["value"], typedValue) {
			foundTypedCandidate = true
		}
		if isSelected, _ := candidate["selected"].(bool); isSelected {
			selected = candidate
			selectedCount++
		}
	}
	if !foundTypedCandidate {
		t.Errorf("%s --%s %s provenance has no typed_parameter_metadata candidate with value %#v", canonicalPath, flagName, field, typedValue)
	}
	if selectedCount != 1 {
		t.Errorf("%s --%s %s provenance selected candidates = %d, want 1", canonicalPath, flagName, field, selectedCount)
		return
	}
	if got, want := parameter[field], selected["value"]; !schemaContractJSONEqual(got, want) {
		t.Errorf("%s --%s final %s = %#v, selected provenance value = %#v", canonicalPath, flagName, field, got, want)
	}
	if got, want := provenance["value"], selected["value"]; !schemaContractJSONEqual(got, want) {
		t.Errorf("%s --%s %s provenance value = %#v, selected candidate value = %#v", canonicalPath, flagName, field, got, want)
	}
	if got, want := provenance["precedence"], selected["precedence"]; got != want {
		t.Errorf("%s --%s %s provenance precedence = %#v, selected candidate precedence = %#v", canonicalPath, flagName, field, got, want)
	}
}

func schemaContractJSONEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func schemaContractObjectSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				out = append(out, object)
			}
		}
		return out
	default:
		return nil
	}
}

func schemaContractFlagDefault(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}
	value := strings.TrimSpace(flag.DefValue)
	if value == "" || value == "0s" || value == "[]" || value == "{}" {
		return ""
	}
	switch flag.Value.Type() {
	case "bool":
		if value == "false" {
			return ""
		}
	case "int", "int8", "int16", "int32", "int64", "float32", "float64":
		if value == "0" {
			return ""
		}
	}
	return value
}

func schemaContractCommandFlag(command *cobra.Command, name string) *pflag.Flag {
	if command == nil {
		return nil
	}
	if flag := command.Flags().Lookup(name); flag != nil {
		return flag
	}
	for current := command; current != nil; current = current.Parent() {
		if flag := current.PersistentFlags().Lookup(name); flag != nil {
			return flag
		}
	}
	return nil
}

func schemaContractMap(value any) map[string]map[string]any {
	switch typed := value.(type) {
	case map[string]map[string]any:
		return typed
	case map[string]any:
		out := make(map[string]map[string]any, len(typed))
		for key, item := range typed {
			if object, ok := item.(map[string]any); ok {
				out[key] = object
			}
		}
		return out
	default:
		return nil
	}
}

func schemaContractString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func schemaContractStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

// schemaContractPayloadForBoundCanonicals builds a small test fixture by
// selecting already validated BoundCommand entries before Schema assembly.
// Production generation deliberately has no post-assembly subset option: its
// delivered set must always equal the public EffectiveCommandRegistry set.
func schemaContractPayloadForBoundCanonicals(t *testing.T, root *cobra.Command, canonicals ...string) cli.SchemaSnapshotPayload {
	t.Helper()
	if _, err := cli.ApplyEmbeddedManualSchemaHints(root); err != nil {
		t.Fatalf("apply manual Schema hints: %v", err)
	}
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("build effective CommandRegistry: %v", err)
	}
	fixtureRegistry := cli.EffectiveCommandRegistry{Commands: make([]cli.CommandSpec, 0, len(canonicals))}
	for _, canonical := range canonicals {
		command, ok := effective.ByCanonical[canonical]
		if !ok {
			t.Fatalf("effective fixture has no canonical %s", canonical)
		}
		fixtureRegistry.Commands = append(fixtureRegistry.Commands, command)
	}
	fixture, err := cli.BindEffectiveCommandRegistry(root, fixtureRegistry)
	if err != nil {
		t.Fatalf("bind synthetic effective CommandRegistry: %v", err)
	}
	registry, err := cli.AssembleSchemaRegistryFromBound(fixture)
	if err != nil {
		t.Fatalf("assemble synthetic bound Schema registry: %v", err)
	}
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatalf("render synthetic bound Schema registry: %v", err)
	}
	return payload
}

func TestChatSchemaSeparatesSendAndReply(t *testing.T) {
	snapshot := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(),
		"chat.send_personal_message",
		"chat.reply_personal_message",
	)

	send, ok := snapshot.Tools["chat.send_personal_message"]
	if !ok || schemaContractString(send["primary_cli_path"]) != "chat message send" {
		t.Fatalf("send definition = %#v", send)
	}
	reply, ok := snapshot.Tools["chat.reply_personal_message"]
	if !ok || schemaContractString(reply["primary_cli_path"]) != "chat message reply" {
		t.Fatalf("reply definition = %#v", reply)
	}
	interfaceRef, _ := reply["interface_ref"].(map[string]any)
	if schemaContractString(interfaceRef["product_id"]) != "chat" || schemaContractString(interfaceRef["rpc_name"]) != "send_personal_message" {
		t.Fatalf("reply interface = %#v", interfaceRef)
	}
	if _, exists := snapshot.Tools["chat.upload_conversation_file"]; exists {
		t.Fatal("downlined chat file upload must not be advertised in Schema")
	}
}

func TestCalendarAttendeeDeleteSchemaMatchesRuntimeGate(t *testing.T) {
	root := NewRootCommand()
	snapshot := schemaContractPayloadForBoundCanonicals(t, root, "calendar.remove_calendar_participant")
	tool := snapshot.Tools["calendar.remove_calendar_participant"]
	// Runtime does not gate this path today; Schema confirmation must follow metadata.runtime_gate.
	if got := tool["confirmation"]; got != "not_required" {
		t.Fatalf("calendar attendee delete confirmation = %#v, want not_required", got)
	}
	if got := tool["risk"]; got != "medium" {
		t.Fatalf("calendar attendee delete risk = %#v, want medium", got)
	}
}

func TestPromptingWritesRequireUserConfirmation(t *testing.T) {
	wantEffects := map[string]string{
		"attendance.class_create":  "write",
		"attendance.class_update":  "write",
		"doc.delete_comment":       "write",
		"doc.version_revert":       "write",
		"drive.publish_set":        "write",
		"drive.publish_unset":      "write",
		"sheet.chart_delete":       "write",
		"sheet.delete_pivot_table": "write",
	}
	wantRisks := map[string]string{
		"attendance.class_create":  "medium",
		"attendance.class_update":  "medium",
		"doc.delete_comment":       "medium",
		"doc.version_revert":       "medium",
		"drive.publish_set":        "medium",
		"drive.publish_unset":      "medium",
		"sheet.chart_delete":       "medium",
		"sheet.delete_pivot_table": "medium",
	}
	wantSources := map[string]string{
		"attendance.class_create":  "internal/cli/schema_hints/metadata/attendance.json",
		"attendance.class_update":  "internal/cli/schema_hints/metadata/attendance.json",
		"doc.delete_comment":       "internal/cli/schema_hints/metadata/doc.json",
		"doc.version_revert":       "internal/cli/schema_hints/metadata/doc.json",
		"drive.publish_set":        "internal/cli/schema_hints/metadata/drive.json",
		"drive.publish_unset":      "internal/cli/schema_hints/metadata/drive.json",
		"sheet.chart_delete":       "internal/cli/schema_hints/metadata/sheet.json",
		"sheet.delete_pivot_table": "internal/cli/schema_hints/metadata/sheet.json",
	}
	canonicals := make([]string, 0, len(wantEffects))
	for canonical := range wantEffects {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)
	snapshot := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for _, canonical := range canonicals {
		tool := snapshot.Tools[canonical]
		if got := tool["effect"]; got != wantEffects[canonical] {
			t.Errorf("%s effect = %#v, want %s", canonical, got, wantEffects[canonical])
		}
		if got := tool["risk"]; got != wantRisks[canonical] {
			t.Errorf("%s risk = %#v, want %s", canonical, got, wantRisks[canonical])
		}
		if got := tool["confirmation"]; got != "user_required" {
			t.Errorf("%s confirmation = %#v, want user_required", canonical, got)
		}
		provenance, _ := tool["field_provenance"].(map[string]any)
		for _, field := range []string{"effect", "risk", "confirmation"} {
			selected, _ := provenance[field].(map[string]any)
			if got := selected["source"]; got != wantSources[canonical] {
				t.Errorf("%s %s provenance source = %#v, want %s", canonical, field, got, wantSources[canonical])
			}
		}
	}
}

func TestNewMainCommandInterfaceConversionsReachFinalSchema(t *testing.T) {
	type conversion struct {
		canonical     string
		flag          string
		cliType       string
		interfaceType string
	}
	wants := []conversion{
		{canonical: "chat.list_message_favorites", flag: "size", cliType: "integer", interfaceType: "string"},
		{canonical: "doc.update_comment", flag: "mention", cliType: "string", interfaceType: "array"},
		{canonical: "sheet.create_pivot_table", flag: "properties", cliType: "string", interfaceType: "object"},
		{canonical: "sheet.table_put", flag: "sheets", cliType: "string", interfaceType: "array"},
		{canonical: "sheet.update_pivot_table", flag: "properties", cliType: "string", interfaceType: "object"},
	}
	canonicals := make([]string, 0, len(wants))
	for _, want := range wants {
		canonicals = append(canonicals, want.canonical)
	}
	snapshot := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for _, want := range wants {
		tool := snapshot.Tools[want.canonical]
		parameters, _ := tool["parameters"].(map[string]any)
		parameter, _ := parameters[want.flag].(map[string]any)
		if got := schemaContractString(parameter["type"]); got != want.cliType {
			t.Errorf("%s --%s CLI type = %q, want %q", want.canonical, want.flag, got, want.cliType)
		}
		if got := schemaContractString(parameter["interface_type"]); got != want.interfaceType {
			t.Errorf("%s --%s interface_type = %q, want %q", want.canonical, want.flag, got, want.interfaceType)
		}
	}
	if got := snapshot.Tools["sheet.create_pivot_table"]["idempotency"]; got != "non_idempotent" {
		t.Errorf("sheet.create_pivot_table idempotency = %#v, want non_idempotent", got)
	}
}

func TestDefaultedPaginationSchemaFlagsAreOptional(t *testing.T) {
	wants := map[string][]string{
		"chat.search_messages_by_time_range": {"limit"},
		"oa.list_user_visible_process":       {"cursor", "limit"},
		"report.get_received_report_list":    {"cursor", "size"},
		"todo.get_user_todos_in_current_org": {"page"},
	}
	canonicals := make([]string, 0, len(wants)+1)
	for canonicalPath := range wants {
		canonicals = append(canonicals, canonicalPath)
	}
	canonicals = append(canonicals, "aitable.section_reorder")
	sort.Strings(canonicals)
	snapshot := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for canonicalPath, flags := range wants {
		parameters := schemaContractMap(snapshot.Tools[canonicalPath]["parameters"])
		for _, flagName := range flags {
			if parameters[flagName]["required"] == true {
				t.Errorf("%s --%s has a CLI default and must remain optional", canonicalPath, flagName)
			}
		}
	}

	targetIndex := schemaContractMap(snapshot.Tools["aitable.section_reorder"]["parameters"])["target-index"]
	if targetIndex["required"] != true {
		t.Error("aitable.section_reorder --target-index uses -1 as a sentinel and must remain required")
	}
}

func TestPATSchemaKeepsCLIContract(t *testing.T) {
	root := NewRootCommand()
	payload := schemaContractPayloadForBoundCanonicals(t, root, "pat.batch_grant")
	tool := payload.Tools["pat.batch_grant"]
	parameters, _ := tool["parameters"].(map[string]any)
	grantType, _ := parameters["grant-type"].(map[string]any)
	if grantType["default"] != "permanent" {
		t.Fatalf("grant-type default = %#v", grantType["default"])
	}
	positionals, _ := tool["positionals"].([]any)
	if len(positionals) != 1 {
		t.Fatalf("PAT positionals = %#v", tool["positionals"])
	}
	positional, _ := positionals[0].(map[string]any)
	if positional["name"] != "scope" || positional["variadic"] != true {
		t.Fatalf("PAT positionals = %#v", tool["positionals"])
	}
}

func exactCommandForTest(root *cobra.Command, path string) *cobra.Command {
	parts := strings.Fields(strings.TrimSpace(path))
	if len(parts) > 0 && parts[0] == root.Name() {
		parts = parts[1:]
	}
	current := root
	for _, name := range parts {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	if current == root {
		return nil
	}
	return current
}
