// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAicardRootDiscovery(t *testing.T) {
	root := NewRootCommand()
	command, rest, err := root.Find([]string{"aicard"})
	if err != nil || command == root || command.Name() != "aicard" || len(rest) != 0 {
		t.Fatalf("aicard must be registered at the root: command=%v rest=%v err=%v", command, rest, err)
	}
	if command.Hidden {
		t.Fatal("aicard must remain visible in root discovery")
	}
	if !builtinCommandNames["aicard"] || !staticCommands["aicard"] || !reservedCommands["aicard"] {
		t.Fatal("aicard must belong to the built-in, visible, and reserved command sets")
	}
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("root help: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "aicard") {
		t.Fatalf("root help omits aicard:\n%s", output.String())
	}
}
func TestCrossPlatformCoverageAicardChartBuildSchemaInputs(t *testing.T) {
	t.Setenv("DWS_SCHEMA_CACHE_DISABLE", "1")
	for _, compact := range []bool{false, true} {
		root := NewRootCommand()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		args := []string{"schema", "--cli-path", "aicard chart build", "--format", "json"}
		if compact {
			args = append(args, "--compact")
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("schema compact=%v: %v; %s", compact, err, stderr.String())
		}
		var leaf struct {
			Parameters map[string]struct {
				Required     bool   `json:"required"`
				RequiredWhen string `json:"required_when"`
			} `json:"parameters"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &leaf); err != nil {
			t.Fatal(err)
		}
		if !leaf.Parameters["file"].Required {
			t.Fatalf("compact=%v: file must be required; %+v", compact, leaf.Parameters["file"])
		}
		if leaf.Parameters["out"].Required || leaf.Parameters["out"].RequiredWhen != "--dry-run is false" {
			t.Fatalf("compact=%v: out must be conditionally required; %+v", compact, leaf.Parameters["out"])
		}
	}
}
