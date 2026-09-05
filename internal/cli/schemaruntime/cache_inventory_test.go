// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestSchemaCacheRuntimeFieldInventory(t *testing.T) {
	tests := []struct {
		value any
		want  []string
	}{
		{SchemaRegistry{}, []string{"Kind:string", "Level:string", "Source:string", "Products:[]schemaruntime.ProductSpec", "AgentMetadata:json.RawMessage"}},
		{ProductSpec{}, []string{"ID:string", "Name:string", "Description:string", "Runtime:bool", "Tools:[]schemaruntime.ToolSpec", "Selection:contract.SelectionSpec", "FieldProvenance:map[string]contract.FieldProvenance"}},
		{ToolSpec{}, []string{"Identity:contract.ToolIdentitySpec", "Display:string", "Title:string", "Description:string", "MetadataSource:string", "Parameters:[]schemaruntime.ParameterSpec", "Constraints:contract.RuntimeSchemaConstraints", "Positionals:[]contract.RuntimeSchemaPositional", "DryRun:*contract.DryRunSpec", "Result:*contract.ResultSpec", "Pagination:*contract.PaginationSpec", "Safety:contract.SafetySpec", "Interface:contract.InterfaceSpec", "Selection:contract.SelectionSpec", "FieldProvenance:map[string]contract.FieldProvenance"}},
		{ParameterSpec{}, []string{"Name:string", "Type:string", "Description:string", "Property:string", "Required:bool", "CLIRequired:bool", "RequiredWhen:string", "Default:json.RawMessage", "InterfaceDefault:json.RawMessage", "Example:json.RawMessage", "Format:string", "Enum:[]string", "InterfaceDescription:string", "InterfaceType:string", "FieldProvenance:map[string]contract.FieldProvenance"}},
		{CommandMeta{}, []string{"Identity:schemaruntime.CommandIdentity", "Safety:schemaruntime.CommandSafety", "Selection:schemaruntime.CommandSelection"}},
		{CommandIdentity{}, []string{"CLIPath:string", "Canonical:string", "Aliases:[]string", "ProductID:string", "Title:string"}},
		{CommandSafety{}, []string{"Effect:string", "Risk:string", "Confirmation:string", "Idempotency:string"}},
		{CommandSelection{}, []string{"AgentSummary:string", "UseWhen:[]string", "AvoidWhen:[]string", "Prerequisites:[]string", "Tips:[]string", "Examples:[]string"}},
		{contract.ToolIdentitySpec{}, []string{"ProductID:string", "SourceProductID:string", "Name:string", "CLIName:string", "CanonicalPath:string", "Path:string", "CLIPath:string", "PrimaryCLIPath:string", "Group:string", "Aliases:[]string", "IsAlias:bool", "Source:string"}},
		{contract.RuntimeSchemaConstraints{}, []string{"MutuallyExclusive:[][]string", "RequireOneOf:[][]string", "RequireTogether:[][]string"}},
		{contract.RuntimeSchemaPositional{}, []string{"Name:string", "Type:string", "Description:string", "Required:bool", "Variadic:bool", "Index:int"}},
		{contract.DryRunSpec{}, []string{"PreviewKind:string", "RemoteReads:bool"}},
		{contract.ResultSpec{}, []string{"Outcomes:[]contract.ResultOutcome", "DataSchema:json.RawMessage", "SensitivePaths:[]string"}},
		{contract.PaginationSpec{}, []string{"Kind:string", "CursorParameter:string", "MetaPath:string", "EndpointExhaustedPath:string", "NextTokenPath:string"}},
		{contract.SafetySpec{}, []string{"Effect:string", "EffectSource:string", "Risk:string", "Confirmation:string", "Idempotency:string"}},
		{contract.InterfaceRefSpec{}, []string{"ProductID:string", "RPCName:string"}},
		{contract.InterfaceSpec{}, []string{"Ref:*contract.InterfaceRefSpec", "Mode:string", "Availability:string", "Reason:string"}},
		{contract.SelectionSpec{}, []string{"AgentSummary:string", "AgentSummarySource:string", "UseWhen:[]string", "AvoidWhen:[]string", "Prerequisites:[]string", "Tips:[]string", "WorkflowRefs:[]string", "Examples:[]string", "ExampleDispositions:[]contract.ExampleDisposition", "Reviewed:*bool", "SourceRefs:[]string", "MetadataSource:string"}},
		{contract.ExampleDisposition{}, []string{"Index:*int", "Mode:contract.ExampleDispositionMode", "ReasonCode:contract.ExampleDispositionReasonCode", "Reason:string", "Reviewed:bool"}},
		{contract.FieldProvenance{}, []string{"Value:json.RawMessage", "Source:string", "SourceRef:string", "Precedence:string", "Resolution:string", "ReviewReason:string", "Candidates:[]contract.FieldCandidateProvenance", "OverriddenCandidates:[]contract.FieldCandidateProvenance"}},
		{contract.FieldCandidateProvenance{}, []string{"Value:json.RawMessage", "Source:string", "SourceRef:string", "Precedence:string", "ReviewReason:string", "Selected:*bool"}},
	}
	for _, test := range tests {
		typeOf := reflect.TypeOf(test.value)
		got := make([]string, typeOf.NumField())
		for i := 0; i < typeOf.NumField(); i++ {
			field := typeOf.Field(i)
			got[i] = field.Name + ":" + shortTypeName(field.Type)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s field inventory changed without Schema cache codec review\nwant: %#v\n got: %#v", typeOf, test.want, got)
		}
	}
}

