// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

// Test-only helpers relocated from production schema files; none of these
// functions has a production caller.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

func schemaProductToolCount(product map[string]any) int {
	switch value := product["tool_count"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	if tools, ok := product["tools"].([]map[string]any); ok {
		return len(tools)
	}
	if tools, ok := product["tools"].([]any); ok {
		return len(tools)
	}
	return 0
}

func normalizeRuntimeSchemaGroups(groups [][]string, minimum int) [][]string {
	// Test-facing thin wrapper over NormalizeConstraints group rules.
	c := RuntimeSchemaConstraints{}
	switch minimum {
	case 1:
		c.RequireOneOf = groups
	default:
		c.MutuallyExclusive = groups
	}
	out := runtimeannotate.NormalizeConstraints(c)
	if minimum == 1 {
		return out.RequireOneOf
	}
	return out.MutuallyExclusive
}

func runtimeFlagRequiredState(flag *pflag.Flag) (bool, bool) {
	// This helper reports the projected Schema annotation first. Cobra's
	// executable marker is retained as a lower-priority observation; the typed
	// contract exposes both candidates when an explicit overlay lowers it.
	if raw := firstFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation); raw != "" {
		required, err := strconv.ParseBool(raw)
		if err == nil {
			return required, true
		}
	}
	if runtimeFlagCobraHardRequired(flag) {
		return true, true
	}
	usage := strings.ToLower(strings.TrimSpace(flag.Usage))
	if usageImpliesRequired(usage) {
		return true, true
	}
	return false, false
}

func deliverySchemaCatalogAvailable() bool {
	return deliverySchemaCatalogError() == nil && len(deliverySchemaCatalog().Index.CanonicalPaths()) > 0
}

func exactSchemaCommand(root *cobra.Command, rawPath string) *cobra.Command {
	if root == nil {
		return nil
	}
	parts := strings.Fields(strings.TrimSpace(rawPath))
	if len(parts) > 0 && parts[0] == root.Name() {
		parts = parts[1:]
	}
	current := root
	for _, part := range parts {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	if current == root {
		return nil
	}
	return current
}

func schemaMap(value any) map[string]map[string]any {
	input, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]map[string]any, len(input))
	for key, value := range input {
		if item, ok := value.(map[string]any); ok {
			out[key] = item
		}
	}
	return out
}

func schemaToolSpecFromPayload(payload map[string]any) (ToolSpec, error) {
	wire, err := schemaToolWireFromPayload(payload)
	if err != nil {
		return ToolSpec{}, err
	}
	return schemaToolSpecFromWire(wire)
}

// runtimeCommandParameters is the compatibility wire adapter used only by
// tests; resolution happens in runtimeCommandParameterSpecs.
func runtimeCommandParameters(cmd *cobra.Command, canonicalPath string, constraints RuntimeSchemaConstraints) (map[string]any, error) {
	specs, err := runtimeCommandParameterSpecsForPayload(cmd, canonicalPath, constraints)
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, nil
	}
	parameters := make(map[string]any, len(specs))
	for _, spec := range specs {
		payload, payloadErr := spec.ToPayload()
		if payloadErr != nil {
			return nil, fmt.Errorf("serialize Schema parameter %q: %w", spec.Name, payloadErr)
		}
		parameters[spec.Name] = payload
	}
	return parameters, nil
}

// walkLeafCommands invokes fn for every runnable leaf command in the tree.
// Test-only: production traversals use their own walkers.
func walkLeafCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd.Runnable() && !cmd.HasSubCommands() {
		fn(cmd)
		return
	}
	for _, sub := range cmd.Commands() {
		if sub.Name() == "help" {
			continue
		}
		if !sub.IsAvailableCommand() && !hasRuntimeSchemaCommand(sub) {
			continue
		}
		walkLeafCommands(sub, fn)
	}
}

func decodeSchemaMetaIndexLookup(data []byte) (map[string]CommandMeta, error) {
	index, err := DecodeSchemaMetaIndex(data)
	if err != nil {
		return nil, err
	}
	return commandMetaLookupFromIndex(index)
}

func applyRuntimeSchemaParameterBindingsFrom(cmd *cobra.Command, canonical string, bindings map[string]map[string]string) {
	for flagName, propertyName := range bindings[strings.TrimSpace(canonical)] {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagBindingPropertyAnnotation, strings.TrimSpace(propertyName))
		}
	}
}

func setRuntimeCommandAnnotation(cmd *cobra.Command, key, value string) {
	runtimeannotate.SetCommandAnnotation(cmd, key, value)
}

func applyRuntimeSchemaParameterMetadata(cmd *cobra.Command, canonicalPath string) {
	metadata, ok := runtimeSchemaParameterMetadataByCanonical[strings.TrimSpace(canonicalPath)]
	if !ok {
		return
	}
	for _, flagName := range metadata.Required {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataRequiredAnnotation, "true")
		}
	}
	for flagName, expression := range metadata.RequiredWhen {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataRequiredWhenAnnotation, strings.TrimSpace(expression))
		}
	}
	for flagName, format := range metadata.Formats {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataFormatAnnotation, strings.TrimSpace(format))
		}
	}
	for flagName, values := range metadata.Enums {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotationValues(flag, runtimeSchemaFlagMetadataEnumAnnotation, values...)
		}
	}
	for flagName, example := range metadata.Examples {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataExampleAnnotation, strings.TrimSpace(example))
		}
	}
}

func publicRunnableSchemaLeaf(command *cobra.Command) bool {
	if command == nil || !command.Runnable() || command.HasSubCommands() {
		return false
	}
	for current := command; current != nil; current = current.Parent() {
		if current.Hidden {
			return false
		}
	}
	return true
}

func setFlagAnnotation(flag *pflag.Flag, key, value string) {
	runtimeannotate.SetFlagAnnotation(flag, key, value)
}

func setFlagAnnotationValues(flag *pflag.Flag, key string, values ...string) {
	runtimeannotate.SetFlagAnnotationValues(flag, key, values...)
}
