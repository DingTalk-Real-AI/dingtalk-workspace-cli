// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
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
	if len(definitions) != 504 {
		t.Fatalf("embedded definitions = %d, want 504", len(definitions))
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
			flag := command.Flags().Lookup(parameter.Name)
			if flag == nil {
				t.Errorf("%s maps parameter %q to missing flag on %q", definition.CanonicalPath, parameter.Name, command.CommandPath())
				continue
			}
			if got := schemaContractFlagDefault(flag); parameter.Default != got {
				t.Errorf("%s parameter %q default = %q, Cobra --help default = %q", definition.CanonicalPath, parameter.Name, parameter.Default, got)
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
