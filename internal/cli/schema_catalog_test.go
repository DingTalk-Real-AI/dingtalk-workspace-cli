// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestEmbeddedSchemaCatalogIntegrity(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	if !embeddedSchemaCatalogAvailable() {
		t.Fatal("embedded schema catalog is unavailable or failed integrity validation")
	}
	if got, want := len(loaded.Snapshot.Tools), 537; got != want {
		t.Fatalf("embedded tools = %d, want %d", got, want)
	}
	if got, want := len(loaded.Products), 20; got != want {
		t.Fatalf("embedded products = %d, want %d", got, want)
	}
	if got := schemaString(loaded.Snapshot.Catalog["source"]); got != "embedded-command-catalog" {
		t.Fatalf("catalog source = %q", got)
	}
}

func TestEmbeddedSchemaCatalogProgressiveQueries(t *testing.T) {
	overview, err := embeddedSchemaPayload(nil)
	if err != nil {
		t.Fatal(err)
	}
	compact := compactSchemaOverviewPayload(overview)
	if got, want := schemaProductToolCount(map[string]any{"tools": compact["products"]}), 20; got != want {
		t.Fatalf("compact product count = %d, want %d", got, want)
	}

	leaf, err := embeddedSchemaPayload([]string{"calendar event create"})
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaString(leaf["canonical_path"]); got != "calendar.create_calendar_event" {
		t.Fatalf("canonical path = %q", got)
	}
	if len(schemaMapSlice(leaf["parameters"])) != 0 {
		t.Fatal("parameters unexpectedly decoded as a list")
	}
	if parameters, ok := leaf["parameters"].(map[string]any); !ok || len(parameters) == 0 {
		t.Fatal("calendar.create_event parameters are empty")
	}

	group, err := embeddedSchemaPayload([]string{"calendar.event"})
	if err != nil {
		t.Fatal(err)
	}
	if schemaProductToolCount(map[string]any{"tools": group["tools"]}) == 0 {
		t.Fatal("calendar.event group is empty")
	}

	alias, err := embeddedSchemaPayload([]string{"aitable record list"})
	if err != nil {
		t.Fatal(err)
	}
	if alias["is_alias"] != true || schemaString(alias["cli_path"]) != "aitable record list" {
		t.Fatalf("alias query did not preserve compatibility path: %#v", alias)
	}
	if schemaString(alias["canonical_path"]) != "aitable.query_records" {
		t.Fatalf("alias canonical path = %q", schemaString(alias["canonical_path"]))
	}
}

func TestCompatibleSchemaCommandPrefersMatchingAnnotatedAlias(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	chat := &cobra.Command{Use: "chat"}
	message := &cobra.Command{Use: "message"}
	send := &cobra.Command{Use: "send", Run: func(*cobra.Command, []string) {}}
	reply := &cobra.Command{Use: "reply", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(send, "chat", "send_personal_message", "hardcoded:chat")
	AttachRuntimeSchema(reply, "chat", "reply_personal_message", "hardcoded:chat")
	message.AddCommand(send, reply)
	chat.AddCommand(message)
	root.AddCommand(chat)

	definition := CatalogCommandDefinition{
		ProductID: "chat",
		ToolName:  "send_personal_message",
		CLIPath:   "chat message reply",
		Aliases:   []string{"chat message send"},
	}
	if got := compatibleSchemaCommand(root, definition.CLIPath, definition); got != nil {
		t.Fatalf("conflicting primary matched %s", got.CommandPath())
	}
	if got := compatibleSchemaCommand(root, definition.Aliases[0], definition); got != send {
		t.Fatalf("matching alias = %v, want send", got)
	}
}

func TestEmbeddedCatalogBackfillsIdentityWithoutReplayingRealFlags(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	aitable := &cobra.Command{Use: "aitable"}
	workflow := &cobra.Command{Use: "workflow"}
	list := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	list.Flags().Int("limit", 20, "optional page size")
	workflow.AddCommand(list)
	aitable.AddCommand(workflow)
	root.AddCommand(aitable)

	AnnotateEmbeddedSchemaCommands(root)
	productID, toolName, _ := runtimeSchemaAnnotations(list)
	if productID != "aitable" || toolName != "workflow_list" {
		t.Fatalf("identity = %s.%s", productID, toolName)
	}
	if _, annotated := runtimeFlagRequiredState(list.Flags().Lookup("limit")); annotated {
		t.Fatal("embedded Catalog replayed stale required metadata onto a real Cobra flag")
	}
}

func TestStripSchemaPayloadCompactLeaf(t *testing.T) {
	leaf, err := embeddedSchemaPayload([]string{"calendar event create"})
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripSchemaPayloadCompact(leaf)

	// Must keep agent-essential fields.
	for _, key := range []string{"cli_path", "canonical_path", "description", "effect", "risk", "confirmation", "parameters", "constraints"} {
		if _, ok := stripped[key]; !ok {
			t.Fatalf("compact leaf missing essential key %q", key)
		}
	}

	// Must strip provenance / redundant fields.
	for _, key := range []string{"agent_metadata_source", "agent_source_refs", "agent_summary_source", "effect_source", "metadata_source", "primary_cli_path", "parameter_count", "has_parameters", "interface_ref", "source", "title", "display"} {
		if _, ok := stripped[key]; ok {
			t.Fatalf("compact leaf still contains stripped key %q", key)
		}
	}

	// Parameters must not contain interface_description / property.
	if params, ok := stripped["parameters"].(map[string]any); ok {
		for name, p := range params {
			if pm, ok := p.(map[string]any); ok {
				for _, stripped := range []string{"interface_description", "interface_type", "property"} {
					if _, present := pm[stripped]; present {
						t.Fatalf("compact param %q still contains %q", name, stripped)
					}
				}
				// Must keep type and required.
				if _, present := pm["type"]; !present {
					t.Fatalf("compact param %q missing type", name)
				}
			}
		}
	}
}

func TestStripSchemaPayloadCompactOverview(t *testing.T) {
	overview, err := embeddedSchemaPayload(nil)
	if err != nil {
		t.Fatal(err)
	}
	overview = compactSchemaOverviewPayload(overview)
	stripped := stripSchemaPayloadCompact(overview)

	// Overview must keep kind/level/count/products.
	for _, key := range []string{"kind", "level", "count", "products"} {
		if _, ok := stripped[key]; !ok {
			t.Fatalf("compact overview missing key %q", key)
		}
	}
	// Must strip agent_metadata / interface_metadata at top level.
	for _, key := range []string{"agent_metadata", "interface_metadata", "source"} {
		if _, ok := stripped[key]; ok {
			t.Fatalf("compact overview still contains stripped key %q", key)
		}
	}
}

func TestStripSchemaPayloadCompactProduct(t *testing.T) {
	product, err := embeddedSchemaPayload([]string{"calendar"})
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripSchemaPayloadCompact(product)

	if _, ok := stripped["product"]; !ok {
		t.Fatal("compact product missing 'product' key")
	}
	prod := stripped["product"].(map[string]any)
	for _, key := range []string{"agent_metadata_source", "agent_source_refs", "source"} {
		if _, ok := prod[key]; ok {
			t.Fatalf("compact product still contains stripped key %q", key)
		}
	}
}
