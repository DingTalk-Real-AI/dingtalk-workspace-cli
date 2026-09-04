// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestCrossPlatformCoverageRequiredParameterToOneOf(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(*toolSchema, *toolSchema)
		compatible bool
	}{
		{name: "required becomes an alternative", compatible: true},
		{name: "required remains required", compatible: true, mutate: func(_, next *toolSchema) {
			p := next.Parameters["enabled"]
			p.Required = true
			next.Parameters["enabled"] = p
		}},
		{name: "historically optional", mutate: func(old, _ *toolSchema) {
			p := old.Parameters["enabled"]
			p.Required = false
			old.Parameters["enabled"] = p
		}},
		{name: "conditional requirement", mutate: func(old, _ *toolSchema) {
			p := old.Parameters["enabled"]
			p.RequiredWhen = "mode=share"
			old.Parameters["enabled"] = p
		}},
		{name: "default does not prove presence", mutate: func(old, next *toolSchema) {
			for _, tool := range []*toolSchema{old, next} {
				p := tool.Parameters["enabled"]
				p.Default = `"false"`
				tool.Parameters["enabled"] = p
			}
		}},
		{name: "required member outside group", mutate: func(_, next *toolSchema) {
			next.Constraints = `{"require_one_of":[["description"]]}`
		}},
		{name: "each new group must be implied", mutate: func(_, next *toolSchema) {
			next.Constraints = `{"require_one_of":[["enabled","description"],["description"]]}`
		}},
		{name: "removed old parameter still fails", mutate: func(_, next *toolSchema) {
			delete(next.Parameters, "enabled")
		}},
		{name: "parameter type drift still fails", mutate: func(_, next *toolSchema) {
			p := next.Parameters["enabled"]
			p.Type = `"boolean"`
			next.Parameters["enabled"] = p
		}},
		{name: "interface type drift still fails", mutate: func(_, next *toolSchema) {
			p := next.Parameters["enabled"]
			p.InterfaceType = "boolean"
			next.Parameters["enabled"] = p
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := toolSchema{Parameters: map[string]parameterSchema{
				"enabled": {Type: `"string"`, Required: true},
			}}
			next := toolSchema{Parameters: map[string]parameterSchema{
				"enabled":     {Type: `"string"`},
				"description": {Type: `"string"`},
			}, Constraints: `{"require_one_of":[["enabled","description"]]}`}
			if tc.mutate != nil {
				tc.mutate(&old, &next)
			}
			failures := checkToolCompatibility("example/example.partial_update", old, next)
			if (len(failures) == 0) != tc.compatible {
				t.Fatalf("compatible=%v failures=%v", tc.compatible, failures)
			}
		})
	}
}