func TestSchemaCacheDescriptorContract(t *testing.T) {
	const expectedProtoSHA256 = "6a3a6c7d0f41c51069690cf563d63a33b52387b52512b54d6a114e4be43d56d6"
	source, err := os.ReadFile("../schemacachepb/schema_cache.proto")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != expectedProtoSHA256 {
		t.Fatalf("schema_cache.proto changed without DTO/version/visitor review: got %s want %s", got, expectedProtoSHA256)
	}
	file := schemacachepb.File_schema_cache_proto
	for i := 0; i < file.Messages().Len(); i++ {
		assertNoProtoMaps(t, file.Messages().Get(i))
	}
	assertFieldNumbers(t, (&schemacachepb.SchemaMetaCache{}).ProtoReflect().Descriptor(), []protoreflect.FieldNumber{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	assertFieldNumbers(t, (&schemacachepb.SchemaProductCache{}).ProtoReflect().Descriptor(), []protoreflect.FieldNumber{1, 2, 3})
}

func shortTypeName(value reflect.Type) string {
	if value.Name() != "" {
		if value.PkgPath() == "" {
			return value.Name()
		}
		parts := strings.Split(value.PkgPath(), "/")
		return parts[len(parts)-1] + "." + value.Name()
	}
	switch value.Kind() {
	case reflect.Pointer:
		return "*" + shortTypeName(value.Elem())
	case reflect.Slice:
		return "[]" + shortTypeName(value.Elem())
	case reflect.Map:
		return "map[" + shortTypeName(value.Key()) + "]" + shortTypeName(value.Elem())
	}
	return value.String()
}

func assertNoProtoMaps(t *testing.T, message protoreflect.MessageDescriptor) {
	t.Helper()
	for i := 0; i < message.Fields().Len(); i++ {
		field := message.Fields().Get(i)
		if field.IsMap() {
			t.Errorf("protobuf field %s is a forbidden map", field.FullName())
		}
	}
	for i := 0; i < message.Messages().Len(); i++ {
		assertNoProtoMaps(t, message.Messages().Get(i))
	}
}

func assertFieldNumbers(t *testing.T, message protoreflect.MessageDescriptor, want []protoreflect.FieldNumber) {
	t.Helper()
	got := make([]protoreflect.FieldNumber, message.Fields().Len())
	for i := range got {
		got[i] = message.Fields().Get(i).Number()
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s stable field numbers changed: got %v want %v", message.FullName(), got, want)
	}
}
