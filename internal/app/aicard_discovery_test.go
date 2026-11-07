// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package app

import (
	"bytes"
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
