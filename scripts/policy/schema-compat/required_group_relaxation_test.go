// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCrossPlatformCoverageRequiredGroupRelaxation(t *testing.T) {
	for _, tc := range []struct {
		name, old, next string
		compatible      bool
	}{
		{"remove only required group", `{"require_one_of":[["a","b"]]}`, "", true},
		{"keep mutual exclusion", `{"require_one_of":[["a","b"]],"mutually_exclusive":[["a","b"]]}`, `{"mutually_exclusive":[["a","b"]]}`, true},
		{"remove one required group", `{"require_one_of":[["a","b"],["c"]]}`, `{"require_one_of":[["a","b"]]}`, true},
		{"retain and expand", `{"require_one_of":[["a"],["b"]]}`, `{"require_one_of":[["a","c"]]}`, true},
		{"replace with unrelated requirement", `{"require_one_of":[["a","b"]]}`, `{"require_one_of":[["c"]]}`, false},
		{"narrow remaining group", `{"require_one_of":[["a","b"],["c"]]}`, `{"require_one_of":[["a"]]}`, false},
		{"new requirement", "", `{"require_one_of":[["a"]]}`, false},
		{"remove mutual exclusion", `{"mutually_exclusive":[["a","b"]]}`, "", false},
		{"remove require together", `{"require_together":[["a","b"]]}`, "", false},
		{"add mutual exclusion on old inputs", `{"require_one_of":[["a","b"]]}`, `{"mutually_exclusive":[["a","b"]]}`, false},
		{"malformed new constraint", `{"require_one_of":[["a","b"]]}`, `{"require_one_of":"a"}`, false},
		{"unknown new constraint", `{"require_one_of":[["a","b"]]}`, `{"unknown":[["a"]]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, next := relaxationTool(tc.old), relaxationTool(tc.next)
			failures := checkToolCompatibility("example/example.create", old, next)
			if (len(failures) == 0) != tc.compatible {
				t.Fatalf("compatible=%v failures=%v", tc.compatible, failures)
			}
		})
	}
	for _, field := range []string{"parameter missing", "type", "format", "required", "primary path", "effect", "confirmation", "identity mode"} {
		t.Run(field, func(t *testing.T) {
			old, next := relaxationTool(`{"require_one_of":[["a","b"]]}`), relaxationTool("")
			p := next.Parameters["a"]
			switch field {
			case "parameter missing":
				delete(next.Parameters, "a")
			case "type":
				p.Type = `"boolean"`
				next.Parameters["a"] = p
			case "format":
				p.Format = "date-time"
				next.Parameters["a"] = p
			case "required":
				p.Required = true
				next.Parameters["a"] = p
			case "primary path":
				next.PrimaryCLIPath = "example other"
			case "effect":
				next.Effect = "write"
			case "confirmation":
				next.Confirmation = "not_required"
			case "identity mode":
				next.InterfaceMode = "mcp"
			}
			if failures := checkToolCompatibility("example/example.create", old, next); len(failures) == 0 {
				t.Fatal("independent contract drift accepted")
			}
		})
	}
}

func relaxationTool(constraints string) toolSchema {
	return toolSchema{Constraints: constraints, Parameters: map[string]parameterSchema{
		"a": {Type: `"string"`}, "b": {Type: `"string"`}, "c": {Type: `"string"`},
	}}
}

// Every accepted transition must preserve all historically valid assignments.
// Check all CNF collections over three optional parameters, not just named cases.
func TestCrossPlatformCoverageRequiredGroupRelaxationPreservesInvocations(t *testing.T) {
	constraints := func(collection int) string {
		groups := [][]string{}
		for members := 1; members < 8; members++ {
			if collection&(1<<(members-1)) == 0 {
				continue
			}
			group := []string{}
			for i, name := range []string{"a", "b", "c"} {
				if members&(1<<i) != 0 {
					group = append(group, name)
				}
			}
			groups = append(groups, group)
		}
		if len(groups) == 0 {
			return ""
		}
		b, _ := json.Marshal(map[string]any{"require_one_of": groups})
		return string(b)
	}
	valid := func(collection, present int) bool {
		for members := 1; members < 8; members++ {
			if collection&(1<<(members-1)) != 0 && members&present == 0 {
				return false
			}
		}
		return true
	}
	accepted := 0
	for old := 0; old < 128; old++ {
		for next := 0; next < 128; next++ {
			if len(checkToolCompatibility("example/example.create", relaxationTool(constraints(old)), relaxationTool(constraints(next)))) != 0 {
				continue
			}
			accepted++
			for present := 0; present < 8; present++ {
				if valid(old, present) && !valid(next, present) {
					t.Fatal(fmt.Sprintf("old=%d next=%d rejects historical assignment=%d", old, next, present))
				}
			}
		}
	}
	if accepted == 0 {
		t.Fatal("no transition accepted")
	}
	t.Logf("checked 16384 transitions; %d accepted transitions preserve every historical presence assignment", accepted)
}
