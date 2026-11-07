// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"reflect"
	"testing"
)

// Discover every field from the type, so adding metadata without updating the
// optimized comparator fails rather than silently weakening cache validation.
func TestCrossPlatformCoverageMetaEqualityCoversEveryField(t *testing.T) {
	var visit func(reflect.Type, []int, string)
	visit = func(typ reflect.Type, index []int, path string) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fieldIndex := append(append([]int(nil), index...), i)
			fieldPath := path + "." + field.Name
			if field.Type.Kind() == reflect.Struct {
				visit(field.Type, fieldIndex, fieldPath)
				continue
			}
			t.Run(fieldPath, func(t *testing.T) {
				var changed CommandMeta
				value := reflect.ValueOf(&changed).Elem().FieldByIndex(fieldIndex)
				switch value.Kind() {
				case reflect.String:
					value.SetString("changed")
				case reflect.Slice:
					if value.Type().Elem().Kind() != reflect.String {
						t.Fatalf("new slice type needs comparator coverage: %v", value.Type())
					}
					// An empty present slice must differ from nil.
					value.Set(reflect.MakeSlice(value.Type(), 0, 0))
				default:
					t.Fatalf("new field type needs comparator coverage: %v", value.Type())
				}
				if equalCommandMeta(CommandMeta{}, changed) || !equalCommandMeta(changed, changed) {
					t.Fatal("comparator missed a field or presence difference")
				}
				if value.Kind() == reflect.Slice {
					empty := changed
					value.Set(reflect.ValueOf([]string{"first", "second"}))
					if equalCommandMeta(empty, changed) {
						t.Fatal("comparator missed slice values")
					}
					forward := changed
					value.Set(reflect.ValueOf([]string{"second", "first"}))
					if equalCommandMeta(forward, changed) {
						t.Fatal("comparator missed slice order")
					}
				}
			})
		}
	}
	visit(reflect.TypeFor[CommandMeta](), nil, "CommandMeta")
}

func TestCrossPlatformCoverageMetaLookupEqualityMatchesReflection(t *testing.T) {
	fixtures := []map[string]CommandMeta{
		nil, {}, {"primary": {}}, {"alias": {}},
		{"primary": {Identity: CommandIdentity{Aliases: []string{}}}},
		{"primary": {}, "alias": {}},
		{"primary": {Safety: CommandSafety{Confirmation: "user_required"}}},
	}
	for i, left := range fixtures {
		for j, right := range fixtures {
			if got, want := equalCommandMetaLookups(left, right), reflect.DeepEqual(left, right); got != want {
				t.Fatalf("fixture %d / %d: got %v want %v", i, j, got, want)
			}
		}
	}
}
