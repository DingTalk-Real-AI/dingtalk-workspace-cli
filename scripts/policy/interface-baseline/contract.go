// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type flagContract struct {
	Name      string
	Shorthand string
	Type      string
}

type commandContract struct {
	Path     string
	Children map[string]struct{}
	Aliases  map[string]struct{}
	Flags    map[string]flagContract
}

type interfaceContract struct {
	Commands map[string]*commandContract
}

func newInterfaceContract() interfaceContract {
	return interfaceContract{Commands: map[string]*commandContract{}}
}

func (c interfaceContract) command(path string) *commandContract {
	if existing := c.Commands[path]; existing != nil {
		return existing
	}
	created := &commandContract{
		Path:     path,
		Children: map[string]struct{}{},
		Aliases:  map[string]struct{}{},
		Flags:    map[string]flagContract{},
	}
	c.Commands[path] = created
	return created
}

func snapshot(root *cobra.Command) interfaceContract {
	contract := newInterfaceContract()
	var walk func(*cobra.Command, []string)
	walk = func(cmd *cobra.Command, parent []string) {
		pathParts := parent
		if cmd.HasParent() {
			pathParts = append(append([]string(nil), parent...), cmd.Name())
		}
		path := "root"
		if len(pathParts) > 0 {
			path = strings.Join(pathParts, ".")
		}
		entry := contract.command(path)
		for _, alias := range cmd.Aliases {
			entry.Aliases[alias] = struct{}{}
		}
		cmd.InitDefaultHelpFlag()
		cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if !f.Hidden {
				entry.Flags[f.Name] = flagContract{Name: f.Name, Shorthand: f.Shorthand, Type: f.Value.Type()}
			}
		})
		for _, child := range publicChildren(cmd) {
			entry.Children[child.Name()] = struct{}{}
			walk(child, pathParts)
		}
	}
	walk(root, nil)
	return contract
}

func publicChildren(cmd *cobra.Command) []*cobra.Command {
	var children []*cobra.Command
	for _, child := range cmd.Commands() {
		if !child.Hidden {
			children = append(children, child)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
	return children
}

func readContract(path string) (interfaceContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return interfaceContract{}, err
	}
	return parseContract(data)
}

func parseContract(data []byte) (interfaceContract, error) {
	contract := newInterfaceContract()
	var current *commandContract
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			path := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			if path == "" {
				return interfaceContract{}, fmt.Errorf("empty command path")
			}
			current = contract.command(path)
			continue
		}
		if current == nil {
			return interfaceContract{}, fmt.Errorf("property before command section: %q", line)
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return interfaceContract{}, fmt.Errorf("invalid contract line %q", line)
		}
		items := splitList(value)
		switch key {
		case "commands":
			for _, item := range items {
				current.Children[item] = struct{}{}
			}
		case "aliases":
			for _, item := range items {
				current.Aliases[item] = struct{}{}
			}
		case "flags":
			for _, item := range items {
				parsed, err := parseFlag(item)
				if err != nil {
					return interfaceContract{}, fmt.Errorf("%s: %w", current.Path, err)
				}
				if old, exists := current.Flags[parsed.Name]; exists && old != parsed {
					return interfaceContract{}, fmt.Errorf("%s: conflicting duplicate flag --%s", current.Path, parsed.Name)
				}
				current.Flags[parsed.Name] = parsed
			}
		default:
			return interfaceContract{}, fmt.Errorf("unknown contract property %q", key)
		}
	}
	if err := scanner.Err(); err != nil {
		return interfaceContract{}, err
	}
	return contract, nil
}

