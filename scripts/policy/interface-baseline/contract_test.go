// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCompatibilityAllowsAdditions(t *testing.T) {
	root := testRoot()
	baseline, err := parseContract([]byte("[root]\n  commands: old\n\n[old]\n  flags: -n/--name:string, -h/--help:bool\n"))
	if err != nil {
		t.Fatal(err)
	}
	if failures := checkCompatibility(root, baseline); len(failures) != 0 {
		t.Fatalf("additions should be compatible: %v", failures)
	}
}

func TestCompatibilityRejectsMissingCommandAndFlag(t *testing.T) {
	root := testRoot()
	baseline, err := parseContract([]byte("[root]\n\n[removed]\n  flags: --gone:string\n\n[old]\n  flags: --gone:string\n"))
	if err != nil {
		t.Fatal(err)
	}
	failures := checkCompatibility(root, baseline)
	if len(failures) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(failures), failures)
	}
}

func TestCompatibilityAllowsNewShorthandButRejectsRemovedShorthand(t *testing.T) {
	root := testRoot()
	baseline, _ := parseContract([]byte("[root]\n\n[old]\n  flags: --name:string\n"))
	if failures := checkCompatibility(root, baseline); len(failures) != 0 {
		t.Fatalf("new shorthand should be compatible: %v", failures)
	}

	baseline, _ = parseContract([]byte("[root]\n\n[old]\n  flags: -x/--name:string\n"))
	if failures := checkCompatibility(root, baseline); len(failures) != 1 {
		t.Fatalf("removed shorthand should fail: %v", failures)
	}
}

func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	old := &cobra.Command{Use: "old"}
	old.Flags().StringP("name", "n", "", "name")
	old.Flags().String("extra", "", "addition")
	root.AddCommand(old, &cobra.Command{Use: "new"})
	root.InitDefaultHelpCmd()
	return root
}
