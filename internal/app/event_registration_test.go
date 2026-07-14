// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"sort"
	"testing"

	"github.com/spf13/cobra"
)

func TestEventCommandRemainsVisibleAsBuiltInPublicGroup(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	event := newEventCommand()
	unregistered := &cobra.Command{Use: "unregistered", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(event, unregistered)

	hideNonDirectRuntimeCommands(root)

	if event.Hidden {
		t.Fatal("built-in event command was hidden by the direct-runtime visibility filter")
	}
	if !unregistered.Hidden {
		t.Fatal("control command outside the built-in/direct-runtime sets remained visible")
	}

	var leaves []string
	for _, command := range event.Commands() {
		if command.Hidden || !command.Runnable() {
			continue
		}
		leaves = append(leaves, command.Name())
	}
	sort.Strings(leaves)
	want := []string{"consume", "list", "schema", "status", "stop"}
	if len(leaves) != len(want) {
		t.Fatalf("public event leaves = %v, want %v", leaves, want)
	}
	for index := range want {
		if leaves[index] != want[index] {
			t.Fatalf("public event leaves = %v, want %v", leaves, want)
		}
	}
}

func TestPluginCannotReplaceBuiltInEventCommand(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	builtIn := newEventCommand()
	root.AddCommand(builtIn)

	pluginEvent := &cobra.Command{Use: "event", Run: func(*cobra.Command, []string) {}}
	addPluginCommandsSafe(root, []*cobra.Command{pluginEvent})

	var eventCommands []*cobra.Command
	for _, command := range root.Commands() {
		if command.Name() == "event" {
			eventCommands = append(eventCommands, command)
		}
	}
	if len(eventCommands) != 1 || eventCommands[0] != builtIn {
		t.Fatalf("event command after plugin registration = %p (%d matches), want built-in %p", firstEventCommand(eventCommands), len(eventCommands), builtIn)
	}
	if pluginEvent.Parent() != nil {
		t.Fatal("conflicting plugin event command was attached to the root")
	}
}

func firstEventCommand(commands []*cobra.Command) *cobra.Command {
	if len(commands) == 0 {
		return nil
	}
	return commands[0]
}
