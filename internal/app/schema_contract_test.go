// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestEmbeddedSchemaContractMapsToExecutableTree(t *testing.T) {
	root := NewRootCommand()
	report := cli.AnnotateEmbeddedSchemaCommands(root)
	definitions := cli.EmbeddedSchemaCommandDefinitions()
	bindings := cli.EmbeddedSchemaParameterBindings()
	if len(definitions) != 538 {
		t.Fatalf("embedded definitions = %d, want 538", len(definitions))
	}
	if report.Matched != len(definitions) || len(report.Missing) != 0 {
		t.Fatalf("schema annotation report = matched:%d missing:%v", report.Matched, report.Missing)
	}

	seen := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if seen[definition.CanonicalPath] {
			t.Fatalf("duplicate canonical path %q", definition.CanonicalPath)
		}
		seen[definition.CanonicalPath] = true
		command := exactCommandForTest(root, definition.CLIPath)
		if command == nil {
			for _, alias := range definition.Aliases {
				if command = exactCommandForTest(root, alias); command != nil {
					break
				}
			}
		}
		if command == nil {
			t.Errorf("%s has no executable CLI path %q", definition.CanonicalPath, definition.CLIPath)
			continue
		}
		for _, parameter := range definition.Parameters {
			flag := schemaContractCommandFlag(command, parameter.Name)
			if flag == nil {
				t.Errorf("%s maps parameter %q to missing flag on %q", definition.CanonicalPath, parameter.Name, command.CommandPath())
				continue
			}
			if got := schemaContractFlagDefault(flag); parameter.Default != got {
				t.Errorf("%s parameter %q default = %q, Cobra --help default = %q", definition.CanonicalPath, parameter.Name, parameter.Default, got)
			}
		}
		for flagName, propertyName := range bindings[definition.CanonicalPath] {
			flag := schemaContractCommandFlag(command, flagName)
			if flag == nil || flag.Hidden {
				t.Errorf("%s binding --%s references a missing or hidden public flag", definition.CanonicalPath, flagName)
				continue
			}
			var found bool
			for _, parameter := range definition.Parameters {
				if parameter.Name == flagName && parameter.Property == propertyName {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s binding --%s -> %s is absent from generated Catalog", definition.CanonicalPath, flagName, propertyName)
			}
		}
	}
}

func TestRuntimeSchemaParameterMetadataMapsToGeneratedCatalog(t *testing.T) {
	root := NewRootCommand()
	cli.AnnotateEmbeddedSchemaCommands(root)
	allowed := map[string]bool{}
	for _, definition := range cli.EmbeddedSchemaCommandDefinitions() {
		allowed[definition.CanonicalPath] = true
	}
	snapshot, err := cli.BuildSchemaCatalogSnapshot(root, cli.SchemaCatalogBuildOptions{AllowedCanonicalPaths: allowed})
	if err != nil {
		t.Fatal(err)
	}

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
			if value := parameter(flagName); value != nil && value["required"] != true {
				t.Errorf("%s --%s required = %#v", canonicalPath, flagName, value["required"])
			}
		}
		for flagName, want := range metadata.RequiredWhen {
			if value := parameter(flagName); value != nil && value["required_when"] != want {
				t.Errorf("%s --%s required_when = %#v, want %q", canonicalPath, flagName, value["required_when"], want)
			}
		}
		for flagName, want := range metadata.Formats {
			if value := parameter(flagName); value != nil && value["format"] != want {
				t.Errorf("%s --%s format = %#v, want %q", canonicalPath, flagName, value["format"], want)
			}
		}
		for flagName, want := range metadata.Examples {
			if value := parameter(flagName); value != nil && value["example"] != want {
				t.Errorf("%s --%s example = %#v, want %q", canonicalPath, flagName, value["example"], want)
			}
		}
		for flagName, want := range metadata.Enums {
			if value := parameter(flagName); value != nil {
				var gotStrings []string
				switch got := value["enum"].(type) {
				case []string:
					gotStrings = append([]string(nil), got...)
				case []any:
					for _, item := range got {
						gotStrings = append(gotStrings, item.(string))
					}
				}
				if !reflect.DeepEqual(gotStrings, want) {
					t.Errorf("%s --%s enum = %#v, want %#v", canonicalPath, flagName, gotStrings, want)
				}
			}
		}
	}
}

