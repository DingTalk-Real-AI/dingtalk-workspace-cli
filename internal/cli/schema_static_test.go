// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

func TestSchemaUsesDeliveryCatalogWithoutRuntimeLoad(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(NewSchemaCommand())
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"schema"})
	if err := root.Execute(); err != nil {
		t.Fatalf("schema execute: %v", err)
	}
	var payload struct {
		Count     int `json:"count"`
		ToolCount int `json:"tool_count"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode schema: %v\n%s", err, stdout.String())
	}
	loaded := deliverySchemaCatalog()
	if payload.Count != len(loaded.Registry.Products) || payload.ToolCount != len(loaded.Index.CanonicalPaths()) {
		t.Fatalf("schema counts = %d/%d, want %d/%d", payload.Count, payload.ToolCount, len(loaded.Registry.Products), len(loaded.Index.CanonicalPaths()))
	}
}

func TestSchemaAllReturnsCompleteDeliveryLeafSchemas(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(NewSchemaCommand())
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"schema", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatalf("schema --all execute: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode schema --all: %v", err)
	}
	expanded := 0
	for _, product := range schemaMapSlice(payload["products"]) {
		for _, tool := range schemaMapSlice(product["tools"]) {
			if _, ok := tool["parameters"].(map[string]any); !ok {
				t.Fatalf("schema --all tool %q has no parameters object", schemaString(tool["canonical_path"]))
			}
			expanded++
		}
	}
	want := len(deliverySchemaCatalog().Index.CanonicalPaths())
	if expanded != want {
		t.Fatalf("schema --all tools = %d, want %d", expanded, want)
	}
}

func TestWalkLeafCommandsTraversesAnnotatedHiddenSubtree(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "compat", Hidden: true, Run: func(*cobra.Command, []string) {}}
	leaf := &cobra.Command{Use: "legacy", Hidden: true, Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(leaf, "compat", "legacy", "test")
	group.AddCommand(leaf)
	root.AddCommand(group)

	var got []*cobra.Command
	walkLeafCommands(root, func(command *cobra.Command) { got = append(got, command) })
	if len(got) != 1 || got[0] != leaf {
		t.Fatalf("walked commands = %#v, want annotated hidden leaf", got)
	}
}
