// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
)

func TestCrossPlatformCoverageDeapAgentLegacyNamesKeepRequestMeaning(t *testing.T) {
	for _, tc := range []struct {
		name    string
		argv    []string
		field   string
		want    any
		profile bool
	}{
		{"create", []string{"create", "--name", "test", "--description", "test", "--main-program-type", "local_agent"}, "type", "local_agent", true},
		{"list", []string{"list", "--main-program-type", "local_agent"}, "type", "local_agent", false},
		{"save", []string{"save-draft", "--agent-uuid", "agent-1", "--main-program-type", "local_agent", "--yes"}, "type", "local_agent", true},
		{"visibility", []string{"set-visibility", "--agent-uuid", "agent-1", "--visibility", "PARTIAL", "--staff-ids", "u1,u2", "--yes"}, "staffIds", []string{"u1", "u2"}, false},
		{"new_type_wins", []string{"list", "--type", "local_agent", "--main-program-type", "open_code"}, "type", "local_agent", false},
		{"new_type_wins_reverse_order", []string{"list", "--main-program-type", "open_code", "--type", "local_agent"}, "type", "local_agent", false},
		{"new_users_win", []string{"set-visibility", "--agent-uuid", "agent-1", "--visibility", "PARTIAL", "--user-ids", "u3", "--staff-ids", "u1,u2", "--yes"}, "staffIds", []string{"u3"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, _ := newDeapAgentTestTree(t, false)
			root := deapHandler{}.Command(&captureRunner{})
			root.PersistentFlags().Bool("yes", false, "confirmation")
			root.SetArgs(append([]string{"manage"}, tc.argv...))
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("unexpected calls: %#v", caller.calls)
			}
			args := caller.calls[0].args
			if tc.profile {
				args = args["digitalTagEmployeeProfile"].(map[string]any)
			}
			if !reflect.DeepEqual(args[tc.field], tc.want) {
				t.Fatalf("legacy request changed: %#v", args)
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentLegacyDraftTypeRejectsBlank(t *testing.T) {
	for _, value := range []string{"", " \t "} {
		caller, _ := newDeapAgentTestTree(t, false)
		root := deapHandler{}.Command(&captureRunner{})
		root.PersistentFlags().Bool("yes", false, "confirmation")
		root.SetArgs([]string{"manage", "save-draft", "--agent-uuid", "agent-1", "--main-program-type", value, "--yes"})
		err := corecmd.ExecuteForTest(root)
		if err == nil || strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("want explicit blank validation, got %v", err)
		}
		if len(caller.calls) != 0 {
			t.Fatal("blank legacy field reached MCP")
		}
	}
}

func TestCrossPlatformCoverageDeapAgentObsoletePublishFlagIsIgnored(t *testing.T) {
	for _, value := range []string{"true", "false", "bare"} {
		for _, dry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry=%t", value, dry), func(t *testing.T) {
				caller, out := newDeapAgentTestTree(t, dry)
				caller.resultText = `{"success":true,"data":{"agentUuid":"agent-1","type":"open_code"}}`
				root := deapHandler{}.Command(&captureRunner{})
				root.PersistentFlags().Bool("yes", false, "confirmation")
				root.PersistentFlags().Bool("dry-run", false, "preview")
				var stderr bytes.Buffer
				root.SetErr(&stderr)
				legacyFlag := "--allow-join-group"
				if value != "bare" {
					legacyFlag += "=" + value
				}
				argv := []string{"manage", "publish", "--agent-uuid", "agent-1", legacyFlag, "--yes"}
				if dry {
					argv = append(argv, "--dry-run")
				}
				root.SetArgs(argv)
				if err := corecmd.ExecuteForTest(root); err != nil {
					t.Fatal(err)
				}
				if stderr.Len() != 0 {
					t.Fatalf("legacy compatibility must be silent: %q", stderr.String())
				}
				if dry {
					var preview struct {
						Arguments map[string]any `json:"arguments"`
					}
					if err := json.Unmarshal(out.Bytes(), &preview); err != nil {
						t.Fatal(err)
					}
					if len(caller.calls) != 0 || !reflect.DeepEqual(preview.Arguments, map[string]any{"agentUuid": "agent-1"}) {
						t.Fatalf("invalid preview: %s calls=%#v", out.String(), caller.calls)
					}
				} else {
					if len(caller.calls) != 2 {
						t.Fatalf("unexpected publish calls: %#v", caller.calls)
					}
					if !reflect.DeepEqual(caller.calls[1].args, map[string]any{"agentUuid": "agent-1"}) {
						t.Fatalf("obsolete flag sent to MCP: %#v", caller.calls[1].args)
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDeapAgentHelpShowsCanonicalFlagsOnly(t *testing.T) {
	for _, tc := range []struct{ command, canonical, legacy string }{
		{"create", "--type", "--main-program-type"},
		{"list", "--type", "--main-program-type"},
		{"save-draft", "--type", "--main-program-type"},
		{"set-visibility", "--user-ids", "--staff-ids"},
		{"publish", "--agent-uuid", "--allow-join-group"},
		{"detail", "--snapshot", "--type"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			caller, _ := newDeapAgentTestTree(t, false)
			root := deapHandler{}.Command(&captureRunner{})
			var help, stderr bytes.Buffer
			root.SetOut(&help)
			root.SetErr(&stderr)
			root.SetArgs([]string{"manage", tc.command, "--help"})
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(help.String(), tc.canonical) || strings.Contains(help.String(), tc.legacy) {
				t.Fatalf("help must show only canonical flags:\n%s", help.String())
			}
			if stderr.Len() != 0 || len(caller.calls) != 0 {
				t.Fatalf("help produced warnings or remote calls: stderr=%q calls=%#v", stderr.String(), caller.calls)
			}
		})
	}
}
