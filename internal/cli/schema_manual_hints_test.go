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
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestManualSchemaHintsIncludeExistingLeafAndOverrideParameters(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	description := "Reviewed query text"
	property := "queryText"
	interfaceType := "object"
	required := true
	requiredWhen := "mode is advanced"
	snapshot := ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.search_items",
			Reason:        "Reviewed public helper",
			Reviewed:      true,
			Parameters: map[string]ManualSchemaParameterHint{
				"query": {
					Description:   &description,
					Property:      &property,
					InterfaceType: &interfaceType,
					Required:      &required,
					RequiredWhen:  &requiredWhen,
				},
			},
		}},
	}

	report, err := applyManualSchemaHints(root, snapshot)
	if err != nil {
		t.Fatalf("applyManualSchemaHints() error = %v", err)
	}
	if len(report.Commands) != 1 || report.Commands[0] != "sample item search" {
		t.Fatalf("report.Commands = %#v", report.Commands)
	}
	productID, toolName, source := runtimeSchemaAnnotations(leaf)
	if productID != "sample" || toolName != "search_items" || source != "manual-schema-hint" {
		t.Fatalf("Schema identity = %s.%s (%s)", productID, toolName, source)
	}

	payload, err := runtimeSchemaPayload(root, []string{"sample.search_items"})
	if err != nil {
		t.Fatalf("runtimeSchemaPayload() error = %v", err)
	}
	parameters := schemaMap(payload["parameters"])
	query := parameters["query"]
	if query["description"] != description || query["property"] != property || query["interface_type"] != interfaceType || query["required"] != true || query["required_when"] != requiredWhen {
		t.Fatalf("query parameter = %#v", query)
	}
	if leaf.Flags().Lookup("query").Usage != "Original query text" {
		t.Fatalf("human Schema hint changed Cobra help: %q", leaf.Flags().Lookup("query").Usage)
	}
	if len(leaf.Flags().Lookup("query").Annotations[cobra.BashCompOneRequiredFlag]) != 0 {
		t.Fatal("manual Schema hint changed Cobra execution validation")
	}
}

func TestManualSchemaHintsRejectInvalidOrStaleInputs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ManualSchemaCommandHint)
		wantErr string
	}{
		{name: "missing command", mutate: func(h *ManualSchemaCommandHint) { h.CLIPath = "sample item missing" }, wantErr: "does not resolve"},
		{name: "wildcard command", mutate: func(h *ManualSchemaCommandHint) { h.CLIPath = "sample item *" }, wantErr: "invalid exact cli_path"},
		{name: "not reviewed", mutate: func(h *ManualSchemaCommandHint) { h.Reviewed = false }, wantErr: "not reviewed"},
		{name: "missing reason", mutate: func(h *ManualSchemaCommandHint) { h.Reason = "" }, wantErr: "has no reason"},
		{name: "bad canonical", mutate: func(h *ManualSchemaCommandHint) { h.CanonicalPath = "sample" }, wantErr: "invalid canonical_path"},
		{name: "missing flag", mutate: func(h *ManualSchemaCommandHint) {
			h.Parameters = map[string]ManualSchemaParameterHint{"missing": {Required: boolPointer(true)}}
		}, wantErr: "missing flag --missing"},
		{name: "invalid interface type", mutate: func(h *ManualSchemaCommandHint) {
			h.Parameters = map[string]ManualSchemaParameterHint{"query": {InterfaceType: stringPointer("made-up")}}
		}, wantErr: "unsupported interface_type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, _ := manualSchemaHintTestTree()
			hint := ManualSchemaCommandHint{
				CLIPath:       "sample item search",
				CanonicalPath: "sample.search_items",
				Reason:        "Reviewed public helper",
				Reviewed:      true,
			}
			test.mutate(&hint)
			_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
				Schema:   manualSchemaHintSchemaRef,
				Version:  manualSchemaHintVersion,
				Commands: []ManualSchemaCommandHint{hint},
			})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestManualSchemaHintsRejectCanonicalConflict(t *testing.T) {
	root, leaf := manualSchemaHintTestTree()
	AttachRuntimeSchema(leaf, "sample", "existing", "test")
	_, err := applyManualSchemaHints(root, ManualSchemaHintSnapshot{
		Schema:  manualSchemaHintSchemaRef,
		Version: manualSchemaHintVersion,
		Commands: []ManualSchemaCommandHint{{
			CLIPath:       "sample item search",
			CanonicalPath: "sample.replacement",
			Reason:        "Reviewed public helper",
			Reviewed:      true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with existing canonical path") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeManualSchemaHintsRejectsUnknownFields(t *testing.T) {
	_, err := decodeManualSchemaHints([]byte(`{"$schema":"./schema_manual_hints.schema.json","version":1,"commands":[],"allow_virtual_commands":true}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeManualSchemaHintsRequiresDiscoverableSchema(t *testing.T) {
	_, err := decodeManualSchemaHints([]byte(`{"version":1,"commands":[]}`))
	if err == nil || !strings.Contains(err.Error(), "must declare $schema") {
		t.Fatalf("error = %v", err)
	}
}

func manualSchemaHintTestTree() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	group := &cobra.Command{Use: "item"}
	leaf := &cobra.Command{Use: "search", RunE: func(*cobra.Command, []string) error { return nil }}
	leaf.Flags().String("query", "", "Original query text")
	group.AddCommand(leaf)
	product.AddCommand(group)
	root.AddCommand(product)
	return root, leaf
}

func boolPointer(value bool) *bool {
	return &value
}

func stringPointer(value string) *string {
	return &value
}