func schemaContractFlagDefault(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}
	value := strings.TrimSpace(flag.DefValue)
	switch flag.Value.Type() {
	case "bool":
		if value == "false" {
			return ""
		}
	case "int", "int8", "int16", "int32", "int64", "float32", "float64":
		if value == "0" {
			return ""
		}
	case "stringSlice", "stringArray":
		if value == "[]" {
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

func TestChatSchemaSeparatesSendAndReply(t *testing.T) {
	definitions := map[string]cli.CatalogCommandDefinition{}
	for _, definition := range cli.EmbeddedSchemaCommandDefinitions() {
		definitions[definition.CanonicalPath] = definition
	}

	send, ok := definitions["chat.send_personal_message"]
	if !ok || send.CLIPath != "chat message send" {
		t.Fatalf("send definition = %#v", send)
	}
	reply, ok := definitions["chat.reply_personal_message"]
	if !ok || reply.CLIPath != "chat message reply" {
		t.Fatalf("reply definition = %#v", reply)
	}
	if reply.SourceProductID != "chat" || reply.RPCName != "send_personal_message" {
		t.Fatalf("reply interface = %s/%s", reply.SourceProductID, reply.RPCName)
	}
	if _, exists := definitions["chat.upload_conversation_file"]; exists {
		t.Fatal("downlined chat file upload must not be advertised in Schema")
	}
}

func TestCalendarAttendeeDeleteSchemaRequiresUserConfirmation(t *testing.T) {
	root := NewRootCommand()
	snapshot, err := cli.BuildSchemaCatalogSnapshot(root, cli.SchemaCatalogBuildOptions{
		AllowedCanonicalPaths: map[string]bool{"calendar.remove_calendar_participant": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := snapshot.Tools["calendar.remove_calendar_participant"]
	if got := tool["risk"]; got != "high" {
		t.Fatalf("calendar attendee delete risk = %#v, want high", got)
	}
	if got := tool["confirmation"]; got != "user_required" {
		t.Fatalf("calendar attendee delete confirmation = %#v, want user_required", got)
	}
}

func TestDefaultedPaginationSchemaFlagsAreOptional(t *testing.T) {
	definitions := map[string]cli.CatalogCommandDefinition{}
	for _, definition := range cli.EmbeddedSchemaCommandDefinitions() {
		definitions[definition.CanonicalPath] = definition
	}
	for canonicalPath, flags := range map[string][]string{
		"chat.search_messages_by_time_range": {"limit"},
		"oa.list_user_visible_process":       {"cursor", "limit"},
		"report.get_received_report_list":    {"cursor", "size"},
		"todo.get_user_todos_in_current_org": {"page"},
	} {
		parameters := map[string]cli.CatalogParameterDefinition{}
		for _, parameter := range definitions[canonicalPath].Parameters {
			parameters[parameter.Name] = parameter
		}
		for _, flagName := range flags {
			if parameters[flagName].Required {
				t.Errorf("%s --%s has a CLI default and must remain optional", canonicalPath, flagName)
			}
		}
	}

	for _, parameter := range definitions["aitable.section_reorder"].Parameters {
		if parameter.Name == "target-index" && !parameter.Required {
			t.Error("aitable.section_reorder --target-index uses -1 as a sentinel and must remain required")
		}
	}
}

func TestPATSchemaKeepsCLIContract(t *testing.T) {
	root := NewRootCommand()
	cli.AnnotateEmbeddedSchemaCommands(root)
	payload, err := cli.BuildSchemaCatalogSnapshot(root, cli.SchemaCatalogBuildOptions{
		AllowedCanonicalPaths: map[string]bool{"pat.batch_grant": true},
	})
	if err != nil {
		t.Fatal(err)
	}
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