func splitList(value string) []string {
	var result []string
	for _, item := range strings.Split(strings.TrimSpace(value), ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func parseFlag(raw string) (flagContract, error) {
	spelling, flagType, ok := strings.Cut(raw, ":")
	if !ok || flagType == "" {
		return flagContract{}, fmt.Errorf("invalid flag contract %q", raw)
	}
	var shorthand string
	longName := spelling
	if before, after, found := strings.Cut(spelling, "/"); found {
		shorthand = strings.TrimPrefix(before, "-")
		longName = after
	}
	longName = strings.TrimPrefix(longName, "--")
	if longName == "" {
		return flagContract{}, fmt.Errorf("invalid flag contract %q", raw)
	}
	return flagContract{Name: longName, Shorthand: shorthand, Type: flagType}, nil
}

func checkCompatibility(root *cobra.Command, baseline interfaceContract) []string {
	var failures []string
	paths := sortedCommandPaths(baseline)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, path := range paths {
		expected := baseline.Commands[path]
		cmd, ok := resolveCommand(root, path)
		if !ok {
			failures = append(failures, fmt.Sprintf("historical command %q is missing", displayPath(path)))
			continue
		}
		if err := cmd.Help(); err != nil {
			failures = append(failures, fmt.Sprintf("%q -h cannot render: %v", displayPath(path), err))
		}
		for _, alias := range sortedSet(expected.Aliases) {
			aliasPath := aliasCommandPath(path, alias)
			resolved, found := resolveCommand(root, aliasPath)
			if !found || resolved != cmd {
				failures = append(failures, fmt.Sprintf("historical alias %q is missing", displayPath(aliasPath)))
			}
		}
		for _, expectedFlag := range sortedFlags(expected.Flags) {
			actual := lookupFlag(cmd, expectedFlag.Name)
			if actual == nil {
				failures = append(failures, fmt.Sprintf("%q lost flag --%s", displayPath(path), expectedFlag.Name))
				continue
			}
			if actual.Value.Type() != expectedFlag.Type {
				failures = append(failures, fmt.Sprintf(
					"%q flag --%s changed type from %s to %s",
					displayPath(path), expectedFlag.Name, expectedFlag.Type, actual.Value.Type(),
				))
			}
			if expectedFlag.Shorthand != "" && actual.Shorthand != expectedFlag.Shorthand {
				failures = append(failures, fmt.Sprintf(
					"%q flag --%s lost shorthand -%s",
					displayPath(path), expectedFlag.Name, expectedFlag.Shorthand,
				))
			}
		}
	}
	return failures
}

func resolveCommand(root *cobra.Command, path string) (*cobra.Command, bool) {
	if path == "root" {
		return root, true
	}
	cmd, remaining, err := root.Find(strings.Split(path, "."))
	return cmd, err == nil && len(remaining) == 0
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	for current := cmd; current != nil; current = current.Parent() {
		current.InitDefaultHelpFlag()
		if f := current.LocalNonPersistentFlags().Lookup(name); f != nil {
			return f
		}
		if f := current.PersistentFlags().Lookup(name); f != nil {
			return f
		}
	}
	return nil
}

func aliasCommandPath(path, alias string) string {
	if path == "root" || !strings.Contains(path, ".") {
		return alias
	}
	parent, _, _ := strings.Cut(path, ".")
	index := strings.LastIndex(path, ".")
	if index >= 0 {
		parent = path[:index]
	}
	return parent + "." + alias
}

func mergeContracts(historical, current interfaceContract) (interfaceContract, []string) {
	merged := cloneContract(historical)
	var failures []string
	for path, addition := range current.Commands {
		target := merged.command(path)
		for child := range addition.Children {
			target.Children[child] = struct{}{}
		}
		for alias := range addition.Aliases {
			target.Aliases[alias] = struct{}{}
		}
		for name, newFlag := range addition.Flags {
			oldFlag, exists := target.Flags[name]
			if !exists {
				target.Flags[name] = newFlag
				continue
			}
			if oldFlag.Type != newFlag.Type {
				failures = append(failures, fmt.Sprintf(
					"%q flag --%s changed type from %s to %s",
					displayPath(path), name, oldFlag.Type, newFlag.Type,
				))
				continue
			}
			if oldFlag.Shorthand != "" && oldFlag.Shorthand != newFlag.Shorthand {
				failures = append(failures, fmt.Sprintf(
					"%q flag --%s lost shorthand -%s",
					displayPath(path), name, oldFlag.Shorthand,
				))
				continue
			}
			if oldFlag.Shorthand == "" && newFlag.Shorthand != "" {
				target.Flags[name] = newFlag
			}
		}
	}
	return merged, failures
}

func cloneContract(source interfaceContract) interfaceContract {
	cloned := newInterfaceContract()
	for path, command := range source.Commands {
		target := cloned.command(path)
		for child := range command.Children {
			target.Children[child] = struct{}{}
		}
		for alias := range command.Aliases {
			target.Aliases[alias] = struct{}{}
		}
		for name, flag := range command.Flags {
			target.Flags[name] = flag
		}
	}
	return cloned
}

func renderContract(w io.Writer, contract interfaceContract) error {
	for index, path := range sortedCommandPaths(contract) {
		if index > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		command := contract.Commands[path]
		if _, err := fmt.Fprintf(w, "[%s]\n", path); err != nil {
			return err
		}
		if values := sortedSet(command.Children); len(values) > 0 {
			fmt.Fprintf(w, "  commands: %s\n", strings.Join(values, ", "))
		}
		if values := sortedSet(command.Aliases); len(values) > 0 {
			fmt.Fprintf(w, "  aliases: %s\n", strings.Join(values, ", "))
		}
		if values := sortedFlags(command.Flags); len(values) > 0 {
			rendered := make([]string, 0, len(values))
			for _, flag := range values {
				name := "--" + flag.Name
				if flag.Shorthand != "" {
					name = "-" + flag.Shorthand + "/" + name
				}
				rendered = append(rendered, name+":"+flag.Type)
			}
			fmt.Fprintf(w, "  flags: %s\n", strings.Join(rendered, ", "))
		}
	}
	return nil
}

func sortedCommandPaths(contract interfaceContract) []string {
	paths := make([]string, 0, len(contract.Commands))
	for path := range contract.Commands {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i] == "root" {
			return true
		}
		if paths[j] == "root" {
			return false
		}
		return paths[i] < paths[j]
	})
	return paths
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedFlags(values map[string]flagContract) []flagContract {
	result := make([]flagContract, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func displayPath(path string) string {
	if path == "root" {
		return "dws"
	}
	return "dws " + strings.ReplaceAll(path, ".", " ")
}
