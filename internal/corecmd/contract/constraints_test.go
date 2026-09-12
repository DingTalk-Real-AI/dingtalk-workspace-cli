// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package contract

import (
	"reflect"
	"testing"
)

func TestRuntimeSchemaConstraintsNormalization(t *testing.T) {
	in := RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{" a ", "b", "a"}, {"only"}},
		RequireOneOf:      [][]string{{" id "}, {"id"}},
		RequireTogether:   [][]string{{"x", " y "}, {"x", "y"}},
	}
	got := NormalizeRuntimeSchemaConstraints(in)
	want := RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{"a", "b"}},
		RequireOneOf:      [][]string{{"id"}},
		RequireTogether:   [][]string{{"x", "y"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeRuntimeSchemaConstraints() = %#v, want %#v", got, want)
	}
	if RuntimeSchemaConstraintsEmpty(got) || !RuntimeSchemaConstraintsEmpty(RuntimeSchemaConstraints{}) {
		t.Fatalf("RuntimeSchemaConstraintsEmpty() mismatch")
	}
}
