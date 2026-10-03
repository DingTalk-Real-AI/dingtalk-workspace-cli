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

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// TestCrossPlatformCoverageSQLSheetKeyDeliveredSchema checks the public command and schema contracts.
func TestCrossPlatformCoverageSQLSheetKeyDeliveredSchema(t *testing.T) {
	for _, tc := range []struct{ action, tool, effect, confirmation string }{
		{"create", "create_sql_sheet_api_key", "write", "user_required"},
		{"list", "list_sql_sheet_api_keys", "read", "not_required"},
		{"revoke", "revoke_sql_sheet_api_key", "destructive", "user_required"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			read := func(compact bool) map[string]any {
				root := NewRootCommand()
				var out, stderr bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&stderr)
				args := []string{"schema", "--cli-path", "aitable api-key " + tc.action, "--format", "json"}
				if compact {
					args = append(args, "--compact")
				}
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("schema: %v %s", err, stderr.String())
				}
				var data map[string]any
				if err := json.Unmarshal(out.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				return data
			}
			full, compact := read(false), read(true)
			if full["canonical_path"] != "aitable."+tc.tool || full["effect"] != tc.effect || full["confirmation"] != tc.confirmation {
				t.Fatalf("incorrect identity/safety: %#v", full)
			}
			if full["result"] == nil || !reflect.DeepEqual(full["result"], compact["result"]) {
				t.Fatal("result contract missing or compact differs")
			}
			params := full["parameters"].(map[string]any)
			if params["base-id"].(map[string]any)["required"] != true {
				t.Fatal("base-id not required")
			}
			if tc.action == "revoke" && params["key-id"].(map[string]any)["required"] != true {
				t.Fatal("key-id not required")
			}
			wantParams := map[string]string{"base-id": "baseId"}
			if tc.action == "revoke" {
				wantParams["key-id"] = "keyId"
			}
			if len(params) != len(wantParams) {
				t.Fatalf("unexpected parameters: %#v", params)
			}
			for name, property := range wantParams {
				if params[name].(map[string]any)["property"] != property {
					t.Fatalf("incorrect parameter mapping: %#v", params[name])
				}
			}
		})
	}
	root := NewRootCommand()
	group, _, err := root.Find([]string{"aitable", "api-key"})
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, command := range group.Commands() {
		commands = append(commands, command.Name())
	}
	sort.Strings(commands)
	if !reflect.DeepEqual(commands, []string{"create", "list", "revoke"}) {
		t.Fatalf("unexpected credential command tree: %v", commands)
	}
}
