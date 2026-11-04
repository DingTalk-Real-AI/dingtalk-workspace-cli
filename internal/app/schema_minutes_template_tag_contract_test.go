// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type minutesTemplateTagFinalSchemaCase struct {
	path        string
	rpc         string
	parameters  map[string]string
	required    []string
	effect      string
	idempotency string
	paged       bool
	partial     bool
}

// These expectations are the reviewed public CLI/MCP contract. The test does
// not import declarations or mapper constants from helpers to build its oracle.
func minutesTemplateTagFinalSchemaCases() []minutesTemplateTagFinalSchemaCase {
	return []minutesTemplateTagFinalSchemaCase{
		{"minutes template list-system", "list_system_templates", map[string]string{"limit": "maxResults", "cursor": "nextToken", "locale": "locale"}, nil, "read", "idempotent", true, false},
		{"minutes template list-custom", "list_custom_templates", map[string]string{"limit": "maxResults", "cursor": "nextToken"}, nil, "read", "idempotent", true, false},
		{"minutes template create", "create_custom_template", map[string]string{"title": "title", "prompt": "prompt", "brief-intro": "briefIntro"}, []string{"title", "prompt"}, "write", "non_idempotent", false, false},
		{"minutes template update", "update_custom_template", map[string]string{"template-key": "templateKey", "title": "title", "prompt": "prompt", "brief-intro": "briefIntro"}, []string{"template-key"}, "write", "idempotent", false, false},
		{"minutes template delete", "delete_custom_template", map[string]string{"template-key": "templateKey"}, []string{"template-key"}, "destructive", "idempotent", false, false},
		{"minutes template set-default", "set_default_template", map[string]string{"template-id": "templateId", "template-key": "templateKey", "locale": "locale"}, nil, "write", "idempotent", false, false},
		{"minutes tag create", "create_minutes_tag", map[string]string{"display-name": "displayName"}, []string{"display-name"}, "write", "non_idempotent", false, false},
		{"minutes tag delete", "delete_minutes_tag", map[string]string{"tag-id": "tagId"}, []string{"tag-id"}, "destructive", "idempotent", false, false},
		{"minutes tag add", "add_tags_to_minutes", map[string]string{"id": "taskUuid", "tag-ids": "tagIds", "locale": "locale"}, []string{"id", "tag-ids"}, "write", "idempotent", false, true},
		{"minutes tag remove", "remove_tags_from_minutes", map[string]string{"id": "taskUuid", "tag-ids": "tagIds"}, []string{"id", "tag-ids"}, "write", "idempotent", false, true},
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagFinalSchemaAndRuntime(t *testing.T) {
	// Reuse one already-prepared app root for metadata-only queries. Each query
	// explicitly sets all schema query flags so Changed/value state cannot leak
	// from compact to full, or from CLI lookup to canonical lookup. Business
	// invocations below each get a fresh app root instead.
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	query := func(t *testing.T, path string, compact, canonical bool) map[string]any {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		args := []string{"schema", "--format=json", "--compact=" + strconv.FormatBool(compact)}
		if canonical {
			args = append(args, "--cli-path=", path)
		} else {
			args = append(args, "--cli-path="+path)
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("final schema query %q compact=%t: %v; stderr=%s", path, compact, err, stderr.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("decode final schema %q: %v; output=%s", path, err, stdout.String())
		}
		return payload
	}
	validators := map[string]*jsonschema.Schema{}
	rpcs := map[string]string{}
	for _, tc := range minutesTemplateTagFinalSchemaCases() {
		rpcs[tc.path] = tc.rpc
		t.Run(tc.rpc, func(t *testing.T) {
			canonical := "minutes." + tc.rpc
			full := query(t, tc.path, false, false)
			compact := query(t, tc.path, true, false)
			byCanonical := query(t, canonical, false, true)
			if full["canonical_path"] != canonical || full["primary_cli_path"] != tc.path || byCanonical["canonical_path"] != canonical || byCanonical["primary_cli_path"] != tc.path {
				t.Fatalf("CLI/canonical binding drift: full=%#v canonical=%#v", full["canonical_path"], byCanonical["primary_cli_path"])
			}
			if full["result"] == nil || !reflect.DeepEqual(full["result"], compact["result"]) || !reflect.DeepEqual(full["result"], byCanonical["result"]) {
				t.Fatal("reviewed Result is missing or differs across full, compact, and canonical delivery")
			}
			ref := schemaInterfaceObject(full["interface_ref"])
			if full["interface_mode"] != "mcp" || ref["product_id"] != "minutes" || ref["rpc_name"] != tc.rpc {
				t.Fatalf("wrong MCP binding: mode=%#v ref=%#v", full["interface_mode"], ref)
			}
			leaf, remainder, err := root.Find(strings.Fields(tc.path))
			if err != nil || len(remainder) != 0 || leaf == nil || !leaf.Runnable() {
				t.Fatalf("delivered leaf is not executable: %v remaining=%v", err, remainder)
			}
			if output.CommandRollout(leaf) != output.RolloutUnifiedActive {
				t.Fatalf("new Result leaf is not unified_active: %s", output.CommandRollout(leaf))
			}
			final, ok := contractfinal.RuntimeContractFinal(leaf)
			if !ok || final.Safety == nil {
				t.Fatal("live leaf has no final Safety declaration")
			}
			confirmation := "user_required"
			risk := "medium"
			if tc.effect == "read" {
				confirmation = "not_required"
				risk = "low"
			} else if tc.effect == "destructive" {
				risk = "high"
			}
			for key, want := range map[string]string{"effect": tc.effect, "risk": risk, "confirmation": confirmation, "idempotency": tc.idempotency} {
				if full[key] != want || compact[key] != want {
					t.Errorf("%s drift: full=%#v compact=%#v want=%q", key, full[key], compact[key], want)
				}
			}
			if final.Safety.Confirmation != confirmation || final.Safety.Effect != tc.effect || final.Safety.Risk != risk || final.Safety.Idempotency != tc.idempotency {
				t.Fatalf("delivered Safety disagrees with the live runtime declaration: %#v", final.Safety)
			}
			if full["dry_run"] == nil || !reflect.DeepEqual(full["dry_run"], compact["dry_run"]) {
				t.Fatal("reviewed preview capability missing or drifting in compact delivery")
			}
			parameters := schemaContractMap(full["parameters"])
			compactParameters := schemaContractMap(compact["parameters"])
			if len(parameters) != len(tc.parameters) || len(compactParameters) != len(parameters) {
				t.Fatalf("unexpected CLI/interface parameters: full=%v compact=%v want=%v", parameters, compactParameters, tc.parameters)
			}
			for flag, property := range tc.parameters {
				param := parameters[flag]
				if param == nil || param["property"] != property || leaf.Flags().Lookup(flag) == nil {
					t.Errorf("--%s binding/executability drift: %#v want property=%s", flag, param, property)
				}
				interfaceType := "string"
				if property == "maxResults" {
					interfaceType = "number"
				} else if property == "tagIds" {
					interfaceType = "array"
				}
				// The wire omits a redundant interface_type when it equals
				// the executable CLI type (runtime_schema.go's projection).
				actualType := param["interface_type"]
				if actualType == nil {
					actualType = param["type"]
				}
				if actualType != interfaceType {
					t.Errorf("--%s effective interface type=%#v want=%s", flag, actualType, interfaceType)
				}
				if compactParameters[flag] == nil || compactParameters[flag]["property"] != nil || compactParameters[flag]["interface_type"] != nil {
					t.Errorf("compact parameter has missing flag or leaked audit mapping: %#v", compactParameters[flag])
				}
			}
			for _, flag := range tc.required {
				if parameters[flag]["required"] != true || compactParameters[flag]["required"] != true {
					t.Errorf("required flag --%s lost during delivery", flag)
				}
			}
			for _, forbidden := range []string{"operatorUid", "operator-uid", "requestSource", "request-source", "mcpContext", "accountId", "key"} {
				if parameters[forbidden] != nil || compactParameters[forbidden] != nil || leaf.Flags().Lookup(forbidden) != nil {
					t.Errorf("trusted/platform value exposed as model input: %s", forbidden)
				}
			}
			result := minutesTemplateTagSchemaObject(t, full["result"], "result")
			outcomes := []string{"success", "failure"}
			if tc.partial {
				outcomes = []string{"success", "partial_failure", "failure"}
			}
			if !schemaContractJSONEqual(result["outcomes"], outcomes) {
				t.Fatalf("terminal outcome contract=%#v want=%v", result["outcomes"], outcomes)
			}
			if tc.paged {
				want := map[string]any{"kind": "cursor", "cursor_parameter": "cursor", "meta_path": "meta.pagination", "endpoint_exhausted_path": "meta.pagination.endpoint_exhausted", "next_token_path": "meta.pagination.next_token"}
				if !schemaContractJSONEqual(full["pagination"], want) || !schemaContractJSONEqual(compact["pagination"], want) {
					t.Fatalf("pagination drift: full=%#v compact=%#v", full["pagination"], compact["pagination"])
				}
				minutesTemplateTagAssertNoPaginationInData(t, result["data_schema"])
			} else if full["pagination"] != nil || compact["pagination"] != nil {
				t.Fatal("non-paged command unexpectedly declares pagination")
			}
			if tc.rpc == "update_custom_template" {
				assertSchemaContractConstraintGroup(t, full, "require_one_of", []string{"title", "prompt", "brief-intro"})
				if !strings.Contains(schemaContractString(compact["description"]), "显式空简介") {
					t.Fatal("compact update contract lost allowed empty-brief semantics")
				}
			}
			if tc.rpc == "set_default_template" {
				assertSchemaContractConstraintGroup(t, full, "require_one_of", []string{"template-id", "template-key"})
				assertSchemaContractConstraintGroup(t, full, "mutually_exclusive", []string{"template-id", "template-key"})
			}
			compiler := jsonschema.NewCompiler()
			compiler.DefaultDraft(jsonschema.Draft2020)
			resource := "https://dws.test/minutes-template-tag/" + tc.rpc + ".json"
			if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
				t.Fatal(err)
			}
			validators[tc.path], err = compiler.Compile(resource)
			if err != nil {
				t.Fatalf("compile final delivered Result schema: %v", err)
			}
		})
	}

	// Validate actual command emissions against the published compact/full
	// contract, not just hand-authored representative DTOs.
	for _, tc := range []struct {
		name, path, response, outcome string
		args                          []string
		code, calls                   int
		check                         func(*testing.T, map[string]any, *minutesTemplateTagContractCaller)
	}{
		{"custom-version-zero-page", "minutes template list-custom", `{"success":true,"result":{"items":[{"templateKey":"personal-1","title":"周会","prompt":"  line 1\n\nline 2  ","version":0}],"hasMore":true,"nextToken":"opaque-next"}}`, "success", nil, 0, 1, func(t *testing.T, env map[string]any, _ *minutesTemplateTagContractCaller) {
			data := minutesTemplateTagSchemaObject(t, env["data"], "data")
			item := minutesTemplateTagSchemaObject(t, data["items"].([]any)[0], "items[0]")
			if item["version"] != float64(0) || item["prompt"] != "  line 1\n\nline 2  " {
				t.Fatal("version zero or prompt bytes were lost")
			}
			meta := minutesTemplateTagSchemaObject(t, env["meta"], "meta")
			paging := minutesTemplateTagSchemaObject(t, meta["pagination"], "pagination")
			if meta["count"] != float64(1) || paging["endpoint_exhausted"] != false || paging["next_token"] != "opaque-next" {
				t.Fatalf("actual continuation drift: %#v", meta)
			}
		}},
		{"system-empty-continuation-page", "minutes template list-system", `{"success":true,"result":{"items":[],"hasMore":true,"nextToken":"opaque-system-next"}}`, "success", nil, 0, 1, func(t *testing.T, env map[string]any, _ *minutesTemplateTagContractCaller) {
			meta := minutesTemplateTagSchemaObject(t, env["meta"], "meta")
			paging := minutesTemplateTagSchemaObject(t, meta["pagination"], "pagination")
			if meta["count"] != float64(0) || paging["endpoint_exhausted"] != false || paging["next_token"] != "opaque-system-next" {
				t.Fatalf("empty page must retain its continuation: %#v", meta)
			}
		}},
		{"delete-unknown-default", "minutes template delete", `{"success":true,"result":{"success":true,"templateKey":"personal-1"}}`, "success", []string{"--template-key=personal-1", "--yes"}, 0, 1, func(t *testing.T, env map[string]any, _ *minutesTemplateTagContractCaller) {
			data := minutesTemplateTagSchemaObject(t, env["data"], "data")
			if value, exists := data["wasDefault"]; !exists || value != nil {
				t.Fatalf("unknown default state must remain explicit null: %#v", data)
			}
		}},
		{"official-default-null-key", "minutes template set-default", `{"success":true,"result":{"success":true,"templateId":"official-1","templateKey":null}}`, "success", []string{"--template-id=official-1", "--yes"}, 0, 1, func(t *testing.T, env map[string]any, _ *minutesTemplateTagContractCaller) {
			data := minutesTemplateTagSchemaObject(t, env["data"], "data")
			if value, exists := data["templateKey"]; !exists || value != nil {
				t.Fatalf("official default key must remain explicit null: %#v", data)
			}
		}},
		{"tag-native-partial", "minutes tag add", `{"success":true,"result":{"success":false,"taskUuid":"note-1","results":[{"tagId":"a","status":"APPLIED"},{"tagId":"b","status":"FAILED","errorCode":"SYSTEM_ERROR","errorMessage":"This change could not be confirmed"},{"tagId":"c","status":"NOT_PROCESSED"}]}}`, "partial_failure", []string{"--id=note-1", "--tag-ids=a,b,c", "--yes"}, 7, 1, func(t *testing.T, env map[string]any, _ *minutesTemplateTagContractCaller) {
			data := minutesTemplateTagSchemaObject(t, env["data"], "data")
			succeeded, failed, unknown := data["succeeded"].([]any), data["failed"].([]any), data["unknown"].([]any)
			if data["total"] != float64(3) || len(succeeded) != 1 || len(failed) != 1 || len(unknown) != 1 {
				t.Fatalf("partial ledger does not account for every requested item: %#v", data)
			}
			if minutesTemplateTagSchemaObject(t, succeeded[0], "succeeded[0]")["id"] != "a" || minutesTemplateTagSchemaObject(t, failed[0], "failed[0]")["id"] != "c" || minutesTemplateTagSchemaObject(t, unknown[0], "unknown[0]")["id"] != "b" {
				t.Fatalf("confirmed, unexecuted, and unknown IDs were conflated: %#v", data)
			}
		}},
		{"tag-first-item-unknown", "minutes tag add", `{"success":true,"result":{"success":false,"taskUuid":"note-1","results":[{"tagId":"a","status":"FAILED","errorCode":"SYSTEM_ERROR","errorMessage":"This change could not be confirmed"},{"tagId":"b","status":"NOT_PROCESSED"}]}}`, "failure", []string{"--id=note-1", "--tag-ids=a,b", "--yes"}, 1, 1, func(t *testing.T, env map[string]any, _ *minutesTemplateTagContractCaller) {
			if env["data"] != nil {
				t.Fatal("all-unconfirmed request must use failure rather than empty-succeeded partial")
			}
			info := minutesTemplateTagSchemaObject(t, env["error"], "error")
			if info["retryable"] == true {
				t.Fatal("unknown write result must not permit blind replay")
			}
			details := minutesTemplateTagSchemaObject(t, info["details"], "error.details")
			receipt := minutesTemplateTagSchemaObject(t, details["receipt"], "error.details.receipt")
			if len(receipt["results"].([]any)) != 2 {
				t.Fatalf("unknown/unexecuted receipt was discarded: %#v", receipt)
			}
		}},
		{"empty-brief-preview", "minutes template update", "", "success", []string{"--template-key=personal-1", "--brief-intro=", "--dry-run"}, 0, 0, func(t *testing.T, env map[string]any, _ *minutesTemplateTagContractCaller) {
			data := minutesTemplateTagSchemaObject(t, env["data"], "data")
			args := minutesTemplateTagSchemaObject(t, data["arguments"], "data.arguments")
			if env["dry_run"] != true || data["executed"] != false || args["briefIntro"] != "" {
				t.Fatalf("explicit empty update did not produce a non-executing preview: %#v", env)
			}
			if _, exists := args["title"]; exists {
				t.Fatal("preview invented an absent title update")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagContractCaller{response: tc.response}
			businessRoot := NewRootCommand()
			helpers.InitDepsForTest(t, caller)
			var out, errOut bytes.Buffer
			businessRoot.SetOut(&out)
			businessRoot.SetErr(&errOut)
			args := append(strings.Fields(tc.path), tc.args...)
			businessRoot.SetArgs(append(args, "--format=json"))
			executed, err := businessRoot.ExecuteC()
			if err != nil {
				t.Fatalf("execute actual command: %v; stderr=%s", err, errOut.String())
			}
			code, emitted, err := output.EmitStoredResult(executed)
			if err != nil || !emitted || code != tc.code || caller.calls != tc.calls {
				t.Fatalf("result code=%d emitted=%t calls=%d err=%v; want code=%d calls=%d", code, emitted, caller.calls, err, tc.code, tc.calls)
			}
			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("decode actual unified result: %v; stdout=%s stderr=%s", err, out.String(), errOut.String())
			}
			if env["outcome"] != tc.outcome || env["ok"] != (tc.outcome == "success") {
				t.Fatalf("terminal result drift: %#v", env)
			}
			if tc.calls > 0 && (caller.product != "minutes" || caller.tool != rpcs[tc.path]) {
				t.Fatalf("wrong MCP route: %s/%s want minutes/%s", caller.product, caller.tool, rpcs[tc.path])
			}
			if data, exists := env["data"]; exists {
				validator := validators[tc.path]
				if validator == nil {
					t.Fatalf("no final delivered Result validator for %s", tc.path)
				}
				if err := validator.Validate(data); err != nil {
					t.Fatalf("actual command data rejected by final full/compact Result: %v; data=%#v", err, data)
				}
			}
			tc.check(t, env, caller)
		})
	}

	for _, tc := range []struct {
		name         string
		args         []string
		confirmation bool
	}{
		{"delete-confirmation-zero-calls", []string{"minutes", "template", "delete", "--template-key=personal-1"}, true},
		{"update-presence-validation-zero-calls", []string{"minutes", "template", "update", "--template-key=personal-1", "--yes"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagContractCaller{}
			businessRoot := NewRootCommand()
			helpers.InitDepsForTest(t, caller)
			businessRoot.SetIn(strings.NewReader(""))
			businessRoot.SetOut(&bytes.Buffer{})
			businessRoot.SetErr(&bytes.Buffer{})
			businessRoot.SetArgs(append(tc.args, "--format=json"))
			_, err := businessRoot.ExecuteC()
			if err == nil || apperrors.ExitCode(err) != 3 || caller.calls != 0 {
				t.Fatalf("expected pre-network validation/confirmation exit 3: err=%v code=%d calls=%d", err, apperrors.ExitCode(err), caller.calls)
			}
			info := errorInfoFromExecutionError(err)
			if tc.confirmation && info.Subtype != "confirmation_required" {
				t.Fatalf("confirmation gate classification drift: %#v", info)
			}
		})
	}
}

func minutesTemplateTagSchemaObject(t testing.TB, raw any, name string) map[string]any {
	t.Helper()
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s=%#v, want JSON object", name, raw)
	}
	return value
}

func minutesTemplateTagAssertNoPaginationInData(t testing.TB, raw any) {
	t.Helper()
	switch value := raw.(type) {
	case map[string]any:
		if properties, ok := value["properties"].(map[string]any); ok {
			for _, field := range []string{"hasMore", "nextToken", "endpointExhausted", "endpoint_exhausted", "meta", "outcome", "ok"} {
				if _, exists := properties[field]; exists {
					t.Errorf("Result.data_schema leaks framework/pagination field %q", field)
				}
			}
		}
		for _, child := range value {
			minutesTemplateTagAssertNoPaginationInData(t, child)
		}
	case []any:
		for _, child := range value {
			minutesTemplateTagAssertNoPaginationInData(t, child)
		}
	}
}

type minutesTemplateTagContractCaller struct {
	response      string
	calls         int
	product, tool string
	args          map[string]any
}

func (c *minutesTemplateTagContractCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	c.product, c.tool, c.args = product, tool, args
	if c.response == "" {
		return nil, fmt.Errorf("unexpected remote call: %s/%s", product, tool)
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, nil
}
func (*minutesTemplateTagContractCaller) Format() string { return "json" }
func (*minutesTemplateTagContractCaller) DryRun() bool   { return false }
func (*minutesTemplateTagContractCaller) Fields() string { return "" }
func (*minutesTemplateTagContractCaller) JQ() string     { return "" }
