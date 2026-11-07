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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
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
				args := []string{"schema", "--cli-path", "aitable supabase api-key " + tc.action, "--format", "json"}
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

// TestCrossPlatformCoverageSQLSheetKeyHelpNavigation checks public navigation and the legacy group's migration guidance.
func TestCrossPlatformCoverageSQLSheetKeyHelpNavigation(t *testing.T) {
	root := NewRootCommand()
	legacy, rest, err := root.Find([]string{"aitable", "api-key"})
	if err != nil || len(rest) != 0 {
		t.Fatalf("find legacy group: rest=%v err=%v", rest, err)
	}
	if legacy.Hidden || legacy.Deprecated == "" || !legacy.Runnable() || legacy.IsAvailableCommand() {
		t.Errorf("legacy group contract: hidden=%v deprecated=%q runnable=%v available=%v", legacy.Hidden, legacy.Deprecated, legacy.Runnable(), legacy.IsAvailableCommand())
	}

	for _, tc := range []struct {
		path     string
		commands []string
		excluded []string
		guidance bool
	}{
		{path: "aitable", commands: []string{"supabase"}, excluded: []string{"api-key"}},
		{path: "aitable supabase", commands: []string{"api-key"}},
		{path: "aitable supabase api-key", commands: []string{"create", "list", "revoke"}},
		{path: "aitable api-key", excluded: []string{"create", "list", "revoke"}, guidance: true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			root := NewRootCommand()
			var out, stderr bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&stderr)
			root.SetArgs(append(strings.Fields(tc.path), "--help"))
			if err := root.Execute(); err != nil || stderr.Len() != 0 {
				t.Fatalf("help failed: %v %s", err, stderr.String())
			}
			help := out.String()
			for _, name := range tc.commands {
				if !strings.Contains(help, "\n  "+name+" ") {
					t.Fatalf("help is missing public command %q:\n%s", name, help)
				}
			}
			for _, name := range tc.excluded {
				if strings.Contains(help, "\n  "+name+" ") {
					t.Fatalf("help exposes compatibility command %q:\n%s", name, help)
				}
			}
			if tc.guidance && (!strings.Contains(help, "dws aitable supabase api-key") || !strings.Contains(help, "原有 create、list、revoke 命令保持兼容")) {
				t.Fatalf("legacy help is missing migration guidance:\n%s", help)
			}
		})
	}
}

// TestCrossPlatformCoverageSQLSheetKeyCompatibilityDiscovery checks shared Schema identity and warning-free leaf help.
func TestCrossPlatformCoverageSQLSheetKeyCompatibilityDiscovery(t *testing.T) {
	root := NewRootCommand()
	states := map[string]bool{}
	for _, command := range interfacesnapshot.Capture(root).Commands {
		if strings.HasPrefix(command.Path, "dws aitable api-key ") || strings.HasPrefix(command.Path, "dws aitable supabase api-key ") {
			if !command.Runnable || len(command.Aliases) != 0 {
				t.Fatalf("unexpected executable entry: %#v", command)
			}
			states[command.Path] = command.Hidden
		}
	}
	want := map[string]bool{}
	for _, action := range []string{"create", "list", "revoke"} {
		primary := "aitable supabase api-key " + action
		legacy := "aitable api-key " + action
		want["dws "+primary], want["dws "+legacy] = false, true
		tool := action + "_sql_sheet_api_key"
		if action == "list" {
			tool += "s"
		}
		canonical := "aitable." + tool
		for _, compact := range []bool{false, true} {
			var primaryPayload map[string]any
			for index, query := range [][]string{{"--cli-path", primary}, {"--cli-path", legacy}, {canonical}} {
				root := NewRootCommand()
				var out, stderr bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&stderr)
				args := append([]string{"schema"}, query...)
				args = append(args, "--format", "json")
				if compact {
					args = append(args, "--compact")
				}
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("schema %v: %v %s", query, err, stderr.String())
				}
				var payload map[string]any
				if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				wantPath := primary
				if index == 1 {
					wantPath = legacy
				}
				isAlias, _ := payload["is_alias"].(bool)
				if payload["canonical_path"] != canonical || payload["cli_path"] != wantPath || (!compact && isAlias != (index == 1)) {
					t.Fatalf("unexpected identity: %#v", payload)
				}
				if !compact && (payload["primary_cli_path"] != primary || !reflect.DeepEqual(payload["aliases"], []any{legacy})) {
					t.Fatalf("primary/alias mapping lost: %#v", payload)
				}
				delete(payload, "cli_path")
				delete(payload, "is_alias")
				if index == 0 {
					primaryPayload = payload
				} else if !reflect.DeepEqual(primaryPayload, payload) {
					t.Fatalf("contract differs between primary, alias and canonical lookup: %v", query)
				}
			}
		}
		for _, path := range []string{primary, legacy} {
			root := NewRootCommand()
			leaf, rest, err := root.Find(strings.Fields(path))
			if err != nil || len(rest) != 0 || leaf.Deprecated != "" || !leaf.Runnable() {
				t.Fatalf("entry %s is not an exact runnable leaf without deprecation", path)
			}
			var out, stderr bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&stderr)
			root.SetArgs(append(strings.Fields(path), "--help"))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if stderr.Len() != 0 || strings.Contains(out.String(), "is deprecated") || !strings.Contains(out.String(), "dws "+path+" [flags]") || !strings.Contains(out.String(), "dws "+primary+" --base-id") {
				t.Fatalf("exact help must remain callable and recommend primary examples: %s %s", out.String(), stderr.String())
			}
		}
	}
	if !reflect.DeepEqual(states, want) {
		t.Fatalf("migration snapshot: got %#v want %#v", states, want)
	}
}
