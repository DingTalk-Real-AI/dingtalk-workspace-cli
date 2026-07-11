// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

type panicCatalogLoader struct{}

func (panicCatalogLoader) Load(context.Context) (Catalog, error) {
	panic("schema must not load a runtime catalog")
}

func TestSchemaUsesEmbeddedCatalogWithoutRuntimeLoad(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(NewSchemaCommand(panicCatalogLoader{}))
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
	if payload.Count != 21 || payload.ToolCount != 504 {
		t.Fatalf("schema counts = %d/%d, want 21/504", payload.Count, payload.ToolCount)
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
