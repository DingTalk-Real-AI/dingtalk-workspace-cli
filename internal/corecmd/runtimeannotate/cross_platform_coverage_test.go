// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimeannotate

import (
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAnnotateRuntimeAPIs(t *testing.T) {
	AttachRuntimeSchema(nil, "p", "t", "s")
	AttachRuntimeSchema(&cobra.Command{}, "", "", "")
	AnnotateRuntimeFlag(nil, "x", "x", "string", false)
	AnnotateRuntimeFlagProperty(nil, "x", "x")
	AnnotateRuntimeRequiredFlags(nil, "x")
	AnnotateRuntimeFlagRequiredValue(nil, "x", false)
	AnnotateRuntimeFlagDescription(nil, "x", "d")
	AnnotateRuntimeFlagInterfaceType(nil, "x", "string")
	AnnotateRuntimeFlagRequiredWhen(nil, "x", "when")
	AnnotateRuntimeFlagFormat(nil, "x", "uri")
	AnnotateRuntimeFlagEnum(nil, "x", "a")
	AnnotateRuntimeFlagExample(nil, "x", "a")
	AnnotateRuntimeContract(nil)
	AnnotateRuntimeConstraints(nil, RuntimeSchemaConstraints{})
	AnnotateRuntimePositionals(nil)
	_ = CommandFlag(nil, "x")
	_ = CommandPositionals(nil)
	_ = CommandConstraints(nil)
	SetCommandAnnotation(nil, "k", "v")
	SetFlagAnnotation(nil, "k", "v")
	SetFlagAnnotationValues(nil, "k", "v")

	parent := &cobra.Command{Use: "parent"}
	parent.PersistentFlags().String("inherited", "", "inherited")
	cmd := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	parent.AddCommand(cmd)
	cmd.Flags().String("value", "", "value")

	AttachRuntimeSchema(cmd, " product ", " tool ", " source ")
	if cmd.Annotations[AnnotationProduct] != "product" || cmd.Annotations[AnnotationTool] != "tool" {
		t.Fatalf("AttachRuntimeSchema annotations = %#v", cmd.Annotations)
	}

	AnnotateRuntimeFlag(cmd, "", "property", "string", true)
	AnnotateRuntimeFlag(cmd, "missing", "property", "string", true)
	AnnotateRuntimeFlag(cmd, "value", " property ", " string ", true)
	AnnotateRuntimeFlagProperty(cmd, "missing", "property")
	AnnotateRuntimeFlagProperty(cmd, "value", "property2")
	AnnotateRuntimeRequiredFlags(cmd, "missing", "value")
	AnnotateRuntimeFlagRequiredValue(cmd, "missing", false)
	AnnotateRuntimeFlagRequiredValue(cmd, "value", true)
	AnnotateRuntimeFlagDescription(cmd, "missing", "desc")
	AnnotateRuntimeFlagDescription(cmd, "value", " desc ")
	AnnotateRuntimeFlagRequiredWhen(cmd, "missing", "when")
	AnnotateRuntimeFlagRequiredWhen(cmd, "value", " when ")
	AnnotateRuntimeFlagFormat(cmd, "missing", "uri")
	AnnotateRuntimeFlagFormat(cmd, "value", " uri ")
	AnnotateRuntimeFlagInterfaceType(cmd, "missing", "string")
	AnnotateRuntimeFlagInterfaceType(cmd, "value", " string ")
	AnnotateRuntimeFlagEnum(cmd, "missing", "a")
	AnnotateRuntimeFlagEnum(cmd, "value", " ", "a", " b ")
	AnnotateRuntimeFlagExample(cmd, "missing", "x")
	AnnotateRuntimeFlagExample(cmd, "value", " example ")

	valueFlag := cmd.Flags().Lookup("value")
	if valueFlag == nil {
		t.Fatal("value flag must exist")
	}
	for key, wants := range map[string][]string{
		AnnotationFlagType:     {"string"},
		AnnotationDescription:  {"desc"},
		AnnotationFlagFormat:   {"uri"},
		AnnotationFlagExample:  {"example"},
		AnnotationFlagRequired: {"true"},
		AnnotationFlagReqWhen:  {"when"},
		AnnotationFlagEnum:     {"a", "b"},
	} {
		got := valueFlag.Annotations[key]
		for _, want := range wants {
			found := false
			for _, v := range got {
				if v == want {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("value flag annotation %s = %#v, want to contain %q", key, got, want)
			}
		}
	}

	if flag := CommandFlag(cmd, "inherited"); flag == nil {
		t.Fatal("CommandFlag must resolve persistent parent flag")
	}
	if CommandFlag(cmd, "") != nil || CommandFlag(cmd, "nope") != nil {
		t.Fatal("CommandFlag guards")
	}

	AnnotateRuntimeContract(cmd)
	AnnotateRuntimeConstraints(cmd, RuntimeSchemaConstraints{})
	AnnotateRuntimeConstraints(cmd, RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{"value", "other"}},
		RequireOneOf:      [][]string{{"value"}, {" ", "value"}},
		RequireTogether:   [][]string{{"value", "other", "other"}},
	})
	AnnotateRuntimeConstraints(cmd, RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"value", "other"}},
	})
	gotConstraints := CommandConstraints(cmd)
	if ConstraintsEmpty(gotConstraints) || len(gotConstraints.RequireOneOf) == 0 {
		t.Fatalf("CommandConstraints = %#v", gotConstraints)
	}
	_ = CommandConstraints(&cobra.Command{Annotations: map[string]string{AnnotationConstraints: "{"}})
	_ = CommandConstraints(&cobra.Command{Annotations: map[string]string{AnnotationConstraints: "   "}})

	AnnotateRuntimePositionals(cmd,
		contract.RuntimeSchemaPositional{Name: "", Index: 0},
		contract.RuntimeSchemaPositional{Name: "bad", Index: -1},
		contract.RuntimeSchemaPositional{Name: " second ", Index: 1, Description: " desc "},
		contract.RuntimeSchemaPositional{Name: "first", Index: 0, Type: " number "},
	)
	if got := CommandPositionals(cmd); len(got) != 2 || got[0].Name != "first" {
		t.Fatalf("CommandPositionals = %#v", got)
	}
	_ = CommandPositionals(&cobra.Command{Annotations: map[string]string{AnnotationPositionals: "{"}})
	_ = CommandPositionals(&cobra.Command{Annotations: map[string]string{AnnotationPositionals: "   "}})
	AnnotateRuntimePositionals(cmd) // empty after filters is no-op path via clean empty? name empty skipped
	AnnotateRuntimePositionals(&cobra.Command{Use: "empty"})

	SetCommandAnnotation(cmd, "k", " ")
	SetCommandAnnotation(cmd, "k", "v")
	flag := cmd.Flags().Lookup("value")
	SetFlagAnnotation(flag, "empty", " ")
	SetFlagAnnotation(flag, "k", "v")
	SetFlagAnnotationValues(flag, "empty", " ")
	SetFlagAnnotationValues(flag, "enums", " ", "a", "b")
	flag.Annotations = nil
	SetFlagAnnotationValues(flag, "enums2", "a")
	if len(flag.Annotations["enums2"]) != 1 {
		t.Fatalf("SetFlagAnnotationValues nil-map init = %#v", flag.Annotations)
	}

	// JSON round-trip smoke for constraints encode path.
	raw, err := json.Marshal(gotConstraints)
	if err != nil || len(raw) == 0 {
		t.Fatalf("marshal constraints: %v", err)
	}
}

func TestCrossPlatformCoverageNormalizeConstraintsEdges(t *testing.T) {
	got := NormalizeConstraints(RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{"a"}, {"a", "b"}, {"a", "b"}, {"", "c", "c", "d"}},
		RequireOneOf:      [][]string{{}, {"x"}, {"x", "y"}},
		RequireTogether:   [][]string{{"only"}, {"p", "q"}},
	})
	if len(got.MutuallyExclusive) != 2 || len(got.RequireOneOf) != 2 || len(got.RequireTogether) != 1 {
		t.Fatalf("NormalizeConstraints = %#v", got)
	}
	if !ConstraintsEmpty(RuntimeSchemaConstraints{}) {
		t.Fatal("empty constraints must report empty")
	}
}
