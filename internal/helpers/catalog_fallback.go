// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package helpers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewCatalogFallbackCommands builds a low-priority executable tree from the
// release catalog. Callers must merge real CLIOverlay/Go commands first and
// use this tree only to fill missing leaves.
func NewCatalogFallbackCommands(runner executor.Runner) []*cobra.Command {
	definitions := cli.EmbeddedSchemaCommandDefinitions()
	roots := map[string]*cobra.Command{}
	paths := map[string]bool{}
	for _, definition := range definitions {
		// PAT is always registered by internal/pat after the helper tree. A
		// fallback PAT root would create duplicate top-level commands and make
		// catalog generation depend on Cobra's ordering of equal command names.
		if definition.ProductID == "pat" {
			continue
		}
		for _, path := range append([]string{definition.CLIPath}, definition.Aliases...) {
			addCatalogFallbackLeaf(roots, paths, runner, definition, path)
		}
	}
	out := make([]*cobra.Command, 0, len(roots))
	for _, root := range roots {
		// Products that exist only in the frozen fallback remain invokable but
		// do not become new top-level help entries. Merging into a real product
		// keeps the existing product visible.
		root.Hidden = true
		out = append(out, root)
	}
	return out
}

func addCatalogFallbackLeaf(roots map[string]*cobra.Command, paths map[string]bool, runner executor.Runner, definition cli.CatalogCommandDefinition, rawPath string) {
	parts := strings.Fields(strings.TrimSpace(rawPath))
	if len(parts) < 2 || parts[0] != definition.ProductID {
		return
	}
	path := strings.Join(parts, " ")
	if paths[path] {
		return
	}
	paths[path] = true
	root := roots[parts[0]]
	if root == nil {
		root = catalogFallbackGroup(parts[0], definition.ProductName)
		roots[parts[0]] = root
	}
	parent := root
	for _, name := range parts[1 : len(parts)-1] {
		child := directCatalogChild(parent, name)
		if child == nil {
			child = catalogFallbackGroup(name, name)
			parent.AddCommand(child)
		}
		parent = child
	}
	leaf := newCatalogFallbackLeaf(runner, definition, parts[len(parts)-1], path)
	parent.AddCommand(leaf)
}

func catalogFallbackGroup(name, description string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               name,
		Short:             strings.TrimSpace(description),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmdutil.SetOverridePriority(cmd, -100)
	return cmd
}

func directCatalogChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func newCatalogFallbackLeaf(runner executor.Runner, definition cli.CatalogCommandDefinition, leafName, legacyPath string) *cobra.Command {
	definitionCopy := definition
	cmd := &cobra.Command{
		Use:               leafName,
		Short:             firstCatalogText(definition.Title, definition.Description, definition.CanonicalPath),
		Long:              strings.TrimSpace(definition.Description),
		Args:              catalogFallbackArgs(definition.Positionals),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := collectCatalogFallbackParams(cmd, args, definitionCopy)
			if err != nil {
				return err
			}
			invocation := executor.NewCompatibilityInvocation(
				legacyPath,
				definitionCopy.SourceProductID,
				definitionCopy.RPCName,
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			if invocation.DryRun {
				return writeCommandPayload(cmd, invocation)
			}
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	for _, parameter := range definition.Parameters {
		registerCatalogFallbackFlag(cmd, parameter)
	}
	registerCatalogFallbackConstraints(cmd, definition)
	cli.AnnotateRuntimePositionals(cmd, definition.Positionals...)
	cli.AttachRuntimeSchema(cmd, definition.ProductID, definition.ToolName, "frozen-catalog")
	cmdutil.SetOverridePriority(cmd, -100)
	return cmd
}

func registerCatalogFallbackFlag(cmd *cobra.Command, parameter cli.CatalogParameterDefinition) {
	usage := strings.TrimSpace(parameter.Description)
	if usage == "" {
		usage = parameter.Property
	}
	switch parameter.Type {
	case "integer":
		value, _ := strconv.Atoi(parameter.Default)
		cmd.Flags().Int(parameter.Name, value, usage)
	case "number":
		value, _ := strconv.ParseFloat(parameter.Default, 64)
		cmd.Flags().Float64(parameter.Name, value, usage)
	case "boolean":
		value, _ := strconv.ParseBool(parameter.Default)
		cmd.Flags().Bool(parameter.Name, value, usage)
	case "array":
		cmd.Flags().StringSlice(parameter.Name, nil, usage)
	default:
		cmd.Flags().String(parameter.Name, parameter.Default, usage)
	}
	interfaceType := parameter.InterfaceType
	if interfaceType == "" {
		interfaceType = parameter.Type
	}
	cli.AnnotateRuntimeFlag(cmd, parameter.Name, parameter.Property, interfaceType, parameter.Required, parameter.Default)
	if parameter.Format != "" {
		cli.AnnotateRuntimeFlagFormat(cmd, parameter.Name, parameter.Format)
	}
	if len(parameter.Enum) > 0 {
		cli.AnnotateRuntimeFlagEnum(cmd, parameter.Name, parameter.Enum...)
	}
	if parameter.Required && parameter.RequiredWhen == "" {
		_ = cmd.MarkFlagRequired(parameter.Name)
	}
}

func registerCatalogFallbackConstraints(cmd *cobra.Command, definition cli.CatalogCommandDefinition) {
	cli.AnnotateRuntimeConstraints(cmd, definition.Constraints)
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		return validateCatalogFallbackConstraints(cmd, args, definition)
	}
}

func validateCatalogFallbackConstraints(cmd *cobra.Command, args []string, definition cli.CatalogCommandDefinition) error {
	providedCount := func(group []string) int {
		count := 0
		for _, name := range group {
			if catalogFallbackValueProvided(cmd, args, definition.Positionals, name) {
				count++
			}
		}
		return count
	}
	for _, group := range definition.Constraints.MutuallyExclusive {
		if providedCount(group) > 1 {
			return apperrors.NewValidation("parameters are mutually exclusive: " + strings.Join(group, ", "))
		}
	}
	for _, group := range definition.Constraints.RequireOneOf {
		if providedCount(group) == 0 {
			return apperrors.NewValidation("at least one parameter is required: " + strings.Join(group, ", "))
		}
	}
	for _, group := range definition.Constraints.RequireTogether {
		if count := providedCount(group); count > 0 && count < len(group) {
			return apperrors.NewValidation("parameters must be provided together: " + strings.Join(group, ", "))
		}
	}
	return nil
}

func catalogFallbackValueProvided(cmd *cobra.Command, args []string, positionals []cli.RuntimeSchemaPositional, name string) bool {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		return true
	}
	for _, positional := range positionals {
		if positional.Name == name && positional.Index >= 0 && positional.Index < len(args) {
			return true
		}
	}
	return false
}

