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
	"reflect"
	"strings"
	"testing"
)

func TestBuildInterfaceRegistryIndexesCanonicalAndSharedRef(t *testing.T) {
	shared := &embeddedMCPInterfaceRef{ProductID: "calendar-rpc", RPCName: "get_event"}
	registry, err := buildInterfaceRegistry(map[string]embeddedMCPToolMetadata{
		"calendar.event_get":  {InterfaceRef: shared},
		"calendar.event_read": {InterfaceRef: shared},
	})
	if err != nil {
		t.Fatalf("buildInterfaceRegistry() error = %v", err)
	}
	if got := registry.ByCanonical["calendar.event_get"].Ref; got.ProductID != "calendar-rpc" || got.RPCName != "get_event" {
		t.Fatalf("ByCanonical ref = %#v", got)
	}
	key := InterfaceRefKey{ProductID: "calendar-rpc", RPCName: "get_event"}
	if got, want := registry.ByInterfaceRef[key], []string{"calendar.event_get", "calendar.event_read"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ByInterfaceRef = %#v, want %#v", got, want)
	}
}

func TestBuildInterfaceRegistryRejectsCanonicalConflict(t *testing.T) {
	ref := &embeddedMCPInterfaceRef{ProductID: "calendar", RPCName: "get_event"}
	_, err := buildInterfaceRegistry(map[string]embeddedMCPToolMetadata{
		"calendar.event_get":   {InterfaceRef: ref},
		" calendar.event_get ": {InterfaceRef: ref},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("buildInterfaceRegistry() error = %v, want canonical conflict", err)
	}
}

func TestRuntimeEmbeddedMCPMetadataBuildsInterfaceRegistry(t *testing.T) {
	if _, err := buildInterfaceRegistry(runtimeMCPMetadata().Tools); err != nil {
		t.Fatalf("buildInterfaceRegistry(runtimeMCPMetadata().Tools) error = %v", err)
	}
}

func TestValidateSchemaRegistryInterfacesUsesExplicitRefNotCanonical(t *testing.T) {
	metadata := embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{
		"calendar.snapshot_name": {
			InterfaceRef: &embeddedMCPInterfaceRef{ProductID: "calendar-rpc", RPCName: "get_event"},
		},
	}}

	err := validateSchemaRegistryInterfacesWithMetadata(schemaWithInterfaceTool(
		"calendar.command_name_differs",
		InterfaceSpec{
			Mode:         "mcp",
			Availability: "available",
			Ref:          &InterfaceRefSpec{ProductID: "calendar-rpc", RPCName: "get_event"},
		},
	), metadata)
	if err != nil {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v", err)
	}
}

func TestValidateSchemaRegistryInterfacesRejectsMissingMCPRef(t *testing.T) {
	err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
		"calendar.event_get",
		InterfaceSpec{Mode: "mcp", Availability: "available"},
	))
	if err == nil || !strings.Contains(err.Error(), "has no interface_ref") {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v, want missing ref", err)
	}
}

func TestValidateSchemaRegistryInterfacesRejectsIncompleteMCPRef(t *testing.T) {
	err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
		"calendar.event_get",
		InterfaceSpec{
			Mode:         "mcp",
			Availability: "available",
			Ref:          &InterfaceRefSpec{ProductID: "calendar"},
		},
	))
	if err == nil || !strings.Contains(err.Error(), "has incomplete interface_ref") {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v, want incomplete ref", err)
	}
}

func TestValidateSchemaRegistryInterfacesRejectsPhantomRef(t *testing.T) {
	err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
		"calendar.event_get",
		InterfaceSpec{
			Mode:         "mcp",
			Availability: "available",
			Ref:          &InterfaceRefSpec{ProductID: "calendar", RPCName: "phantom"},
		},
	))
	if err == nil || !strings.Contains(err.Error(), "unknown MCP interface calendar.phantom") {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v, want phantom ref", err)
	}
}

func TestValidateSchemaRegistryInterfacesAllowsLocalWithoutRef(t *testing.T) {
	err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
		"calendar.local_cache",
		InterfaceSpec{Mode: "local", Availability: "available", Reason: "implemented by the local CLI"},
	))
	if err != nil {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v", err)
	}
}

func TestValidateSchemaRegistryInterfacesAllowsCompositeWithoutRef(t *testing.T) {
	err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
		"calendar.composite_sync",
		InterfaceSpec{Mode: "composite", Availability: "available", Reason: "orchestrates multiple operations"},
	))
	if err != nil {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v", err)
	}
}

func TestValidateSchemaRegistryInterfacesRejectsAnyLocalRef(t *testing.T) {
	err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
		"calendar.local_cache",
		InterfaceSpec{
			Mode:         "local",
			Availability: "available",
			Reason:       "implemented by the local CLI",
			Ref:          &InterfaceRefSpec{ProductID: "calendar", RPCName: "get_event"},
		},
	))
	if err == nil || !strings.Contains(err.Error(), "must not declare interface_ref") {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v, want forbidden local ref", err)
	}
}

