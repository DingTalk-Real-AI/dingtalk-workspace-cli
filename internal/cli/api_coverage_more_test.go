package cli

import (
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRuntimeAnnotationAPIsCoverage(t *testing.T) {
	AttachRuntimeSchema(nil, "p", "t", "s")
	AttachRuntimeSchema(&cobra.Command{}, "", "", "")
	AnnotateRuntimeFlag(nil, "x", "x", "string", false)
	AnnotateRuntimeFlagProperty(nil, "x", "x")
	AnnotateRuntimeRequiredFlags(nil, "x")
	AnnotateRuntimeFlagRequiredValue(nil, "x", false)
	AnnotateRuntimeFlagInterfaceType(nil, "x", "string")
	AnnotateRuntimeFlagRequiredWhen(nil, "x", "when")
	AnnotateRuntimeFlagFormat(nil, "x", "uri")
	AnnotateRuntimeFlagEnum(nil, "x", "a")
	AnnotateRuntimeFlagExample(nil, "x", "a")
	AnnotateRuntimeConstraints(nil, RuntimeSchemaConstraints{})
	AnnotateRuntimePositionals(nil)

	cmd := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().StringP("value", "v", "", "value")
	AttachRuntimeSchema(cmd, " product ", " tool ", " source ")
	AnnotateRuntimeFlag(cmd, "", "property", "string", true)
	AnnotateRuntimeFlag(cmd, "missing", "property", "string", true)
	AnnotateRuntimeFlag(cmd, "value", " property ", " string ", true)
	AnnotateRuntimeFlagProperty(cmd, "missing", "property")
	AnnotateRuntimeFlagProperty(cmd, "value", "property2")
	AnnotateRuntimeRequiredFlags(cmd, "missing", "value")
	AnnotateRuntimeFlagRequiredWhen(cmd, "missing", "when")
	AnnotateRuntimeFlagRequiredWhen(cmd, "value", " when ")
	AnnotateRuntimeFlagFormat(cmd, "missing", "uri")
	AnnotateRuntimeFlagFormat(cmd, "value", " uri ")
	AnnotateRuntimeFlagEnum(cmd, "missing", "a")
	AnnotateRuntimeFlagEnum(cmd, "value", " ", "a", " b ")
	AnnotateRuntimeFlagExample(cmd, "missing", "x")
	AnnotateRuntimeFlagExample(cmd, "value", " example ")
	AnnotateRuntimeConstraints(cmd, RuntimeSchemaConstraints{})
	AnnotateRuntimeConstraints(cmd, RuntimeSchemaConstraints{RequireOneOf: [][]string{{"value", "other"}}})
	AnnotateRuntimeConstraints(cmd, RuntimeSchemaConstraints{RequireTogether: [][]string{{"value", "other"}}})
	AnnotateRuntimePositionals(cmd,
		contract.RuntimeSchemaPositional{Name: "", Index: 0},
		contract.RuntimeSchemaPositional{Name: "bad", Index: -1},
		contract.RuntimeSchemaPositional{Name: " second ", Index: 1, Description: " desc "},
		contract.RuntimeSchemaPositional{Name: "first", Index: 0, Type: " number "},
	)
	setRuntimeCommandAnnotation(cmd, "empty", " ")
	setFlagAnnotation(nil, "x", "y")
	setFlagAnnotation(cmd.Flags().Lookup("value"), "empty", " ")
	setFlagAnnotationValues(nil, "x", "y")
	setFlagAnnotationValues(cmd.Flags().Lookup("value"), "empty", " ")
	if cmd.Annotations[runtimeSchemaProductAnnotation] != "product" {
		t.Fatalf("command annotations = %#v", cmd.Annotations)
	}
}

func TestCrossPlatformCoverageRuntimeRegistriesCoverage(t *testing.T) {
	originalConstraints := runtimeSchemaConstraintsByCanonical
	originalParameters := runtimeSchemaParameterMetadataByCanonical
	t.Cleanup(func() {
		runtimeSchemaConstraintsByCanonical = originalConstraints
		runtimeSchemaParameterMetadataByCanonical = originalParameters
	})
	runtimeSchemaConstraintsByCanonical = map[string]RuntimeSchemaConstraints{}
	RegisterRuntimeSchemaConstraints("", RuntimeSchemaConstraints{})
	RegisterRuntimeSchemaConstraints("sample.run", RuntimeSchemaConstraints{MutuallyExclusive: [][]string{{"a", "b"}}})
	if len(runtimeSchemaConstraintsByCanonical) != 1 {
		t.Fatalf("constraints = %#v", runtimeSchemaConstraintsByCanonical)
	}
	runtimeSchemaParameterMetadataByCanonical = map[string]RuntimeSchemaParameterMetadata{}
	RegisterRuntimeSchemaParameterMetadata("", RuntimeSchemaParameterMetadata{})
	metadata := RuntimeSchemaParameterMetadata{
		Inherited:    []string{"global"},
		Required:     []string{"required"},
		RequiredWhen: map[string]string{"conditional": "when"},
		Formats:      map[string]string{"formatted": "uri"},
		Enums:        map[string][]string{"enum": {"a", "b"}},
		Examples:     map[string]string{"example": "value"},
	}
	RegisterRuntimeSchemaParameterMetadata("sample.run", metadata)
	definitions := RuntimeSchemaParameterMetadataDefinitions()
	definitions["sample.run"].Required[0] = "changed"
	definitions["sample.run"].Enums["enum"][0] = "changed"
	if reflect.DeepEqual(definitions["sample.run"], runtimeSchemaParameterMetadataByCanonical["sample.run"]) {
		t.Fatal("RuntimeSchemaParameterMetadataDefinitions returned an aliased value")
	}
	if cloneRuntimeSchemaStringMap(nil) != nil {
		t.Fatal("cloneRuntimeSchemaStringMap(nil) != nil")
	}
	assertPanics(t, func() { RegisterRuntimeSchemaParameterMetadata("sample.run", metadata) })
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