func collectCatalogFallbackParams(cmd *cobra.Command, args []string, definition cli.CatalogCommandDefinition) (map[string]any, error) {
	params := map[string]any{}
	byName := map[string]cli.CatalogParameterDefinition{}
	for _, parameter := range definition.Parameters {
		byName[parameter.Name] = parameter
	}
	var collectErr error
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		if collectErr != nil {
			return
		}
		parameter, ok := byName[flag.Name]
		if !ok {
			return
		}
		value, err := catalogFallbackFlagValue(cmd, parameter)
		if err != nil {
			collectErr = err
			return
		}
		params[parameter.Property] = value
	})
	if collectErr != nil {
		return nil, collectErr
	}
	for _, positional := range definition.Positionals {
		if positional.Index >= 0 && positional.Index < len(args) {
			if positional.Variadic {
				params[positional.Name] = append([]string(nil), args[positional.Index:]...)
			} else {
				params[positional.Name] = args[positional.Index]
			}
		}
	}
	return params, nil
}

func catalogFallbackFlagValue(cmd *cobra.Command, parameter cli.CatalogParameterDefinition) (any, error) {
	switch parameter.Type {
	case "integer":
		return cmd.Flags().GetInt(parameter.Name)
	case "number":
		return cmd.Flags().GetFloat64(parameter.Name)
	case "boolean":
		return cmd.Flags().GetBool(parameter.Name)
	case "array":
		return cmd.Flags().GetStringSlice(parameter.Name)
	}
	value, err := cmd.Flags().GetString(parameter.Name)
	if err != nil {
		return nil, err
	}
	switch parameter.InterfaceType {
	case "number", "integer":
		if parameter.Format == "date-time" {
			parsed, parseErr := time.Parse(time.RFC3339, value)
			if parseErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("--%s must be ISO-8601/RFC3339", parameter.Name))
			}
			return parsed.UnixMilli(), nil
		}
	case "array":
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out, nil
	case "object":
		var object map[string]any
		if err := json.Unmarshal([]byte(value), &object); err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("--%s must be a JSON object", parameter.Name))
		}
		return object, nil
	}
	return value, nil
}

func catalogFallbackArgs(positionals []cli.RuntimeSchemaPositional) cobra.PositionalArgs {
	if len(positionals) == 0 {
		return cobra.NoArgs
	}
	minimum := 0
	maximum := len(positionals)
	variadic := false
	for _, positional := range positionals {
		if positional.Required && positional.Index+1 > minimum {
			minimum = positional.Index + 1
		}
		variadic = variadic || positional.Variadic
	}
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < minimum || (!variadic && len(args) > maximum) {
			return apperrors.NewValidation(fmt.Sprintf("%s expects %d..%d argument(s)", cmd.CommandPath(), minimum, maximum))
		}
		return nil
	}
}

func firstCatalogText(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "Catalog command"
}
