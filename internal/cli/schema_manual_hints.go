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

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

const manualSchemaHintVersion = 1
const manualSchemaHintSchemaRef = "./schema_manual_hints.schema.json"

//go:embed schema_manual_hints.json
var embeddedManualSchemaHintsJSON []byte

// ManualSchemaHintSnapshot is the human-owned bridge from an existing
// public Cobra leaf to Schema. It cannot create commands, flags, exclusions, or
// interfaces: every referenced command and flag is checked against the live
// tree before any annotation is applied.
type ManualSchemaHintSnapshot struct {
	Schema   string                    `json:"$schema"`
	Version  int                       `json:"version"`
	Commands []ManualSchemaCommandHint `json:"commands"`
}

// ManualSchemaCommandHint opts one exact existing Cobra leaf into
// Schema and optionally reviews its CLI-facing parameter projection.
type ManualSchemaCommandHint struct {
	CLIPath       string                               `json:"cli_path"`
	CanonicalPath string                               `json:"canonical_path"`
	Reason        string                               `json:"reason"`
	Reviewed      bool                                 `json:"reviewed"`
	Parameters    map[string]ManualSchemaParameterHint `json:"parameters,omitempty"`
}

// ManualSchemaParameterHint changes only Schema annotations on a real
// flag. Pointer fields distinguish an omitted override from an explicit false.
type ManualSchemaParameterHint struct {
	Description   *string `json:"description,omitempty"`
	Property      *string `json:"property,omitempty"`
	InterfaceType *string `json:"interface_type,omitempty"`
	Required      *bool   `json:"required,omitempty"`
	RequiredWhen  *string `json:"required_when,omitempty"`
}

// ManualSchemaHintReport records the exact reviewed inputs applied to
// a command tree. It is useful to generators and tests; no runtime discovery is
// performed.
type ManualSchemaHintReport struct {
	Commands   []string
	Parameters []string
}

var (
	manualSchemaHintsOnce     sync.Once
	manualSchemaHintsSnapshot ManualSchemaHintSnapshot
	manualSchemaHintsErr      error
)

// ApplyEmbeddedManualSchemaHints applies the committed human review
// file to an already-built Cobra tree. The operation is deterministic and
// idempotent.
func ApplyEmbeddedManualSchemaHints(root *cobra.Command) (ManualSchemaHintReport, error) {
	snapshot, err := embeddedManualSchemaHints()
	if err != nil {
		return ManualSchemaHintReport{}, err
	}
	return applyManualSchemaHints(root, snapshot)
}

func embeddedManualSchemaHints() (ManualSchemaHintSnapshot, error) {
	manualSchemaHintsOnce.Do(func() {
		manualSchemaHintsSnapshot, manualSchemaHintsErr = decodeManualSchemaHints(embeddedManualSchemaHintsJSON)
	})
	return manualSchemaHintsSnapshot, manualSchemaHintsErr
}

