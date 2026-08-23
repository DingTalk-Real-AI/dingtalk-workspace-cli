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

package runtimeannotate

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// AttachRuntimeSchema records optional implementation-side identity evidence
// on a runnable command. Stable command identity is collected from
// ContractFinal.Identity on the live Cobra leaves (EffectiveCommandRegistry);
// the binder accepts an absent annotation and rejects an annotation that
// disagrees with the collected identity.
func AttachRuntimeSchema(cmd *cobra.Command, productID, toolName, source string) {
	if cmd == nil {
		return
	}
	productID = strings.TrimSpace(productID)
	toolName = strings.TrimSpace(toolName)
	if productID == "" || toolName == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AnnotationProduct] = productID
	cmd.Annotations[AnnotationTool] = toolName
	if source = strings.TrimSpace(source); source != "" {
		cmd.Annotations[AnnotationSource] = source
	}
}

// AnnotateRuntimeFlag adds parameter metadata to an already-registered flag.
func AnnotateRuntimeFlag(cmd *cobra.Command, flagName, propertyName, paramType string, required bool) {
	if cmd == nil {
		return
	}
	flagName = strings.TrimSpace(flagName)
	if flagName == "" {
		return
	}
	flag := CommandFlag(cmd, flagName)
	if flag == nil {
		return
	}
	SetFlagAnnotation(flag, AnnotationFlagProperty, strings.TrimSpace(propertyName))
	SetFlagAnnotation(flag, AnnotationFlagType, strings.TrimSpace(paramType))
	SetFlagAnnotation(flag, AnnotationFlagRequired, strconv.FormatBool(required))
}

// AnnotateRuntimeFlagProperty records only the stable CLI flag to interface
// property binding.
func AnnotateRuntimeFlagProperty(cmd *cobra.Command, flagName, propertyName string) {
	if cmd == nil {
		return
	}
	if flag := CommandFlag(cmd, flagName); flag != nil {
		SetFlagAnnotation(flag, AnnotationFlagProperty, strings.TrimSpace(propertyName))
	}
}

// AnnotateRuntimeRequiredFlags records schema-only required semantics.
func AnnotateRuntimeRequiredFlags(cmd *cobra.Command, flagNames ...string) {
	if cmd == nil {
		return
	}
	for _, name := range flagNames {
		flag := CommandFlag(cmd, name)
		if flag != nil {
			SetFlagAnnotation(flag, AnnotationFlagRequired, "true")
		}
	}
}

// AnnotateRuntimeFlagRequiredValue sets an explicit required value on a flag.
func AnnotateRuntimeFlagRequiredValue(cmd *cobra.Command, flagName string, required bool) {
	if cmd == nil {
		return
	}
	if flag := CommandFlag(cmd, flagName); flag != nil {
		v := "false"
		if required {
			v = "true"
		}
		SetFlagAnnotation(flag, AnnotationFlagRequired, v)
	}
}

// AnnotateRuntimeFlagDescription records the Schema parameter description.
func AnnotateRuntimeFlagDescription(cmd *cobra.Command, flagName, description string) {
	if cmd == nil {
		return
	}
	if flag := CommandFlag(cmd, flagName); flag != nil {
		SetFlagAnnotation(flag, AnnotationDescription, strings.TrimSpace(description))
	}
}

// AnnotateRuntimeFlagRequiredWhen records a conditional CLI requirement.
func AnnotateRuntimeFlagRequiredWhen(cmd *cobra.Command, flagName, expression string) {
	if cmd == nil {
		return
	}
	if flag := CommandFlag(cmd, flagName); flag != nil {
		SetFlagAnnotation(flag, AnnotationFlagReqWhen, strings.TrimSpace(expression))
	}
}

// AnnotateRuntimeFlagFormat records a machine-readable value format.
func AnnotateRuntimeFlagFormat(cmd *cobra.Command, flagName, format string) {
	if cmd == nil {
		return
	}
	if flag := CommandFlag(cmd, flagName); flag != nil {
		SetFlagAnnotation(flag, AnnotationFlagFormat, strings.TrimSpace(format))
	}
}

// AnnotateRuntimeFlagInterfaceType records the wire type for a flag's interface
// property.
func AnnotateRuntimeFlagInterfaceType(cmd *cobra.Command, flagName, interfaceType string) {
	if cmd == nil {
		return
	}
	if flag := CommandFlag(cmd, flagName); flag != nil {
		SetFlagAnnotation(flag, AnnotationFlagType, strings.TrimSpace(interfaceType))
	}
}