func TestValidateSchemaRegistryInterfacesAcceptsReviewedUnavailable(t *testing.T) {
	err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
		"calendar.retired",
		InterfaceSpec{Mode: "local", Availability: "unavailable", Reason: "the upstream operation was retired"},
	))
	if err != nil {
		t.Fatalf("validateSchemaRegistryInterfaces() error = %v", err)
	}
}

func TestInterfaceSpecDispositionMatrix(t *testing.T) {
	validRef := &InterfaceRefSpec{ProductID: "calendar", RPCName: "get_event"}
	for _, test := range []struct {
		name       string
		spec       InterfaceSpec
		wantError  string
		executable bool
	}{
		{name: "mcp available", spec: InterfaceSpec{Mode: "mcp", Availability: "available", Ref: validRef}, executable: true},
		{name: "local available", spec: InterfaceSpec{Mode: "local", Availability: "available"}, executable: true},
		{name: "composite available", spec: InterfaceSpec{Mode: "composite", Availability: "available", Reason: "orchestrates operations"}, executable: true},
		{name: "mcp unavailable", spec: InterfaceSpec{Mode: "mcp", Availability: "unavailable", Reason: "RPC retired"}},
		{name: "local unavailable", spec: InterfaceSpec{Mode: "local", Availability: "unavailable", Reason: "compatibility command frozen"}},
		{name: "composite unavailable", spec: InterfaceSpec{Mode: "composite", Availability: "unavailable", Reason: "workflow retired"}},
		{name: "unavailable is not a mode", spec: InterfaceSpec{Mode: "unavailable", Availability: "unavailable", Reason: "retired"}, wantError: "legacy interface_mode=unavailable; migrate"},
		{name: "missing mode", spec: InterfaceSpec{Availability: "available"}, wantError: "has no interface mode"},
		{name: "missing availability", spec: InterfaceSpec{Mode: "local"}, wantError: "has no interface availability"},
		{name: "unknown availability", spec: InterfaceSpec{Mode: "local", Availability: "disabled"}, wantError: `unknown interface availability "disabled"`},
		{name: "unavailable with ref", spec: InterfaceSpec{Mode: "mcp", Availability: "unavailable", Reason: "retired", Ref: validRef}, wantError: "unavailable interface must not declare interface_ref"},
		{name: "unavailable without reason", spec: InterfaceSpec{Mode: "local", Availability: "unavailable"}, wantError: "must declare interface_reason"},
		{name: "available mcp without ref", spec: InterfaceSpec{Mode: "mcp", Availability: "available"}, wantError: "has no interface_ref"},
		{name: "available local with ref", spec: InterfaceSpec{Mode: "local", Availability: "available", Ref: validRef}, wantError: "must not declare interface_ref"},
		{name: "available composite with ref", spec: InterfaceSpec{Mode: "composite", Availability: "available", Reason: "orchestrates operations", Ref: validRef}, wantError: "must not declare a single interface_ref"},
		{name: "available composite without reason", spec: InterfaceSpec{Mode: "composite", Availability: "available"}, wantError: "must declare interface_reason"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool("calendar.event_get", test.spec))
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateSchemaRegistryInterfaces() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateSchemaRegistryInterfaces() error = %v, want %q", err, test.wantError)
			}
			if got := test.spec.AgentExecutable(); got != test.executable {
				t.Fatalf("AgentExecutable() = %v, want %v", got, test.executable)
			}
		})
	}
}

func TestValidateSchemaRegistryInterfacesRejectsUnknownAndMissingMode(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
			"calendar.event_get",
			InterfaceSpec{Availability: "available"},
		))
		if err == nil || !strings.Contains(err.Error(), "has no interface mode") {
			t.Fatalf("validateSchemaRegistryInterfaces() error = %v, want missing mode", err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		err := validateSchemaRegistryInterfacesForTest(schemaWithInterfaceTool(
			"calendar.event_get",
			InterfaceSpec{Mode: "remote", Availability: "available"},
		))
		if err == nil || !strings.Contains(err.Error(), `unknown interface mode "remote"`) {
			t.Fatalf("validateSchemaRegistryInterfaces() error = %v, want unknown mode", err)
		}
	})
}

func schemaWithInterfaceTool(canonical string, interfaceSpec InterfaceSpec) SchemaRegistry {
	parts := strings.SplitN(canonical, ".", 2)
	identity := ToolIdentitySpec{CanonicalPath: canonical}
	if len(parts) == 2 {
		identity.ProductID = parts[0]
		identity.Name = parts[1]
	}
	return SchemaRegistry{Products: []ProductSpec{{
		ID: identity.ProductID,
		Tools: []ToolSpec{{
			Identity:  identity,
			Interface: interfaceSpec,
		}},
	}}}
}

func validInterfaceTools() map[string]embeddedMCPToolMetadata {
	return map[string]embeddedMCPToolMetadata{
		"calendar.event_get": {
			InterfaceRef: &embeddedMCPInterfaceRef{ProductID: "calendar", RPCName: "get_event"},
		},
	}
}

func validateSchemaRegistryInterfacesForTest(schema SchemaRegistry) error {
	return validateSchemaRegistryInterfacesWithMetadata(schema, embeddedMCPMetadata{Tools: validInterfaceTools()})
}
