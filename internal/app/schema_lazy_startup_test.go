// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

const schemaLazyStartupChildEnv = "DWS_SCHEMA_LAZY_STARTUP_CHILD"

// TestOrdinaryRootCommandsDoNotLoadSchemaMetadata uses a fresh process so its
// counters describe package init, root construction, help, and version only;
// unrelated Schema tests cannot have initialized the snapshots first.
func TestOrdinaryRootCommandsDoNotLoadSchemaMetadata(t *testing.T) {
	if os.Getenv(schemaLazyStartupChildEnv) == "1" {
		assertSchemaMetadataNotLoaded(t, "package init")

		root := NewRootCommand()
		assertSchemaMetadataNotLoaded(t, "NewRootCommand")
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"--help"})
		if err := root.Execute(); err != nil {
			t.Fatalf("root --help: %v", err)
		}
		assertSchemaMetadataNotLoaded(t, "root --help")

		root = NewRootCommand()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"version"})
		if err := root.Execute(); err != nil {
			t.Fatalf("dws version: %v", err)
		}
		assertSchemaMetadataNotLoaded(t, "dws version")
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestOrdinaryRootCommandsDoNotLoadSchemaMetadata$", "-test.count=1")
	command.Env = append(os.Environ(), schemaLazyStartupChildEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("lazy startup child failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func assertSchemaMetadataNotLoaded(t *testing.T, stage string) {
	t.Helper()
	if counts := cli.RuntimeSchemaMetadataLoadCounts(); counts != (cli.SchemaMetadataLoadCounts{}) {
		t.Fatalf("%s loaded Schema metadata: %#v", stage, counts)
	}
}