// AnnotateRuntimeFlagEnum records the accepted values for a flag.
func AnnotateRuntimeFlagEnum(cmd *cobra.Command, flagName string, values ...string) {
	if cmd == nil {
		return
	}
	flag := CommandFlag(cmd, flagName)
	if flag == nil {
		return
	}
	SetFlagAnnotationValues(flag, AnnotationFlagEnum, values...)
}

// AnnotateRuntimeFlagExample records a valid representative CLI value.
func AnnotateRuntimeFlagExample(cmd *cobra.Command, flagName, example string) {
	if cmd == nil {
		return
	}
	if flag := CommandFlag(cmd, flagName); flag != nil {
		SetFlagAnnotation(flag, AnnotationFlagExample, strings.TrimSpace(example))
	}
}

// AnnotateRuntimeContract marks a command as carrying a command/LeafSpec
// Contract surface.
func AnnotateRuntimeContract(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	SetCommandAnnotation(cmd, AnnotationContract, "command")
}

// RuntimeContractRisk returns the Contract Risk annotation when present.
// Writers are retired: production leaves declare contract.SafetySpec through
// ContractFinal, and assembly consults this residual annotation only as the
// fallback when a leaf carries no ContractFinal Safety.
func RuntimeContractRisk(cmd *cobra.Command) (string, bool) {
	if cmd == nil || cmd.Annotations == nil {
		return "", false
	}
	risk := strings.TrimSpace(cmd.Annotations[AnnotationRisk])
	if risk == "" {
		return "", false
	}
	return risk, true
}

// RuntimeContractGate returns the annotated runtime confirmation gate when
// present. Like RuntimeContractRisk it is read-only residual plumbing: the
// writer is retired and declared Contract SafetySpec is the production path.
func RuntimeContractGate(cmd *cobra.Command) (string, bool) {
	if cmd == nil || cmd.Annotations == nil {
		return "", false
	}
	gate := strings.TrimSpace(cmd.Annotations[AnnotationRuntimeGate])
	if gate == "" {
		return "", false
	}
	return gate, true
}

// AnnotateRuntimePositionals records ordered positional arguments for agents.
func AnnotateRuntimePositionals(cmd *cobra.Command, positionals ...contract.RuntimeSchemaPositional) {
	if cmd == nil {
		return
	}
	clean := make([]contract.RuntimeSchemaPositional, 0, len(positionals))
	for _, positional := range positionals {
		positional.Name = strings.TrimSpace(positional.Name)
		positional.Type = strings.TrimSpace(positional.Type)
		positional.Description = strings.TrimSpace(positional.Description)
		if positional.Name == "" || positional.Index < 0 {
			continue
		}
		if positional.Type == "" {
			positional.Type = "string"
		}
		clean = append(clean, positional)
	}
	if len(clean) == 0 {
		return
	}
	sort.SliceStable(clean, func(i, j int) bool { return clean[i].Index < clean[j].Index })
	encoded, _ := json.Marshal(clean)
	SetCommandAnnotation(cmd, AnnotationPositionals, string(encoded))
}

// CommandPositionals reads annotated positionals (nil on miss/error).
func CommandPositionals(cmd *cobra.Command) []contract.RuntimeSchemaPositional {
	if cmd == nil || cmd.Annotations == nil {
		return nil
	}
	raw := strings.TrimSpace(cmd.Annotations[AnnotationPositionals])
	if raw == "" {
		return nil
	}
	var positionals []contract.RuntimeSchemaPositional
	if json.Unmarshal([]byte(raw), &positionals) != nil {
		return nil
	}
	sort.SliceStable(positionals, func(i, j int) bool { return positionals[i].Index < positionals[j].Index })
	return positionals
}

// CommandFlag resolves local flags plus product/group persistent flags.
// Root persistent flags are intentionally available only when explicitly
// requested; they are global execution controls, not tool parameters.
func CommandFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if cmd == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag
	}
	for current := cmd; current != nil; current = current.Parent() {
		if flag := current.PersistentFlags().Lookup(name); flag != nil {
			return flag
		}
	}
	return nil
}

// SetCommandAnnotation writes a non-blank command annotation.
func SetCommandAnnotation(cmd *cobra.Command, key, value string) {
	if cmd == nil || strings.TrimSpace(value) == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[key] = value
}

// SetFlagAnnotation writes a single non-blank flag annotation value.
func SetFlagAnnotation(flag *pflag.Flag, key, value string) {
	if flag == nil || strings.TrimSpace(value) == "" {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[key] = []string{value}
}

// SetFlagAnnotationValues writes a trimmed, non-empty flag annotation list.
func SetFlagAnnotationValues(flag *pflag.Flag, key string, values ...string) {
	if flag == nil {
		return
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			clean = append(clean, value)
		}
	}
	if len(clean) == 0 {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[key] = clean
}