func decodeManualSchemaHints(data []byte) (ManualSchemaHintSnapshot, error) {
	var snapshot ManualSchemaHintSnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, fmt.Errorf("decode manual Schema hints: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return snapshot, fmt.Errorf("decode manual Schema hints: %w", err)
	}
	if snapshot.Version != manualSchemaHintVersion {
		return snapshot, fmt.Errorf("unsupported manual Schema hint version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.Schema) != manualSchemaHintSchemaRef {
		return snapshot, fmt.Errorf("manual Schema hints must declare $schema=%q", manualSchemaHintSchemaRef)
	}
	return snapshot, nil
}

type validatedManualSchemaHint struct {
	hint    ManualSchemaCommandHint
	command *cobra.Command
}

func applyManualSchemaHints(root *cobra.Command, snapshot ManualSchemaHintSnapshot) (ManualSchemaHintReport, error) {
	if root == nil {
		return ManualSchemaHintReport{}, fmt.Errorf("apply manual Schema hints: root is nil")
	}
	if snapshot.Version != manualSchemaHintVersion {
		return ManualSchemaHintReport{}, fmt.Errorf("unsupported manual Schema hint version %d", snapshot.Version)
	}

	validated := make([]validatedManualSchemaHint, 0, len(snapshot.Commands))
	seenPaths := map[string]bool{}
	for _, raw := range snapshot.Commands {
		hint := raw
		hint.CLIPath = normalizeSchemaCLIPath(hint.CLIPath)
		hint.CanonicalPath = strings.TrimSpace(hint.CanonicalPath)
		hint.Reason = strings.TrimSpace(hint.Reason)
		if hint.CLIPath == "" || strings.ContainsAny(hint.CLIPath, "*?[]") {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint has invalid exact cli_path %q", raw.CLIPath)
		}
		if seenPaths[hint.CLIPath] {
			return ManualSchemaHintReport{}, fmt.Errorf("duplicate manual Schema hint for %q", hint.CLIPath)
		}
		seenPaths[hint.CLIPath] = true
		if !hint.Reviewed || hint.Reason == "" {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q is not reviewed or has no reason", hint.CLIPath)
		}
		productID, toolName, ok := splitManualSchemaCanonicalPath(hint.CanonicalPath)
		if !ok {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q has invalid canonical_path %q", hint.CLIPath, hint.CanonicalPath)
		}
		command := exactSchemaCommand(root, hint.CLIPath)
		if command == nil {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q does not resolve to an existing Cobra command", hint.CLIPath)
		}
		if !publicRunnableSchemaLeaf(command) {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q must target a public runnable Cobra leaf", hint.CLIPath)
		}
		existingProduct, existingTool, _ := runtimeSchemaAnnotations(command)
		if (existingProduct == "") != (existingTool == "") {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q found an incomplete existing Schema identity", hint.CLIPath)
		}
		if existingProduct != "" && (existingProduct != productID || existingTool != toolName) {
			return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q conflicts with existing canonical path %s.%s", hint.CLIPath, existingProduct, existingTool)
		}
		for flagName, parameter := range hint.Parameters {
			flagName = strings.TrimSpace(flagName)
			if flagName == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q contains an empty flag name", hint.CLIPath)
			}
			if runtimeCommandFlag(command, flagName) == nil {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q references missing flag --%s", hint.CLIPath, flagName)
			}
			if parameter.Description == nil && parameter.Property == nil && parameter.InterfaceType == nil && parameter.Required == nil && parameter.RequiredWhen == nil {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has no Schema overrides", hint.CLIPath, flagName)
			}
			if parameter.Description != nil && strings.TrimSpace(*parameter.Description) == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has an empty description override", hint.CLIPath, flagName)
			}
			if parameter.Property != nil && strings.TrimSpace(*parameter.Property) == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has an empty property override", hint.CLIPath, flagName)
			}
			if parameter.InterfaceType != nil && !supportedManualSchemaInterfaceType(*parameter.InterfaceType) {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has unsupported interface_type %q", hint.CLIPath, flagName, *parameter.InterfaceType)
			}
			if parameter.RequiredWhen != nil && strings.TrimSpace(*parameter.RequiredWhen) == "" {
				return ManualSchemaHintReport{}, fmt.Errorf("manual Schema hint %q flag --%s has an empty required_when override", hint.CLIPath, flagName)
			}
		}
		validated = append(validated, validatedManualSchemaHint{hint: hint, command: command})
	}

	report := ManualSchemaHintReport{}
	for _, item := range validated {
		productID, toolName, _ := splitManualSchemaCanonicalPath(item.hint.CanonicalPath)
		existingProduct, existingTool, _ := runtimeSchemaAnnotations(item.command)
		if existingProduct == "" && existingTool == "" {
			AttachRuntimeSchema(item.command, productID, toolName, "manual-schema-hint")
		}
		report.Commands = append(report.Commands, item.hint.CLIPath)
		flagNames := make([]string, 0, len(item.hint.Parameters))
		for flagName := range item.hint.Parameters {
			flagNames = append(flagNames, flagName)
		}
		sort.Strings(flagNames)
		for _, flagName := range flagNames {
			parameter := item.hint.Parameters[flagName]
			flag := runtimeCommandFlag(item.command, flagName)
			if parameter.Description != nil {
				setFlagAnnotation(flag, runtimeSchemaFlagDescriptionAnnotation, strings.TrimSpace(*parameter.Description))
			}
			if parameter.Property != nil {
				AnnotateRuntimeFlagProperty(item.command, flagName, strings.TrimSpace(*parameter.Property))
			}
			if parameter.InterfaceType != nil {
				setFlagAnnotation(flag, runtimeSchemaFlagTypeAnnotation, strings.TrimSpace(*parameter.InterfaceType))
			}
			if parameter.Required != nil {
				setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, strconv.FormatBool(*parameter.Required))
			}
			if parameter.RequiredWhen != nil {
				AnnotateRuntimeFlagRequiredWhen(item.command, flagName, strings.TrimSpace(*parameter.RequiredWhen))
			}
			report.Parameters = append(report.Parameters, item.hint.CLIPath+" --"+flagName)
		}
	}
	sort.Strings(report.Commands)
	return report, nil
}

func supportedManualSchemaInterfaceType(value string) bool {
	switch strings.TrimSpace(value) {
	case "string", "array", "object", "integer", "number", "boolean":
		return true
	default:
		return false
	}
}

func splitManualSchemaCanonicalPath(path string) (string, string, bool) {
	path = strings.TrimSpace(path)
	productID, toolName, ok := strings.Cut(path, ".")
	productID = strings.TrimSpace(productID)
	toolName = strings.TrimSpace(toolName)
	if !ok || productID == "" || toolName == "" || strings.ContainsAny(productID+toolName, " \t\r\n") {
		return "", "", false
	}
	return productID, toolName, true
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
