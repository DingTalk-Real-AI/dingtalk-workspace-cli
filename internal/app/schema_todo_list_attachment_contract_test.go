// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTodoListAttachmentDeliveredSchemaMatchesExecutableHelp(t *testing.T) {
	const (
		canonicalPath = "todo.list_todo_attachment"
		cliPath       = "todo task list-attachment"
	)

	root := NewRootCommand()
	command := exactCommandForTest(root, cliPath)
	if command == nil {
		t.Fatalf("executable command %q is missing", cliPath)
	}

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"schema", cliPath, "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute embedded schema leaf: %v; stderr=%s", err, stderr.String())
	}

	var tool map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &tool); err != nil {
		t.Fatalf("decode embedded schema leaf: %v", err)
	}
	if got := schemaContractString(tool["canonical_path"]); got != canonicalPath {
		t.Fatalf("canonical_path = %q, want %q", got, canonicalPath)
	}
	if got := schemaContractString(tool["primary_cli_path"]); got != cliPath {
		t.Fatalf("primary_cli_path = %q, want %q", got, cliPath)
	}
	if got := schemaContractString(tool["availability"]); got != "available" {
		t.Fatalf("availability = %q, want available", got)
	}
	if problem := schemaHelpFlagCompletenessProblem(canonicalPath, cliPath, command, tool); problem != "" {
		t.Fatal(problem)
	}

	taskID := schemaContractMap(tool["parameters"])["task-id"]
	if taskID == nil {
		t.Fatal("delivered Schema is missing --task-id")
	}
	if required, ok := taskID["required"].(bool); !ok || !required {
		t.Fatalf("task-id required = %#v, want true", taskID["required"])
	}
}
