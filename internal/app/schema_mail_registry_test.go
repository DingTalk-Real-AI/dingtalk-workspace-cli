// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestCrossPlatformCoverageMailUserLookupDeliveredSchema(t *testing.T) {
	object := func(value any) map[string]any {
		t.Helper()
		valueMap, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("expected object, got %T: %#v", value, value)
		}
		return valueMap
	}
	query := func(args ...string) map[string]any {
		t.Helper()
		root := NewRootCommand()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetArgs(append([]string{"schema"}, args...))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	for _, tc := range []struct{ tool, command, flag, property, kind string }{
		{"get_user_by_org_email", "get", "org-email", "orgEmail", "string"},
		{"batch_get_users_by_org_emails", "batch-get", "org-emails", "orgEmails", "array"},
	} {
		canonical, path := "mail."+tc.tool, "mail user "+tc.command
		full := query(canonical, "--format", "json")
		compact := query(canonical, "--compact", "--format", "json")
		byPath := query("--cli-path", path, "--format", "json")
		if full["cli_path"] != path || full["effect"] != "read" || full["confirmation"] != "not_required" {
			t.Fatalf("incorrect lookup contract: %#v", full)
		}
		params := object(full["parameters"])
		email := object(params[tc.flag])
		if len(params) != 1 || email["property"] != tc.property || email["required"] != true || email["cli_required"] != true || email["type"] != tc.kind {
			t.Fatalf("lookup %s must expose only required %s: %#v", canonical, tc.flag, params)
		}
		iface := object(full["interface_ref"])
		if iface["product_id"] != "mail" || iface["rpc_name"] != tc.tool {
			t.Fatalf("incorrect interface: %#v", iface)
		}
		if full["result"] == nil || !reflect.DeepEqual(full["result"], compact["result"]) || !reflect.DeepEqual(full["result"], byPath["result"]) {
			t.Fatal("Result must be preserved by canonical, CLI-path and compact delivery")
		}
		result := object(full["result"])
		properties := object(object(result["data_schema"])["properties"])
		employee := object(properties["result"])
		if tc.command == "batch-get" {
			for _, view := range []map[string]any{full, compact} {
				if !strings.Contains(schemaContractString(view["description"]), "仍读取 data.succeeded 中 found=true 的 user") {
					t.Fatal("batch Schema must tell Agents to retain confirmed members after exit code 7")
				}
			}
			if !reflect.DeepEqual(result["outcomes"], []any{"success", "partial_failure", "failure"}) {
				t.Fatalf("batch partial outcome missing: %#v", result["outcomes"])
			}
			for _, key := range []string{"succeeded", "failed", "unknown"} {
				if object(properties[key])["type"] != "array" {
					t.Fatalf("batch partial channel %s missing", key)
				}
			}
			if object(properties["total"])["type"] != "integer" {
				t.Fatal("batch partial count missing")
			}
			batch := object(employee["properties"])
			users := object(batch["users"])
			missing := object(batch["notFoundOrgEmails"])
			if users["type"] != "array" || missing["type"] != "array" || object(missing["items"])["type"] != "string" {
				t.Fatalf("batch arrays missing: %#v", batch)
			}
			if !strings.Contains(schemaContractString(email["description"]), "100") {
				t.Fatalf("batch limit missing from parameter declaration: %#v", email)
			}
			employee = object(users["items"])
		} else if !reflect.DeepEqual(employee["type"], []any{"object", "null"}) {
			t.Fatalf("not-found employee must remain nullable: %#v", employee)
		}
		fields := object(employee["properties"])
		staffID := object(fields["staffId"])
		if !strings.Contains(schemaContractString(staffID["description"]), "不是工号") {
			t.Fatalf("staffId meaning missing: %#v", staffID)
		}
	}
	for _, path := range []string{"mail", "mail user"} {
		navigation, _ := json.Marshal(query(path, "--compact", "--format", "json"))
		if !strings.Contains(string(navigation), "mail.get_user_by_org_email") || !strings.Contains(string(navigation), "mail.batch_get_users_by_org_emails") {
			t.Fatalf("%s navigation cannot discover lookup: %s", path, navigation)
		}
	}
}

// Keep the folder-list wrapper and the free-form KQL search command as two
// independent Agent contracts. The list command translates --folder-id into a
// query before calling the same RPC, so treating it as a search alias loses a
// real executable parameter surface.
func TestMailListAndSearchRemainDistinctRegistryCommands(t *testing.T) {
	root := NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry() error = %v", err)
	}

	list, listOK := effective.ByCanonical["mail.list_emails"]
	search, searchOK := effective.ByCanonical["mail.search_emails"]
	if !listOK || !searchOK {
		t.Fatalf("mail registry split missing: list=%t search=%t", listOK, searchOK)
	}
	if list.PrimaryCLIPath != "mail message list" || search.PrimaryCLIPath != "mail message search" {
		t.Fatalf("mail registry paths: list=%q search=%q", list.PrimaryCLIPath, search.PrimaryCLIPath)
	}
	if len(list.Aliases) != 0 || len(search.Aliases) != 0 {
		t.Fatalf("mail list/search must not alias each other: list=%v search=%v", list.Aliases, search.Aliases)
	}

	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	listCommand := bound.ByCanonical[list.CanonicalPath].PrimaryCommand
	searchCommand := bound.ByCanonical[search.CanonicalPath].PrimaryCommand
	if listCommand == nil || searchCommand == nil || listCommand == searchCommand {
		t.Fatalf("mail list/search Cobra bindings are not distinct: list=%p search=%p", listCommand, searchCommand)
	}
	if listCommand.Flags().Lookup("folder-id") == nil || listCommand.Flags().Lookup("query") != nil {
		t.Fatal("mail list Cobra surface must expose --folder-id and not --query")
	}
	if searchCommand.Flags().Lookup("query") == nil || searchCommand.Flags().Lookup("folder-id") != nil {
		t.Fatal("mail search Cobra surface must expose --query and not --folder-id")
	}
}
